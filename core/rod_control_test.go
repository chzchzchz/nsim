package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Tavg rod-control program anchors on turbine load: zero demand
// holds hot-standby Tavg, full demand holds the rated midpoint, and
// inputs outside [0,1] clamp.
//
// Spec:
//
//	"The rod control system automatically controls RCS average
//	coolant temperature (Tavg) as a programmed function of turbine
//	load."
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
func TestRodProgramAnchors(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, RCSNoLoadTavgF, programmedRodTavg(0), 1e-9)
	assert.InDelta(t, RCSFullLoadTavgF, programmedRodTavg(1), 1e-9)
	assert.InDelta(t, RCSNoLoadTavgF, programmedRodTavg(-0.5), 1e-9,
		"negative load clamps to the no-load anchor")
	assert.InDelta(t, RCSFullLoadTavgF, programmedRodTavg(1.5), 1e-9,
		"over-range load clamps to the full-load anchor")
}

// Inside the deadband the controller holds rod motion; outside it the
// demand slews toward the program (insert above, withdraw below),
// bounded by the slew cap.
//
// Spec:
//
//	"The rod control system also has an expandable temperature error
//	control deadband for use at reduced power levels [...]"
//	specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:77-81
func TestRodAutoControlDeadbandAndSlew(t *testing.T) {
	t.Parallel()
	fullTarget := programPowerFraction(programmedRodTavg(1))

	// No-load program maps to hot zero power; carrying real power
	// against it is an over-power error that inserts rods.
	hot := ControlRods{
		programPrimed:         true,
		programTargetFraction: 0,
	}
	hot.applyAutoControl(DefaultTimeStep,
		Temperature{Val: RCSNoLoadTavgF + 20, Unit: u.Fahrenheit},
		0, 0.5, 0)
	assert.Greater(t, hot.DemandInsertion, 0.0,
		"power above the no-load program inserts rods")

	// Full-load program implies roughly half rated fission power;
	// running well under it withdraws rods toward the target.
	cold := ControlRods{
		DemandInsertion:       0.05,
		programPrimed:         true,
		programTargetFraction: fullTarget,
	}
	cold.applyAutoControl(DefaultTimeStep,
		Temperature{Val: programmedRodTavg(1), Unit: u.Fahrenheit},
		1, fullTarget/2, 0)
	assert.Less(t, cold.DemandInsertion, 0.05,
		"power under the program withdraws rods")

	// On-target power inside the deadband freezes rod motion.
	frozen := ControlRods{
		DemandInsertion:       0.05,
		programPrimed:         true,
		programTargetFraction: fullTarget,
	}
	frozen.applyAutoControl(DefaultTimeStep,
		Temperature{Val: programmedRodTavg(1), Unit: u.Fahrenheit},
		1, fullTarget+RodCtrlDeadbandFrac/2, 0)
	assert.InDelta(t, 0.05, frozen.DemandInsertion, 1e-12,
		"settled state freezes rod motion")
}

// With automatic control disengaged the demand is the operator's
// alone; the simulator never rewrites it.
func TestRodManualDemandUntouched(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0.25

	stepN(&p, int(5*time.Second/DefaultTimeStep), DefaultTimeStep)

	assert.InDelta(t, 0.25,
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion,
		1e-12, "manual demand must not move")
}

// A latched trip freezes the integrator: the scram forces full
// insertion regardless of demand, and pre-spinning withdrawal for a
// post-trip reset is not an automatic action. The trip is driven
// through the RPS manual latch, the same operator path the bridge
// exposes.
func TestRodAutoControlFrozenWhileTripped(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true
	p.ReactorBuilding.ReactorProtectionSystem.ManualTrip = true

	stepN(&p, 1, DefaultTimeStep)
	require.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"fixture: manual trip must latch")
	demand := p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion

	stepN(&p, int(time.Second/DefaultTimeStep), DefaultTimeStep)
	assert.InDelta(t, demand,
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion,
		1e-12, "tripped controller must not integrate")
	assert.InDelta(t, 1.0,
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion, 1e-9,
		"trip forces full insertion")
}

