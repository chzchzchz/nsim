package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/core"
)

type command struct {
	timeOffsetMs int
	controlKey   string
	value        string
}

func parseCommandFile(r io.Reader) ([]command, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read command file: %w", err)
	}
	var cmds []command
	for lineNum, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("line %d: want 3 fields (TimeOffset Part.Label Value), got %d", lineNum+1, len(fields))
		}
		var offset int
		if _, err := fmt.Sscanf(fields[0], "%d", &offset); err != nil {
			return nil, fmt.Errorf("line %d: invalid TimeOffset %q: %w", lineNum+1, fields[0], err)
		}
		cmds = append(cmds, command{
			timeOffsetMs: offset,
			controlKey:   fields[1],
			value:        fields[2],
		})
	}
	return cmds, nil
}

func resolveControlKey(m bridge.ControlMap, raw string) string {
	if _, ok := m[raw]; ok {
		return raw
	}
	slashKey := strings.ReplaceAll(raw, ".", "/")
	if _, ok := m[slashKey]; ok {
		return slashKey
	}
	return raw
}

func applyCommand(p *core.Plant, m bridge.ControlMap, cmd command) error {
	key := resolveControlKey(m, cmd.controlKey)
	c, ok := m[key]
	if !ok {
		return fmt.Errorf("unknown control %q", cmd.controlKey)
	}
	if c.Set == nil {
		return fmt.Errorf("control %q has no Set function", cmd.controlKey)
	}
	if err := c.Set(cmd.value); err != nil {
		return fmt.Errorf("set control %q to %q: %w", cmd.controlKey, cmd.value, err)
	}
	return nil
}

func main() {
	plantPath := flag.String("plant", "", "plant JSON file (defaults to MakePlant)")
	seconds := flag.Float64("seconds", 0, "number of seconds to step")
	cmdPath := flag.String("cmd", "", "command file (TimeOffset Part.Label Value per line)")
	flag.Parse()

	p := core.MakePlant()
	if *plantPath != "" {
		data, err := os.ReadFile(*plantPath)
		if err != nil {
			log.Fatalf("failed to read plant file: %v", err)
		}
		defaults, err := json.Marshal(p)
		if err != nil {
			log.Fatalf("failed to marshal defaults: %v", err)
		}
		var defMap, usrMap map[string]json.RawMessage
		if err := json.Unmarshal(defaults, &defMap); err != nil {
			log.Fatalf("failed to parse defaults: %v", err)
		}
		if err := json.Unmarshal(data, &usrMap); err != nil {
			log.Fatalf("failed to parse plant JSON: %v", err)
		}
		for k, v := range usrMap {
			defMap[k] = v
		}
		merged, err := json.Marshal(defMap)
		if err != nil {
			log.Fatalf("failed to merge defaults: %v", err)
		}
		if err := json.Unmarshal(merged, &p); err != nil {
			log.Fatalf("failed to apply merged defaults: %v", err)
		}
	}

	controlMap := p.GetControlMap()

	var cmds []command
	if *cmdPath != "" {
		f, err := os.Open(*cmdPath)
		if err != nil {
			log.Fatalf("failed to open command file: %v", err)
		}
		defer f.Close()
		cmds, err = parseCommandFile(f)
		if err != nil {
			log.Fatalf("failed to parse command file: %v", err)
		}
	}

	steps := int(*seconds / core.TimeStep.Seconds())
	cmdIndex := 0
	for i := 0; i < steps; i++ {
		elapsedMs := i * int(core.TimeStep.Milliseconds())
		for cmdIndex < len(cmds) && cmds[cmdIndex].timeOffsetMs <= elapsedMs {
			if err := applyCommand(&p, controlMap, cmds[cmdIndex]); err != nil {
				log.Printf("command error at %d ms: %v", cmds[cmdIndex].timeOffsetMs, err)
			}
			cmdIndex++
		}
		p.Step()
		p.Update()
	}
	for cmdIndex < len(cmds) {
		if err := applyCommand(&p, controlMap, cmds[cmdIndex]); err != nil {
			log.Printf("command error at %d ms: %v", cmds[cmdIndex].timeOffsetMs, err)
		}
		cmdIndex++
	}

	out, err := json.Marshal(p)
	if err != nil {
		log.Fatalf("failed to marshal plant JSON: %v", err)
	}
	fmt.Println(string(out))
}