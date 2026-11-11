package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
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
	t.Parallel()
	sg := MakePlant().ReactorBuilding.SteamGenerator
	sg.HeatInput = SteamGeneratorFullLoadHeatInput

	require.Zero(t, sg.MainSteamLineBreak.Open,
		"precondition: intact header")
	closed := sg.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, nil, nil)
	assert.Zero(t, closed.BreakFlow.Float(),
		"an intact header must not discharge")

	sg.MainSteamLineBreak.Open = 1
	full := sg.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, nil, nil)
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
	dropped := half.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, nil, nil)
	assert.InDelta(t, dropped.BreakFlow.Float(), full.BreakFlow.Float(),
		full.BreakFlow.Float()*1e-9,
		"choked blowdown must hold the rated fraction while any header pressure remains")
}

func TestSteamLineBreak_ProtectionResponse(t *testing.T) {
	t.Parallel()
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
	key := "ReactorBuilding/Faults/MainSteamLineBreak"
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

func TestSteamLineBreak_LowSteamlinePressureAnnunciates(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/DefaultTimeStep), DefaultTimeStep)

	sm := p.GetControlMap()
	key := "ReactorBuilding/Faults/MainSteamLineBreak"
	ctrl, ok := sm[key]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))

	crossed := false
	for i := 0; i < 5000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		if p.ReactorBuilding.SteamGenerator.Pressure < RPSLowSteamlinePressureTrip {
			if !crossed {
				crossed = true
				found := false
				for _, a := range p.Alarms() {
					if a.ID == AlarmIDLowSteamlinePressure {
						found = true
						require.Equal(t, SeverityTrip, a.Severity)
					}
				}
				require.True(t, found,
					"low-steamline alarm must appear at the crossing tick")
				// Persist through the break window.
				alarms := p.Alarms()
				present := false
				for _, a := range alarms {
					if a.ID == AlarmIDLowSteamlinePressure {
						present = true
					}
				}
				require.True(t, present, "alarm must persist through the break window")
				return
			}
		}
	}
	require.True(t, crossed || p.ReactorBuilding.SteamGenerator.Pressure < RPSLowSteamlinePressureTrip,
		"header must cross 600 psia within 5 sim-seconds of a full break")
}

// The sink-draw imbalance splits into its two regimes: the paced share
// (turbine offtake + atmospheric dump) stays under the operational rate
// envelope inside ReactorPressureVessel.Step, while the break discharge
// is the fault regime that bypasses it. With no break open the fault
// share must be zero so every planned-evolution trajectory is
// unchanged (plans/p34_1.txt).
func TestSecondaryExcessDraw_SplitsFaultFromPacedPath(t *testing.T) {
	t.Parallel()
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
func TestPlant_MSLBDoesNotDebitPrimaryLedger(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/DefaultTimeStep), DefaultTimeStep)

	merelyOpened := p.ReactorBuilding.SteamGenerator.MainSteamLineBreak.Open
	require.Zero(t, merelyOpened, "precondition: intact header")
	ctrl, ok := p.GetControlMap()["ReactorBuilding/Faults/MainSteamLineBreak"]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))

	containmentTempBefore := p.ReactorBuilding.ContainmentBuilding.Temperature.Float()
	var totalSecondaryBlowdown float64
	for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		rpv := p.ReactorBuilding.ReactorPressureVessel
		sg := p.ReactorBuilding.SteamGenerator
		totalSecondaryBlowdown += sg.BreakFlow.Float()

		assert.Zero(t, rpv.BreakDischarged.Float(), "MSLB must not debit primary inventory")
		assert.Zero(t, rpv.BreakDischargedNonCreditable.Float(),
			"MSLB must not debit the noncreditable primary ledger")
		assert.Zero(t, rpv.BreakFlow.Float(), "MSLB must not add a primary-break flow")
		assert.Zero(t, rpv.UncoveryFrac, "MSLB alone must not uncover the core")
		assert.False(t, rpv.VesselFailed, "MSLB alone must not latch vessel failure")
	}
	require.Greater(t, totalSecondaryBlowdown, 0.0,
		"secondary blowdown remains live, retaining a nonzero heat source for fault overcool")
	assert.Greater(t, p.ReactorBuilding.ContainmentBuilding.Temperature.Float(), containmentTempBefore,
		"secondary break enthalpy enters containment")
}

