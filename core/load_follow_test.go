package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Load-follow operating points for scenario 8 (plans/x/scenario/8.txt):
// the reference 12-3-6-3 daily load cycle is followed with rods cracked
// near the withdrawal limit while boron carries the shim, preserving
// reserve capability for return to full power at the design ramp rate.
//   "the plant maintains full reserve capability for returning to full
//    power at the design ramp rate of 5 percent per minute [...] kept
//    [...]" (paraphrase) specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:109-123
//
// The rod-position program I(pf) holds rho = 0 on the Tavg program
// T(pf) = 558 + 63*pf at B = 600 ppm, inside the negative-coef region
// where the power/temperature loop is self-stabilizing (net temperature
// coefficient negative with depletion,
// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122).

const (
	loadFollowBoron        = 600.0
	loadFollowBoronPlateau = 614.17
	loadFollowRodsFull     = 0.04222222
	loadFollowRodsHalf     = 0.04362222
)

func loadFollowFixture(insertion, boron, outletF float64, seedPower Watts) ReactorPressureVessel {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{
			Insertion:          insertion,
			DemandInsertion:    insertion,
			BoronConcentration: boron,
		},
		Inlet:  Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: outletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(seedPower)
	// Pin fission products at their RATED-power equilibrium: the
	// programmed rod positions solve criticality against the
	// rod/boron terms alone, whose calibration already carries the
	// rated-equilibrium xenon load ("the rod/boron calibration
	// above already carries the nominal equilibrium load"). Seeding
	// partial-power xenon instead adds a positive offset at reduced
	// power that has nothing to do with the program under test.
	r.Iodine135 = 1.0
	r.Xenon135 = xenonEquilibriumPhysical(1.0)
	return r
}

// Wiring check: each programmed operating point sits at rho = 0 under the
// production reactivity block. Expected values are derived from the same
// constants the model uses, so this catches wiring mistakes (wrong rod
// worth scaling, coef interpolation regressions), not model errors.
func TestLoadFollow_ProgrammedOperatingPointsAreCritical(t *testing.T) {
	full := loadFollowFixture(loadFollowRodsFull, loadFollowBoron,
		CoreReferenceInletF+(CoreReferenceOutletF-CoreReferenceInletF), // 621 F
		CoreRatedThermalPower)
	full = full.Step(RCPRatedSpeed)
	require.InDelta(t, 0.0, float64(full.Reactivity), 1e-4,
		"full-power point (I=%.8f, B=%.0f, T=621) should be critical, rho=%v",
		loadFollowRodsFull, loadFollowBoron, full.Reactivity)

	halfT := CoreReferenceInletF + 0.5*(CoreReferenceOutletF-CoreReferenceInletF)
	half := loadFollowFixture(loadFollowRodsHalf, loadFollowBoron, halfT,
		Watts(0.5*float64(CoreRatedThermalPower)))
	half = half.Step(RCPRatedSpeed)
	require.InDelta(t, 0.0, float64(half.Reactivity), 1e-4,
		"50-percent point (I=%.8f, B=%.0f, T=%.1f) should be critical, rho=%v",
		loadFollowRodsHalf, loadFollowBoron, halfT, half.Reactivity)

	plateau := loadFollowFixture(loadFollowRodsFull, loadFollowBoronPlateau, halfT,
		Watts(0.5*float64(CoreRatedThermalPower)))
	plateau = plateau.Step(RCPRatedSpeed)
	require.InDelta(t, 0.0, float64(plateau.Reactivity), 1e-4,
		"plateau reserve point (rods restored, B=%.2f) should be critical, rho=%v",
		loadFollowBoronPlateau, plateau.Reactivity)
}

// Physics-forced sign test: boration moves the doppler term too because
// the net temperature coefficient depends on boron concentration. The
// naive rod-worth/0.0005 conversion (+12.6 ppm -> 612.6) leaves the core
// supercritical at the plateau; the solve that includes dc/dB (+14.17 ppm
// -> 614.17) does not. Locks the correction into the repo so a future
// refactor cannot silently drop the coef shift.
func TestLoadFollow_NaiveBorationLeavesCoreSupercritical(t *testing.T) {
	halfT := CoreReferenceInletF + 0.5*(CoreReferenceOutletF-CoreReferenceInletF)
	naive := loadFollowFixture(loadFollowRodsFull, 612.6, halfT,
		Watts(0.5*float64(CoreRatedThermalPower)))
	naive = naive.Step(RCPRatedSpeed)
	assert.Greater(t, float64(naive.Reactivity), 5e-4,
		"naive 12.6 ppm boration should leave positive reactivity, rho=%v",
		naive.Reactivity)

	exact := loadFollowFixture(loadFollowRodsFull, loadFollowBoronPlateau, halfT,
		Watts(0.5*float64(CoreRatedThermalPower)))
	exact = exact.Step(RCPRatedSpeed)
	assert.Less(t, float64(exact.Reactivity), 1e-4,
		"exact boration solve should be critical, rho=%v", exact.Reactivity)
}

