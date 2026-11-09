package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/internal/plantinit"
)

// loadCommands is kept as a thin wrapper over the shared loader in
// internal/plantinit so the batch CLI and the HTTP server parse
// command files through one code path.
func loadCommands(path string) ([]cmdfile.Command, string, error) {
	return plantinit.LoadCommands(path)
}

// buildInitialPlant mirrors the shared builder in
// internal/plantinit; see that package for the construction rules.
func buildInitialPlant(scenarioID, plantPath string) (core.Plant, []core.ScriptCommand, *core.Scenario, error) {
	return plantinit.BuildInitialPlant(scenarioID, plantPath)
}

// applyDue applies every command with offset <= limitMs starting at
// *cmdIndex, advancing it past them. The chunked driver assigned a
// boundary-offset command to the chunk ending there (applied trailing),
// so the snapshot loop calls this with the boundary time just before
// an interior emit; without it the next tick's head-of-loop apply
// would include the command one snapshot too late.
func applyDue(cmds []cmdfile.Command, controlMap bridge.ControlMap, cmdIndex *int, limitMs int64) {
	for *cmdIndex < len(cmds) && cmds[*cmdIndex].TimeOffsetMs <= limitMs {
		if err := cmdfile.Apply(controlMap, cmds[*cmdIndex]); err != nil {
			log.Printf("command error at %d ms: %v", cmds[*cmdIndex].TimeOffsetMs, err)
		}
		*cmdIndex++
	}
}

// runLoop steps p for steps ticks, applying cmds at their absolute
// offsets, and calls emit (if non-nil) at t=0, at each cadence
// boundary, and finally at total. emit's elapsedMs is the simulated
// time at the emit point and matches the times list Python charts.
// RunProgress observation and trailing-offset applies match the
// single-run path; emitting does not perturb progress timing.
func runLoop(p *core.Plant, cmds []cmdfile.Command, controlMap bridge.ControlMap, progress *core.RunProgress, steps int, dt time.Duration, cadenceMs int64, emit func(elapsedMs int64, p core.Plant) error) error {
	if emit != nil {
		if err := emit(0, *p); err != nil {
			return err
		}
	}
	cmdIndex := 0
	var nextEmit int64
	if cadenceMs > 0 {
		nextEmit = cadenceMs
	}
	totalMs := int64(steps) * int64(dt.Milliseconds())
	for i := 0; i < steps; i++ {
		elapsedMs := i * int(dt.Milliseconds())
		applyDue(cmds, controlMap, &cmdIndex, int64(elapsedMs))
		p.Step(dt)
		p.Update()
		if progress != nil {
			progress.Observe(p, int64(elapsedMs)*int64(time.Millisecond))
		}
		if emit != nil && cadenceMs > 0 && int64((i+1)*int(dt.Milliseconds())) >= nextEmit {
			at := int64((i + 1) * int(dt.Milliseconds()))
			if at < totalMs {
				applyDue(cmds, controlMap, &cmdIndex, at)
				if err := emit(at, *p); err != nil {
					return err
				}
			}
			nextEmit += cadenceMs
		}
	}
	applyDue(cmds, controlMap, &cmdIndex, totalMs)
	// Commands past total still apply after the final step (documented
	// -cmd behavior); the final snapshot must reflect them.
	for cmdIndex < len(cmds) {
		if err := cmdfile.Apply(controlMap, cmds[cmdIndex]); err != nil {
			log.Printf("command error at %d ms: %v", cmds[cmdIndex].TimeOffsetMs, err)
		}
		cmdIndex++
	}
	if emit != nil && steps > 0 {
		if err := emit(totalMs, *p); err != nil {
			return err
		}
	}
	return nil
}

// sampleTicks mirrors Python's whole-run step recompute
// (n = ceil(total/step), step = total/n) under Go's tick truncation.
// It returns the total tick count and the per-snapshot cadence in ms
// that the old chunked driver charted. Callers pass stepSeconds <= 0
// through as no-sampling.
func sampleTicks(totalSeconds, stepSeconds float64, dt time.Duration) (steps int, cadenceMs int64) {
	if stepSeconds <= 0 || totalSeconds <= 0 {
		return 0, int64(stepSeconds * 1000)
	}
	n := int(ceilTotalSteps(totalSeconds, stepSeconds))
	per := totalSeconds / float64(n)
	ticks := int(per / dt.Seconds())
	return n * ticks, int64(ticks) * int64(dt.Milliseconds())
}

func ceilTotalSteps(total, step float64) float64 {
	q := total / step
	i := int64(q)
	if float64(i) < q {
		i++
	}
	return float64(i)
}

