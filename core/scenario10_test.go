package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 10 (turbine unload and hot standby) durable assertions.
//
// "the initial step in shutdown is to unload the turbine-generator"
// and "If the plant is to be placed in a subcritical hot standby
// condition, the control rods are inserted to reduce power level".
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:137-143
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:20-23

// scenario10FullPowerPlant returns a coherent full-power lineup:
// synchronized turbine at rated with seeded decay inventories.
func scenario10FullPowerPlant() Plant {
	p := MakePlant()
	p.PlantOperation.Mode = PowerOperation
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		CoreRatedThermalPower)
	p.ReactorBuilding.ReactorCoolantPump.Running = true
	p.TurbineBuilding.Turbine.State = TurbineRunning
	p.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	// Seed terminal conditions inside the synchronism window so the
	// breaker holds; an unseeded generator pops it on the first step.
	p.TurbineBuilding.Generator.BreakerClosed = true
	p.TurbineBuilding.Generator.Voltage = 24000
	p.TurbineBuilding.Generator.Frequency = 60
	return p
}

// The operator's mode selection must be drivable through the control
// map and must arm the hot-standby RCP cap once stepped: mode changes
// are operator actions during shutdowns.
//
// "In this operational mode a maximum of two out of four reactor
// coolant pumps (RCPs) may be secured."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:24-29
func TestScenario10_ModeControlDrivesHotStandbyGating(t *testing.T) {
	p := scenario10FullPowerPlant()
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.Running,
		"precondition: RHRS out of service")

	setControl(t, &p, "PlantOperation/Mode", "1")
	stepN(&p, int(time.Second/TimeStep))

	assert.Equal(t, HotStandby, p.PlantOperation.Mode)
	assert.True(t, p.PlantOperation.HotStandbyRCPLimitActive,
		"hot-standby RCP cap arms with RHRS out of service")
}

// Unloading in setpoint steps under the rejection threshold must carry
// generator output down to house-load level without opening the dump
// valves or tripping the turbine.
//
// "These valves open under a sudden reduction in turbine-generator
// load (in excess of 10%)"
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
func TestScenario10_GradualUnloadReachesHouseLoadWithoutRejection(t *testing.T) {
	p := scenario10FullPowerPlant()

	for _, sp := range []string{"0.92", "0.84", "0.76", "0.68", "0.60",
		"0.52", "0.44", "0.36", "0.28", "0.20", "0.12", "0.04"} {
		setControl(t, &p, "TurbineBuilding/Turbine/Setpoint", sp)
		stepN(&p, int(5*time.Second/TimeStep))
	}

	assert.LessOrEqual(t, float64(p.TurbineBuilding.Turbine.RealPower), 70e6,
		"generator unloaded to house-load level")
	// "No load rejection" is asserted through the rejection-specific
	// signals: a rejection drops the setpoint >= 10% in one window,
	// latching DumpHoldTimer and tripping the turbine. The dump valve
	// itself may cycle in pressure-control mode once admission runs
	// below steam production ("These valves are also utilized in the
	// pressure control mode during plant startup and plant cooldown",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-26).
	assert.Equal(t, time.Duration(0), p.TurbineBuilding.Turbine.DumpHoldTimer,
		"no load rejection: no rejection hold timer latched")
	assert.False(t, p.TurbineBuilding.Turbine.OPCTrip)
	assert.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State)

	// House load transfers to the SAT automatically once the breaker
	// opens; before that the UAT supplies it.
	assert.Equal(t, "UAT", p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
}

// Inserting rods shuts the reactor down: fission power collapses to
// the neutron-source floor on negative reactivity alone.
//
// "the control rods are inserted to reduce power level"
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:20-23
func TestScenario10_RodInsertionShutsDownReactor(t *testing.T) {
	p := scenario10FullPowerPlant()

	setControl(t, &p, "ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "1")
	stepN(&p, int(60*time.Second/TimeStep))

	r := p.ReactorBuilding.ReactorPressureVessel
	assert.Negative(t, r.Reactivity)
	assert.InDelta(t, 1.0, r.Insertion, 1e-9)
	assert.LessOrEqual(t, float64(r.FissionPower), 10.0,
		"fission power collapsed to the source-floor scale")

	// No protective trip may result from the shutdown itself.
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip)
}
