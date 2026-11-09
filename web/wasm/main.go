//go:build js

// Command wasm is the browser entry point for the simulator.
// It exports a small JavaScript-friendly surface that drives a
// *core.Plant through syscall/js. The TypeScript front-end in
// web/src imports the glue module produced by `go build` and
// calls into these functions.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/core/svg"
)

// genCache memoizes one derived string (sensor batch, objective
// banner, P&ID SVG) alongside the stateGen it was computed at, so
// repeated JS crossings within a single simulation state reuse the
// work instead of re-rendering or re-marshaling per frame.
type genCache struct {
	gen int64
	val string
	err error
}

// plantHandle is the JavaScript-visible wrapper around a
// *core.Plant. JavaScript holds a reference to the wrapper; the
// wrapper keeps the plant pointer. Step / GetSvg / SetControl
// / GetSensor all delegate through this struct.
type plantHandle struct {
	p          *core.Plant
	scenarioID string
	simTimeNs  int64
	createdAt  time.Time
	script     []core.ScriptCommand
	progress   *core.RunProgress
	queue      *cmdfile.Queue
	journal    *cmdfile.Journal
	controls   bridge.ControlMap
	sensors    bridge.SensorMap
	// stateGen bumps on every plant or control mutation; caches are
	// valid only while their gen matches.
	stateGen     int64
	sensorCache  genCache
	objectiveCch genCache
	svgCache     genCache
}

// newPlantFromJSON reconstructs a plantHandle from a
// JSON-serialized *core.Plant (the shape GET /state on nsimserve
// returns via json.Marshal). It must mirror newPlantHandle's
// derived maps so getSvg/sensor/control accessors work on the
// reconstructed plant. Scenario script/progress state is not
// serialized in the plant JSON and is left nil, so objectiveJSON
// correctly reports inactive for remote plants.
func newPlantFromJSON(data string) (*plantHandle, error) {
	var p core.Plant
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, fmt.Errorf("decode plant JSON: %w", err)
	}
	h, err := newPlantHandle("")
	if err != nil {
		return nil, err
	}
	h.p = &p
	h.scenarioID = ""
	h.script = nil
	h.progress = nil
	h.controls = p.GetControlMap()
	h.sensors = p.GetSensorMap()
	h.stateGen = 1
	return h, nil
}

func newPlantHandle(scenarioID string) (*plantHandle, error) {
	p := core.MakePlant()
	var script []core.ScriptCommand
	var progress *core.RunProgress
	if scenarioID != "" {
		sc, ok := core.ScenarioByID(scenarioID)
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q", scenarioID)
		}
		sc.Apply(&p)
		script = append(script, sc.Script...)
		progress = core.NewRunProgress(sc, 0)
	}
	return &plantHandle{
		p:          &p,
		scenarioID: scenarioID,
		createdAt:  time.Now(),
		script:     script,
		progress:   progress,
		queue:      &cmdfile.Queue{},
		journal:    &cmdfile.Journal{},
		controls:   p.GetControlMap(),
		sensors:    p.GetSensorMap(),
		// Zero-value caches carry gen 0; starting at 1 forces the
		// first accessor of each to compute.
		stateGen: 1,
	}, nil
}

// bumpState invalidates every derived cache. Script and queue
// commands mutate control state inside step, so one bump after the
// tick loop covers them; operator sets bump directly.
func (h *plantHandle) bumpState() {
	h.stateGen++
}

// cached returns gc when it matches the current generation,
// otherwise computes, stores, and returns fresh values.
func (h *plantHandle) cached(
	gc *genCache,
	compute func() (string, error),
) (string, error) {
	if gc.gen == h.stateGen {
		return gc.val, gc.err
	}
	gc.gen = h.stateGen
	gc.val, gc.err = compute()
	return gc.val, gc.err
}

// step advances the plant by ticks `Step()`/`Update()` cycles.
// Each cycle is one `core.DefaultTimeStep` (1 ms) of simulated time.
// Queued console commands and the active scenario's opening script
// fire at their exact simulated millisecond, independent of ticks
// per call; scripted actions journal like operator input so exported
// sessions replay faithfully.
func (h *plantHandle) step(ticks int) {
	if ticks <= 0 {
		return
	}
	for range ticks {
		next := h.simTimeNs + int64(core.DefaultTimeStep)
		h.fireDueScript(next)
		h.queue.FireDue(next, h.controls)
		h.p.Step(core.DefaultTimeStep)
		h.p.Update()
		h.simTimeNs = next
		if h.progress != nil {
			h.progress.Observe(h.p, h.simTimeNs)
		}
	}
	h.bumpState()
}

