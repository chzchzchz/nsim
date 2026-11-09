package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/internal/plantinit"
)

// Request bounds shared by flags and HTTP handlers so the two entry
// points cannot disagree about what is allowed.
const (
	// maxStepsPerSecond caps wall pacing (flags and POST /sim/speed alike).
	maxStepsPerSecond = 1_000_000
	// maxTimestepMs caps a single simulated step from POST /sim/timestep.
	maxTimestepMs = 60_000
	// maxStepRequest caps the steps one POST /sim/step may advance.
	maxStepRequest = 1_000_000
	// maxRequestBody caps control-command and control-value bodies.
	maxRequestBody = 1 << 20
)

// Server owns the optional loop-side state; handlers never touch
// *core.Plant outside the RWMutex. siteName is server identity only
// and never enters core.Plant.
type Server struct {
	mu sync.RWMutex
	// plant is the current simulated plant. initialPlant is the
	// deep-copy of plant at run start (boot or last reset); it
	// serves GET /initial-state and never steps. initialBytes is
	// the cached marshal at snapshot time (never rewritten except
	// by reset/restore).
	plant        *core.Plant
	initialPlant *core.Plant
	initialBytes []byte
	controls     bridge.ControlMap
	sensors      bridge.SensorMap
	queue        *cmdfile.Queue
	journal      *cmdfile.Journal
	log          *cmdfile.Log
	script       []core.ScriptCommand
	scenario     *core.Scenario
	progress     *core.RunProgress

	siteName   string
	plantPath  string
	scenarioID string
	cmdPath    string

	simTimeNs      int64
	runId          int64
	paused         bool
	stepsPerSecond int
	dt             time.Duration
	rev            int64
	stepsTotal     int64

	// ready drives /readyz transitions during /sim/reset rebuilds.
	ready atomic.Bool

	// shutdown is closed by POST /sim/shutdown; main owns process
	// teardown and observes this signal.
	shutdown chan struct{}
	// shutdownOnce makes concurrent POST /sim/shutdown requests safe.
	shutdownOnce sync.Once

	// routes is built once in NewServer; methodAllowed and isAPIPath
	// read it on every request without re-allocating the table.
	routes []route
}

// NewServer builds a server around an initial plant. script and
// scenario come from plantinit.BuildInitialPlant; cmdPath is an
// optional startup command file whose scenario field fills in an
// absent scenarioID, mirroring nsimrun's flag logic.
//
// The returned struct publishes the initial progress state. Only
// runtime construction needs it; run-loop progress and journal
// entries are established here before the server is published.
func NewServer(siteName, scenarioID, plantPath, cmdPath string, paused bool, stepsPerSecond int, dt time.Duration) (*Server, error) {
	if stepsPerSecond <= 0 {
		return nil, fmt.Errorf("stepsPerSecond must be positive, got %d", stepsPerSecond)
	}
	if stepsPerSecond > maxStepsPerSecond {
		return nil, fmt.Errorf("stepsPerSecond %d exceeds maximum %d", stepsPerSecond, maxStepsPerSecond)
	}
	if dt <= 0 {
		return nil, fmt.Errorf("timestep must be positive, got %v", dt)
	}
	if dt > maxTimestepMs*time.Millisecond {
		return nil, fmt.Errorf("timestep %v exceeds maximum %v", dt, maxTimestepMs*time.Millisecond)
	}
	var cmds []cmdfile.Command
	fileScenario := ""
	if cmdPath != "" {
		var err error
		cmds, fileScenario, err = plantinit.LoadCommands(cmdPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load command file: %w", err)
		}
	}
	effective := scenarioID
	switch {
	case effective == "":
		effective = fileScenario
	case fileScenario != "" && fileScenario != effective:
		return nil, fmt.Errorf("-scenario %q conflicts with export scenario %q", effective, fileScenario)
	}
	p, script, sc, err := plantinit.BuildInitialPlant(effective, plantPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build initial plant: %w", err)
	}
	s := &Server{
		plant:          &p,
		siteName:       siteName,
		plantPath:      plantPath,
		scenarioID:     effective,
		cmdPath:        cmdPath,
		script:         script,
		scenario:       sc,
		paused:         paused,
		stepsPerSecond: stepsPerSecond,
		dt:             dt,
		initialPlant:   mustClonePlant(&p),
		initialBytes:   mustMarshalPlant(&p),
		queue:          &cmdfile.Queue{},
		journal:        &cmdfile.Journal{},
		log:            &cmdfile.Log{},
		shutdown:       make(chan struct{}),
	}
	s.routes = s.buildRoutes()
	s.refreshMapsLocked()
	if sc != nil {
		// Progress is created inline in /status; a scenario's
		// milestones and Check are pure functions of the current
		// plant state plus the latched met time tracked by
		// RunProgress, so no long-lived tracker survives reset.
		s.progress = core.NewRunProgress(*sc, 0)
	}
	// Startup commands ride the same queue path as POST /commands.
	if len(cmds) > 0 {
		texts := make([]string, len(cmds))
		for i, c := range cmds {
			texts[i] = fmt.Sprintf("%d %s %s", c.TimeOffsetMs, c.ControlKey, c.Value)
		}
		s.queue.SubmitAll(cmds, texts, 0)
		for _, e := range s.queue.Entries() {
			s.journal.Add(e.Cmd, e.DueNs)
			s.log.AddOperation(e.Cmd, e.DueNs, "")
		}
	}
	s.ready.Store(true)
	return s, nil
}

