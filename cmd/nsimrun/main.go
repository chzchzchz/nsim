package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
)

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

	var cmds []cmdfile.Command
	if *cmdPath != "" {
		f, err := os.Open(*cmdPath)
		if err != nil {
			log.Fatalf("failed to open command file: %v", err)
		}
		defer f.Close()
		cmds, err = cmdfile.Parse(f)
		if err != nil {
			log.Fatalf("failed to parse command file: %v", err)
		}
	}

	steps := int(*seconds / core.TimeStep.Seconds())
	cmdIndex := 0
	for i := 0; i < steps; i++ {
		elapsedMs := i * int(core.TimeStep.Milliseconds())
		for cmdIndex < len(cmds) && cmds[cmdIndex].TimeOffsetMs <= int64(elapsedMs) {
			if err := cmdfile.Apply(controlMap, cmds[cmdIndex]); err != nil {
				log.Printf("command error at %d ms: %v", cmds[cmdIndex].TimeOffsetMs, err)
			}
			cmdIndex++
		}
		p.Step()
		p.Update()
	}
	for cmdIndex < len(cmds) {
		if err := cmdfile.Apply(controlMap, cmds[cmdIndex]); err != nil {
			log.Printf("command error at %d ms: %v", cmds[cmdIndex].TimeOffsetMs, err)
		}
		cmdIndex++
	}

	out, err := json.Marshal(p)
	if err != nil {
		log.Fatalf("failed to marshal plant JSON: %v", err)
	}
	fmt.Println(string(out))
}
