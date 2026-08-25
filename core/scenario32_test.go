package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 32 (residual heat removal loop takeover and realignment):
// durable assertions for the RHR loop's spec sequence - mini-flow
// warmup before the suction valves open, takeover on first heat
// rejection to component cooling water, and the stopped-but-aligned
// reverse leg.
//
//   - "RHRS operation generally begins with a short period wherein the
//     pump is run on its miniflow circuit in order to warm the pump and
//     heat exchanger and avoid sudden thermal expansion"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:26-30
//   - "At this point the reactor cooling function has been taken over by
//     the RHRS"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:35-37
//   - "One reactor coolant pump is kept running to provide for uniform
//     cooldown of all loops" (lumped single-loop model: implicit)
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:38-40

func scenario32Train() RHRSTrain {
	return RHRSTrain{
		Pump:       Pump{Running: true},
		DriveReady: true,
	}
}

func scenario32Temps() (Temperature, Temperature) {
	hot := Temperature{Val: RHRSTempThreshold - 10, Unit: u.Fahrenheit}
	ccw := Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit}
	return hot, ccw
}

// Mini-flow circulation passes no HX duty until the warmup window has
// elapsed AND the suction path is open; either gate alone holds duty at
// zero while the pump keeps circulating.
func TestScenario32_MiniflowWarmupGatesHxDuty(t *testing.T) {
	overrideTimeStep(t, time.Second)
	hot, ccw := scenario32Temps()
	train := scenario32Train()

	warmupSteps := int(RHRSTrainWarmupTime / TimeStep)

	// First third with suctions shut: miniflow only, zero duty.
	for i := 0; i < warmupSteps/3; i++ {
		train = train.Step(hot, ccw, false)
	}
	require.False(t, train.WarmupComplete, "warmup must still be running")
	assert.Positive(t, train.ReturnFlow.Float(), "mini-flow circulates")
	assert.Zero(t, float64(train.HeatExchanger.HeatTransferRate),
		"a shut suction path rejects no heat")

	// Open the suctions before warmup completes: still zero duty.
	train = train.Step(hot, ccw, true)
	require.False(t, train.WarmupComplete)
	assert.Zero(t, float64(train.HeatExchanger.HeatTransferRate),
		"warmup completion gates the heat exchanger, not the suction")

	// Ride out the rest of the window with suctions open: the next
	// step after WarmupComplete carries full HX flow and real duty.
	for !train.WarmupComplete {
		train = train.Step(hot, ccw, true)
	}
	train = train.Step(hot, ccw, true)
	assert.Positive(t, float64(train.HeatExchanger.HeatTransferRate),
		"post-warmup with suctions open the HX must reject heat")
	assert.Positive(t, train.ReturnFlow.Float())
}

// The plant-level latch places the RHRS in operation at the entry line
// and marks takeover on the first step that actually rejects heat.
func TestScenario32_TakeoverLatchesOnFirstHeatRejection(t *testing.T) {
	overrideTimeStep(t, time.Second)
	hot, ccw := scenario32Temps()
	ready := func() RHRSTrain {
		return RHRSTrain{
			Pump:          Pump{Running: true},
			DriveReady:    true,
			SuctionValves: [2]Valve{{Open: 1}, {Open: 1}},
		}
	}
	r := RHRS{Running: true, TrainA: ready(), TrainB: scenario32Train()}

	// Suctions open but the mini-flow warmup has not run: running
	// without any heat rejection yet.
	r = r.Step(hot, RHRSPressureThreshold-1, ccw)
	require.True(t, r.Running)
	assert.False(t, r.HasTakenOver,
		"no takeover before any heat reaches component cooling water")

	for range int(RHRSTrainWarmupTime/time.Second) + 1 {
		r = r.Step(hot, RHRSPressureThreshold-1, ccw)
		if r.HasTakenOver {
			break
		}
	}
	require.True(t, r.TrainA.WarmupComplete, "warmup must have elapsed")
	assert.True(t, r.HasTakenOver,
		"takeover must latch on the first nonzero heat rejection")
	assert.Positive(t, float64(r.TrainA.HeatExchanger.HeatTransferRate))
}

// Reverse leg, stopped-but-aligned: securing the pump zeroes return
// flow and duty while the operator leaves the suction path open for
// the later bubble-formation and isolation steps.
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_087.txt:46-48
func TestScenario32_StoppedButAlignedKeepsPathOpen(t *testing.T) {
	overrideTimeStep(t, time.Second)
	hot, ccw := scenario32Temps()
	train := scenario32Train()
	train.WarmupComplete = true

	train = train.Step(hot, ccw, true)
	require.Positive(t, float64(train.HeatExchanger.HeatTransferRate),
		"precondition: aligned train is rejecting heat")

	train.Pump.Running = false
	train = train.Step(hot, ccw, true)
	assert.Zero(t, train.ReturnFlow.Float(),
		"secured pump stops the return flow")
	assert.Zero(t, float64(train.HeatExchanger.HeatTransferRate),
		"secured pump stops the heat rejection")
	assert.False(t, train.Pump.Running,
		"the aligned-but-stopped lineup keeps the operator state")
}
