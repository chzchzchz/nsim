package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The exercise inserts a "100 % main steam header break" upstream of
// the MSV inside containment and observes the protective response:
//   "This malfunction event causes steam pipe break in the main steam
//    line before the main steam stop valve (MSV) inside containment,
//    leading to rapid depressurizaton of the main steam pressure.
//    Turbine generator will be runback rapidly and will be tripped by
//    zero forward power. The turbine trip initiates a reactor power
//    stepback."
//   specs/pwr_sim/chapter_4/page_053.txt:3-8
//
// Excessive cooldown protection actuates safety injection for
// secondary breaks through the low compensated steamline pressure
// channel:
//   "Excessive cooldown protection signals (1) Low compensated steam
//    line pressure as reported by two out of four signals in any
//    steam line. This protects against secondary breaks during power
//    generation."
//   specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:65-69

func TestSteamGenerator_BreakDischargeScalesWithHeader(t *testing.T) {
	sg := MakePlant().ReactorBuilding.SteamGenerator
	sg.HeatInput = SteamGeneratorFullLoadHeatInput

	require.Zero(t, sg.MainSteamLineBreak.Open,
		"precondition: intact header")
	closed := sg.Step(DefaultTimeStep, 0)
	assert.Zero(t, closed.BreakFlow.Float(),
		"an intact header must not discharge")

	sg.MainSteamLineBreak.Open = 1
	full := sg.Step(DefaultTimeStep, 0)
	require.Greater(t, full.BreakFlow.Float(), 0.0,
		"an open break must discharge")
	wantFull := SteamLineBreakCapacityFraction *
		SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
		DefaultTimeStep.Seconds()
	// Wiring check: the expectation is recomputed from the same
	// constants production uses; it pins the plumbing, not physics.
	assert.InDelta(t, wantFull, full.BreakFlow.Float(), wantFull*1e-9,
		"full-open discharge at nominal pressure must match the capacity fraction")
	assert.Less(t, full.Water.Float(), sg.Volume.Float()*0.8,
		"the discharge must debit shell inventory")

	half := sg
	half.Pressure = SteamGeneratorShellPressure / 2
	dropped := half.Step(DefaultTimeStep, 0)
	assert.InDelta(t, dropped.BreakFlow.Float(), full.BreakFlow.Float(),
		full.BreakFlow.Float()*1e-9,
		"choked blowdown must hold the rated fraction while any header pressure remains")
}

func TestSteamLineBreak_ProtectionResponse(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/DefaultTimeStep), DefaultTimeStep)
	require.False(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated,
		"precondition: SI clear before the break")

	// Insert the malfunction through the control map so the
	// registration grammar is exercised with the dynamics.
	sm := p.GetControlMap()
	key := "ReactorBuilding/SteamGenerator/MainSteamLineBreak/Open"
	ctrl, ok := sm[key]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))
	val := sm[key].Get()
	assert.Contains(t, val, "1", "control readback must show the inserted break")

	contTempPre := p.ReactorBuilding.ContainmentBuilding.Temperature.Float()
	waterPre := p.ReactorBuilding.SteamGenerator.Water.Float()

	var siLagSteps, tripAfterSI = -1, -1
	steps := int(30 * time.Second / DefaultTimeStep)
	for i := 0; i < steps; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		rps := p.ReactorBuilding.ReactorProtectionSystem
		if siLagSteps < 0 && rps.SafetyInjectionActuated {
			siLagSteps = i
			if p.ReactorBuilding.SteamGenerator.Pressure >=
				RPSLowSteamlinePressureTrip {
				t.Fatalf("SI latched while the header still read %.1f psia",
					float64(p.ReactorBuilding.SteamGenerator.Pressure))
			}
			continue
		}
		if siLagSteps >= 0 && rps.SafetyInjectionActuated && rps.ReactorTripped {
			tripAfterSI = i - siLagSteps
			break
		}
	}
	require.GreaterOrEqual(t, siLagSteps, 0,
		"low compensated steamline pressure must latch safety injection")
	require.GreaterOrEqual(t, tripAfterSI, 0, "SI must vote the reactor trip")
	assert.LessOrEqual(t, tripAfterSI, 5,
		"reactor trip must follow the SI latch within a few ticks")

	// The protective cascades read last tick's latches; give them
	// a few steps to land before asserting the end state.
	stepN(&p, int(3*time.Second/DefaultTimeStep), DefaultTimeStep)

	assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationActuated,
		"steamline isolation shares the low-steamline trigger")

	assert.Less(t, float64(p.ReactorBuilding.SteamGenerator.Pressure),
		float64(RPSLowSteamlinePressureTrip),
		"header must sit below the protection setpoint after the transient")
	assert.Less(t, p.ReactorBuilding.SteamGenerator.Water.Float(), waterPre,
		"the break must have spent secondary inventory")
	assert.Greater(t, p.ReactorBuilding.ContainmentBuilding.Temperature.Float(), contTempPre,
		"containment atmosphere must absorb break enthalpy")

	assert.NotEqual(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"turbine must be off its running state after the collapse (trip or coast)")
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed &&
		p.TurbineBuilding.Turbine.RealPower > 0.1*TurbineRatedkW*1000,
		"a synchronized machine must not keep exporting rated power into the break")
}