// End-to-end: dropping turbine load onto a partial plateau walks
// reactor power down the load-referenced program without collapsing
// to the neutron-source floor or pinning at rated, holds the plateau,
// and restoring demand returns power toward rated. This is the
// scenario family gap #7 declared unexecutable.
//
// Chemistry band: the fixture dilutes to 500 ppm so the net
// temperature coefficient sits negative — the regime the spec's
// maneuvering narrative assumes ("through the negative moderator
// coefficient",
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:81-83;
// "the coefficient is substantially more negative due to depletion
// and removal of shim boron",
// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:119-121).
// At fresh-fuel chemistry the positive coefficient leaves rods no
// authority below roughly half load; deep plateaus there are a
// dilution-path concern outside this slice.
//
// Assertion style: state bands along the trajectory (the internal
// program slews at the spec's 5-percent-per-minute loading rate), not
// fixed-horizon spot values.
//
// Time-step policy: uniform 25 ms. This test originally exposed the
// reactor's step-quantized fission compounding ("scales the previous
// fission power by 1 + rho/100 every step",
// specs/the_westinghouse_pressurized_water_react/glossary.md:142 via
// the ReactorPressureVessel.Step update): at a uniform 10 ms tick the
// descent parked at 0.82 of rated against a < 0.65 band, and at 100 ms
// power barely left rated, because loop gain scaled with tick count
// instead of simulated time. That channel is now anchored to
// DefaultTimeStep and raised to the elapsed tick count, so one 10 ms
// Step lands where ten 1 ms Steps would (verified by
// TestReactorFissionPower_WidthInvariance) and the whole maneuver runs
// wide.
//
// Spec:
//
//	"The rod control system automatically adjusts the power level of
//	the reactor to match the power demanded by the turbine [...]"
//	specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-71
func TestRodAutoControlPartialPowerPlateau(t *testing.T) {
	t.Parallel()
	// simulation state.
	testDt := 25 * time.Millisecond
	p := MakePlant()
	// At-power maneuvering requires forced flow: "Natural
	// circulation flow is sufficient only for decay heat removal of
	// a shutdown reactor, not for power operation."
	// specs/westinghouse_tech_manual/chapter_3/page_0325.txt:14-15
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	boron := 500.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = boron
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = boron
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true

	// Chemistry trim rebalance before maneuvering.
	stepN(&p, int(30*time.Second/testDt), testDt)
	startPower := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower)

	const load = 0.7
	p.TurbineBuilding.Turbine.Setpoint = load
	stepN(&p, int(150*time.Second/testDt), testDt)
	descending := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)
	startFrac := startPower / float64(CoreRatedThermalPower)
	assert.Less(t, descending, startFrac*0.98,
		"power follows demand downward")
	assert.Greater(t, descending, 0.5,
		"descent stays quasi-static above mid-band this early")

	// Under the split temperature feedback the descent glides slower
	// than the walked program: the strongly negative combined
	// coefficient returns reactivity as Tavg falls, which the rods
	// must spend insertion overcoming. Give the glide room to reach
	// the plateau band before asserting it.
	stepN(&p, int(450*time.Second/testDt), testDt)
	rpv := p.ReactorBuilding.ReactorPressureVessel
	plateauFrac := float64(rpv.FissionPower) / float64(CoreRatedThermalPower)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"maneuver must not trip the plant")
	assert.Greater(t, plateauFrac, 0.35,
		"plateau holds power far above the neutron-source floor")
	// Band re-derived from the measured trajectory under the split
	// feedback (item 15): the recalibrated reactivity-setpoint gain
	// settles the plateau just under the 0.70 demand instead of the
	// old 0.65 line.
	assert.Less(t, plateauFrac, 0.68,
		"power settled onto the partial-plateau band")
	// Measured-coolant-temperature duty ("the needed control rod
	// movement is based on the reactor coolant temperature needed to
	// meet the turbine power demand [...] and the measured reactor
	// coolant temperature",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-76)
	// rides in applyAutoControl as a bounded trim since the cold-leg
	// anchor recalibration closed plans/x/gaps.md item 14, so the
	// park no longer carries the part-load line offset that used to
	// sit here as a recorded deviation.

	// The plateau must hold, not glide into collapse.
	stepN(&p, int(90*time.Second/testDt), testDt)
	held := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)
	assert.Greater(t, held, 0.35, "plateau still held after holding window")
	assert.Less(t, held, 0.68, "plateau stable, not climbing away")
}

// The ascent leg of the partial-plateau evolution, restored once the
// cold-leg anchor recalibration put the part-load heat-balance line on
// the Tavg program (plans/x/gaps.md item 14).
//
//	"to overcome "power defect" during power ascension"
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:54-55
func TestRodAutoControlPartialPowerPlateauAscent(t *testing.T) {
	t.Parallel()
	testDt := 25 * time.Millisecond
	p := MakePlant()
	// Forced flow for an at-power arc (see the plateau test above).
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = 500
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = 500
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true
	stepN(&p, int(30*time.Second/testDt), testDt)
	p.TurbineBuilding.Turbine.Setpoint = 0.7
	stepN(&p, int(600*time.Second/testDt), testDt)
	held := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)

	p.PlantOperation.LoadFollowActive = true
	p.ReactorBuilding.Grid.LoadDemand = 1000
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = 450
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = 450
	stepN(&p, int(450*time.Second/testDt), testDt)
	recovered := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)
	assert.Greater(t, recovered, held+0.1,
		"restoring demand raises power back toward rated")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
}