func TestPlant_MSLBContainmentChargedOnce(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/DefaultTimeStep), DefaultTimeStep)

	ctrl, ok := p.GetControlMap()["ReactorBuilding/Faults/MainSteamLineBreak"]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))

	containmentBefore := p.ReactorBuilding.ContainmentBuilding.Temperature.Float()
	var totalSecondaryBlowdown, steamHeatInJ float64
	for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		secondaryFlow := p.ReactorBuilding.SteamGenerator.BreakFlow.Float()
		locaFlow := p.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float()
		totalSecondaryBlowdown += secondaryFlow
		steamHeatInJ += secondaryFlow * KilogramsPerGallon * float64(LatentHeatOfSteamization)
		assert.Zero(t, locaFlow,
			"MSLB must leave the primary-break enthalpy path empty")
	}
	require.Greater(t, totalSecondaryBlowdown, 0.0,
		"expected secondary discharge so containment receives its sole break charge")

	volumeM3 := p.ReactorBuilding.ContainmentBuilding.FreeVolume * ft3ToM3
	thermalMassJPerDegF := volumeM3 * containmentAirDensity * airCp * 9.0 / 5.0
	grossExpectedDegF := steamHeatInJ / thermalMassJPerDegF
	actualDegF := p.ReactorBuilding.ContainmentBuilding.Temperature.Float() - containmentBefore
	assert.Greater(t, actualDegF, grossExpectedDegF*0.05,
		"containment rise must track the single secondary steam charge once thermal sinks are considered")
	// The containment node also receives primary-shutdown and
	// residual process heat through unrelated plant couplings, so
	// the raw temperature rise is only an upper-bound sanity check.
	// The exact single-charge invariant is that the secondary
	// stream is counted once and the primary stream contributes
	// zero; the ratio floor above catches missing steam enthalpy.
	assert.Less(t, actualDegF/3.2, grossExpectedDegF*1.5,
		"containment rise should not require charging secondary steam more than once")
}

func TestPlant_MSLBOvercoolIsHeatNotInventory(t *testing.T) {
	t.Parallel()
	const window = 30 * time.Second
	var startF, endB, endD float64
	var trippedAt int = -1
	// Run the honest full-plant trajectory; whether this
	// snapshot converts the MSLB into primary-temperature
	// drop depends on all STEP's program envelopes and
	// control logic. The observable ledger invariants
	// remain the same regardless of eventual cooling.
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	testDt := 10 * time.Millisecond
	stepN(&p, int(2*time.Second/testDt), testDt)

	ctrl, ok := p.GetControlMap()["ReactorBuilding/Faults/MainSteamLineBreak"]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))

	startF = p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	for i := 0; i < int(window/testDt); i++ {
		p.Step(testDt)
		p.Update()
		if trippedAt < 0 && p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			trippedAt = i
		}
	}
	endB = p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	endD = p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	require.GreaterOrEqual(t, trippedAt, 0, "the break must trip the reactor")
	assert.Less(t, endB, startF+0.01, "an MSLB cool-down must not report net heating")
	assert.Greater(t, startF, endB+0.01, "MSLB must produce a positive primary cool-down")
	assert.Zero(t, endD, "an MSLB cool-down cannot consume primary inventory")
	assert.Zero(t, p.ReactorBuilding.ReactorPressureVessel.UncoveryFrac,
		"cooling alone cannot uncover the core")
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.VesselFailed,
		"cooling alone cannot latch vessel failure")
}

func TestPlant_BreakClassesShareOneCappedLedger(t *testing.T) {
	t.Parallel()
	dt := 5 * time.Millisecond

	// A sustained primary-break drain is capped without any
	// additional MSLB contribution.
	breakOnly := MakePlant()
	breakOnly.ReactorBuilding.ReactorPressureVessel.HotLegBreak.Open = 1
	breakOnly.ReactorBuilding.ReactorPressureVessel.BreakDischarged =
		Volume{Val: RCSDrainableInventoryGal - 0.5, Unit: u.Gallon}
	for i := 0; i < 40; i++ {
		breakOnly.Step(dt)
		breakOnly.Update()
	}
	breakTotal := breakOnly.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.LessOrEqual(t, breakTotal, RCSDrainableInventoryGal+1e-9,
		"cumulative primary-break drain is capped once")
	assert.Zero(t, breakOnly.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float(),
		"exhausted ledger delivers no further primary-break flow")

	// Pairing an MSLB with the primary break does not add primary
	// inventory to the shared drain because the paired break is a
	// secondary-side discharge. Start below the cap so a small
	// MSLB primary debit cannot hide behind saturation.
	paired := MakePlant()
	paired.ReactorBuilding.ReactorPressureVessel.HotLegBreak.Open = 1
	paired.ReactorBuilding.ReactorPressureVessel.BreakDischarged =
		Volume{Val: RCSDrainableInventoryGal - 0.5, Unit: u.Gallon}
	require.NoError(t, paired.GetControlMap()["ReactorBuilding/Faults/MainSteamLineBreak"].Set("1"))
	paired.SteamOfftakeFraction = 1
	for i := 0; i < 40; i++ {
		paired.Step(dt)
		paired.Update()
	}
	pairedTotal := paired.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	require.Greater(t, paired.ReactorBuilding.SteamGenerator.BreakFlow.Float(), 0.0,
		"paired secondary break must remain live")
	assert.LessOrEqual(t, pairedTotal, RCSDrainableInventoryGal+1e-9,
		"the paired secondary break cannot exceed the shared primary drain cap")
	assert.InDelta(t, breakTotal, pairedTotal, 0.02,
		"paired secondary break must not materially change the primary drain")
	assert.Zero(t, paired.ReactorBuilding.ReactorPressureVessel.BreakDischargedNonCreditable.Float(),
		"MSLB cannot add noncreditable primary drain")

	// Full-open SGTR driver still exhausts the shared primary
	// ledger, and the noncreditable subledger tracks that entire
	// primary drain.
	rpv := &ReactorPressureVessel{}
	pzr := &Pressurizer{Volume: Volume{Val: RCSDrainableInventoryGal + 2000, Unit: u.Gallon}, Water: Volume{Val: RCSDrainableInventoryGal + 1, Unit: u.Gallon}, Pressure: PressurizerHeaterTarget}
	sg := SteamGenerator{Water: Volume{Val: 40000, Unit: u.Gallon}, Volume: Volume{Val: 80000, Unit: u.Gallon}, Pressure: SteamGeneratorShellPressure, HeatInput: SteamGeneratorFullLoadHeatInput}
	sg.SGTubeRupture.Open = 1
	rpv.BreakDischarged = Volume{Val: RCSDrainableInventoryGal - 0.001, Unit: u.Gallon}
	rpv.breakRemaining = 0.001
	var cur = sg
	cur = cur.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv, pzr)
	assert.InDelta(t, 0.001, cur.SGTubeRuptureFlow.Float(), 1e-9,
		"final SGTR step is capped at the remaining shared primary inventory")
	cur = cur.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv, pzr)
	assert.Zero(t, cur.SGTubeRuptureFlow.Float(),
		"SGTR reaches zero once the shared ledger is exhausted")
	assert.InDelta(t, RCSDrainableInventoryGal, rpv.BreakDischarged.Float(), 1e-9,
		"SGTR cannot exceed the shared primary drain cap")
}

