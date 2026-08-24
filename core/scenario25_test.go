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
	closed := sg.Step(0)
	assert.Zero(t, closed.BreakFlow.Float(),
		"an intact header must not discharge")

	sg.MainSteamLineBreak.Open = 1
	full := sg.Step(0)
	require.Greater(t, full.BreakFlow.Float(), 0.0,
		"an open break must discharge")
	wantFull := SteamLineBreakCapacityFraction *
		SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
		TimeStep.Seconds()
	// Wiring check: the expectation is recomputed from the same
	// constants production uses; it pins the plumbing, not physics.
	assert.InDelta(t, wantFull, full.BreakFlow.Float(), wantFull*1e-9,
		"full-open discharge at nominal pressure must match the capacity fraction")
	assert.Less(t, full.Water.Float(), sg.Volume.Float()*0.8,
		"the discharge must debit shell inventory")

	half := sg
	half.Pressure = SteamGeneratorShellPressure / 2
	dropped := half.Step(0)
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
	stepN(&p, int(2*time.Second/TimeStep))
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
	steps := int(30 * time.Second / TimeStep)
	for i := 0; i < steps; i++ {
		p.Step()
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
	stepN(&p, int(3*time.Second/TimeStep))

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