// refreshMapsLocked rebuilds the bridge maps from the current plant.
// Callers hold the write lock (or the server is unpublished).
func (s *Server) refreshMapsLocked() {
	s.controls = s.plant.GetControlMap()
	s.sensors = s.plant.GetSensorMap()
}

// stepOnce performs one simulation tick while holding the write
// lock: fire the scenario script and queued commands, step the
// plant, advance the clock, and observe the scenario progress.
func (s *Server) stepOnce() {
	dt := s.dt
	next := s.simTimeNs + int64(dt)
	s.fireDueScriptLocked(next)
	s.queue.FireDue(next, s.controls)
	s.plant.Step(dt)
	s.plant.Update()
	s.simTimeNs = next
	s.stepsTotal++
	if s.progress != nil {
		s.progress.Observe(s.plant, s.simTimeNs)
	}
	s.rev++
}

// fireDueScriptLocked applies the script prefix that has come due
// at nowNs, journaling each fired command at its fire time, the
// same behavior as the WASM handle.
func (s *Server) fireDueScriptLocked(nowNs int64) {
	due := 0
	for due < len(s.script) && s.script[due].AtMs*int64(time.Millisecond) <= nowNs {
		due++
	}
	if due == 0 {
		return
	}
	fired := make([]core.ScriptCommand, due)
	copy(fired, s.script[:due])
	core.ApplyScript(&s.script, nowNs, s.controls)
	for _, cmd := range fired {
		logged := cmdfile.Command{
			ControlKey: cmd.ControlKey,
			Value:      cmd.Value,
		}
		s.journal.Add(logged, nowNs)
		s.log.AddOperation(logged, nowNs, "script")
	}
}

// loop runs the stepping ticker until stop is closed. Pacing a
// fresh timer each iteration (rather than a fixed ticker) lets the
// cadence pick up POST /sim/speed changes immediately.
func (s *Server) loop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		default:
		}
		interval := time.Second
		s.mu.RLock()
		sps := s.stepsPerSecond
		paused := s.paused
		s.mu.RUnlock()
		if sps > 0 {
			interval = time.Second / time.Duration(sps)
			if interval <= 0 {
				interval = time.Nanosecond
			}
		}
		t := time.NewTimer(interval)
		select {
		case <-stop:
			t.Stop()
			return
		case <-t.C:
		}
		if paused {
			continue
		}
		s.mu.Lock()
		// Re-check under the write lock: pause or reset may have
		// landed while the timer was ticking.
		if !s.paused {
			s.stepOnce()
		}
		s.mu.Unlock()
	}
}