// fireDueScript applies the script prefix that has come due at nowNs,
// journaling each fired command at its fire time.
func (h *plantHandle) fireDueScript(nowNs int64) {
	due := 0
	for due < len(h.script) &&
		h.script[due].AtMs*int64(time.Millisecond) <= nowNs {
		due++
	}
	if due == 0 {
		return
	}
	fired := make([]core.ScriptCommand, due)
	copy(fired, h.script[:due])
	core.ApplyScript(&h.script, nowNs, h.controls)
	for _, cmd := range fired {
		h.journal.Add(cmdfile.Command{
			ControlKey: cmd.ControlKey,
			Value:      cmd.Value,
		}, nowNs)
	}
}

// controlKeys and sensorKeys reflect the cached bridge maps,
// whose closures dereference live plant state on every call (see
// core.GetControlMap), so the lists survive Step/Update cycles.
func (h *plantHandle) controlKeys() []string {
	keys := make([]string, 0, len(h.controls))
	for k := range h.controls {
		keys = append(keys, k)
	}
	return keys
}

func (h *plantHandle) sensorKeys() []string {
	keys := make([]string, 0, len(h.sensors))
	for k := range h.sensors {
		keys = append(keys, k)
	}
	return keys
}

// setControl applies a UI control change and journals it at the
// current simulated time so sessions can be exported and
// replayed with nsimrun.
func (h *plantHandle) setControl(id, value string) error {
	key := cmdfile.ResolveKey(h.controls, id)
	c, ok := h.controls[key]
	if !ok {
		return fmt.Errorf("unknown control %q", id)
	}
	if err := c.Set(value); err != nil {
		return err
	}
	h.bumpState()
	h.journal.Add(cmdfile.Command{ControlKey: key, Value: value}, h.simTimeNs)
	return nil
}

