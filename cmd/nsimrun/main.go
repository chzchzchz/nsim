package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
)

// loadCommands dispatches on file extension: *.json files hold
// the web console export envelope (which may also name the scenario
// the session ran under), anything else uses the line grammar (see
// cmd/nsimrun/README.md).
func loadCommands(path string) ([]cmdfile.Command, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	if strings.EqualFold(filepath.Ext(path), ".json") {
		var env struct {
			Scenario string            `json:"scenario"`
			Commands []cmdfile.Command `json:"commands"`
		}
		if err := json.NewDecoder(f).Decode(&env); err != nil {
			return nil, "", fmt.Errorf("parse command JSON: %w", err)
		}
		return env.Commands, env.Scenario, nil
	}
	cmds, err := cmdfile.Parse(f)
	return cmds, "", err
}

// buildInitialPlant resolves the starting lineup: a catalog scenario
// when named, otherwise a -plant JSON merged over design defaults,
// otherwise plain MakePlant. The returned script holds the scenario's
// opening commands for the run loop to fire; the returned scenario is
// nil when none applies.
func buildInitialPlant(scenarioID, plantPath string) (core.Plant, []core.ScriptCommand, *core.Scenario, error) {
	if scenarioID != "" && plantPath != "" {
		return core.Plant{}, nil, nil,
			fmt.Errorf("-scenario and -plant are mutually exclusive")
	}
	p := core.MakePlant()
	var script []core.ScriptCommand
	var sc *core.Scenario
	switch {
	case scenarioID != "":
		s, ok := core.ScenarioByID(scenarioID)
		if !ok {
			return core.Plant{}, nil, nil, fmt.Errorf("unknown scenario %q", scenarioID)
		}
		s.Apply(&p)
		script = s.Script
		sc = &s
	case plantPath != "":
		data, err := os.ReadFile(plantPath)
		if err != nil {
			return core.Plant{}, nil, nil, fmt.Errorf("read plant file: %w", err)
		}
		defaults, err := json.Marshal(p)
		if err != nil {
			return core.Plant{}, nil, nil, fmt.Errorf("marshal defaults: %w", err)
		}
		var defMap, usrMap map[string]json.RawMessage
		if err := json.Unmarshal(defaults, &defMap); err != nil {
			return core.Plant{}, nil, nil, fmt.Errorf("parse defaults: %w", err)
		}
		if err := json.Unmarshal(data, &usrMap); err != nil {
			return core.Plant{}, nil, nil, fmt.Errorf("parse plant JSON: %w", err)
		}
		for k, v := range usrMap {
			defMap[k] = v
		}
		merged, err := json.Marshal(defMap)
		if err != nil {
			return core.Plant{}, nil, nil, fmt.Errorf("merge defaults: %w", err)
		}
		if err := json.Unmarshal(merged, &p); err != nil {
			return core.Plant{}, nil, nil, fmt.Errorf("apply merged defaults: %w", err)
		}
	}
	return p, script, sc, nil
}

func main() {
	plantPath := flag.String("plant", "", "plant JSON file (defaults to MakePlant)")
	seconds := flag.Float64("seconds", 0, "number of seconds to step")
	cmdPath := flag.String("cmd", "", "command file: TimeOffset Part.Label Value lines, or web-exported *.json")
	scenarioID := flag.String("scenario", "", "starting scenario id from the catalog (see core.Scenarios); cannot be combined with -plant")
	flag.Parse()

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

	steps := int(*seconds / core.DefaultTimeStep.Seconds())
	cmdIndex := 0
	for i := 0; i < steps; i++ {
		elapsedMs := i * int(core.DefaultTimeStep.Milliseconds())
		for cmdIndex < len(cmds) && cmds[cmdIndex].TimeOffsetMs <= int64(elapsedMs) {
			if err := cmdfile.Apply(controlMap, cmds[cmdIndex]); err != nil {
				log.Printf("command error at %d ms: %v", cmds[cmdIndex].TimeOffsetMs, err)
			}
			cmdIndex++
		}
		p.Step(core.DefaultTimeStep)
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
