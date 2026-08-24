package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// overrideTimeStep widens the package-wide simulation step to dt for
// the remainder of the calling test and restores DefaultTimeStep on
// cleanup.
//
// WARNING: use only when absolutely needed. A coarser step reduces
// simulation fidelity: trips, ramp limiters, and valve strokes
// calibrated for the default 1 ms tick clip or alias, so assertions
// touching those dynamics can be wrong even though the test passes.
// Reserve this for tests covering many hours of slowly evolving
// physics, and never call it from a test that uses t.Parallel: the
// override is global state shared by every concurrently running test.
func overrideTimeStep(tb testing.TB, dt time.Duration) {
	tb.Helper()
	if dt <= 0 {
		tb.Fatalf("overrideTimeStep requires dt > 0, got %v", dt)
	}
	tb.Logf("WARNING: TimeStep widened to %v for %s; this lowers simulation fidelity, use only when absolutely needed",
		dt, tb.Name())
	prev := TimeStep
	tb.Cleanup(func() { TimeStep = prev })
	TimeStep = dt
}

// One coarse CCWS.Step at 1 s must reproduce one thousand default
// 1 ms steps through production code: CCWS relaxes on a closed-form
// first-order lag, so equal simulated time must land on equal
// temperatures regardless of step width.
func TestOverrideTimeStep_CoarseStepMatchesFineStepping(t *testing.T) {
	start := Temperature{Val: 120, Unit: u.Fahrenheit}
	supply := Temperature{Val: 70, Unit: u.Fahrenheit}

	fine := CCWS{PumpRunning: true, DriveReady: true, InletTemp: start}
	for i := 0; i < 1000; i++ {
		fine = fine.Step(supply, true)
	}

	overrideTimeStep(t, time.Second)
	coarse := CCWS{PumpRunning: true, DriveReady: true, InletTemp: start}
	coarse = coarse.Step(supply, true)

	assert.InDelta(t, fine.InletTemp.Float(), coarse.InletTemp.Float(), 1e-9,
		"equal simulated time must reach the same inlet temperature")
	assert.Equal(t, fine.HXHeatDuty, coarse.HXHeatDuty,
		"equal simulated time must produce the same HX duty")
}

func TestOverrideTimeStep_RestoresDefaultAfterTest(t *testing.T) {
	t.Cleanup(func() {
		assert.Equal(t, DefaultTimeStep, TimeStep,
			"the override must not leak past the calling test")
	})
	overrideTimeStep(t, 250*time.Millisecond)
	assert.Equal(t, 250*time.Millisecond, TimeStep)
}

// The anchored fission compounding makes trajectories a function of
// simulated time alone: five simulated seconds of a boron-driven fold
// must land on the same power whether stepped at the default tick or
// in fifty 100 ms chunks. This is the property that lets long-span
// tests widen the tick without changing what they observe.
func TestReactorFissionPower_WidthInvariance(t *testing.T) {
	run := func(steps int) float64 {
		var rpv ReactorPressureVessel
		rpv.SeedSteadyStatePower(CoreRatedThermalPower)
		rpv.ControlRods.Insertion = 0
		rpv.ControlRods.BoronConcentration = CVCSInitialBoron + 200
		for i := 0; i < steps; i++ {
			rpv = rpv.Step(RCPRatedSpeed, 0)
		}
		return float64(rpv.FissionPower)
	}

	fine := run(5000) // five seconds at the default tick
	overrideTimeStep(t, 100*time.Millisecond)
	coarse := run(50) // the same five seconds at 100 ms

	assert.InDelta(t, fine, coarse, fine*1e-9,
		"equal simulated time must reach equal fission power at any step width")
}
