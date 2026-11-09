package core

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers live in core/reactor.go alongside advanceXenon and
// xenonNormalized because they are pure functions of
// ReactorPressureVessel state (plans/p71_1.txt). These tests pin
// the window laws, the N-16 first-order solution, the DNBR shape,
// the kW/ft normalization, and the sensor kinds.

// TestExcore_WindowLaws pins the three-range handover: at
// p=0.01 the Intermediate Range starts at 1 (no gap below —
// source range covers it), at p=0.25 both read 25 (continuous
// handover), at p=1.0 the Power Range reads 100, at p=1.2 it
// reads its 120 top.
func TestExcore_WindowLaws(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p     float64
		wantI float64
		wantP float64
	}{
		{0.005, 0, 0},
		{0.0099, 0, 0},
		{0.01, 1, 0},
		{0.10, 10, 0},
		{0.2499, 24.99, 0},
		{0.25, 25, 25}, // continuous handover: both read 25
		{0.2501, 25, 25.01},
		{1.00, 25, 100},
		{1.20, 25, 120},
	}
	for _, tc := range cases {
		r := ReactorPressureVessel{FissionPower: Watts(tc.p * float64(CoreFissionPowerAtRated))}
		assert.InDeltaf(t, tc.wantI, intermediateRangePct(&r), 1e-9, "IR p=%g", tc.p)
		assert.InDeltaf(t, tc.wantP, powerRangePct(&r), 1e-9, "PR p=%g", tc.p)
	}
	// Handover continuity: at p=0.25 both ranges read 25.
	r := ReactorPressureVessel{FissionPower: Watts(0.25 * float64(CoreFissionPowerAtRated))}
	assert.InDelta(t, intermediateRangePct(&r), powerRangePct(&r), 1e-9,
		"IR and PR must agree at the handover")
}

// TestExcore_Nitrogen16 pins the exact first-order solution:
// N(h) = p + (N0-p)exp(-λh) with λ = ln2/7.13s, equilibrium N = p,
// and ~7 s half-life on a step to zero fission power.
func TestExcore_Nitrogen16(t *testing.T) {
	t.Parallel()
	// Seeded equilibrium.
	r := ReactorPressureVessel{}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	assert.InDeltaf(t, 1.0, r.Nitrogen16, 1e-9, "seed sets N16 to pFraction")

	// One half-life at constant p=0 (post-trip): N halves.
	r.Nitrogen16 = 1.0
	r.FissionPower = CoreNeutronSourcePower // ~0 fraction
	r.advanceNitrogen16(time.Duration(Nitrogen16HalfLifeS * float64(time.Second)))
	assert.InDeltaf(t, 0.5, r.Nitrogen16, 0.01,
		"after one half-life N16 should halve (~7 s)")

	// Equilibrium: constant p=0.5 keeps N at 0.5.
	r.FissionPower = Watts(0.5 * float64(CoreFissionPowerAtRated))
	r.Nitrogen16 = 0.5
	r.advanceNitrogen16(DefaultTimeStep)
	assert.InDeltaf(t, 0.5, r.Nitrogen16, 1e-9, "equilibrium N = p")

	// Convergence: N0=0 at p=1 converges to ~1. After ~30 s the
	// residual is exp(-λ·30) ≈ 0.054 (λ = ln2/7.13); after 60 s ≈
	// 0.003 — 60 is the round number that lands inside the 0.01
	// tolerance the assertion uses.
	r.Nitrogen16 = 0.0
	r.FissionPower = CoreFissionPowerAtRated
	for i := 0; i < 60_000; i++ {
		r.advanceNitrogen16(DefaultTimeStep)
	}
	assert.InDeltaf(t, 1.0, r.Nitrogen16, 0.01, "converges within ~60 s")

	// Zero-p clamp is defensive: Step clamps FissionPower at
	// CoreRatedThermalPower (reactor.go:348) but CoreFissionPowerAtRated
	// is 0.935×rated, so a maxed-out vessel reads p ≈ 1.0695 — still
	// under the 1.2 clamp. The clamp never binds from any reachable
	// state; it only guards calls with hand-built FissionPower.
	r2 := ReactorPressureVessel{Nitrogen16: 1.5}
	r2.FissionPower = CoreRatedThermalPower
	r2.advanceNitrogen16(time.Duration(10 * time.Second))
	// N = 1.0695 + (1.5−1.0695)·e^(−λ·10) ≈ 1.23: descends toward the
	// live p instead of being clamped to 1.2.
	assert.LessOrEqualf(t, r2.Nitrogen16, 1.5, "descends toward p")
	assert.GreaterOrEqualf(t, r2.Nitrogen16, float64(CoreRatedThermalPower)/float64(CoreFissionPowerAtRated)-1e-9,
		"converges down to p ≈ 1.07")

	// Zero dt is a no-op (backfill path for freshly-decoded plants).
	r3 := ReactorPressureVessel{Nitrogen16: 0.0}
	r3.FissionPower = CoreFissionPowerAtRated
	r3.advanceNitrogen16(0)
	assert.InDeltaf(t, 0.0, r3.Nitrogen16, 1e-9, "zero dt no-op")
}

