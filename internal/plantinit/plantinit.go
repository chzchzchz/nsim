// Package plantinit builds an initial plant for the simulator
// binaries. It owns the shared construction rules that used to live
// in cmd/nsimrun: catalog scenario lineups and -plant JSON files
// merged over design defaults.
package plantinit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
)

// LoadCommands reads command operations from path. JSON files hold
// the web-console export envelope (which may name the scenario the
// session ran under); anything else uses the line grammar. The
// returned scenario is empty for line files and for JSON envelopes
// without a scenario name.
func LoadCommands(path string) ([]cmdfile.Command, string, error) {
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
		if env.Commands == nil {
			env.Commands = []cmdfile.Command{}
		}
		return env.Commands, env.Scenario, nil
	}
	cmds, err := cmdfile.Parse(f)
	if cmds == nil {
		cmds = []cmdfile.Command{}
	}
	return cmds, "", err
}

// BuildInitialPlant resolves the starting lineup: a catalog scenario
// when named, otherwise a plantPath JSON merged over design defaults,
// otherwise plain MakePlant. The returned script holds the scenario's
// opening commands for the run loop to fire; the returned scenario is
// nil when none applies.
func BuildInitialPlant(scenarioID, plantPath string) (core.Plant, []core.ScriptCommand, *core.Scenario, error) {
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
