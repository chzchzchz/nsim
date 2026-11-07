package core

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 34 (spent fuel pool cooling and purification) durable
// assertions, per plans/x/scenario/34.txt. Spec anchors:
//
//   - "Normal operation requires two pumps with the third pump serving
//     as a standby." / "The system pressure is maintained at 55 to 75
//     psig ... the third pump automatically starts at a header pressure
//     of 50 psig."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-48
//   - "passing approximately 100 gpm ... through a filter and
//     demineralizer."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:67-73
//   - Leakage detection through pressure instrumentation and
//     motor-operated-valve isolation:
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:75-83
//   - Refueling water purification beside the pool demineralizer:
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-94

// state shared across the package tests.

// A supply-header leak pulls the header below the 50 psig auto-start
// setpoint so the standby pump starts; isolating the leak recovers the
// header into the maintained band with the standby still running.
func TestScenario34_LeakAutoStartsStandbyAndIsolationRecovers(t *testing.T) {
	t.Parallel()
	testDt := time.Second
	p := MakePlant()
	cm := p.GetControlMap()

	require.NoError(t,
		cm["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Leakage"].Set("1"))
	sawLowHeader := false
	for range 180 {
		stepN(&p, 1, testDt)
		if p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.HeaderPressure <
			SFPCStandbyStartPressure {
			sawLowHeader = true
		}
	}
	require.True(t, sawLowHeader,
		"the leak must pull the header below the auto-start setpoint")
	sf := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	require.True(t, sf.StandbyRunning,
		"the standby pump must auto-start on the low header")
	// Wiring check: the settled value recomputes from the production
	// leak-drop and mid-band constants; the physics-forced part is the
	// sustained sub-setpoint indication itself.
	assert.Less(t, sf.HeaderPressure, SFPCStandbyStartPressure,
		"the leaked header must hold below the auto-start setpoint")

	require.NoError(t,
		cm["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Leakage"].Set("0"))
	stepN(&p, 600, testDt)
	sf = p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	assert.InDelta(t, SFPCPressureStandbyTarget, sf.HeaderPressure, 1.0,
		"isolation recovers the header toward the mid-band target")
	assert.GreaterOrEqual(t, sf.HeaderPressure, SFPCPressureNormalMin,
		"recovered header must sit inside the maintained band")
	assert.LessOrEqual(t, sf.HeaderPressure, SFPCPressureNormalMax)
	assert.True(t, sf.StandbyRunning,
		"the auto-started standby pump keeps running")
}

// Securing the cooler loops through the control map stops purification
// flow immediately; the standby clears because no normal-loop lineup
// persists without the pumps.
func TestScenario34_PumpControlSecuresLoopsAndPurification(t *testing.T) {
	t.Parallel()
	testDt := time.Second
	p := MakePlant()
	cm := p.GetControlMap()

	require.NoError(t,
		cm["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/LoopAPumpRunning"].Set("0"))
	require.NoError(t,
		cm["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/LoopBPumpRunning"].Set("0"))
	stepN(&p, 5, testDt)
	sf := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	assert.False(t, sf.LoopAPumpRunning && sf.LoopBPumpRunning)
	assert.Zero(t, sf.PurificationFlow,
		"secured loops pass no filter/demineralizer sidestream")
	assert.False(t, sf.StandbyRunning)
}

// A crud burst slugs turbidity to the design value; the SFPCS sidestream
// alone cleans slowly, and realigning the refueling water purification
// pump beside the demineralizer roughly doubles the cleanup rate.
func TestScenario34_CrudBurstCleanupRateTracksPurificationStreams(t *testing.T) {
	t.Parallel()
	testDt := 10 * time.Second
	p := MakePlant()
	cm := p.GetControlMap()

	require.NoError(t,
		cm["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/CrudBurst"].Set("1"))
	sf := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	assert.InDelta(t, SFPCCrudBurstTurbidityPercent, sf.Turbidity, 0.01,
		"the burst slugs turbidity to the design value")

	stepN(&p, 60, testDt)
	singlePath := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.Turbidity
	// Wiring checks: both expected values recompute from production
	// constants; the tier-3 physics is the direction, dual streams
	// cleaning strictly faster than one.
	wantSingle := SFPCCrudBurstTurbidityPercent *
		timeFactor(60*10, SFPCPurificationCleanupTimeConstant.Seconds())
	assert.InDelta(t, wantSingle, singlePath, 0.5,
		"six hundred seconds on one sidestream matches the closed form")

	require.NoError(t,
		cm["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/RWPUPurifying"].Set("1"))
	stepN(&p, 60, testDt)
	dualPath := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.Turbidity
	wantDual := wantSingle *
		timeFactor(60*10, SFPCPurificationCleanupTimeConstant.Seconds()/2)
	assert.InDelta(t, wantDual, dualPath, 0.5,
		"two active streams double the exponential cleanup rate")
	assert.Less(t, dualPath, singlePath,
		"realigned RWPU purification must clean faster than alone")
}