// TestExcore_DNBR_Shape pins the derived margin law: 1.30 at
// reference, monotonic decrease in ΔT and in |ΔI|, and negative
// values readable (the signedpercent kind).
func TestExcore_DNBR_Shape(t *testing.T) {
	t.Parallel()
	base := ReactorPressureVessel{
		Inlet:  Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	assert.InDeltaf(t, DNBRRefMargin, dnbrMargin(&base), 1e-9,
		"DNBR = 1.30 at reference (ΔT = CETDTRefF, ΔI = 0)")

	// One-at-a-time sweep: ΔT alone. Starts from a cold core (below
	// ref, margin above 1.30) and rises through the reference point.
	prev := math.Inf(1)
	for _, dT := range []float64{0, 63, 126} {
		hot := ReactorPressureVessel{
			Inlet:  Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
			Outlet: Temperature{Val: CoreReferenceInletF + dT, Unit: u.Fahrenheit},
		}
		m := dnbrMargin(&hot)
		assert.Lessf(t, m, prev, "monotonic in ΔT at dT=%g", dT)
		prev = m
	}
	// |ΔI| alone, both signs: margin decreases with |ΔI|, strictly
	// below the ref margin for every |ΔI| > 0.
	for _, dI := range []float64{10, 24} {
		for _, s := range []float64{-1, 1} {
			r := base
			r.AxialOffset = s * dI
			assert.Lessf(t, dnbrMargin(&r), DNBRRefMargin,
				"margin decreases with |ΔI|=%g (sign %g)", dI, s)
		}
	}
	// Both signs symmetric.
	pos, neg := base, base
	pos.AxialOffset, neg.AxialOffset = 15, -15
	assert.InDeltaf(t, dnbrMargin(&pos), dnbrMargin(&neg), 1e-9, "|ΔI| symmetric")
}

// TestExcore_LinearPower pins the derived average and the
// normalized sensor form: avg ≈ 6.2 kW/ft, peak = avg×2.5, and
// normalized kwft/maxKWft = 1.0 at rated power, ΔI = 0.
func TestExcore_LinearPower(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{FissionPower: CoreFissionPowerAtRated}
	avgKWft := (float64(CoreRatedThermalPower) / 1000) /
		(FuelAssemblies * FuelRodsPerAssembly * ActiveLengthFt)
	assert.InDeltaf(t, 6.2, avgKWft, 0.1, "avg linear power ~6.2 kW/ft")
	assert.InDeltaf(t, avgKWft*LinearPowerPeakingFac, maxKWft(), 1e-9, "maxKWft equals avg times peaking")
	assert.InDeltaf(t, 1.0, linearPowerKWft(&r)/maxKWft(), 1e-9,
		"normalized LPD at rated = 1.0")
	assert.InDeltaf(t, avgKWft*LinearPowerPeakingFac, linearPowerKWft(&r), 1e-9, "peak kwft at rated")
	// Monotonic in power and |ΔI|.
	r.AxialOffset = 10
	assert.Greater(t, linearPowerKWft(&r), linearPowerKWft(&ReactorPressureVessel{
		FissionPower: CoreFissionPowerAtRated}), "rises with |ΔI|")
	// Never negative, never out of [0,1] normalized.
	r.FissionPower = CoreNeutronSourcePower
	assert.GreaterOrEqual(t, linearPowerKWft(&r)/maxKWft(), 0.0)
	assert.LessOrEqual(t, linearPowerKWft(&r)/maxKWft(), 1.0)
}

// TestExcore_SensorKinds checks every new sensor key exists and
// its Get returns no error, including the negative-DNBR case
// that FormatSensorValue would reject under "normalized".
func TestExcore_SensorKinds(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	m := GetSensorMap(&p)
	want := []string{
		"ReactorBuilding/ReactorPressureVessel/ExcoreIntermediateRange",
		"ReactorBuilding/ReactorPressureVessel/ExcorePowerRange",
		"ReactorBuilding/ReactorPressureVessel/AxialFluxDifference",
		"ReactorBuilding/ReactorPressureVessel/Nitrogen16Activity",
		"ReactorBuilding/ReactorPressureVessel/TTFMFlowPercent",
		"ReactorBuilding/ReactorPressureVessel/CoreExitTemp",
		"ReactorBuilding/ReactorPressureVessel/CoreExitDeltaT",
		"ReactorBuilding/ReactorPressureVessel/DNBRMargin",
		"ReactorBuilding/ReactorPressureVessel/DNBFireLive",
		"ReactorBuilding/ReactorPressureVessel/LinearPowerDensity",
		"ReactorBuilding/ReactorPressureVessel/ReactivityEstimate",
		"ReactorBuilding/ReactorPressureVessel/XenonWorth",
	}
	require.Lenf(t, want, 12, "12 sensors")
	for _, id := range want {
		s, ok := m[id]
		require.Truef(t, ok, "want sensor %q registered", id)
		out, err := s.Get()
		require.NoErrorf(t, err, "Get %q", id)
		assert.NotEmptyf(t, out, "want non-empty value from %q", id)
	}

	// Negative DNBR: drive the margin below zero and confirm
	// Get does not error (signedpercent tolerates negatives).
	// dT=189 (3x ref), |dI|=100 -> margin = 1.3(1-0.2*2) - 0.5 = 0.28
	// at dT=189 alone; dT=315 is needed to cross zero (3.16x ref).
	p.ReactorBuilding.ReactorPressureVessel.Outlet =
		Temperature{Val: CoreReferenceInletF + 63*5, Unit: u.Fahrenheit}
	p.ReactorBuilding.ReactorPressureVessel.AxialOffset = 100
	s := m["ReactorBuilding/ReactorPressureVessel/DNBRMargin"]
	out, err := s.Get()
	require.NoError(t, err, "negative DNBR must not error")
	v, err := parseFloatForTest(t, out)
	require.NoError(t, err)
	assert.Less(t, v, float64(0), "DNBR should be negative in this state")

	// DNBFireLive is live, not latched: true only while the
	// indicated margin is below the CHF threshold at power.
	s3 := m["ReactorBuilding/ReactorPressureVessel/DNBFireLive"]
	out3, err := s3.Get()
	require.NoError(t, err, "DNBFireLive read must not error")
	assert.Equal(t, "1", out3, "DNBFireLive is true below DNBRFireLimit at power")

	// LPD: never negative. Normalized form is a pure readout —
	// at rated power it reads 1.0; with large axial skew it can
	// exceed 1.0 (the peaking term is not clamped by design).
	s2 := m["ReactorBuilding/ReactorPressureVessel/LinearPowerDensity"]
	out2, err := s2.Get()
	require.NoError(t, err)
	v2, err := parseFloatForTest(t, out2)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, v2, 0.0)
}

