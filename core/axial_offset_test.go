package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Axial-offset instrumentation for the power-distribution observable
// (core/axial_offset.go). The load-follow discipline under test:
// boron adjustments are made whenever the rods approach their AFD
// target band limits, which keeps the distribution near the unrodded
// shape while preserving full reserve capability.
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:109-123
//   specs/westinghouse_plant_operations/chapter_19/page_012.txt:4-17

func axialFixture(insertion float64, fissionFrac float64) ReactorPressureVessel {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{
			Insertion:          insertion,
			DemandInsertion:    insertion,
			BoronConcentration: CVCSInitialBoron,
		},
		Inlet:  Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
	}
	r.FissionPower = Watts(fissionFrac * float64(CoreFissionPowerAtRated))
	// Rated-equilibrium fission products: the rod/boron calibration
	// already carries the nominal xenon load, mirroring the
	// load-follow fixture convention.
	r.Iodine135 = 1.0
	r.Xenon135 = xenonEquilibriumPhysical(1.0)
	return r
}

// settleAxial advances only the axial subsystem in coarse chunks,
// standing in for long simulated horizons without per-millisecond
// stepping.
func settleAxial(r *ReactorPressureVessel, horizon time.Duration) {
	const chunk = 60 * time.Second
	for elapsed := time.Duration(0); elapsed < horizon; elapsed += chunk {
		r.advanceAxialOffset(chunk)
	}
}

// A symmetric core (zero fission-product half-differences) holds the
// unrodded anchor with no rods and must not drift: the deviation ODEs
// have no source when flux shares are equal.
func TestAxialOffset_SymmetricCoreHoldsAnchor(t *testing.T) {
	r := axialFixture(0, 1.0)
	settleAxial(&r, 24*time.Hour)
	assert.InDelta(t, CoreAxialOffsetUnroddedPct, r.AxialOffset, 1e-9)
	assert.InDelta(t, 0.0, r.Iodine135AxialDeviation, 1e-12)
	assert.InDelta(t, 0.0, r.Xenon135AxialDeviation, 1e-12)
}

// Rod insertion drives the realized offset onto the rod-shift line,
// and the skew correction settles on the restoring side of it: at
// partial power the settled offset sits slightly ABOVE the static rod
// line (iodine-driven production imbalance leaves the lower core
// xenon-poor), while the transient burn-side reinforcement peaks
// between them. Settling long enough to expose the steady line keeps
// the assertion independent of the transient.
func TestAxialOffset_OperationalWindowStaysInTargetBand(t *testing.T) {
	for _, tc := range []struct {
		name        string
		insertion   float64
		fissionFrac float64
	}{
		{"descent-stage", 0.04362222, 0.5},
		{"reserve-restored", 0.04222222, 0.5},
		{"band-edge-proxy", 0.05, 1e-9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := axialFixture(tc.insertion, tc.fissionFrac)
			settleAxial(&r, 48*time.Hour)
			rodLine := CoreAxialOffsetUnroddedPct +
				CoreAxialOffsetRodShiftPctPerUnitInsertion*tc.insertion
			if tc.fissionFrac < 1e-6 {
				assert.InDelta(t, rodLine, r.AxialOffset, 1e-6,
					"zero-fission edge case pins the rod line exactly")
			} else {
				assert.InDelta(t, rodLine, r.AxialOffset, 0.15)
				assert.Greater(t, r.AxialOffset, rodLine,
					"steady skew correction must act in the restoring direction")
			}
			assert.LessOrEqual(t, r.AxialOffset, CoreAFDTargetBandPct)
			assert.GreaterOrEqual(t, r.AxialOffset, -CoreAFDTargetBandPct)
		})
	}
}