// The sink-draw imbalance splits into its two regimes: the paced share
// (turbine offtake + atmospheric dump) stays under the operational rate
// envelope inside ReactorPressureVessel.Step, while the break discharge
// is the fault regime that bypasses it. With no break open the fault
// share must be zero so every planned-evolution trajectory is
// unchanged (plans/p34_1.txt).
func TestSecondaryExcessDraw_SplitsFaultFromPacedPath(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	rb := &p.ReactorBuilding
	op := PlantOperation{Mode: HotStandby}
	sg := &rb.SteamGenerator
	sg.HeatInput = 0

	reset := func() {
		rb.SecondaryImbalanceFiltered = 0
		sg.AtmosphericDumpValve.Open = 0
		sg.MainSteamLineBreak.Open = 0
	}

	reset()
	paced, fault := rb.secondaryExcessDrawW(0, false, op, DefaultTimeStep)
	assert.Zero(t, paced, "an intact header draws nothing")
	assert.Zero(t, fault)

	reset()
	sg.AtmosphericDumpValve.Open = 0.5
	paced, fault = rb.secondaryExcessDrawW(0, false, op, DefaultTimeStep)
	assert.Greater(t, float64(paced), 0.0,
		"the atmospheric dump is operator-paced draw")
	assert.Zero(t, fault, "a commanded dump is not a fault")

	reset()
	sg.MainSteamLineBreak.Open = 1
	paced, fault = rb.secondaryExcessDrawW(0, false, op, DefaultTimeStep)
	assert.Zero(t, paced)
	assert.Greater(t, float64(fault), 0.0,
		"the break discharge carries the whole excess as fault draw")

	reset()
	sg.AtmosphericDumpValve.Open = 0.5
	sg.MainSteamLineBreak.Open = 1
	paced, fault = rb.secondaryExcessDrawW(0, false, op, DefaultTimeStep)
	assert.Greater(t, float64(paced), 0.0)
	assert.Greater(t, float64(fault), 0.0,
		"both regimes draw at once when both paths are open")
}

// Full-plant acceptance: after the break trips the plant, the primary
// cools measurably past anything the 50 F/hr envelope permits in the
// same window (the old clamp rode exactly the program pace).
//
// "Therefore, coolant temperature and pressure will drop."
// specs/pwr_sim/chapter_4/page_053.txt:11
func TestPlant_SteamLineBreakOvercoolsPastProgramEnvelope(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/DefaultTimeStep), DefaultTimeStep)

	ctrl, ok := p.GetControlMap()["ReactorBuilding/SteamGenerator/MainSteamLineBreak/Open"]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))

	startF := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	trippedAt := -1
	maxCoolFps := 0.0
	// 30 s captures the full fault signature: the trip lands within
	// seconds of the break and both thresholds are pace-relative, so
	// halving the window tightens rather than loosens them.
	const window = 30 * time.Second
	for i := 0; i < int(window/DefaultTimeStep); i++ {
		pre := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
		p.Step(DefaultTimeStep)
		p.Update()
		post := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
		if trippedAt < 0 {
			if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
				trippedAt = i
			}
			continue
		}
		maxCoolFps = max(maxCoolFps, (pre-post)/DefaultTimeStep.Seconds())
	}
	require.GreaterOrEqual(t, trippedAt, 0, "the break must trip the reactor")
	endF := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()

	drop := startF - endF
	programDrop := OpHeatupRateLimit * window.Seconds()
	t.Logf("measured: drop %.2f F in %v (program allows %.2f F), max cooling %.3f F/s (envelope %.4f F/s)",
		drop, window, programDrop, maxCoolFps, OpHeatupRateLimit)
	assert.Greater(t, drop, programDrop*5,
		"the fault overcool must outrun the planned-cooldown pace (pre-fix code rides at 1x)")
	assert.Greater(t, maxCoolFps, OpHeatupRateLimit*5,
		"instantaneous cooling must exceed what pacing physics permits")
}
