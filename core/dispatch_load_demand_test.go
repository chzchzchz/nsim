package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	p := MakePlant()
	p.PlantOperation.LoadFollowActive = true
	p.ReactorBuilding.Grid.LoadDemand = 600

	stepN(&p, 1)

	assert.InDelta(t, 0.6,
		p.TurbineBuilding.Turbine.LoadDemandRef, 1e-9,
		"600 MW converts to the 0.6 setpoint scale")
}

// Without the permissive the grid cannot move the turbine load
// reference.
func TestDispatchDemandBlockedWithoutPermissive(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Turbine.LoadDemandRef = 0.25
	p.ReactorBuilding.Grid.LoadDemand = 900

	stepN(&p, 1)

	assert.InDelta(t, 0.25,
		p.TurbineBuilding.Turbine.LoadDemandRef, 1e-9,
		"no permissive, no dispatch path")
}

// Leaving power-operation mode clears the dispatch permissive: the
// PlantOperation layer owns it and force-clears outside PowerOperation
// (ColdShutdown / HotStandby branches).
func TestDispatchPermissiveClearsOutsidePowerOperation(t *testing.T) {
	p := MakePlant()
	p.PlantOperation.Mode = HotStandby
	p.PlantOperation.LoadFollowActive = true

	stepN(&p, 1)

	assert.False(t, p.PlantOperation.LoadFollowActive,
		"permissive must not survive outside power operation")
}
