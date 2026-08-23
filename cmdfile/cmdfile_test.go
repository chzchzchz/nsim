package cmdfile

import (
	"fmt"
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

func TestParse(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cmds, err := Parse(strings.NewReader(
			"0 ReactorBuilding.ReactorCoolantPump.Running 1\n"+
				"500 AuxiliaryBuilding.DieselGenerator.Start Manual\n"+
				"\n"+
				"# comment\n"+
				"1000\tReactorBuilding/ReactorPressureVessel/ControlRods/Insertion\t0.5\n",
		))
		require.NoError(t, err)
		require.Len(t, cmds, 3)
		assert.Equal(t, int64(0), cmds[0].TimeOffsetMs)
		assert.Equal(t, "ReactorBuilding.ReactorCoolantPump.Running", cmds[0].ControlKey)
		assert.Equal(t, "1", cmds[0].Value)
		assert.Equal(t, int64(500), cmds[1].TimeOffsetMs)
		assert.Equal(t, "AuxiliaryBuilding.DieselGenerator.Start", cmds[1].ControlKey)
		assert.Equal(t, "Manual", cmds[1].Value)
		assert.Equal(t, int64(1000), cmds[2].TimeOffsetMs)
		assert.Equal(t, "ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", cmds[2].ControlKey)
		assert.Equal(t, "0.5", cmds[2].Value)
	})

	t.Run("read error", func(t *testing.T) {
		errReader := &errorReader{err: fmt.Errorf("read failure")}
		_, err := Parse(errReader)
		require.Error(t, err)
	})

	t.Run("bad fields", func(t *testing.T) {
		_, err := Parse(strings.NewReader("only_one_field\n"))
		require.Error(t, err)
	})

	t.Run("bad offset", func(t *testing.T) {
		_, err := Parse(strings.NewReader("abc Part.Label 1\n"))
		require.Error(t, err)
	})
}

func TestResolveKey(t *testing.T) {
	m := bridge.ControlMap{
		"ReactorBuilding/ReactorCoolantPump/Running":     {},
		"ReactorBuilding/ReactorPressureVessel/Trip": {},
	}
	assert.Equal(t, "ReactorBuilding/ReactorCoolantPump/Running", ResolveKey(m, "ReactorBuilding/ReactorCoolantPump/Running"))
	assert.Equal(t, "ReactorBuilding/ReactorCoolantPump/Running", ResolveKey(m, "ReactorBuilding.ReactorCoolantPump.Running"))
	assert.Equal(t, "ReactorBuilding/ReactorPressureVessel/Trip", ResolveKey(m, "ReactorBuilding.ReactorPressureVessel.Trip"))
	assert.Equal(t, "unknown.key", ResolveKey(m, "unknown.key"))
}

func TestApply(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		c := m["ReactorBuilding/ReactorCoolantPump/Running"]
		require.NotNil(t, c)
		require.NoError(t, Apply(m, Command{ControlKey: "ReactorBuilding/ReactorCoolantPump/Running", Value: "1"}))
		assert.True(t, p.ReactorBuilding.ReactorCoolantPump.Running)
	})

	t.Run("dot notation", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		require.NoError(t, Apply(m, Command{ControlKey: "ReactorBuilding.ReactorCoolantPump.Running", Value: "1"}))
		assert.True(t, p.ReactorBuilding.ReactorCoolantPump.Running)
	})

	t.Run("unknown control", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		err := Apply(m, Command{ControlKey: "NoSuch/Control", Value: "1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown control")
	})

	t.Run("set error", func(t *testing.T) {
		p := core.MakePlant()
		m := p.GetControlMap()
		c := m["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"]
		require.NotNil(t, c)
		err := Apply(m, Command{ControlKey: "ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", Value: "bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "set control")
	})
}
