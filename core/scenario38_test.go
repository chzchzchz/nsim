package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 38 (steam generator and primary equipment preventive
// maintenance) durable assertions, per plans/x/scenario/38.txt. Spec
// anchors:
//
//   - "When the reactor coolant shutdown temperature is reached,
//     operating equipment of the RHR and component cooling systems are
//     reduced to the minimum required to maintain temperature."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:78-81
//   - "When the reactor coolant reaches 200°F (93°C), the steam sides of
//     the steam generators are filled to the vents for wet layup."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:53-55
//   - "If the pressurizer is filled solid, rapid rises in primary system
//     pressure can occur due to small inputs of water mass or heat.
//     Solid plant primary pressure control utilizes a pressure control
//     valve between the RHR and CVC systems which is set to maintain a
//     selected pressure ..."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:25-32

// state shared across the package tests.

func outageScenarioPlant() Plant {
	p := maintenanceOutageLineup()
	return p
}

// Securing the redundant train and re-trimming the running train's
// bypass throttle leaves one RHR heat exchanger balancing the decay
// pool: the shutdown temperature holds instead of drifting away, and
// the secured pump reads stopped.
// Justification tier: spec-stated reduction requirement; the +-2 F band
// is an invented convention (A-2, plans/x/scenario/38.txt) asserted as a
// hold-band, not an exact value.
func TestScenario38_ReducedRHRHoldsShutdownTemperature(t *testing.T) {
	testDt := time.Second
	p := outageScenarioPlant()
	cm := p.GetControlMap()

	require.NoError(t, cm["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/PumpRunning"].Set("0"))
	require.NoError(t, cm["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/BypassValve/Open"].Set("0.73"))

	for range 600 {
		stepN(&p, 1, testDt)
	}

	assert.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Pump.Running,
		"the reduced train must read secured")
	assert.True(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.Running,
		"the remaining train must still be in service")
	tavg := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	assert.InDelta(t, scenarioColdLoopF, tavg, 2.0,
		"a single trimmed train alone must hold the shutdown temperature")
}

// The wet-layup fill path tops the shell up to the vents while the
// header sits under its fill cap, and never lets the header rise.
// Justification tier: fill-to-the-vents is spec-stated
// (page_228.txt:53-55); the fill rate is an invented constant asserted
// only through completion within the window, while the header cap is
// physics-forced (a laid-up shell must not pressurise from filling).
func TestScenario38_WetLayupFillsShellToTheVents(t *testing.T) {
	testDt := time.Second
	p := outageScenarioPlant()
	sg := &p.ReactorBuilding.SteamGenerator
	startWater := sg.Water.Float()
	require.LessOrEqual(t, float64(sg.Pressure), SGWetLayupMaxHeaderPsia,
		"the quenched outage header must sit under the fill cap")

	cm := p.GetControlMap()
	require.NoError(t, cm["ReactorBuilding/SteamGenerator/WetLayupFillValve/Open"].Set("1"))

	filled := false
	for range 1200 {
		stepN(&p, 1, testDt)
		assert.LessOrEqual(t, float64(sg.Pressure), SGWetLayupMaxHeaderPsia,
			"filling must not pressurise the laid-up header")
		if !filled && sg.Water.Float() >= sg.Volume.Float()*scenarioLayupFilledFrac {
			filled = true
		}
	}
	require.True(t, filled, "the shell must top off within the window")
	assert.Greater(t, sg.Water.Float(), startWater,
		"the fill path must have added shell inventory")
}