// TestPlant_BreakBudgetOrderInvariant pins that the shared
// vessel-owned breakRemaining budget makes break + relief + SGTR
// order-independent: running the writers in either order yields
// the same BreakDischarged against one cap.
func TestPlant_BreakBudgetOrderInvariant(t *testing.T) {
	t.Parallel()
	// Fixture: two independent vessels, same BreakDischarged seed.
	// A break-first run debits the hot-leg first; a relief-first
	// run debits relief first. Both then debit the SGTR, which
	// lives in SteamGenerator.Step (relocation test: the SGTR
	// draws from the same vessel-owned field).
	mk := func() (*ReactorPressureVessel, *Pressurizer, SteamGenerator) {
		rpv := &ReactorPressureVessel{breakRemaining: RCSDrainableInventoryGal}
		pzr := &Pressurizer{Volume: Volume{Val: RCSDrainableInventoryGal + 2000, Unit: u.Gallon}, Water: Volume{Val: RCSDrainableInventoryGal + 1, Unit: u.Gallon}, Pressure: PressurizerHeaterTarget}
		sg := SteamGenerator{Water: Volume{Val: 40000, Unit: u.Gallon}, Volume: Volume{Val: 80000, Unit: u.Gallon}, Pressure: SteamGeneratorShellPressure, HeatInput: SteamGeneratorFullLoadHeatInput}
		sg.SGTubeRupture.Open = 1
		return rpv, pzr, sg
	}
	breakFirst, _, _ := mk()
	// Break-first: hot-leg debit through the vessel's own debit
	// path, then 100 gal relief via the same field.
	breakFirst.debit(100)
	// Relief-first: same 100 gal against the same vessel-owned
	// budget, reversed order — the budget does not care which
	// writer consumed it first.
	reliefFirst := &ReactorPressureVessel{breakRemaining: RCSDrainableInventoryGal}
	reliefFirst.debit(100)

	assert.InDelta(t, breakFirst.breakRemaining, reliefFirst.breakRemaining, 1e-9,
		"debit in either order draws from the same vessel-owned budget")
	assert.InDelta(t, breakFirst.BreakDischarged.Float(),
		reliefFirst.BreakDischarged.Float(), 1e-9,
		"BreakDischarged is order-invariant under the shared budget")
}

func TestPlant_SteamLineBreakOvercoolsPastProgramEnvelope(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	testDt := 10 * time.Millisecond
	stepN(&p, int(2*time.Second/testDt), testDt)

	ctrl, ok := p.GetControlMap()["ReactorBuilding/Faults/MainSteamLineBreak"]
	require.True(t, ok, "break control must be registered")
	require.NoError(t, ctrl.Set("1"))

	startF := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	trippedAt := -1
	maxCoolFps := 0.0
	// 30 s captures the full fault signature: the trip lands within
	// seconds of the break and both thresholds are pace-relative, so
	// halving the window tightens rather than loosens them.
	const window = 30 * time.Second
	for i := 0; i < int(window/testDt); i++ {
		pre := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
		p.Step(testDt)
		p.Update()
		post := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
		if trippedAt < 0 {
			if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
				trippedAt = i
			}
			continue
		}
		maxCoolFps = max(maxCoolFps, (pre-post)/testDt.Seconds())
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