// The scenario 8 reference cycle in miniature under the integrated
// control lineup: automatic rod control walks power down the Tavg
// program to the half-load plateau and back while the automatic boron
// trim carries the slow iodine/xenon swings that would otherwise bury
// or strand the rod bank, holding the bank near its withdrawal limit
// (full reserve capability) with the axial offset inside its target
// band at every checkpoint.
//
//	"12 hours at full power, decreasing load to 50-percent power over
//	 a 3 hour period, remaining at 50-percent power for 6 hours and
//	 returning to full power over a 3 hour period"
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_230.txt:79-87
//	"In plants with the Integrated Control System the boron
//	 adjustments are done automatically."
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:114-116
//	"the control system makes boron concentration adjustments whenever
//	 the control rods approach the limits of their AFD target band"
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:120-123
//
// Windows are shortened to the controller's own timescales (the
// internal program walks at the spec loading rate, so a half-load
// drop needs ten minutes regardless of the wall-clock legs of the
// reference day); multi-hour xenon evolution is covered by the
// reserve-point and iodine/xenon tests.
func TestRodAutoControlDailyLoadFollowArc(t *testing.T) {
	t.Parallel()
	// Width note: every channel here is width-safe after the
	// anchoring. The program walk rides the fission update verified
	// by TestReactorFissionPower_WidthInvariance; the automatic boron
	// trim selects a fixed-rate BTRS mode on rod position alone
	// (applyAutoBoronControl) whose ppm integration is linear in dt;
	testDt := 25 * time.Millisecond
	p := MakePlant()
	// Forced flow for an at-power arc (see the plateau test above).
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	// Main feedwater carries the arc's whole-day hold: the corrected
	// secondary balance drains the boiling stock at the rated
	// evaporation rate whenever the core is making power
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:68-76).
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	for _, boron := range []*float64{
		&p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration,
		&p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration,
	} {
		*boron = loadFollowBoron
	}
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.ControlRods.AutoControl = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true
	// Coherent dispatch: auto control follows the turbine demand, so
	// a settled-at-rated start needs the demand standing at rated.
	// Leaving the MakePlant zero-demand seed makes the controller
	// correctly walk power down and the precondition reads as a bug.
	p.TurbineBuilding.Turbine.Setpoint = 1.0

	fissionFrac := func() float64 {
		return float64(rpv.FissionPower) / float64(CoreRatedThermalPower)
	}
	assertAOInBand := func(when string) {
		t.Helper()
		assert.GreaterOrEqual(t, rpv.AxialOffset, -CoreAFDTargetBandPct-0.5,
			"%s: axial offset below band (small transient margin)", when)
		assert.LessOrEqual(t, rpv.AxialOffset, CoreAFDTargetBandPct+0.5,
			"%s: axial offset above band (small transient margin)", when)
	}

	stepN(&p, int(60*time.Second/testDt), testDt)
	require.Greater(t, fissionFrac(), 0.9,
		"settled start must sit near rated before maneuvering")
	assertAOInBand("start")

	// Descent leg: the walked program takes ten minutes down.
	p.TurbineBuilding.Turbine.Setpoint = 0.5
	stepN(&p, int(660*time.Second/testDt), testDt)
	descended := fissionFrac()
	assert.Greater(t, descended, 0.38, "descent undershoots the plateau")
	assert.Less(t, descended, 0.62, "descent overshoots the program")
	assertAOInBand("descent")
	require.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)

	// Plateau: the boron trim keeps the rod bank inside its window
	// against the post-drop xenon rise - reserve stays intact.
	stepN(&p, int(240*time.Second/testDt), testDt)
	assert.Greater(t, fissionFrac(), 0.38, "plateau collapsed")
	assert.Less(t, fissionFrac(), 0.62, "plateau climbed away")
	assert.LessOrEqual(t, rpv.ControlRods.Insertion,
		CVCSAutoBoronHighInsertionFrac+0.01,
		"boron trim must keep the bank near its withdrawal limit")
	assertAOInBand("plateau")

	// Ascent leg back toward rated on restored demand.
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(480*time.Second/testDt), testDt)
	assert.Greater(t, fissionFrac(), descended+0.15,
		"restored demand climbs back toward rated")
	assertAOInBand("ascent")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
}
