package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

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

func main() {
	plantPath := flag.String("plant", "", "plant JSON file (defaults to MakePlant)")
	seconds := flag.Float64("seconds", 0, "number of seconds to step")
	cmdPath := flag.String("cmd", "", "command file: TimeOffset Part.Label Value lines, or web-exported *.json")
	scenarioID := flag.String("scenario", "", "starting scenario id from the catalog (see core.Scenarios); cannot be combined with -plant")
	timestep := flag.Duration("timestep", core.DefaultTimeStep, "per-step simulated duration (default 1ms)")
	flag.Parse()
	if *timestep <= 0 {
		log.Fatalf("-timestep must be positive, got %v", *timestep)
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
	cmdIndex := 0
	for i := 0; i < steps; i++ {
		elapsedMs := i * int(dt.Milliseconds())
		for cmdIndex < len(cmds) && cmds[cmdIndex].TimeOffsetMs <= int64(elapsedMs) {
			if err := cmdfile.Apply(controlMap, cmds[cmdIndex]); err != nil {
				log.Printf("command error at %d ms: %v", cmds[cmdIndex].TimeOffsetMs, err)
			}
			cmdIndex++
		}
		p.Step(dt)
		p.Update()
		if progress != nil {
			progress.Observe(&p, int64(elapsedMs)*int64(time.Millisecond))
		}
	}
	for cmdIndex < len(cmds) {
		if err := cmdfile.Apply(controlMap, cmds[cmdIndex]); err != nil {
			log.Printf("command error at %d ms: %v", cmds[cmdIndex].TimeOffsetMs, err)
		}
		cmdIndex++
	}
	if progress != nil {
		res := progress.Result(&p)
		if res.Done {
			fmt.Fprintf(os.Stderr, "objective: MET (%s)\n", res.Detail)
		} else {
			fmt.Fprintf(os.Stderr, "objective pending: %s\n", res.Detail)
		}
	}

	// Output stays a single top-level plant document so -plant
	// chaining keeps working; the alarm list rides alongside as one
	// extra key (unknown keys are ignored on re-ingest). Mechanical
	// Phase-4 checks read it instead of scraping stderr.
	plantJSON, err := json.Marshal(p)
	if err != nil {
		log.Fatalf("failed to marshal plant JSON: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(plantJSON, &doc); err != nil {
		log.Fatalf("failed to reparse plant JSON: %v", err)
	}
	alarmsJSON, err := json.Marshal(p.Alarms())
	if err != nil {
		log.Fatalf("failed to marshal alarms: %v", err)
	}
	doc["Alarms"] = alarmsJSON
	out, err := json.Marshal(doc)
	if err != nil {
		log.Fatalf("failed to marshal output document: %v", err)
	}
	fmt.Println(string(out))

	// End-of-run alarm summary on stderr so scenario replays can
	// "note any alarms encountered during the power changes"
	// (specs/pwr_sim/chapter_3/page_039.txt:44-46); stdout stays a
	// clean single JSON document for -plant chaining.
	for _, a := range p.Alarms() {
		fmt.Fprintf(os.Stderr, "alarm at %d ms: %s\n",
			steps*int(core.DefaultTimeStep.Milliseconds()), a)
	}
}
