package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/internal/plantinit"
)

// serverSnapshotVersion identifies the -resume state format.
const serverSnapshotVersion = 1

// serverSnapshot is the restartable state written by -resume.
// It deliberately stores server state, not process state: the plant,
// simulated clock, pacing, paused bit, scenario identity, run history,
// operation log, and remaining scenario script. A restarted process
// takes its identity, initial lineup flags, and listener address from
// the new invocation's flags and NewServer arguments, so a snapshot can
// be resumed under a different -name without rewriting its content.
type serverSnapshot struct {
	Version        int                  `json:"version"`
	Plant          core.Plant           `json:"plant"`
	Script         []core.ScriptCommand `json:"script"`
	ScenarioID     string               `json:"scenarioId,omitempty"`
	PlantPath      string               `json:"plantPath,omitempty"`
	CmdPath        string               `json:"cmdPath,omitempty"`
	SimTimeNs      int64                `json:"simTimeNs"`
	RunID          int64                `json:"runId"`
	Paused         bool                 `json:"paused"`
	StepsPerSecond int                  `json:"stepsPerSecond"`
	TimestepNs     int64                `json:"timestepNs"`
	Rev            int64                `json:"rev"`
	StepsTotal     int64                `json:"stepsTotal"`
	Queue          cmdfile.Queue        `json:"queue"`
	Journal        cmdfile.Journal      `json:"journal"`
	Log            cmdfile.Log          `json:"log"`
}

// captureLocked returns an exact snapshot of the current server
// state. The caller must hold the server write lock so the plant,
// clock, and histories form one consistent point in time.
func (s *Server) captureLocked() serverSnapshot {
	script := append([]core.ScriptCommand(nil), s.script...)
	return serverSnapshot{
		Version:        serverSnapshotVersion,
		Plant:          *s.plant,
		Script:         script,
		ScenarioID:     s.scenarioID,
		PlantPath:      s.plantPath,
		CmdPath:        s.cmdPath,
		SimTimeNs:      s.simTimeNs,
		RunID:          s.runId,
		Paused:         s.paused,
		StepsPerSecond: s.stepsPerSecond,
		TimestepNs:     int64(s.dt),
		Rev:            s.rev,
		StepsTotal:     s.stepsTotal,
		Queue:          *s.queue,
		Journal:        *s.journal,
		Log:            *s.log,
	}
}

// saveResumeState marshals one consistent state snapshot and writes
// it atomically: a temporary file in the same directory is renamed
// over the target so a failed write never leaves a partial file.
func saveResumeState(server *Server, path string) error {
	server.mu.Lock()
	b, err := json.MarshalIndent(server.captureLocked(), "", "  ")
	server.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal resume state: %w", err)
	}
	return writeAtomicFile(path, b, "resume state")
}

// readResumeState reads and validates one snapshot file. It is the
// load-side twin of saveResumeState and never mutates the server, so
// a bad file is reported before any state is installed.
func readResumeState(path string) (serverSnapshot, error) {
	var snap serverSnapshot
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snap, err
		}
		return snap, fmt.Errorf("read resume state: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return snap, fmt.Errorf("resume state is empty")
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return snap, fmt.Errorf("parse resume state: %w", err)
	}
	if snap.Version != serverSnapshotVersion {
		return snap, fmt.Errorf("unsupported resume state version %d", snap.Version)
	}
	if snap.StepsPerSecond <= 0 || snap.StepsPerSecond > maxStepsPerSecond {
		return snap, fmt.Errorf("resume state has invalid stepsPerSecond %d", snap.StepsPerSecond)
	}
	if snap.TimestepNs <= 0 || snap.TimestepNs > int64(maxTimestepMs)*int64(time.Millisecond) {
		return snap, fmt.Errorf("resume state has invalid timestep %d", snap.TimestepNs)
	}
	if snap.SimTimeNs < 0 {
		return snap, fmt.Errorf("resume state has negative simTimeNs %d", snap.SimTimeNs)
	}
	return snap, nil
}

// restoreResumeState installs snap onto s after the initial plant
// and scenario have been constructed for validation. Plant and
// control/sensor maps are rebuilt from the saved plant; the saved
// scenario is re-resolved from its ID by plantinit, which is how the
// server obtains the scenario object and progress tracker, while
// the saved script bytes preserve which opening commands remain.
// Fresh startup -cmd scheduling is intentionally not repeated:
// the saved queue, journal, and log already contain that work.
func (s *Server) restoreResumeState(snap serverSnapshot) error {
	if snap.ScenarioID != "" {
		_, _, _, err := plantinit.BuildInitialPlant(snap.ScenarioID, "")
		if err != nil {
			return fmt.Errorf("resume scenario %q: %w", snap.ScenarioID, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	plant := snap.Plant
	s.plant = &plant
	s.controls = plant.GetControlMap()
	s.sensors = plant.GetSensorMap()
	s.script = snap.Script
	s.scenarioID = snap.ScenarioID
	s.plantPath = snap.PlantPath
	s.cmdPath = snap.CmdPath
	s.simTimeNs = snap.SimTimeNs
	s.runId = snap.RunID
	s.paused = snap.Paused
	s.stepsPerSecond = snap.StepsPerSecond
	s.dt = time.Duration(snap.TimestepNs)
	s.rev = snap.Rev
	s.stepsTotal = snap.StepsTotal
	queue := snap.Queue
	journal := snap.Journal
	log := snap.Log
	s.queue = &queue
	s.journal = &journal
	s.log = &log
	if s.shutdown == nil {
		s.shutdown = make(chan struct{})
	}
	if snap.ScenarioID != "" {
		scenario, ok := core.ScenarioByID(snap.ScenarioID)
		if !ok {
			return fmt.Errorf("unknown resumed scenario %q", snap.ScenarioID)
		}
		s.scenario = &scenario
		s.progress = core.NewRunProgress(scenario, snap.SimTimeNs)
	} else {
		s.scenario = nil
		s.progress = nil
	}
	return nil
}
