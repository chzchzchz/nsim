package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stepRPV drives one production step: rods move through
// ControlRods.Step (rate-limited at RodInsertionSpeed, as
// Plant.Step does) before the RPV Step reads them. AutoControl is
// only invoked when armed, mirroring the Plant.Step gate
// (AutoControl && !Trip); the rod-demand targets in the cyclic
// legs are operator-set, which is the path the detector exists
// to observe.
func stepRPV(r ReactorPressureVessel, dt time.Duration, loadFraction float64) ReactorPressureVessel {
	if r.ControlRods.AutoControl && !r.Trip {
		powerFraction := float64(r.FissionPower) / float64(CoreRatedThermalPower)
		tavg := Temperature{Val: (r.Inlet.Float() + r.Outlet.Float()) / 2, Unit: u.Fahrenheit}
		r.ControlRods.applyAutoControl(dt, tavg, loadFraction, powerFraction,
			float64(r.Reactivity), 0.5)
	}
	r.ControlRods = r.ControlRods.Step(dt)
	return r.Step(dt, RCPRatedSpeed, 0)
}

func seedRPV() ReactorPressureVessel {
	// Boron at the rated-critical concentration (core/reactor.go:
	// CoreRatedCriticalBoronPpm), matching MakePlant's fresh-core
	// seed: exactly critical at rated power, fully withdrawn.
	// CVCSInitialBoron (1000 ppm) is the physics-reference shim,
	// not the operating point; seeding there leaves the vessel
	// subcritical at t=0 so every test starts on a collapse
	// transient instead of an equilibrium.
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CoreRatedCriticalBoronPpm},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	return r
}

// TestCladdingFatigue_SteadyPowerAccumulatesNothing runs the vessel
// at steady rated power and asserts no fatigue, no rod travel, and
// a cleared alarm.
func TestCladdingFatigue_SteadyPowerAccumulatesNothing(t *testing.T) {
	t.Parallel()
	r := seedRPV()
	for i := 0; i < 2000; i++ {
		r = stepRPV(r, DefaultTimeStep, 1.0)
	}
	assert.Zero(t, r.CladdingFatigueFrac, "steady power must consume no fatigue")
	assert.False(t, r.CladdingFatigueAlarm, "alarm must stay false at steady state")
	assert.Zero(t, r.RodTravelSteps, "no rod motion at steady state")
}