// controlValuesJSON returns {"values":{id:value}} for every
// control so the UI seeds all widget initial states with a single
// JS<->Go crossing per panel rebuild.
func (h *plantHandle) controlValuesJSON() (string, error) {
	values := make(map[string]string, len(h.controls))
	for id, c := range h.controls {
		values[id] = c.Get()
	}
	b, err := json.Marshal(struct {
		Values map[string]string `json:"values"`
	}{values})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// sensorJSON returns the sensor value as a plain string.
func (h *plantHandle) sensorJSON(id string) (string, error) {
	s, ok := h.sensors[id]
	if !ok {
		return "", fmt.Errorf("unknown sensor %q", id)
	}
	return s.Get()
}

// sensorValuesJSON returns {"values":{id:value},"errors":{id:msg}}
// for every sensor so the UI polls all readings with a single
// JS<->Go crossing per frame. The batch is memoized per simulation
// state; idle frames re-cross at string-copy cost.
func (h *plantHandle) sensorValuesJSON() (string, error) {
	return h.cached(&h.sensorCache, h.computeSensorValuesJSON)
}

func (h *plantHandle) computeSensorValuesJSON() (string, error) {
	values := make(map[string]string, len(h.sensors))
	errs := make(map[string]string)
	for id, s := range h.sensors {
		v, err := s.Get()
		if err != nil {
			errs[id] = err.Error()
			continue
		}
		values[id] = v
	}
	out := struct {
		Values map[string]string `json:"values"`
		Errors map[string]string `json:"errors"`
	}{values, errs}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// submitCommands parses console text and schedules each
// command relative to the current simulation time. Scheduled
// entries are journaled at their due time.
func (h *plantHandle) submitCommands(text string) error {
	before := len(h.queue.Entries())
	if err := h.queue.Submit(text, h.simTimeNs); err != nil {
		return err
	}
	for _, e := range h.queue.Entries()[before:] {
		h.journal.Add(e.Cmd, e.DueNs)
	}
	return nil
}

type queueEntryJSON struct {
	Seq    int64  `json:"seq"`
	Text   string `json:"text"`
	DueMs  int64  `json:"dueMs"`
	Status string `json:"status"`
	Err    string `json:"err,omitempty"`
}

type queueJSON struct {
	Rev     int64            `json:"rev"`
	Entries []queueEntryJSON `json:"entries"`
}

// commandQueueRev returns the queue revision so the UI can skip
// marshaling the full snapshot on idle frames.
func (h *plantHandle) commandQueueRev() int64 {
	return h.queue.Rev()
}

// commandQueueJSON snapshots the console queue for the UI list.
func (h *plantHandle) commandQueueJSON() (string, error) {
	entries := h.queue.Entries()
	out := queueJSON{Rev: h.queue.Rev(), Entries: make([]queueEntryJSON, len(entries))}
	for i, e := range entries {
		out.Entries[i] = queueEntryJSON{
			Seq:    e.Seq,
			Text:   e.Text,
			DueMs:  e.DueNs / int64(time.Millisecond),
			Status: e.Status.String(),
			Err:    e.Err,
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// commandLogJSON renders every journaled control command as the
// console-export envelope loadable via `nsimrun -cmd *.json`. The
// Scenario field names the active evolution so a replay rebuilds the
// same starting state without a separate flag.
type consoleExport struct {
	Scenario string            `json:"scenario,omitempty"`
	Commands []cmdfile.Command `json:"commands"`
}

func (h *plantHandle) commandLogJSON() (string, error) {
	out := consoleExport{
		Scenario: h.scenarioID,
		Commands: h.journal.CommandsFrom(0),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// getSvg renders the P&ID document for the current plant state,
// memoized per simulation state so the 1 Hz diagram poll is free
// whenever nothing stepped since the last render.
func (h *plantHandle) getSvg() (string, error) {
	return h.cached(&h.svgCache, h.computeSvg)
}

func (h *plantHandle) computeSvg() (string, error) {
	var buf bytes.Buffer
	opts := svg.RenderOptions{PhaseMs: float64(h.simTimeNs) / 1e6}
	if err := svg.WritePlantSVGWithOptions(&buf, h.p, svg.CurrentLayout(), opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (h *plantHandle) simTimeMs() int64 {
	return h.simTimeNs / int64(time.Millisecond)
}

func (h *plantHandle) plantJSON() (string, error) {
	b, err := json.Marshal(h.p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// objectiveStatus reports the active scenario's completion against
// the live plant. Done latches once met; MetMs carries the simulated
// time it first held.
type objectiveStatus struct {
	Active bool   `json:"active"`
	ID     string `json:"id,omitempty"`
	Title  string `json:"title,omitempty"`
	Done   bool   `json:"done"`
	MetMs  int64  `json:"metMs,omitempty"`
	Detail string `json:"detail"`
}

func (h *plantHandle) objectiveJSON() (string, error) {
	return h.cached(&h.objectiveCch, h.computeObjectiveJSON)
}

func (h *plantHandle) computeObjectiveJSON() (string, error) {
	out := objectiveStatus{}
	if sc, ok := core.ScenarioByID(h.scenarioID); ok {
		res := h.progress.Result(h.p)
		out.Active = true
		out.ID = sc.ID
		out.Title = sc.Title
		out.Done = res.Done
		if m := h.progress.MetMs(); m >= 0 {
			out.MetMs = m
		}
		out.Detail = res.Detail
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// scenariosJSON snapshots the operator-evolution catalog as JSON so
// the front-end can render the selection screen without one JS<->Go
// crossing per entry.
type scenarioInfoJSON struct {
	ID           string `json:"id"`
	Number       int    `json:"number"`
	Category     string `json:"category"`
	Title        string `json:"title"`
	StartState   string `json:"startState"`
	Objective    string `json:"objective"`
	DurationHint string `json:"duration,omitempty"`
}

func scenariosJSON() (string, error) {
	catalog := core.Scenarios()
	out := make([]scenarioInfoJSON, len(catalog))
	for i, s := range catalog {
		out[i] = scenarioInfoJSON{
			ID: s.ID, Number: s.Number, Category: s.Category,
			Title: s.Title, StartState: s.StartState, Objective: s.Objective,
			DurationHint: s.DurationHint,
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// jsStringSlice converts a Go []string into a JS Array<string>.
func jsStringSlice(s []string) js.Value {
	arr := js.Global().Get("Array").New(len(s))
	for i, v := range s {
		arr.SetIndex(i, js.ValueOf(v))
	}
	return arr
}

// jsError boxes a Go error as a JavaScript Error value, the
// convention every handle method uses for failures.
func jsError(err error) js.Value {
	return js.Global().Get("Error").New(err.Error())
}

// jsStringMethod boxes a () -> (string, error) call as a JS method
// returning the string, or a JS Error on failure.
func jsStringMethod(fn func() (string, error)) js.Func {
	return js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := fn()
		if err != nil {
			return jsError(err)
		}
		return s
	})
}

// jsStringArgMethod boxes a (string) -> error call as a JS method.
func jsStringArgMethod(fn func(string) error) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		if err := fn(args[0].String()); err != nil {
			return jsError(err)
		}
		return nil
	})
}

// jsMeta returns {part, label, kind, enum?} for the given
// bridge entry, or an Error if the id is unknown. The
// TypeScript UI uses this to label, group, and unit-tag
// controls and sensors instead of reverse-engineering the id
// string. The two maps are probed in turn; an id absent from
// both is unknown. Enum is only set for
// controls that populate the optional bridge.Control.Enum
// slice; nil slices are skipped so the TS side can use a
// presence test.
func jsMeta(
	id string,
	controls bridge.ControlMap,
	sensors bridge.SensorMap,
) js.Value {
	meta := func(part, label, kind string, enumVals []string) js.Value {
		obj := js.Global().Get("Object").New()
		obj.Set("part", part)
		obj.Set("label", label)
		obj.Set("kind", kind)
		if len(enumVals) > 0 {
			obj.Set("enum", jsStringSlice(enumVals))
		}
		return obj
	}
	if c, ok := controls[id]; ok {
		return meta(c.Part, c.Label, c.Kind, c.Enum)
	}
	if s, ok := sensors[id]; ok {
		return meta(s.Part, s.Label, s.Kind, nil)
	}
	return js.Global().Get("Error").New(fmt.Sprintf("unknown id %q", id))
}

// jsHandle boxes the *plantHandle as a JS object whose methods
// the TypeScript wrapper invokes directly.
func jsHandle(h *plantHandle) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("step", js.FuncOf(func(_ js.Value, args []js.Value) any {
		h.step(args[0].Int())
		return nil
	}))
	obj.Set("controlKeys", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return jsStringSlice(h.controlKeys())
	}))
	obj.Set("sensorKeys", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return jsStringSlice(h.sensorKeys())
	}))
	obj.Set("getMeta", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return jsMeta(args[0].String(), h.controls, h.sensors)
	}))
	obj.Set("setControl", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if err := h.setControl(args[0].String(), args[1].String()); err != nil {
			return jsError(err)
		}
		return nil
	}))
	obj.Set("controlValuesJSON", jsStringMethod(h.controlValuesJSON))
	obj.Set("sensorJSON", js.FuncOf(func(_ js.Value, args []js.Value) any {
		v, err := h.sensorJSON(args[0].String())
		if err != nil {
			return jsError(err)
		}
		return v
	}))
	obj.Set("sensorValuesJSON", jsStringMethod(h.sensorValuesJSON))
	obj.Set("submitCommands", jsStringArgMethod(h.submitCommands))
	obj.Set("commandQueueRev", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return h.commandQueueRev()
	}))
	obj.Set("commandQueueJSON", jsStringMethod(h.commandQueueJSON))
	obj.Set("commandLogJSON", jsStringMethod(h.commandLogJSON))
	obj.Set("getSvg", jsStringMethod(h.getSvg))
	obj.Set("simTimeMs", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return h.simTimeMs()
	}))
	obj.Set("toJSON", jsStringMethod(h.plantJSON))
	obj.Set("objectiveJSON", jsStringMethod(h.objectiveJSON))
	return obj
}