// loadFollowFixedPoint seeds the vessel at the true steady fixed point
// for the given power: fission products at their partial-power
// equilibrium (SeedSteadyStatePower), and rod insertion solved so total
// reactivity is exactly zero at this state, xenon term included. From
// here the only thing that moves rho is the perturbation under test —
// iodine/xenon are already at equilibrium, so poison drift cannot mask
// the feedback sign.
func loadFollowFixedPoint(boron, outletF float64, seedPower Watts) ReactorPressureVessel {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{BoronConcentration: boron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: outletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(seedPower)
	dop := (outletF - float64(CoreDopplerReferenceTempF)) *
		netTempCoef(boron) / 1000
	bor := (boron - CVCSInitialBoron) / 100 * float64(CoreBoronWorthPer100ppm)
	xe := CoreXenonEquilibriumWorthPct * (xenonNormalized(r.Xenon135) - 1.0)
	insertion := -(dop + bor + xe) / (float64(CoreRodWorthABCD) * 100)
	r.ControlRods.Insertion = insertion
	r.ControlRods.DemandInsertion = insertion
	return r
}

// Physics-forced stability direction: the net temperature coefficient
// (Doppler + moderator) is negative at maneuvering chemistry and flips
// positive near fresh-fuel boron concentrations — the sign that decides
// whether a power rise is opposed or reinforced once the thermal lag
// plays out.
//   "the coefficient is substantially more negative due to depletion
//    and removal of shim boron"
//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
// Asserted on the coefficient function directly: a dynamic version of
// this check cannot isolate the sign, because the xenon worth (amplified
// through the iodine-to-xenon inventory ratio) responds to any power
// perturbation an order of magnitude faster and stronger than the
// temperature feedback it is meant to expose.
func TestLoadFollow_StabilitySignFlipsWithBoron(t *testing.T) {
	assert.Negative(t, netTempCoef(loadFollowBoron),
		"maneuvering chemistry must carry a negative net coefficient")
	assert.Positive(t, netTempCoef(900.0),
		"fresh-fuel chemistry must carry a positive net coefficient")
}

// The load level the operator targets is total thermal output, not
// instantaneous fission: decay heat contributes 6.5 percent of steady
// reactor heat output (specs/wiki/Decay_heat.txt), and the outlet
// program tracks TotalThermalPower/rated. At the 50-percent plateau the
// fission-only reading sits about 3.3 percent low while the plant holds
// the programmed temperature exactly. The fixture starts at the true
// partial-power fixed point (critical insertion solved with the
// partial-power xenon offset, see loadFollowFixedPoint) nudged +0.25 F
// off the plateau. Assertions are directional over a short window:
// the outlet relaxes DOWN toward the total-power program (a
// fission-only program would sit below the start point, not above it)
// while total output stays pinned. A fixed-horizon convergence band is
// deliberately not asserted: over minutes the iodine/xenon pair drifts
// the critical insertion under fixed rods, which is the slow channel
// the automatic rod controller exists to trim.
func TestLoadFollow_PlateauHoldsOnTotalThermalPower(t *testing.T) {
	halfT := CoreReferenceInletF + 0.5*(CoreReferenceOutletF-CoreReferenceInletF)
	seed := Watts(0.5 * float64(CoreRatedThermalPower))
	r := loadFollowFixedPoint(loadFollowBoronPlateau, halfT+0.25, seed)

	total := float64(r.TotalThermalPower()) / float64(CoreRatedThermalPower)
	fission := float64(r.FissionPower) / float64(CoreRatedThermalPower)
	require.InDelta(t, 0.5, total, 1e-9)
	assert.Less(t, fission, 0.48,
		"fission alone should undershoot the 50-percent load level by the decay share")
	startOutlet := r.Outlet.Float()

	for range int(45 * time.Second / TimeStep) {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Less(t, r.Outlet.Float(), startOutlet,
		"outlet relaxes downward toward the total-power program")
	assert.Greater(t, r.Outlet.Float(), halfT,
		"relaxation approaches the program from above, not past it")
	assert.InDelta(t, 0.5,
		float64(r.TotalThermalPower())/float64(CoreRatedThermalPower), 0.005)
	assert.Less(t, float64(r.FissionPower)/float64(CoreRatedThermalPower), 0.48,
		"fission alone should still undershoot the 50-percent load level")
}
