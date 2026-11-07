package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// One coarse CCWS.Step at 1 s must reproduce one thousand default
// 1 ms steps through production code: CCWS relaxes on a closed-form
// first-order lag, so equal simulated time must land on equal
// temperatures regardless of step width.
func TestCoarseStepMatchesFineStepping(t *testing.T) {
	t.Parallel()
	start := Temperature{Val: 120, Unit: u.Fahrenheit}
	supply := Temperature{Val: 70, Unit: u.Fahrenheit}

	fine := CCWS{PumpRunning: true, DriveReady: true, InletTemp: start}
	for i := 0; i < 1000; i++ {
		fine = fine.Step(DefaultTimeStep, supply, true)
	}

	coarse := CCWS{PumpRunning: true, DriveReady: true, InletTemp: start}
	coarse = coarse.Step(time.Second, supply, true)

	assert.InDelta(t, fine.InletTemp.Float(), coarse.InletTemp.Float(), 1e-9,
		"equal simulated time must reach the same inlet temperature")
	assert.Equal(t, fine.HXHeatDuty, coarse.HXHeatDuty,
		"equal simulated time must produce the same HX duty")
}

// The anchored fission compounding makes trajectories a function of
// simulated time alone: five simulated seconds of a boron-driven fold
// must land on the same power whether stepped at the default tick or
// in fifty 100 ms chunks. This is the property that lets long-span
// tests widen the tick without changing what they observe.
func TestReactorFissionPower_WidthInvariance(t *testing.T) {
	t.Parallel()
	run := func(dt time.Duration, steps int) float64 {
		var rpv ReactorPressureVessel
		rpv.SeedSteadyStatePower(CoreRatedThermalPower)
		rpv.ControlRods.Insertion = 0
		rpv.ControlRods.BoronConcentration = CVCSInitialBoron + 200
		for i := 0; i < steps; i++ {
			rpv = rpv.Step(dt, RCPRatedSpeed, 0)
		}
		return float64(rpv.FissionPower)
	}

	fine := run(DefaultTimeStep, 5000) // five seconds at the default tick
	coarse := run(100*time.Millisecond, 50)

	assert.InDelta(t, fine, coarse, fine*1e-9,
		"equal simulated time must reach equal fission power at any step width")
}