func main() {
	// Expose a single factory to JS. The TypeScript wrapper calls
	// nsimNewPlant() to obtain a handle, then drives it through
	// the methods exposed above.
	// The optional string argument selects a starting scenario from
	// the catalog; an empty or absent argument builds the default
	// full-load plant.
	js.Global().Set("nsimNewPlant", js.FuncOf(func(_ js.Value, args []js.Value) any {
		scenarioID := ""
		if len(args) > 0 && args[0].Type() == js.TypeString {
			scenarioID = args[0].String()
		}
		h, err := newPlantHandle(scenarioID)
		if err != nil {
			return jsError(err)
		}
		return jsHandle(h)
	}))
	js.Global().Set("nsimScenarios", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := scenariosJSON()
		if err != nil {
			return jsError(err)
		}
		return s
	}))
	// nsimPlantFromJSON reconstructs a plant handle from a JSON
	// snapshot of *core.Plant (the shape GET /state on nsimserve
	// returns). Remote mode uses it to render the P&ID without
	// needing a server SVG endpoint: the client already ships the
	// SVG renderer in this wasm blob.
	js.Global().Set("nsimPlantFromJSON", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 || args[0].Type() != js.TypeString {
			return jsError(fmt.Errorf("nsimPlantFromJSON: missing JSON string arg"))
		}
		h, err := newPlantFromJSON(args[0].String())
		if err != nil {
			return jsError(err)
		}
		return jsHandle(h)
	}))

	// Block forever. The wasm module's lifecycle is tied to the
	// JavaScript runtime that loaded it.
	select {}
}