// A brief heater pulse against the water-solid primary shows the spec'd
// rapid pressure rise without challenging any relief setpoint, and the
// solid-plant pressure-control valve bleeds the overpressure back onto
// the selected pressure before standing down at it.
// Justification tier: rapid-rise warning and bleed-to-selected-pressure
// are spec-stated (page_228.txt:25-32); the peak value recomputed from
// PressurizerHeaterRate is a wiring check against production constants,
// marked as such.
func TestScenario38_SolidPlantReliefTrimsHeaterTestOverpressure(t *testing.T) {
	testDt := time.Second
	p := outageScenarioPlant()
	cm := p.GetControlMap()

	require.NoError(t, cm["ReactorBuilding/Pressurizer/HeatersOn"].Set("1"))
	stepN(&p, 2, testDt)
	pulsePeak := p.ReactorBuilding.Pressurizer.Pressure
	assert.InDelta(t, float64(scenarioColdPsia)+200*2, float64(pulsePeak), 5,
		"two seconds of heaters must show the rapid solid-plant rise")
	assert.Less(t, float64(pulsePeak), float64(PressurizerPORVTrip),
		"the test pulse must stay well below the PORV setpoint")
	require.NoError(t, cm["ReactorBuilding/Pressurizer/HeatersOn"].Set("0"))

	require.NoError(t, cm["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureSetpoint"].
		Set("415"))
	require.NoError(t, cm["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureValve/Open"].
		Set("1"))
	require.NoError(t, cm["ReactorBuilding/Pressurizer/HeaterAutoControl"].Set("1"))
	require.NoError(t, cm["ReactorBuilding/Pressurizer/PressureSetpoint"].Set("415"))

	settled := false
	bled := false
	for range 60 {
		stepN(&p, 1, testDt)
		if p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantReliefFlow.Float() > 0 {
			bled = true
		}
		if !settled && bled &&
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantReliefFlow.Float() == 0 {
			settled = true
		}
	}
	require.True(t, bled, "the relief valve must bleed the test overpressure")
	require.True(t, settled, "the valve must stand down once settled")
	assert.InDelta(t, scenarioMaintenancePzrSetpoint,
		float64(p.ReactorBuilding.Pressurizer.Pressure),
		float64(PressurizerAutoHeaterBand),
		"the selected pressure must be restored within the auto band")

	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"no protection actuation may follow the bounded test")
}

// Below the at-power permissive the SG level channels neither vote nor
// latch their ESFAS outputs, so wet layup's deliberate fill to the
// vents is silent; above the permissive they behave as Table 7-1
// channels again. Regression proof: this failed on pre-fix code with a
// standing level vote plus both ESFAS latches set at zero power.
// Justification tier: spec-stated gating convention
// (chapter_16/page_228.txt:53-55; chapter_19/page_018.txt:3-7;
// chapter_7/page_140.txt:37,73-76), asserted hard on both sides.
func TestScenario38_SGLevelChannelsRespectAtPowerPermissive(t *testing.T) {
	r := ReactorProtectionSystem{}
	idle := r.Step(DefaultTimeStep, SteamGeneratorShellPressure, PressurizerHeaterTarget, 95,
		CoreReferenceInletF, 0, 0, 1, 0,
		false)
	assert.Zero(t, idle.TripSignalCount,
		"wet-layup level must not vote below the permissive")
	assert.False(t, idle.FeedwaterIsolationActuated,
		"layup fill must not isolate feedwater in cold shutdown")
	assert.False(t, idle.TurbineTripActuated,
		"layup fill must not trip a secured turbine")

	atPower := r.Step(DefaultTimeStep, SteamGeneratorShellPressure, PressurizerHeaterTarget, 95,
		CoreReferenceInletF, 0, 0.5, 1, 0,
		false)
	assert.Equal(t, 1, atPower.TripSignalCount,
		"above the permissive the high level votes")
	assert.True(t, atPower.FeedwaterIsolationActuated,
		"Table 7-1 feedwater isolation on high level")
	assert.True(t, atPower.TurbineTripActuated,
		"Table 7-1 turbine trip on high level")

	fresh := ReactorProtectionSystem{}
	lowLevelStart := fresh.Step(DefaultTimeStep,
		SteamGeneratorShellPressure, PressurizerHeaterTarget, 5,
		CoreReferenceInletF, 0, 0, 1, 0,
		false)
	assert.True(t, lowLevelStart.AuxiliaryFeedPumpsStartActuated,
		"AFW auto-start on low level stays live in every mode")
}