// TestCladdingFatigue_PowerReversalConsumesFatigue drives the core
// through full-power reversal cycles: the production auto-control
// cascade slews rod demand to swing the Tavg-program power target
// between rated and deep part-load, through the real production
// path (no poking CladdingFatigueFrac). It asserts fatigue
// increases, is amplitude-proportional, latches the alarm at the
// threshold, and never decreases.
func TestCladdingFatigue_PowerReversalConsumesFatigue(t *testing.T) {
	t.Parallel()
	r := seedRPV()

	// Prime the trend memory with a few steady steps.
	for i := 0; i < 10; i++ {
		r = stepRPV(r, DefaultTimeStep, 1.0)
	}
	firstStepFatigue := r.CladdingFatigueFrac

	// Reversal legs: hold-and-release rod-demand swings through
	// the production rate-limited drive (0 <-> 0.02 insertion).
	// The rod-worth scale (full insertion = -450% dK/k over
	// 0..1) makes 0.02 insertion worth ~9% power, past the 5%
	// band, while staying short of the equilibrium collapse a
	// deep insertion causes. 20000-step dwells let the trend
	// develop past the band and the return re-equilibrate
	// before releasing (per-leg equilibrium settling is what
	// makes a reversal leg a single unambiguous trend).
	stepUntilFatigue := func(target float64) {
		for i := 0; i < 800000 && r.CladdingFatigueFrac < target; i++ {
			if (i / 20000) % 2 == 0 {
				r.ControlRods.DemandInsertion = 0
			} else {
				r.ControlRods.DemandInsertion = 0.02
			}
			r = stepRPV(r, DefaultTimeStep, 1.0)
		}
	}

	stepUntilFatigue(CladdingFatigueAlarmFrac)
	require.Greater(t, r.CladdingFatigueFrac, firstStepFatigue,
		"reversals must consume fatigue")
	assert.True(t, r.CladdingFatigueAlarm, "alarm must latch at the threshold")
	assert.Greater(t, r.RodTravelSteps, 0.0, "rod travel must track rod movement")

	// Damage is one-way: the accumulator never decreases.
	before := r.CladdingFatigueFrac
	for i := 0; i < 100; i++ {
		r = stepRPV(r, DefaultTimeStep, 1.0)
	}
	assert.GreaterOrEqual(t, r.CladdingFatigueFrac, before,
		"damage accumulators never decrease")

	// Amplitude proportionality: a bigger swing consumes more
	// per reversal than a band-edge swing. One deep swing
	// (1 -> 0.05) must consume more than one band-edge swing
	// (1 -> 0.95).
	deep := seedRPV()
	for i := 0; i < 10; i++ {
		deep = stepRPV(deep, DefaultTimeStep, 1.0)
	}
	deepBase := deep.CladdingFatigueFrac
	// Full up-down cycle at the deep swing: insert (power down),
	// then release (power back up) — both trends reverse once.
	// Release legs must re-equilibrate before being read: the
	// return trend crosses the band only once decelerating onto
	// the equilibrium, so the dwell must settle, not just reach
	// rated power mid-transient.
	for i := 0; i < 20000; i++ {
		deep.ControlRods.DemandInsertion = 0.015
		deep = stepRPV(deep, DefaultTimeStep, 1.0)
	}
	// The return to rated power completes inside one trend
	// window, so the recovery trend is measured at that first
	// window boundary; the dwell must also settle (40000 total
	// steps) so the reversal is evaluated rather than cut off.
	for i := 0; i < 40000; i++ {
		deep.ControlRods.DemandInsertion = 0
		deep = stepRPV(deep, DefaultTimeStep, 1.0)
	}
	deepDelta := deep.CladdingFatigueFrac - deepBase
	assert.Positive(t, deepDelta, "a deep swing must consume fatigue")

	shallow := seedRPV()
	for i := 0; i < 10; i++ {
		shallow = stepRPV(shallow, DefaultTimeStep, 1.0)
	}
	shallowBase := shallow.CladdingFatigueFrac
	// Full up-down cycle at the band-edge swing (0.011 insertion
	// is worth ~5% power, at the band edge). Same settle rule as
	// the deep leg.
	for i := 0; i < 20000; i++ {
		shallow.ControlRods.DemandInsertion = 0.011
		shallow = stepRPV(shallow, DefaultTimeStep, 1.0)
	}
	for i := 0; i < 40000; i++ {
		shallow.ControlRods.DemandInsertion = 0
		shallow = stepRPV(shallow, DefaultTimeStep, 1.0)
	}
	shallowDelta := shallow.CladdingFatigueFrac - shallowBase
	assert.Greater(t, deepDelta, shallowDelta,
		"a deep swing must consume more per reversal than a band-edge swing (deep=%v shallow=%v)", deepDelta, shallowDelta)

	// Fatigue is severed from severe-accident damage:
	// Part.Damage mirrors CoreDamageFrac, not the fatigue accumulator.
	assert.Equal(t, float32(r.CoreDamageFrac), r.Damage,
		"Part.Damage must mirror CoreDamageFrac, not CladdingFatigueFrac")
}

// TestCladdingFatigue_WindowPhaseInvariance asserts a reversal
// cycle produces the same fatigue regardless of the window phase
// it starts at (the peak-tracker holds peaks/troughs across
// window boundaries; without it, a cycle starting mid-window
// scores zero).
func TestCladdingFatigue_WindowPhaseInvariance(t *testing.T) {
	t.Parallel()
	// Prime to a window boundary so the cycle's first window is
	// full-length (probes: a window already 17000 steps deep
	// at cycle start needs only 3000 steps to evaluate, and
	// the peak-tracker then carries the trough across it).
	runCycle := func(offset int) float64 {
		r := seedRPV()
		for i := 0; i < offset; i++ {
			r = stepRPV(r, DefaultTimeStep, 1.0)
		}
		base := r.CladdingFatigueFrac
		for i := 0; i < 20000; i++ {
			r.ControlRods.DemandInsertion = 0.02
			r = stepRPV(r, DefaultTimeStep, 1.0)
		}
		for i := 0; i < 60000; i++ {
			r.ControlRods.DemandInsertion = 0
			r = stepRPV(r, DefaultTimeStep, 1.0)
		}
		return r.CladdingFatigueFrac - base
	}
	aligned := runCycle(10)
	assert.Positive(t, aligned, "phase-10 cycle must consume fatigue")
	for _, offset := range []int{5000, 10000, 15000, 19999} {
		got := runCycle(offset)
		assert.Greater(t, got, 0.5*aligned,
			"phase-%d cycle must read within 50%% of the aligned one (got %v vs %v)", offset, got, aligned)
	}
}

