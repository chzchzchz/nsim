package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MakePlant seeds the fission-product pair to rated equilibrium along
// with the fission/decay inventories. Intentional update (physical-
// xenon storage): the stored Xenon135 is the physical concentration,
// so the rated equilibrium is asserted through xenonNormalized.
func TestXenon_MakePlantSeedsEquilibrium(t *testing.T) {
	p := MakePlant()
	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.InDelta(t, 1.0, rpv.Iodine135, 1e-9,
		"iodine should seed at rated equilibrium")
	assert.InDelta(t, xenonEquilibriumPhysical(1), rpv.Xenon135, 1e-9,
		"xenon should seed at the physical rated equilibrium")
	assert.InDelta(t, 1.0, xenonNormalized(rpv.Xenon135), 1e-6,
		"worth scale should read rated equilibrium")
}

// Steady operation at rated holds both inventories flat: production
// balances destruction by construction of the normalization.
// Regression (physical-xenon storage): asserted on the worth scale.
func TestXenon_EquilibriumHoldsAtRated(t *testing.T) {
	var rpv ReactorPressureVessel
	rpv.SeedSteadyStatePower(CoreRatedThermalPower)
	for range 8 {
		rpv.advanceXenon(6 * time.Hour)
	}
	assert.InDelta(t, 1.0, rpv.Iodine135, 1e-6,
		"iodine should hold equilibrium at rated power")
	assert.InDelta(t, 1.0, xenonNormalized(rpv.Xenon135), 1e-3,
		"xenon should hold equilibrium at rated power")
}

// After a shutdown from rated equilibrium the xenon inventory peaks
// about 11 hours later at a multiple of its equilibrium level, then
// decays. The window is anchored by the two decay constants; the peak
// height also depends on the derived burnup share, so only a bound is
// asserted.
//
// Intentional update (physical-xenon storage): bounds are read on the
// worth scale via xenonNormalized because the stored concentration is
// physical.
//
// "The 135Xe concentration peaks about 11 hours after reactor power is
// decreased." specs/wiki/Xenon-135.txt
// "Produced via iodine-135, Xenon-135 is a fission product with a 6.7
// hour half-life."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:164-165
func TestXenon_PostShutdownPeakTimingAndBound(t *testing.T) {
	var rpv ReactorPressureVessel
	rpv.SeedSteadyStatePower(CoreRatedThermalPower)
	rpv.FissionPower = CoreNeutronSourcePower

	peak := 0.0
	peakHour := 0.0
	for h := 1; h <= 72; h++ {
		rpv.advanceXenon(time.Hour)
		if n := xenonNormalized(rpv.Xenon135); n > peak {
			peak = n
			peakHour = float64(h)
		}
	}
	assert.GreaterOrEqual(t, peak, 2.0,
		"post-shutdown xenon should build a multi-equilibrium iodine pit")
	assert.LessOrEqual(t, peak, 6.0,
		"post-shutdown xenon peak should stay bounded")
	assert.GreaterOrEqual(t, peakHour, 4.0,
		"peak must ride the multi-hour iodine clock, not minutes")
	assert.LessOrEqual(t, peakHour, 14.0,
		"peak should land near the ~11 hour anchor")
	assert.Less(t, xenonNormalized(rpv.Xenon135), peak*0.5,
		"by ~72 hours the pit should have decayed toward recovery")
}

// A sustained power reduction drops xenon burnup destruction faster
// than production, so the inventory climbs immediately (the classic
// load-follow transient operators compensate with rods/boron). This is
// the mechanism that makes intermediate-power plateaus unstable under
// static rod position during load maneuvers.
//
// Intentional update (physical-xenon storage): seeded at the physical
// rated equilibrium and read on the worth scale.
func TestXenon_InventoryRisesAfterPowerReduction(t *testing.T) {
	var rpv ReactorPressureVessel
	rpv.Iodine135 = 1
	rpv.Xenon135 = xenonEquilibriumPhysical(1)
	p := 0.9
	rpv.FissionPower = Watts(p * float64(CoreFissionPowerAtRated))

	prev := xenonNormalized(rpv.Xenon135)
	for i := 1; i <= 10; i++ {
		rpv.advanceXenon(60 * time.Second)
		cur := xenonNormalized(rpv.Xenon135)
		assert.Greater(t, cur, prev,
			"xenon must climb while power sits below the iodine equilibrium")
		prev = cur
	}
	assert.Greater(t, prev, 1.0005,
		"ten minutes at 90 percent power must show measurable xenon growth")
	assert.Less(t, prev, 1.06,
		"growth should stay on the minutes-to-hours physical scale")
}

// The reactivity term folds its reference at rated equilibrium:
// surplus xenon subtracts reactivity, depleted xenon adds it. Wiring
// check recomputed from CoreXenonEquilibriumWorthPct.
//
// Regression (physical-xenon storage): fixtures store the physical
// concentration whose worth-scale reading matches the deviation under
// test.
func TestXenon_ReactivityFollowsInventoryDeviation(t *testing.T) {
	eq := xenonEquilibriumPhysical(1)
	mk := func(xe float64) float32 {
		var rpv ReactorPressureVessel
		rpv.ControlRods.Insertion = 0
		rpv.ControlRods.BoronConcentration = CVCSInitialBoron
		rpv.Outlet = Temperature{Val: float64(CoreDopplerReferenceTempF)}
		rpv.Xenon135 = xe * eq
		return rpv.Step(DefaultTimeStep, 0, 0).Reactivity
	}
	base := mk(1.0)
	require.InDelta(t, 0.0, base, 1e-3,
		"at rated equilibrium the folded xenon term must vanish")

	for _, tc := range []struct {
		dev float64
	}{
		{dev: 0.1},
		{dev: -0.1},
	} {
		got := mk(1.0 + tc.dev)
		want := base + float32(CoreXenonEquilibriumWorthPct*tc.dev)
		assert.InDelta(t, want, got, 1e-4,
			"reactivity shift should be worth*(xe-1) for dev %v", tc.dev)
	}
}

// The stored-vs-displayed split keeps rated worth neutral and the
// conversion exact: normalizing the physical rated equilibrium lands
// on 1 by construction of the ratio constant.
//
// Intentional update (physical-xenon storage): pins the acceptance
// criterion xenonNormalized(xenonEquilibriumPhysical(1)) ~= 1.
func TestXenon_NormalizationRoundTripAtRated(t *testing.T) {
	assert.InDelta(t, 1.0, xenonNormalized(xenonEquilibriumPhysical(1)), 1e-3,
		"rated physical equilibrium must normalize to 1")
	assert.Less(t, xenonNormalized(xenonEquilibriumPhysical(0.5)), 1.0,
		"part-power equilibrium carries less poison than rated")
	assert.Zero(t, xenonEquilibriumPhysical(0),
		"a fresh core holds no xenon")
}
