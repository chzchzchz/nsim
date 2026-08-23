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

// Physics-forced stability direction: at 600 ppm the net coefficient is
// negative, so a power perturbation produces OPPOSING reactivity once the
// outlet starts tracking the new target; near 1100 ppm the coefficient is
// positive and the same perturbation is REINFORCED. Sign-only assertions;
// magnitudes depend on calibration.
func TestLoadFollow_StabilitySignFlipsWithBoron(t *testing.T) {
	halfT := CoreReferenceInletF + 0.5*(CoreReferenceOutletF-CoreReferenceInletF)
	steps := int(20 * time.Second / TimeStep)

	run := func(boron, insertion float64) float32 {
		r := loadFollowFixture(insertion, boron, halfT,
			Watts(0.5*float64(CoreRatedThermalPower)))
		r.FissionPower = Watts(float64(r.FissionPower) * 1.10)
		for i := 0; i < steps; i++ {
			r = r.Step(RCPRatedSpeed)
		}
		return r.Reactivity
	}

	stable := run(loadFollowBoron, loadFollowRodsHalf)
	// B=900 sits in the positive-coef window (~667..1000 ppm) while still
	// reachable criticality at 50-percent power: I solves
	// -4.5I + 18.5*c(900)/1000 + (1000-900)*0.0005 = 0.
	unstable := run(900.0, (0.01295+0.05)/4.5)
	assert.Negative(t, stable,
		"negative-coef boron should oppose a power rise after thermal lag, rho=%v", stable)
	assert.Positive(t, unstable,
		"positive-coef boron should reinforce a power rise after thermal lag, rho=%v", unstable)
}

// The load level the operator targets is total thermal output, not
// instantaneous fission: decay heat contributes 6.5 percent of steady
// reactor heat output (specs/wiki/Decay_heat.txt), and the outlet
// program tracks TotalThermalPower/rated. At the 50-percent plateau the
// fission-only reading sits about 3.3 percent low while the plant holds
// the programmed temperature exactly. The fixture starts +0.25 F off
// the plateau so the trajectory is exercised: a fission-only program
// would relax toward 587.45 F instead of back to 589.5 F. The offset is
// kept small because the plateau loop is underdamped (tau=90 s thermal
// lag on delayed-neutron kinetics): the response scales with it, and
// 450 s lets the residual fall inside the band.
func TestLoadFollow_PlateauHoldsOnTotalThermalPower(t *testing.T) {
	halfT := CoreReferenceInletF + 0.5*(CoreReferenceOutletF-CoreReferenceInletF)
	seed := Watts(0.5 * float64(CoreRatedThermalPower))
	r := loadFollowFixture(loadFollowRodsFull, loadFollowBoronPlateau, halfT+0.25, seed)

	total := float64(r.TotalThermalPower()) / float64(CoreRatedThermalPower)
	fission := float64(r.FissionPower) / float64(CoreRatedThermalPower)
	require.InDelta(t, 0.5, total, 1e-9)
	assert.Less(t, fission, 0.48,
		"fission alone should undershoot the 50-percent load level by the decay share")

	for range int(450 * time.Second / TimeStep) {
		r = r.Step(RCPRatedSpeed)
	}
	assert.InDelta(t, halfT, r.Outlet.Float(), 0.05,
		"plateau outlet should converge on the total-power program, got %v", r.Outlet.Float())
	assert.InDelta(t, 0.5,
		float64(r.TotalThermalPower())/float64(CoreRatedThermalPower), 0.005)
	assert.Less(t, float64(r.FissionPower)/float64(CoreRatedThermalPower), 0.48,
		"fission alone should still undershoot the 50-percent load level")
}