// After a load drop the bottom-heavy distribution depletes lower-core
// iodine/xenon relative to the top (production follows flux share).
// The resulting top-poor xenon deviation carries positive offset worth,
// pulling the realized offset back up away from the pure rod-shift
// line, the restoring direction of the oscillation narrative.
//   "the power distribution is forced to the opposite end of the core"
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:85-93
func TestAxialOffset_SkewRestoresAfterLoadDrop(t *testing.T) {
	r := axialFixture(0.04362222, 0.47)
	settleAxial(&r, 30*time.Minute)
	require.Positive(t, r.Xenon135AxialDeviation,
		"the fast burn asymmetry must deplete lower-core xenon first")

	settleAxial(&r, 48*time.Hour)
	assert.Negative(t, r.Xenon135AxialDeviation,
		"the slow iodine-driven production imbalance must reverse the deviation sign")
	rodLine := CoreAxialOffsetUnroddedPct +
		CoreAxialOffsetRodShiftPctPerUnitInsertion*r.ControlRods.Insertion
	assert.Greater(t, r.AxialOffset, rodLine+0.01,
		"the restored skew worth must pull the realized offset back up the rod line")
	assert.Less(t, r.AxialOffset, CoreAFDTargetBandPct,
		"restoring drift must not overshoot into the upper band")
}

// With zero half-differences the reactivity block must reduce exactly
// to the lumped xenon form: the two-node extension may not move any
// calibrated operating point until an actual skew develops.
func TestAxialOffset_ZeroDeviationReducesToLumpedReactivity(t *testing.T) {
	r := axialFixture(0.04222222, 1.0)
	r.BoronConcentration = loadFollowBoron
	r.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	stepped := r.Step(RCPRatedSpeed, 0)

	rodPct := float32(0.04222222) * CoreRodWorthABCD * 100
	powerFrac := float64(r.TotalThermalPower()) / float64(CoreRatedThermalPower)
	dopplerBasePct := float32(powerFrac * (float64(CoreReferenceOutletF) -
		float64(CoreDopplerReferenceTempF)) *
		CoreDopplerBasePcmPerDegF) / 1000
	moderatorPct := float32((float64(CoreReferenceOutletF) -
		float64(CoreDopplerReferenceTempF)) *
		netTempCoef(loadFollowBoron)) / 1000
	boronPct := float32((loadFollowBoron - CVCSInitialBoron) / 100.0 *
		float64(CoreBoronWorthPer100ppm))
	xenonPct := float32(CoreXenonEquilibriumWorthPct *
		(xenonNormalized(xenonEquilibriumPhysical(1.0)) - 1.0))
	assert.InDelta(t, float64(rodPct+dopplerBasePct+moderatorPct+boronPct+xenonPct),
		float64(stepped.Reactivity), 1e-4,
		"zero-skew reactivity must equal the lumped calibration")
}

// The xenon inventory enters the reactivity block flux-weighted across
// the halves: X_eff = X + a*dXe/2. Locks the weighted-mean arithmetic
// so a refactor cannot silently unweight the poison.
func TestAxialOffset_SkewWeightsXenonWorthByFluxShare(t *testing.T) {
	aofrac, dev := -0.04, -2e-3
	r := axialFixture(0.05, 0.5)
	r.AxialOffset = 100 * aofrac
	r.Xenon135AxialDeviation = dev

	stepped := r.Step(RCPRatedSpeed, 0)
	wantXenon := float32(CoreXenonEquilibriumWorthPct *
		(xenonNormalized(xenonEquilibriumPhysical(1.0)+aofrac*dev/2) - 1.0))
	gotXenon := stepped.Reactivity - (float32(r.ControlRods.Insertion)*CoreRodWorthABCD*100 +
		float32((r.Outlet.Float()-float64(CoreDopplerReferenceTempF))*
			netTempCoef(r.ControlRods.BoronConcentration))/1000 +
		float32((r.ControlRods.BoronConcentration-CVCSInitialBoron)/100.0*
			float64(CoreBoronWorthPer100ppm)))
	assert.InDelta(t, float64(wantXenon), float64(gotXenon), 1e-4)
}

// Full-step wiring: the production Step path advances the offset, so a
// planted off-balance insertion shows up as negative AO motion within
// seconds of simulated time.
func TestAxialOffset_StepPathAdvancesObservable(t *testing.T) {
	r := axialFixture(0.05, 1.0)
	start := r.AxialOffset
	for range int(20 * time.Second / TimeStep) {
		r = r.Step(RCPRatedSpeed, 0)
	}
	assert.Negative(t, r.AxialOffset-start,
		"deepening insertion must drive the realized offset negative")
	assert.Greater(t, r.AxialOffset, -CoreAFDTargetBandPct,
		"short-horizon motion stays inside the band")
}
