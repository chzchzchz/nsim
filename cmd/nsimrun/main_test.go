package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func captureLogs(fn func()) []string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	fn()
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func TestParseCommandFile(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cmds, err := parseCommandFile(strings.NewReader(
			"0 ReactorBuilding.ReactorCoolantPump.Running 1\n"+
				"500 AuxiliaryBuilding.DieselGenerator.Start Manual\n"+
				"\n"+
				"# comment\n"+
				"1000\tReactorBuilding/ReactorPressureVessel/ControlRods/Insertion\t0.5\n",
		))
		require.NoError(t, err)
		require.Len(t, cmds, 3)
		assert.Equal(t, 0, cmds[0].timeOffsetMs)
		assert.Equal(t, "ReactorBuilding.ReactorCoolantPump.Running", cmds[0].controlKey)
		assert.Equal(t, "1", cmds[0].value)
		assert.Equal(t, 500, cmds[1].timeOffsetMs)
		assert.Equal(t, "AuxiliaryBuilding.DieselGenerator.Start", cmds[1].controlKey)
		assert.Equal(t, "Manual", cmds[1].value)
		assert.Equal(t, 1000, cmds[2].timeOffsetMs)
		assert.Equal(t, "ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", cmds[2].controlKey)
		assert.Equal(t, "0.5", cmds[2].value)
	})

	t.Run("read error", func(t *testing.T) {
		errReader := &errorReader{err: fmt.Errorf("read failure")}
		_, err := parseCommandFile(errReader)
		require.Error(t, err)
	})

	t.Run("bad fields", func(t *testing.T) {
		_, err := parseCommandFile(strings.NewReader("only_one_field\n"))
		require.Error(t, err)
	})

	t.Run("bad offset", func(t *testing.T) {
		_, err := parseCommandFile(strings.NewReader("abc Part.Label 1\n"))
		require.Error(t, err)
	})
}

func TestResolveControlKey(t *testing.T) {
	m := bridge.ControlMap{
		"ReactorBuilding/ReactorCoolantPump/Running":  {},
		"ReactorBuilding/ReactorPressureVessel/Trip": {},
	}
	assert.Equal(t, "ReactorBuilding/ReactorCoolantPump/Running", resolveControlKey(m, "ReactorBuilding/ReactorCoolantPump/Running"))
	assert.Equal(t, "ReactorBuilding/ReactorCoolantPump/Running", resolveControlKey(m, "ReactorBuilding.ReactorCoolantPump.Running"))
	assert.Equal(t, "ReactorBuilding/ReactorPressureVessel/Trip", resolveControlKey(m, "ReactorBuilding.ReactorPressureVessel.Trip"))
	assert.Equal(t, "unknown.key", resolveControlKey(m, "unknown.key"))
}

func TestApplyCommand(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		c := m["ReactorBuilding/ReactorCoolantPump/Running"]
		require.NotNil(t, c)
		require.NoError(t, applyCommand(&p, m, command{controlKey: "ReactorBuilding/ReactorCoolantPump/Running", value: "1"}))
		assert.True(t, p.ReactorBuilding.ReactorCoolantPump.Running)
	})

	t.Run("dot notation", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		require.NoError(t, applyCommand(&p, m, command{controlKey: "ReactorBuilding.ReactorCoolantPump.Running", value: "1"}))
		assert.True(t, p.ReactorBuilding.ReactorCoolantPump.Running)
	})

	t.Run("unknown control", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		err := applyCommand(&p, m, command{controlKey: "NoSuch/Control", value: "1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown control")
	})

	t.Run("set error", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		c := m["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"]
		require.NotNil(t, c)
		err := applyCommand(&p, m, command{controlKey: "ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", value: "bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "set control")
	})
}

func TestNSimRunCommandIntegration(t *testing.T) {
	defaults, err := json.Marshal(core.MakePlant())
	require.NoError(t, err)

	// Simulate a partial plant JSON by round-tripping defaults
	// through a map so the merge path is exercised.
	var defMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(defaults, &defMap))

	logs := captureLogs(func() {
		// Simulate what main does by calling the relevant logic
		// directly (test the integration of parsing + apply + step).
		p := core.MakePlant()
		usrMap := make(map[string]json.RawMessage)
		for k, v := range defMap {
			usrMap[k] = v
		}
		for k, v := range usrMap {
			defMap[k] = v
		}
		merged, err := json.Marshal(defMap)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(merged, &p))

		cmds, err := parseCommandFile(strings.NewReader(
			"0 ReactorBuilding.ReactorCoolantPump.Running 1\n"+
				"1000 ReactorBuilding.ReactorCoolantPump.Running 0\n",
		))
		require.NoError(t, err)
		m := p.GetControlMap()
		steps := int(1.5 / core.TimeStep.Seconds())
		cmdIndex := 0
		for i := 0; i < steps; i++ {
			elapsedMs := i * int(core.TimeStep.Milliseconds())
			for cmdIndex < len(cmds) && cmds[cmdIndex].timeOffsetMs <= elapsedMs {
				require.NoError(t, applyCommand(&p, m, cmds[cmdIndex]))
				cmdIndex++
			}
			p.Step()
			p.Update()
		}

		result, err := json.Marshal(p)
		require.NoError(t, err)
		var final core.Plant
		require.NoError(t, json.Unmarshal(result, &final))
		assert.False(t, final.ReactorBuilding.ReactorCoolantPump.Running)
	})

	assert.Empty(t, logs, "expected no log errors, got: %v", logs)
}