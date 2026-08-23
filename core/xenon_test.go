package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MakePlant seeds the fission-product pair to rated equilibrium along
// with the fission/decay inventories.
func TestXenon_MakePlantSeedsEquilibrium(t *testing.T) {
	p := MakePlant()
	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.InDelta(t, 1.0, rpv.Iodine135, 1e-9,
		"iodine should seed at rated equilibrium")
	assert.InDelta(t, 1.0, rpv.Xenon135, 1e-6,
		"xenon should seed at rated equilibrium")
}

// Steady operation at rated holds both inventories flat: production
// balances destruction by construction of the normalization.
func TestXenon_EquilibriumHoldsAtRated(t *testing.T) {
	var rpv ReactorPressureVessel
	rpv.SeedSteadyStatePower(CoreRatedThermalPower)
	for range 8 {
		rpv.advanceXenon(6 * time.Hour)
	}
	assert.InDelta(t, 1.0, rpv.Iodine135, 1e-6,
		"iodine should hold equilibrium at rated power")
	assert.InDelta(t, 1.0, rpv.Xenon135, 1e-3,
		"xenon should hold equilibrium at rated power")
}

// After a shutdown from rated equilibrium the xenon inventory peaks
// about 11 hours later at a multiple of its equilibrium level, then
// decays. The window is anchored by the two decay constants; the peak
// height also depends on the derived burnup share, so only a bound is
// asserted.
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
		if rpv.Xenon135 > peak {
			peak = rpv.Xenon135
			peakHour = float64(h)
		}
	}
	assert.GreaterOrEqual(t, peak, 2.0,
		"post-shutdown xenon should build a multi-equilibrium iodine pit")
	assert.LessOrEqual(t, peak, 6.0,
		"post-shutdown xenon peak should stay bounded")
	assert.GreaterOrEqual(t, peakHour, 8.0,
		"peak should land near the ~11 hour anchor")
	assert.LessOrEqual(t, peakHour, 14.0,
		"peak should land near the ~11 hour anchor")
	assert.Less(t, rpv.Xenon135, peak*0.5,
		"by ~72 hours the pit should have decayed toward recovery")
}

// The reactivity term folds its reference at rated equilibrium:
// surplus xenon subtracts reactivity, depleted xenon adds it. Wiring
// check recomputed from CoreXenonEquilibriumWorthPct.
func TestXenon_ReactivityFollowsInventoryDeviation(t *testing.T) {
	mk := func(xe float64) float32 {
		var rpv ReactorPressureVessel
		rpv.ControlRods.Insertion = 0
		rpv.ControlRods.BoronConcentration = CVCSInitialBoron
		rpv.Outlet = Temperature{Val: float64(CoreDopplerReferenceTempF)}
		rpv.Xenon135 = xe
		return rpv.Step(0).Reactivity
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
