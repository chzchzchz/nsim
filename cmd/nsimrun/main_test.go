package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadCommands(t *testing.T) {
	t.Run("json envelope", func(t *testing.T) {
		path := writeTemp(t, "cmds.json",
			`{"commands":[{"offsetMs":500,"control":"Part/Label","value":"1"}]}`)
		cmds, err := loadCommands(path)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, int64(500), cmds[0].TimeOffsetMs)
		assert.Equal(t, "Part/Label", cmds[0].ControlKey)
	})

	t.Run("json extension is case insensitive", func(t *testing.T) {
		path := writeTemp(t, "cmds.JSON", `{"commands":[]}`)
		cmds, err := loadCommands(path)
		require.NoError(t, err)
		assert.Empty(t, cmds)
	})

	t.Run("text lines", func(t *testing.T) {
		path := writeTemp(t, "cmds.txt", "100 Part.Label 1\n")
		cmds, err := loadCommands(path)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, int64(100), cmds[0].TimeOffsetMs)
		assert.Equal(t, "Part.Label", cmds[0].ControlKey)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := loadCommands(filepath.Join(t.TempDir(), "nope.txt"))
		require.Error(t, err)
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

		cmds, err := cmdfile.Parse(strings.NewReader(
			"0 ReactorBuilding.ReactorCoolantPump.Running 1\n"+
				"1000 ReactorBuilding.ReactorCoolantPump.Running 0\n",
		))
		require.NoError(t, err)
		m := p.GetControlMap()
		steps := int(1.5 / core.TimeStep.Seconds())
		cmdIndex := 0
		for i := 0; i < steps; i++ {
			elapsedMs := i * int(core.TimeStep.Milliseconds())
			for cmdIndex < len(cmds) && cmds[cmdIndex].TimeOffsetMs <= int64(elapsedMs) {
				require.NoError(t, cmdfile.Apply(m, cmds[cmdIndex]))
				cmdIndex++
			}
			p.Step()
			p.Update()
		}

		result, err := json.Marshal(p)
		require.NoError(t, err)
		var final core.Plant
		require.NoError(t, json.Unmarshal(result, &final))
		require.False(t, final.ReactorBuilding.ReactorCoolantPump.Running)
	})

	require.Empty(t, logs, "expected no log errors, got: %v", logs)
}
