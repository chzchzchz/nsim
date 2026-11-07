package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dispatch demand reaches the turbine load reference only under
// the operator's permissive, converted once from MW at the boundary:
// 600 MW against a 1_000_000e3 W turbine rating is exactly 0.6 of
// rated.
//
// Spec: "The dispatch system decides how much power [...]"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:44-46,
// permissive: page_135.txt:10-12.
func TestDispatchDemandLatchesUnderPermissive(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.PlantOperation.LoadFollowActive = true
	p.ReactorBuilding.Grid.LoadDemand = 600

	stepN(&p, 1, DefaultTimeStep)

	assert.InDelta(t, 0.6,
		p.TurbineBuilding.Turbine.LoadDemandRef, 1e-9,
		"600 MW converts to the 0.6 setpoint scale")
}

// TestLoadDemand_UnitRoundTrip pins the G6 unit contract (wiring check,
// CHECK_SKILL §8): Grid.LoadDemand is stored in MW (bridge kind "mw"),
// not the Setpoint-scale fraction carried by Turbine.LoadDemandRef
// (CHECK_SKILL §10 look-alike twin), and Plant.Step converts MW to that
// fraction only at the plant.go boundary under LoadFollowActive. 600 MW
// on a 1000 MW machine is 0.6 of rated.
func TestLoadDemand_UnitRoundTrip(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cm := GetControlMap(&p)
	c, ok := cm["Electrical/Grid/LoadDemand"]
	require.True(t, ok, "LoadDemand control registered")

	require.NoError(t, c.Set("600"))
	assert.Equal(t, "600", c.Get(),
		"dispatcher MW round-trips through the bridge unchanged")
	assert.Equal(t, 600.0, p.ReactorBuilding.Grid.LoadDemand,
		"stored value is MW, not a fraction")
	assert.Equal(t, 0.0, p.TurbineBuilding.Turbine.LoadDemandRef,
		"LoadDemandRef is the fraction twin: bridge write must not reach it")

	p.PlantOperation.LoadFollowActive = true
	stepN(&p, 1, DefaultTimeStep)

	assert.InDelta(t, 0.6,
		p.TurbineBuilding.Turbine.LoadDemandRef, 1e-9,
		"one step converts 600 MW to the 0.6 setpoint scale")
}

// Without the permissive the grid cannot move the turbine load
// reference.
func TestDispatchDemandBlockedWithoutPermissive(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.Turbine.LoadDemandRef = 0.25
	p.ReactorBuilding.Grid.LoadDemand = 900

	stepN(&p, 1, DefaultTimeStep)

	assert.InDelta(t, 0.25,
		p.TurbineBuilding.Turbine.LoadDemandRef, 1e-9,
		"no permissive, no dispatch path")
}

// Leaving power-operation mode clears the dispatch permissive: the
// PlantOperation layer owns it and force-clears outside PowerOperation
// (ColdShutdown / HotStandby branches).
func TestDispatchPermissiveClearsOutsidePowerOperation(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.PlantOperation.Mode = HotStandby
	p.PlantOperation.LoadFollowActive = true

	stepN(&p, 1, DefaultTimeStep)

	assert.False(t, p.PlantOperation.LoadFollowActive,
		"permissive must not survive outside power operation")
}