// stepMsLocked advances ceil(ms/dt) ticks, capped at the step
// request bound; the caller holds the write lock.
func (s *Server) stepMsLocked(ms int64) int {
	if ms <= 0 {
		return 0
	}
	n := int((ms * int64(time.Millisecond)) / int64(s.dt))
	n = min(n, maxStepRequest)
	if n <= 0 {
		n = 1
	}
	for range n {
		s.stepOnce()
	}
	return n
}

// resetLocked rebuilds the plant via the shared builder and clears
// the clock, queue, and journal. The caller holds the write lock.
func (s *Server) resetLocked(scenarioID, plantPath string) error {
	p, script, sc, err := plantinit.BuildInitialPlant(scenarioID, plantPath)
	if err != nil {
		return err
	}
	s.plant = &p
	cp, cb := snapshotPlant(&p)
	s.initialPlant = cp
	s.initialBytes = cb
	s.script = script
	s.scenario = sc
	s.progress = nil
	if sc != nil {
		s.progress = core.NewRunProgress(*sc, 0)
	}
	s.refreshMapsLocked()
	s.queue.Clear()
	s.journal.Clear()
	s.log.Clear()
	s.simTimeNs = 0
	s.stepsTotal = 0
	s.rev = 0
	s.runId++
	s.scenarioID = scenarioID
	s.plantPath = plantPath
	return nil
}

func snapshotPlant(p *core.Plant) (*core.Plant, []byte) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, nil
	}
	var c core.Plant
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, nil
	}
	return &c, data
}

func clonePlant(p *core.Plant) *core.Plant {
	cp, _ := snapshotPlant(p)
	return cp
}

func mustClonePlant(p *core.Plant) *core.Plant { return clonePlant(p) }
func mustMarshalPlant(p *core.Plant) []byte {
	_, b := snapshotPlant(p)
	return b
}

// sortedKeys returns the map keys in sorted order for stable output.
func sortedKeys[V any](m map[string]V) []string {
	keys := slices.Sorted(maps.Keys(m))
	return keys
}

// sensorEntry is the JSON shape for one sensor reading.
type sensorEntry struct {
	Key       string `json:"key"`
	Part      string `json:"part"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	SimTimeMs int64  `json:"simTimeMs"`
}

// controlEntry is the JSON shape for one control definition/value.
type controlEntry struct {
	Key   string   `json:"key"`
	Part  string   `json:"part"`
	Label string   `json:"label"`
	Kind  string   `json:"kind"`
	Enum  []string `json:"enum,omitempty"`
	Value string   `json:"value"`
}

func sensorValue(s *bridge.Sensor) (string, error) {
	if s == nil || s.Get == nil {
		return "", fmt.Errorf("sensor has no Get function")
	}
	return s.Get()
}

func controlValue(c *bridge.Control) string {
	if c == nil || c.Get == nil {
		return ""
	}
	return c.Get()
}

// metricName sanitizes a sensor key for use as a Prometheus metric
// name suffix: slashes and any non-alphanumeric character become
// underscores.
func metricName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return "nsim_" + b.String()
}

// metricUnit maps a bridge sensor kind to its unit label. Kinds
// that are already SI-ish or dimensionless leave the unit empty.
func metricUnit(kind string) string {
	switch kind {
	case "bool", "normalized", "percent", "signedpercent", "enum":
		return ""
	default:
		return kind
	}
}

// parseFloat parses a sensor value string for Prometheus exposition.
// Non-numeric values return an error so callers can skip the series.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// durationFromMs validates a hot-change timestep in milliseconds
// and converts it to a time.Duration. It uses the same strict
// bound as NewServer (> maxTimestepMs) so the two entry points
// cannot disagree about the boundary value itself.
func durationFromMs(ms float64) (time.Duration, error) {
	if ms <= 0 {
		return 0, fmt.Errorf("dtMs must be positive")
	}
	if ms > float64(maxTimestepMs) {
		return 0, fmt.Errorf("dtMs %v exceeds maximum %d", ms, maxTimestepMs)
	}
	d := time.Duration(ms * float64(time.Millisecond))
	if d <= 0 {
		return 0, fmt.Errorf("dtMs %v too small", ms)
	}
	return d, nil
}