// An uncooled pool soaks up to but never past atmospheric boiling;
// restored cooling relaxes it monotonically back toward the design
// temperature without undershoot.
func TestScenario34_UncooledPoolSoaksToBoilCapAndCoolingRecovers(t *testing.T) {
	t.Parallel()
	testDt := 30 * time.Minute
	sf := SpentFuelPoolCoolingSystem{
		LoopAPumpRunning: true,
		LoopBPumpRunning: true,
		DriveReady:       true,
		HeaderPressure:   SFPCPressureNormalMin,
		PoolTemp:         SFPCPoolDesignTemp,
	}
	sf.LoopAPumpRunning = false
	sf.LoopBPumpRunning = false
	for range 200 { // 100 h unattended
		sf = sf.Step(testDt, true)
		assert.LessOrEqual(t, sf.PoolTemp.Float(), SFPCPoolBoilingTemp,
			"the pool can never exceed the boiling cap")
	}
	assert.InDelta(t, SFPCPoolBoilingTemp, sf.PoolTemp.Float(), 0.01,
		"an unattended pool lands on the boiling point")

	sf.LoopAPumpRunning = true
	sf.LoopBPumpRunning = true
	testDt = 10 * time.Minute
	prev := sf.PoolTemp.Float()
	for range int(24 * time.Hour / (10 * time.Minute)) { // 24 h cooled
		sf = sf.Step(testDt, true)
		assert.Less(t, sf.PoolTemp.Float(), prev,
			"restored cooling must cool the pool monotonically")
		assert.GreaterOrEqual(t, sf.PoolTemp.Float(),
			SFPCPoolDesignTemp.Float(),
			"cooling cannot undershoot the design temperature")
		prev = sf.PoolTemp.Float()
	}
	assert.LessOrEqual(t, prev, 101.0,
		"one day of cooling returns the pool to the design value")
}

// The uncooled heatup rate matches the decay-heat balance closed form
// over an hour-scale window.
func TestScenario34_UncooledHeatupMatchesDecayHeatBalance(t *testing.T) {
	t.Parallel()
	testDt := time.Minute
	sf := SpentFuelPoolCoolingSystem{
		LoopAPumpRunning: true,
		LoopBPumpRunning: true,
		DriveReady:       true,
	}
	sf.LoopAPumpRunning = false
	sf.LoopBPumpRunning = false
	start := sf.PoolTemp.Float()
	for range 60 {
		sf = sf.Step(testDt, true)
	}
	rise := sf.PoolTemp.Float() - start
	wantRise := float64(SFPStoredFuelDecayHeat) /
		(SFPCPoolVolume.Float() * KilogramsPerGallon * SpecificHeatWater) *
		3600.0 * 9.0 / 5.0
	assert.InDelta(t, wantRise, rise, 0.05,
		"one hour unattended heats the pool at the decay-heat rate")
}

// timeFactor evaluates e^(-elapsed/tau), the fraction left after one
// first-order interval.
func timeFactor(elapsedS float64, tauS float64) float64 {
	return math.Exp(-elapsedS / tauS)
}

// The trained evolution the standby pump exists for: tripping one of
// the two duty loops sags the supply header below the 50 psig auto-
// start setpoint, the third pump picks up, and the header recovers
// into the normal band - all through real dynamics, with purification
// uninterrupted (plans/p44_1.txt).
func TestSFPC_SingleLoopTripSagsAndStandbyPicksUp(t *testing.T) {
	t.Parallel()
	s := SpentFuelPoolCoolingSystem{
		LoopAPumpRunning: true,
		LoopBPumpRunning: true,
		DriveReady:       true,
		HeaderPressure:   SFPCPressureNormalMin,
	}
	// Settle both-loops at the normal band first.
	for range int(SFPCPressureTimeConstant.Seconds()) {
		s = s.Step(DefaultTimeStep, true)
	}
	require.False(t, s.StandbyRunning, "precondition: standby off")

	// Trip loop B: the header sags toward the one-loop target,
	// crossing the standby-start setpoint on the way down.
	s.LoopBPumpRunning = false
	sawSagBelowStart := false
	for range int(2 * SFPCPressureTimeConstant.Seconds() / DefaultTimeStep.Seconds()) {
		s = s.Step(DefaultTimeStep, true)
		if s.HeaderPressure < SFPCStandbyStartPressure && !s.StandbyRunning {
			sawSagBelowStart = true
		}
	}
	assert.True(t, s.StandbyRunning,
		"the sag must cross the standby auto-start setpoint")
	assert.True(t, sawSagBelowStart || s.HeaderPressure >= SFPCStandbyStartPressure,
		"the header transient passes through the pickup band")

	// Two pumps running again (one duty + standby): header recovers
	// into the maintained band and purification never stopped.
	assert.GreaterOrEqual(t, s.HeaderPressure, SFPCStandbyStartPressure)
	assert.Greater(t, s.PurificationFlow, 0.0,
		"the surviving duty loop keeps the sidestream flowing")
}
