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
//   "The rod control system automatically controls RCS average
//   coolant temperature (Tavg) as a programmed function of turbine
//   load."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
func TestRodProgramAnchors(t *testing.T) {
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
//   "The rod control system also has an expandable temperature error
//   control deadband for use at reduced power levels [...]"
//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:77-81
func TestRodAutoControlDeadbandAndSlew(t *testing.T) {
	fullTarget := programPowerFraction(programmedRodTavg(1))

	// No-load program maps to hot zero power; carrying real power
	// against it is an over-power error that inserts rods.
	hot := ControlRods{
		programPrimed:         true,
		programTargetFraction: 0,
	}
	hot.applyAutoControl(
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
	cold.applyAutoControl(
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
	frozen.applyAutoControl(
		Temperature{Val: programmedRodTavg(1), Unit: u.Fahrenheit},
		1, fullTarget+RodCtrlDeadbandFrac/2, 0)
	assert.InDelta(t, 0.05, frozen.DemandInsertion, 1e-12,
		"settled state freezes rod motion")
}

// With automatic control disengaged the demand is the operator's
// alone; the simulator never rewrites it.
func TestRodManualDemandUntouched(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0.25

	stepN(&p, int(5*time.Second/TimeStep))

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
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true
	p.ReactorBuilding.ReactorProtectionSystem.ManualTrip = true

	stepN(&p, 1)
	require.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"fixture: manual trip must latch")
	demand := p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion

	stepN(&p, int(time.Second/TimeStep))
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
// Time-step policy: uniform 10 ms. This test originally exposed the
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
// wide. Must not run in parallel: the override mutates process-global
// state.
//
// Spec:
//   "The rod control system automatically adjusts the power level of
//   the reactor to match the power demanded by the turbine [...]"
//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-71
func TestRodAutoControlPartialPowerPlateau(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)
	p := MakePlant()
	boron := 500.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = boron
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = boron
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true

	// Chemistry trim rebalance before maneuvering.
	stepN(&p, int(30*time.Second/TimeStep))
	startPower := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower)

	const load = 0.7
	p.TurbineBuilding.Turbine.Setpoint = load
	stepN(&p, int(150*time.Second/TimeStep))
	descending := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)
	assert.Less(t, descending, startPower/float64(CoreRatedThermalPower)*0.95,
		"power follows demand downward")
	assert.Greater(t, descending, 0.5,
		"descent stays quasi-static above mid-band this early")

	stepN(&p, int(150*time.Second/TimeStep))
	rpv := p.ReactorBuilding.ReactorPressureVessel
	plateauFrac := float64(rpv.FissionPower) / float64(CoreRatedThermalPower)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"maneuver must not trip the plant")
	assert.Greater(t, plateauFrac, 0.35,
		"plateau holds power far above the neutron-source floor")
	assert.Less(t, plateauFrac, 0.65,
		"power settled onto the partial-plateau band")
	// Known deviation, recorded rather than absorbed into a band:
	// settled Tavg sits ~8 F above programmedRodTavg(load) because
	// the cascade references the walked POWER target only — the
	// measured-Tavg error term ("the needed control rod movement is
	// based on the reactor coolant temperature needed to meet the
	// turbine power demand [...] and the measured reactor coolant
	// temperature",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-76)
	// is not yet integrated into the reactivity setpoint, so Tavg
	// parks on the plant's open-loop temperature line instead of the
	// program line. Power-vs-demand tracking is asserted above; the
	// Tavg-program identity lands with that feedback term.

	// The plateau must hold, not glide into collapse.
	stepN(&p, int(90*time.Second/TimeStep))
	held := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)
	assert.Greater(t, held, 0.35, "plateau still held after holding window")
	assert.Less(t, held, 0.65, "plateau stable, not climbing away")

	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(250*time.Second/TimeStep))
	recovered := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreRatedThermalPower)
	assert.Greater(t, recovered, held+0.1,
		"restoring demand raises power back toward rated")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
}
