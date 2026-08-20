package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openValve returns a stop-valve-open value of 1.0 as a *float64,
// matching the new Step signature.
func openValve() *float64 {
	v := 1.0
	return &v
}

// closedValve returns a stop-valve-open value of 0.0 as a *float64.
func closedValve() *float64 {
	v := 0.0
	return &v
}

func TestTurbine_SteamFlowStartsRunning(t *testing.T) {
	tr := Turbine{}
	for i := 0; i < 100; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	assert.Equal(t, TurbineRunning, tr.State, "want TurbineRunning")
	assert.Positive(t, tr.Velocity, "want Velocity > 0 after steam admission")
	assert.Positive(t, tr.RealPower, "want RealPower > 0")
}

func TestTurbine_ShortSteamLossStaysRunning(t *testing.T) {
	tr := Turbine{}
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	require.Equal(t, TurbineRunning, tr.State, "precondition: want Running")
	velBefore := tr.Velocity
	// Cut steam briefly — well under TurbineSteamLossDeadline. The
	// rotor coasts down but the turbine remains Running; once steam
	// returns the rotor accelerates again.
	for i := 0; i < int(100*time.Millisecond/TimeStep); i++ {
		tr = tr.Step(0, 0, false, openValve())
	}
	assert.Equal(t, TurbineRunning, tr.State,
		"want still Running after brief transient, got %v", tr.State)
	assert.Less(t, tr.Velocity, velBefore,
		"want Velocity to coast down, before %v after %v", velBefore, tr.Velocity)
	// Restore steam and confirm we accelerate back up.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	assert.GreaterOrEqual(t, tr.Velocity, velBefore,
		"want Velocity restored, before %v after %v", velBefore, tr.Velocity)
}

func TestTurbine_SustainedSteamLossTrips(t *testing.T) {
	tr := Turbine{}
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(0, 0, false, openValve())
	}
	assert.Equal(t, TurbineTripped, tr.State,
		"want Tripped after sustained loss of steam, got %v", tr.State)
}

func TestTurbine_OverspeedTrips(t *testing.T) {
	tr := Turbine{State: TurbineRunning, Velocity: TurbineRatedSpeed}
	// Pre-load the rotor past the trip threshold; the Step must latch
	// TurbineTripped on the very next tick.
	tr.Velocity = Velocity(float64(TurbineRatedSpeed) * TurbineOverspeedTripRatio * 1.5)
	tr = tr.Step(1000, 0, false, openValve())
	assert.Equal(t, TurbineTripped, tr.State,
		"want Tripped on overspeed, got %v (velocity=%v)", tr.State, tr.Velocity)
}

func TestTurbine_TrippedRequiresReset(t *testing.T) {
	tr := Turbine{}
	// Trip via sustained steam loss.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(0, 0, false, openValve())
	}
	require.Equal(t, TurbineTripped, tr.State, "precondition: want Tripped")
	// Steam returns but reset is still false — turbine must remain
	// tripped until an explicit reset clears it.
	for i := 0; i < 5000; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	assert.Equal(t, TurbineTripped, tr.State, "want Tripped until reset, got %v", tr.State)
	// Wait for Velocity to decay to 0 (coast ~1/3 s).
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		tr = tr.Step(0, 0, false, openValve())
	}
	for i := 0; i < 10; i++ {
		tr = tr.Step(0, 0, true, openValve())
	}
	assert.Equal(t, TurbineOff, tr.State, "want Off after reset, got %v", tr.State)
}

// With the stop valve closed, no steam admission occurs regardless
// of the upstream pressure; the turbine stays off.
func TestTurbine_StopValveClosedBlocksSteam(t *testing.T) {
	tr := Turbine{}
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, false, closedValve())
	}
	assert.Equal(t, TurbineOff, tr.State,
		"want Off with stop valve closed, got %v", tr.State)
	assert.Zero(t, tr.Velocity, "want Velocity=0 with stop valve closed, got %v", tr.Velocity)
}

// With the stop valve open and full upstream pressure the turbine
// reaches rated speed; closing the stop valve mid-run causes the
// rotor to coast down and the turbine eventually trips.
func TestTurbine_StopValveMidRunCoastsDown(t *testing.T) {
	tr := Turbine{}
	// Reach rated speed.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, false, openValve())
	}
	require.Equal(t, TurbineRunning, tr.State, "precondition: want Running")
	velBefore := tr.Velocity
	// Close the stop valve; no more steam admission. The rotor
	// coasts down. After TurbineSteamLossDeadline the turbine
	// trips.
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(1000, 0, false, closedValve())
	}
	assert.Less(t, tr.Velocity, velBefore,
		"want coast-down, before %v after %v", velBefore, tr.Velocity)
	assert.Equal(t, TurbineTripped, tr.State,
		"want Tripped after stop valve held closed, got %v", tr.State)
}

// Nil stopValveOpen pointer preserves the legacy admission path:
// steam flow is gated only by the upstream pressure differential.
func TestTurbine_NilStopValveAllowsSteam(t *testing.T) {
	tr := Turbine{}
	for i := 0; i < 100; i++ {
		tr = tr.Step(1000, 0, false, nil)
	}
	assert.Equal(t, TurbineRunning, tr.State,
		"want Running with nil stop valve pointer and steam, got %v", tr.State)
}
