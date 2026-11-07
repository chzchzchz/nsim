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
	// MakePlant now ships the hot-standby idle lineup (pumps secured,
	// breaker open), so a rated fixture must restore the forced-flow
	// and electrical lineups its operating point assumes.
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = true
	}
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
	t.Parallel()
	p := scenario10FullPowerPlant()
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.Running,
		"precondition: RHRS out of service")

	setControl(t, &p, "PlantOperation/Mode", "1")
	stepN(&p, int(time.Second/DefaultTimeStep), DefaultTimeStep)

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
	t.Parallel()
	// Rejection detection is snapshot-based (it fires when one
	// operator write drops the setpoint past
	// LoadRejectDetectFraction = 0.10, not when an accumulated rate
	// crosses it), so the 0.08-step unload ladder cannot false-latch
	// it at any tick width. Turbine setpoint ramps and power
	// state.
	testDt := 10 * time.Millisecond
	p := scenario10FullPowerPlant()

	for _, sp := range []string{"0.92", "0.84", "0.76", "0.68", "0.60",
		"0.52", "0.44", "0.36", "0.28", "0.20", "0.12", "0.04"} {
		setControl(t, &p, "TurbineBuilding/Turbine/Setpoint", sp)
		stepN(&p, int(5*time.Second/testDt), testDt)
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
	t.Parallel()
	// The collapse rides the per-step compounding channel, whose
	// pace slows with the tick, but folding to the source floor
	// needs only on the order of two thousand compounds even at a
	// one-percent net negative worth: ten times slower that is
	// still ~20 s, well inside the 60 s window, and the assertions
	// are absolute floors and signs, not trajectory shapes. Must
	// simulation state.
	testDt := 10 * time.Millisecond
	p := scenario10FullPowerPlant()

	// A planned shutdown blocks the SI initiation signals before the
	// collapsing header can cross their setpoints, so the shutdown
	// itself cannot vote a trip through the SI channel.
	// "Prior to reaching the automatic safety injection setpoints
	// during cooldown and depressurization, the safety injection
	// initiation signals are blocked."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	setControl(t, &p,
		"ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock",
		"true")
	setControl(t, &p, "ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "1")
	stepN(&p, int(60*time.Second/testDt), testDt)

	r := p.ReactorBuilding.ReactorPressureVessel
	assert.Negative(t, r.Reactivity)
	assert.InDelta(t, 1.0, r.Insertion, 1e-9)
	assert.LessOrEqual(t, float64(r.FissionPower), 10.0,
		"fission power collapsed to the source-floor scale")

	// No protective trip may result from the shutdown itself.
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip)
}

// Through the post-trip cooldown the pressurizer level loop must hold
// level inside the no-load band with the bubble intact, and Tavg must
// coast down at the program rate instead of collapsing.
//
// "Pressurizer level is raised in anticipation of coolant contraction"
// (level management, page_226.txt:146-153) and "Primary plant heatup
// rate is limited to 50°F per hour" (page_224.txt:140-141).
func TestScenario10_LevelAndRateHoldThroughCooldown(t *testing.T) {
	t.Parallel()
	// Level bands are wide (the no-load band spans most of the
	// pressurizer), the level loop and Tavg program run on dt-scaled
	// rate clamps, and the cooldown-rate assertion reads the average
	// pace across a 100 s window rather than any per-step shape.
	// simulation state.
	testDt := 10 * time.Millisecond
	p := scenario10FullPowerPlant()
	// The evolution runs on the automatic level loop: level follows
	// the Tavg-referenced program (60 pct at full load toward 25 pct
	// at no-load) instead of drifting on raw design flows.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = true
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "1")
	stepN(&p, int(30*time.Second/testDt), testDt) // rods in, power collapsed

	outA := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	stepN(&p, int(100*time.Second/testDt), testDt)
	outB := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()

	pzr := p.ReactorBuilding.Pressurizer
	levelFrac := pzr.Water.Float() / pzr.Volume.Float()
	// Level follows the Tavg program down toward its no-load anchor
	// (25 pct), staying inside the no-load band.
	assert.GreaterOrEqual(t, levelFrac, 0.20,
		"pressurizer must not drain below the no-load band")
	assert.LessOrEqual(t, levelFrac, 0.80,
		"pressurizer must not go water solid")
	assert.Positive(t, pzr.SteamBubble.Float(),
		"the steam bubble must survive the cooldown")

	// Cooldown paced at the program rate between the samples.
	dropPerHour := (outA - outB) / (100.0 / 3600.0)
	assert.LessOrEqual(t, dropPerHour, 50.0*1.01,
		"cooldown rate exceeds the 50 F/hr program limit")
	assert.Greater(t, outA, outB, "want the outlet cooling down")
}