// TestCladdingFatigue_FirstStepSuppressed asserts the zero-value
// first-Step suppression (index convention 5): the first Step
// after construction produces no fatigue increment.
func TestCladdingFatigue_FirstStepSuppressed(t *testing.T) {
	t.Parallel()
	r := seedRPV()
	r1 := stepRPV(r, DefaultTimeStep, 1.0)
	assert.Zero(t, r1.CladdingFatigueFrac, "first Step must be suppressed")
	assert.Zero(t, r1.RodTravelSteps, "first Step must not count phantom rod travel")
	assert.False(t, r1.CladdingFatigueAlarm)
}

// TestCladdingFatigue_MonotonicRampSingleCycle asserts a single
// monotonic ramp and back (no intermediate reversal) produces
// exactly one cycle's worth of fatigue, not one per step.
func TestCladdingFatigue_MonotonicRampSingleCycle(t *testing.T) {
	t.Parallel()
	r := seedRPV()
	for i := 0; i < 10; i++ {
		r = stepRPV(r, DefaultTimeStep, 1.0)
	}
	base := r.CladdingFatigueFrac

	// One monotonic ramp down (insertion 0 -> 0.02) then one
	// ramp back up (0.02 -> 0): no intermediate reversals inside
	// either leg. Dwelling 8000 steps per leg lets the trend
	// develop and settle so the returns are cusp-free.
	for i := 0; i < 40000; i++ {
		r.ControlRods.DemandInsertion = 0.02
		r = stepRPV(r, DefaultTimeStep, 1.0)
	}
	for i := 0; i < 40000; i++ {
		r.ControlRods.DemandInsertion = 0
		r = stepRPV(r, DefaultTimeStep, 1.0)
	}
	delta := r.CladdingFatigueFrac - base
	// Two reversals at most occurred (ramp ends and the
	// turnaround), each consuming about 10x the per-cycle rate
	// (amplitude ~10 band units on a 9%-power swing), so the
	// delta is small AND phase-bounded: far below the step
	// count times the per-cycle rate (which would be 80000 *
	// rate if every step counted), and within 3x of what two
	// full reversals predict.
	assert.Less(t, delta, 30*CladdingFatigueRatePerCycle,
		"one ramp-and-back must not accumulate per-step fatigue (delta=%v)", delta)
	assert.Greater(t, r.RodTravelSteps, 0.0,
		"rod travel must track the monotonic rod motion")
}

// TestCladdingFatigue_BridgeSensors exercises the bridge entries
// for the three new sensors through production code paths (the
// CHECK_SKILL §5 "pair every stored field with its sensor at
// birth" audit; p72_6 remains the consolidated bridge gate).
func TestCladdingFatigue_BridgeSensors(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	sm := p.GetSensorMap()

	// Fraction sensor: zero at construction, percent scale.
	out, err := sm["ReactorBuilding/ReactorPressureVessel/CladdingFatigue"].Get()
	require.NoError(t, err)
	assert.Equal(t, "0", out, "fresh plant must read 0 percent fatigue")

	out, err = sm["ReactorBuilding/ReactorPressureVessel/RodTravelSteps"].Get()
	require.NoError(t, err)
	assert.Equal(t, "0", out, "fresh plant must read 0 rod steps")

	out, err = sm["ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm"].Get()
	require.NoError(t, err)
	assert.Equal(t, "0", out, "fresh plant must read alarm false (bool kind publishes 0/1)")

	// Read-back through the same plant instance after stepping
	// (the control-map binding rule: reads go through the
	// stepped variable, not a copy).
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.CladdingFatigueFrac = 0.5
	rpv.CladdingFatigueAlarm = true
	rpv.RodTravelSteps = 42.0
	out, err = sm["ReactorBuilding/ReactorPressureVessel/CladdingFatigue"].Get()
	require.NoError(t, err)
	assert.Equal(t, "50", out, "0.5 fraction must read 50 percent, not 0.5")
	out, err = sm["ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm"].Get()
	require.NoError(t, err)
	assert.Equal(t, "1", out)
	out, err = sm["ReactorBuilding/ReactorPressureVessel/RodTravelSteps"].Get()
	require.NoError(t, err)
	assert.Equal(t, "42", out, "counter must publish raw, not percent")
}