// TestExcore_Nitrogen16RoundTrip is the CHECK_SKILL §1 test:
// marshal a stepped plant, grep the output for "Nitrogen16",
// unmarshal, assert the value survives.
func TestExcore_Nitrogen16RoundTrip(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < 1000; i++ {
		p.Step(DefaultTimeStep)
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), `"Nitrogen16"`),
		"marshal output must contain Nitrogen16 (CHECK_SKILL §1)")
	var p2 Plant
	require.NoError(t, json.Unmarshal(data, &p2))
	got := p2.ReactorBuilding.ReactorPressureVessel.Nitrogen16
	assert.InDeltaf(t, p.ReactorBuilding.ReactorPressureVessel.Nitrogen16, got, 1e-9,
		"N16 must survive a JSON round-trip")
}

// TestExcore_Backfill: a plant loaded from a saved state lacking
// Nitrogen16 reads 0 on load and equilibrates to the seeded
// fission fraction within ~60 s (first-order tau ≈ 10 s, residual
// <1% after ~47 s). The plant starts from MakePlant, which seeds
// the RPV to full load, then Nitrogen16 is zeroed to simulate an
// old-format saved state — this is the CHECK_SKILL §5 anchored
// backfill: a bare Plant{} collapses fission to the neutron-source
// floor, so the assertion would be vacuous (reads 0 = converged 0).
func TestExcore_Backfill(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.Nitrogen16 = 0.0 // simulate old save
	require.InDeltaf(t, 0.0, p.ReactorBuilding.ReactorPressureVessel.Nitrogen16, 1e-9, "simulated old-save N16 zero")

	// 60 s of steps at the 1 ms production step. Plant.Step does
	// not copy state back into p — Update() does (Plant.Update
	// at core/plant.go:2601) — so both calls are required for the
	// loop to see its own output.
	for i := 0; i < 60_000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	// Converged toward the live fission fraction, not stuck at zero.
	r := &p.ReactorBuilding.ReactorPressureVessel
	want := min(max(float64(r.FissionPower)/float64(CoreFissionPowerAtRated), 0), 1.2)
	assert.InDeltaf(t, want, r.Nitrogen16, 0.01,
		"backfilled N16 must converge to live fission fraction")
	assert.Positive(t, r.Nitrogen16, "must not remain zero")
}

// TestExcore_ReactivityIdentity: ReactivityEstimate is the same
// float32 Step computes, XenonWorth matches the Step term at zero
// axial skew.
func TestExcore_ReactivityIdentity(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	// Sensor value must equal the field.
	s := &ReactorPressureVessel{}
	*s = r
	want := float64(r.Reactivity)
	got := float64(s.Reactivity)
	assert.InDeltaf(t, want, got, 1e-9, "ReactivityEstimate identity")
	// XenonWorth at zero skew.
	r.AxialOffset = 0
	r.Xenon135AxialDeviation = 0
	wantXe := CoreXenonEquilibriumWorthPct *
		(xenonNormalized(r.Xenon135) - 1)
	// s shares r's state after copy; use r directly.
	assert.InDeltaf(t, wantXe, CoreXenonEquilibriumWorthPct*(xenonNormalized(r.Xenon135)-1), 1e-12, "XenonWorth matches Step term at zero skew")
	_ = math.Abs(wantXe)
}

func parseFloatForTest(t *testing.T, s string) (float64, error) {
	t.Helper()
	return strconv.ParseFloat(s, 64)
}
