package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		cmds, _, err := loadCommands(path)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, int64(500), cmds[0].TimeOffsetMs)
		assert.Equal(t, "Part/Label", cmds[0].ControlKey)
	})

	t.Run("json envelope names its scenario", func(t *testing.T) {
		path := writeTemp(t, "session.json",
			`{"scenario":"rcp-trip","commands":[]}`)
		_, scenario, err := loadCommands(path)
		require.NoError(t, err)
		assert.Equal(t, "rcp-trip", scenario)
	})

	t.Run("json extension is case insensitive", func(t *testing.T) {
		path := writeTemp(t, "cmds.JSON", `{"commands":[]}`)
		cmds, _, err := loadCommands(path)
		require.NoError(t, err)
		assert.Empty(t, cmds)
	})

	t.Run("text lines", func(t *testing.T) {
		path := writeTemp(t, "cmds.txt", "100 Part.Label 1\n")
		cmds, _, err := loadCommands(path)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, int64(100), cmds[0].TimeOffsetMs)
		assert.Equal(t, "Part.Label", cmds[0].ControlKey)
	})

	t.Run("missing file", func(t *testing.T) {
		_, _, err := loadCommands(filepath.Join(t.TempDir(), "nope.txt"))
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
			"0 ReactorBuilding.ReactorCoolantPump.Running 1\n" +
				"1000 ReactorBuilding.ReactorCoolantPump.Running 0\n",
		))
		require.NoError(t, err)
		m := p.GetControlMap()
		steps := int(1.5 / core.DefaultTimeStep.Seconds())
		cmdIndex := 0
		for i := 0; i < steps; i++ {
			elapsedMs := i * int(core.DefaultTimeStep.Milliseconds())
			for cmdIndex < len(cmds) && cmds[cmdIndex].TimeOffsetMs <= int64(elapsedMs) {
				require.NoError(t, cmdfile.Apply(m, cmds[cmdIndex]))
				cmdIndex++
			}
			p.Step(core.DefaultTimeStep)
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

// buildInitialPlant must land the catalog lineup and hand back its
// opening script; -plant stays exclusive with -scenario.
func TestBuildInitialPlant(t *testing.T) {
	t.Run("default plant without flags", func(t *testing.T) {
		p, script, sc, err := buildInitialPlant("", "")
		require.NoError(t, err)
		assert.Nil(t, sc, "no flag means free play")
		assert.Equal(t, core.MakePlant().ReactorBuilding.ReactorPressureVessel.Inlet,
			p.ReactorBuilding.ReactorPressureVessel.Inlet)
		assert.Empty(t, script)
	})

	t.Run("scenario applies its lineup and script", func(t *testing.T) {
		p, script, sc, err := buildInitialPlant("rcp-trip", "")
		require.NoError(t, err)
		require.NotNil(t, sc)
		// The at-power lineup opens synchronized; the loop-A trip
		// rides the script instead of the initial state.
		assert.True(t,
			p.ReactorBuilding.ReactorCoolantPumps.LoopA.Running)
		require.Len(t, script, 1)
		assert.Equal(t,
			"ReactorBuilding/ReactorCoolantPumps/A/Running", script[0].ControlKey)

		cm := p.GetControlMap()
		core.ApplyScript(&script, 30_000*int64(time.Millisecond), cm)
		assert.False(t, p.ReactorBuilding.ReactorCoolantPumps.LoopA.Running,
			"the scripted trip fires once due")
	})

	t.Run("mutually exclusive flags are rejected", func(t *testing.T) {
		_, _, _, err := buildInitialPlant("rcp-trip", "whatever.json")
		require.ErrorContains(t, err, "mutually exclusive")
	})
}

// Snapshots must carry the same trajectories the single-document path
// produces: cadence derived from whole-run step recompute with Go's
// per-chunk tick truncation, boundary commands included in their
// boundary snapshot, trailing commands in the final snapshot.
func TestSnapshotCadenceAndEmission(t *testing.T) {
	t.Run("cadence matches Python recompute", func(t *testing.T) {
		_, cadence := sampleTicks(5, 1, core.DefaultTimeStep)
		require.Equal(t, int64(1000), cadence)
		// 5/3 chunks of 1.6667s, truncated to 1666 ticks each.
		_, cadence = sampleTicks(5, 2, core.DefaultTimeStep)
		require.Equal(t, int64(1666), cadence)
		// 3s with step 10s: one chunk covering the whole run.
		_, cadence = sampleTicks(3, 10, core.DefaultTimeStep)
		require.Equal(t, int64(3000), cadence)
	})

	t.Run("runLoop emits t0 interior and final in order", func(t *testing.T) {
		p := core.MakePlant()
		steps := 5000
		_, cadenceMs := sampleTicks(5, 1, core.DefaultTimeStep)
		var at []int64
		err := runLoop(&p, nil, p.GetControlMap(), nil, steps, core.DefaultTimeStep, cadenceMs,
			func(elapsedMs int64, snap core.Plant) error {
				at = append(at, elapsedMs)
				return nil
			})
		require.NoError(t, err)
		assert.Equal(t, []int64{0, 1000, 2000, 3000, 4000, 5000}, at)
	})

	t.Run("boundary command lands in its boundary snapshot", func(t *testing.T) {
		p := core.MakePlant()
		cmds, err := cmdfile.Parse(strings.NewReader(
			"1000 ReactorBuilding/ReactorCoolantPump/Running 0\n"))
		require.NoError(t, err)
		// t=0 plus interior at 1000ms plus final at 2000ms: 3 plants.
		var snaps []core.Plant
		err = runLoop(&p, cmds, p.GetControlMap(), nil, 1500, core.DefaultTimeStep, 1000,
			func(_ int64, snap core.Plant) error {
				snaps = append(snaps, snap)
				return nil
			})
		require.NoError(t, err)
		require.Len(t, snaps, 3)
		// The control map writes through to the loop pumps, so check
		// whichever pump the command actually drives.
		assert.False(t, snaps[1].ReactorBuilding.ReactorCoolantPumps.AnyRunning(),
			"boundary snapshot must reflect the command at its offset")
	})

	t.Run("non-divisible cadence emits interior points without duplication", func(t *testing.T) {
		p := core.MakePlant()
		// main() sets steps = n*truncated-ticks (3*1666=4998) for the
		// sampling path; the interior emit at tick 4998 would be the
		// final point, so only 3 cadence emits + t0 + final.
		steps := 4998
		_, cadenceMs := sampleTicks(5, 2, core.DefaultTimeStep)
		var at []int64
		err := runLoop(&p, nil, p.GetControlMap(), nil, steps, core.DefaultTimeStep, cadenceMs,
			func(elapsedMs int64, snap core.Plant) error {
				at = append(at, elapsedMs)
				return nil
			})
		require.NoError(t, err)
		assert.Equal(t, []int64{0, 1666, 3332, 4998}, at)
	})
}
