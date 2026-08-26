package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Calibration-scale witnesses (gaps.md item 18 stage 1): pin the two
// reactivity scales - soluble boron coupling and rod-bank authority -
// that any auto-boron exchange budget must be denominated in.
// Differential measurement through a stepped plant proved
// environment-sensitive (the moderator term rides dt-dependent thermal
// evolution), so the scales are pinned at their production seams: the
// pure netTempCoef interpolation and the bank's own step response.

// The ABCD rod bank's measured worth per unit insertion must reproduce
// CoreRodWorthABCD x 100 pct: the scale the cascade's slew authority
// competes against.
func TestWitness_RodBankWorthPerInsertion(t *testing.T) {
	low, high := MakePlant(), MakePlant()
	for i, p := range []*Plant{&low, &high} {
		insertion := []float64{0.10, 0.12}[i]
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		rpv.ControlRods.Insertion = insertion
		rpv.ControlRods.DemandInsertion = insertion
	}
	stepN(&low, 1)
	stepN(&high, 1)

	rhoLow := float64(low.ReactorBuilding.ReactorPressureVessel.Reactivity)
	rhoHigh := float64(high.ReactorBuilding.ReactorPressureVessel.Reactivity)
	// low carries LESS insertion, so its rho reads higher.
	want := -float64(CoreRodWorthABCD) * 100 * 0.02
	assert.InDelta(t, want, rhoLow-rhoHigh, 1e-3,
		"0.02 of rod travel must be worth %v pct through production, got %v",
		want, rhoLow-rhoHigh)
}

// The moderator coefficient's boron interpolation is the coupling that
// makes effective boron worth concentration-dependent: linear from the
// zero-boron anchor to the rated-critical anchor, then clamped above
// it. Pinning the anchors and the clamp keeps the coupling from
// drifting under a future exchange retune.
func TestWitness_ModeratorCoefBoronInterpolation(t *testing.T) {
	atRef := netTempCoef(float64(CVCSInitialBoron))
	assert.Equal(t, float64(CoreNetTempCoefAtRefBoronPcmPerDegF), atRef,
		"rated-critical anchor must sit on the reference coefficient")

	mid := netTempCoef(float64(CVCSInitialBoron) / 2)
	wantMid := CoreNetTempCoefAtZeroBoronPcmPerDegF +
		(CoreNetTempCoefAtRefBoronPcmPerDegF-CoreNetTempCoefAtZeroBoronPcmPerDegF)/2
	assert.InDelta(t, wantMid, mid, 1e-12,
		"midpoint must be the linear interpolation")

	above := netTempCoef(float64(CVCSInitialBoron) + 200)
	assert.Equal(t, atRef, above,
		"the interpolation clamps above the rated-critical anchor")
}

// Unit coherence for the exchange budget: full-drive demand-tier
// exchange moves reactivity at exactly
// CVCSBorationRate x |CoreBoronWorthPer100ppm| / 100 pct per second,
// and that budget sits inside the rod cascade's slew authority
// expressed in the same units. If a future change breaks this
// relationship (rate rescaled without re-deriving against the worth,
// or the worth constant edited in different units), this fails.
func TestWitness_ExchangeBudgetUnitCoherence(t *testing.T) {
	worthPctPerPpm := float64(-CoreBoronWorthPer100ppm) / 100.0
	exchangePctPerSec := CVCSBorationRate * worthPctPerPpm

	rodWorthPctPerInsertion := -float64(CoreRodWorthABCD) * 100
	rodAuthorityPctPerSec := RodCtrlMaxDemandSlewPerSec *
		rodWorthPctPerInsertion

	assert.Greater(t, rodAuthorityPctPerSec, exchangePctPerSec,
		"rod slew authority must outpace full-drive chemistry")
	assert.Less(t, exchangePctPerSec, rodAuthorityPctPerSec/10,
		"chemistry is a trim, not a competing actuator: full-drive "+
			"exchange must stay an order of magnitude inside rod authority, got %v vs %v",
		exchangePctPerSec, rodAuthorityPctPerSec)
}