func main() {
	plantPath := flag.String("plant", "", "plant JSON file (defaults to MakePlant)")
	seconds := flag.Float64("seconds", 0, "number of seconds to step")
	cmdPath := flag.String("cmd", "", "command file: TimeOffset Part.Label Value lines, or web-exported *.json")
	scenarioID := flag.String("scenario", "", "starting scenario id from the catalog (see core.Scenarios); cannot be combined with -plant")
	timestep := flag.Duration("timestep", core.DefaultTimeStep, "per-step simulated duration (default 1ms)")
	snapshotStep := flag.Float64("snapshot-step", 0, "emit plant snapshots every N seconds of simulated time as JSONL (cadence = total/ceil(total/N))")
	snapshotOut := flag.String("snapshot-out", "", "write snapshot JSONL to this file instead of stdout")
	flag.Parse()
	if *timestep <= 0 {
		log.Fatalf("-timestep must be positive, got %v", *timestep)
	}
	if *snapshotStep < 0 {
		log.Fatalf("-snapshot-step must be non-negative, got %v", *snapshotStep)
	}
	dt := *timestep

	// Commands load first: a web export names the scenario its
	// session ran under, which fills in an absent -scenario flag.
	var cmds []cmdfile.Command
	fileScenario := ""
	if *cmdPath != "" {
		var err error
		cmds, fileScenario, err = loadCommands(*cmdPath)
		if err != nil {
			log.Fatalf("failed to load command file: %v", err)
		}
	}
	effective := *scenarioID
	switch {
	case effective == "":
		effective = fileScenario
	case fileScenario != "" && fileScenario != effective:
		log.Fatalf("-scenario %q conflicts with export scenario %q",
			effective, fileScenario)
	}

	p, script, sc, err := buildInitialPlant(effective, *plantPath)
	if err != nil {
		log.Fatalf("failed to build initial plant: %v", err)
	}

	controlMap := p.GetControlMap()
	var progress *core.RunProgress
	if sc != nil {
		progress = core.NewRunProgress(*sc, 0)
	}

	// The scenario's opening script rides ahead of any -cmd file at
	// equal offsets; both lists are consumed by the same sorted walk.
	scriptCmds := make([]cmdfile.Command, 0, len(script))
	for _, c := range script {
		scriptCmds = append(scriptCmds, cmdfile.Command{
			TimeOffsetMs: c.AtMs,
			ControlKey:   c.ControlKey,
			Value:        c.Value,
		})
	}
	cmds = append(scriptCmds, cmds...)
	sort.SliceStable(cmds, func(i, j int) bool {
		return cmds[i].TimeOffsetMs < cmds[j].TimeOffsetMs
	})

	steps := int(*seconds / dt.Seconds())
	if *snapshotStep > 0 {
		steps, cadenceMs := sampleTicks(*seconds, *snapshotStep, dt)
		out := io.Writer(os.Stdout)
		if *snapshotOut != "" {
			f, err := os.Create(*snapshotOut)
			if err != nil {
				log.Fatalf("failed to open -snapshot-out: %v", err)
			}
			defer f.Close()
			out = f
		}
		enc := json.NewEncoder(out)
		emit := func(elapsedMs int64, snap core.Plant) error {
			return enc.Encode(snap)
		}
		if err := runLoop(&p, cmds, controlMap, progress, steps, dt, cadenceMs, emit); err != nil {
			log.Fatalf("snapshot emit failed: %v", err)
		}
		// Alarms ride as one trailer line so chart consumers can read
		// them without a per-snapshot reparse; snapshot lines decode
		// as bare Plants.
		if err := enc.Encode(map[string]json.RawMessage{"Alarms": mustAlarms(p)}); err != nil {
			log.Fatalf("failed to marshal alarms trailer: %v", err)
		}
		printProgress(p, sc, progress)
		printAlarmSummary(p, steps, dt)
		return
	}
	if err := runLoop(&p, cmds, controlMap, progress, steps, dt, 0, nil); err != nil {
		log.Fatalf("run failed: %v", err)
	}

	// Output stays a single top-level plant document so -plant
	// chaining keeps working; the alarm list rides alongside as one
	// extra key (unknown keys are ignored on re-ingest). Mechanical
	// Phase-4 checks read it instead of scraping stderr.
	alarmsJSON, err := json.Marshal(p.Alarms())
	if err != nil {
		log.Fatalf("failed to marshal alarms: %v", err)
	}
	var out []byte
	if len(alarmsJSON) <= 2 {
		// No active alarms: marshal the plant directly and add the
		// empty alarm list by string surgery, instead of round-tripping
		// the document through a RawMessage map to inject it.
		out, err = json.Marshal(p)
		if err != nil {
			log.Fatalf("failed to marshal plant JSON: %v", err)
		}
		out = append([]byte(`{"Alarms":null,`), out[1:]...)
	} else {
		plantJSON, err := json.Marshal(p)
		if err != nil {
			log.Fatalf("failed to marshal plant JSON: %v", err)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(plantJSON, &doc); err != nil {
			log.Fatalf("failed to reparse plant JSON: %v", err)
		}
		doc["Alarms"] = alarmsJSON
		out, err = json.Marshal(doc)
		if err != nil {
			log.Fatalf("failed to marshal output document: %v", err)
		}
	}
	fmt.Println(string(out))

	printProgress(p, sc, progress)
	printAlarmSummary(p, steps, dt)
}

func printProgress(p core.Plant, sc *core.Scenario, progress *core.RunProgress) {
	if sc == nil || progress == nil {
		return
	}
	res := progress.Result(&p)
	if res.Done {
		fmt.Fprintf(os.Stderr, "objective: MET (%s)\n", res.Detail)
	} else {
		fmt.Fprintf(os.Stderr, "objective pending: %s\n", res.Detail)
	}
}

func printAlarmSummary(p core.Plant, steps int, dt time.Duration) {
	// End-of-run alarm summary on stderr so scenario replays can
	// "note any alarms encountered during the power changes"
	// (specs/pwr_sim/chapter_3/page_039.txt:44-46); stdout stays a
	// clean single JSON document for -plant chaining.
	for _, a := range p.Alarms() {
		fmt.Fprintf(os.Stderr, "alarm at %d ms: %s\n",
			steps*int(dt.Milliseconds()), a)
	}
}

func mustAlarms(p core.Plant) json.RawMessage {
	b, err := json.Marshal(p.Alarms())
	if err != nil {
		log.Fatalf("failed to marshal alarms: %v", err)
	}
	return b
}
