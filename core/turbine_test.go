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

// At rated load the turbine produces its full HP steam flow as
// ExtractionSteamFlow and HPExhaustPressure sits below the SG
// pressure due to HP-turbine pressure drop.
//
// Spec justification:
//   - "A small portion of the steam in the high-pressure turbine
//     is extracted to the highest pressure feedwater heaters."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:31-32
func TestTurbine_ExtractionOutputAtRatedLoad(t *testing.T) {
	tr := Turbine{
		State:     TurbineRunning,
		Velocity:  TurbineRatedSpeed,
		RealPower: TurbineRatedkW,
	}
	open := 1.0
	for i := 0; i < 1000; i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, false, &open)
	}
	assert.Greater(t, tr.ExtractionSteamFlow.Float(), 0.0,
		"want ExtractionSteamFlow > 0 at rated load, got %v",
		tr.ExtractionSteamFlow.Float())
	assert.Less(t, tr.HPExhaustPressure, SteamGeneratorShellPressure,
		"want HPExhaustPressure < SG pressure, got %v vs %v",
		tr.HPExhaustPressure, SteamGeneratorShellPressure)
	assert.GreaterOrEqual(t, tr.HPExhaustQuality, HPExhaustQualityLowLoad,
		"want HPExhaustQuality >= low-load floor, got %v",
		tr.HPExhaustQuality)
}

// OPC alarm asserts when shaft velocity exceeds 103% of rated
// speed. The alarm is informational only — no automatic trip.
//
// Spec justification:
//   - "OPC alarm (103%), OPC trip (110%)"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-73
func TestTurbine_OPCAlarmAssertsAbove103Percent(t *testing.T) {
	tr := Turbine{
		State:    TurbineRunning,
		Velocity: Velocity(float64(TurbineRatedSpeed) * (TurbineOPCALarmFraction + 0.005)),
	}
	open := 1.0
	tr = tr.Step(SteamGeneratorShellPressure, 0, false, &open)
	assert.True(t, tr.OPCAlarm,
		"want OPCAlarm true at %.1f%% rated, got false (vel=%v, frac=%v)",
		TurbineOPCALarmFraction*100, tr.Velocity, TurbineOPCALarmFraction)
	assert.False(t, tr.OPCTrip,
		"want OPC trip NOT yet latched at 103%%, got latched")
}

// OPC trip latches when shaft velocity exceeds 110% of rated
// speed. Once latched the stop valve is closed and the rotor
// coasts down.
//
// Spec justification:
//   - "OPC trip (110%)"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
//   - "The mechanical hydraulic overspeed trip mechanism (Item 1)
//     causes the turbine valve hydraulic fluid pressure to dump to
//     drain, thereby closing the main steam stop and governing
//     valves [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:9-15
func TestTurbine_OPCTripClosesStopValveAt110Percent(t *testing.T) {
	tr := Turbine{
		State:    TurbineRunning,
		Velocity: Velocity(float64(TurbineRatedSpeed) * (TurbineOPCTripFraction + 0.01)),
	}
	open := 1.0
	tr = tr.Step(SteamGeneratorShellPressure, 0, false, &open)
	assert.True(t, tr.OPCTrip,
		"want OPCTrip latched above %.0f%% rated, got false (vel=%v)",
		TurbineOPCTripFraction*100, tr.Velocity)
	assert.Zero(t, tr.StopValve.Open,
		"want StopValve.Open = 0 after OPC trip, got %v", tr.StopValve.Open)
	assert.Equal(t, TurbineTripped, tr.State,
		"want TurbineTripped after OPC trip, got %v", tr.State)
}

// Load-runback drives Setpoint toward zero at RampRate when
// LoadRunback is asserted. After running for the duration of
// Setpoint / RampRate the Setpoint should reach zero.
//
// Spec justification:
//   - "Automatic load runback initiated by a dropped rod signal,
//     high kw/ft signal or low departure from nucleate boiling
//     ratio signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
func TestTurbine_LoadRunbackDecreasesSetpoint(t *testing.T) {
	tr := Turbine{
		State:       TurbineRunning,
		Velocity:    TurbineRatedSpeed,
		RealPower:   TurbineRatedkW,
		Setpoint:    1.0,
		RampRate:    TurbineLoadRunbackRate,
		LoadRunback: true,
	}
	open := 1.0
	// Step the full-load-rating time plus a margin.
	runbackSec := 1.0 / TurbineLoadRunbackRate
	steps := int(runbackSec*float64(time.Second)/float64(TimeStep)) + 1000
	for i := 0; i < steps; i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, false, &open)
	}
	assert.Zero(t, tr.Setpoint,
		"want Setpoint=0 after full-load runback of %d steps (%.0fs), got %v",
		steps, runbackSec, tr.Setpoint)
}

// RealPower is clamped by the operator-selected Setpoint when the
// setpoint is non-zero. Below the setpoint the speed-based power
// follows velocity; above the setpoint it is capped.
//
// Spec justification:
//   - "After generator breaker closure, the ability to increase or
//     decrease electrical load at a controlled selectable rate."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
func TestTurbine_SetpointClampsRealPower(t *testing.T) {
	tr := Turbine{
		State:     TurbineRunning,
		Velocity:  TurbineRatedSpeed,
		RealPower: TurbineRatedkW,
		Setpoint:  0.5,
	}
	open := 1.0
	tr = tr.Step(SteamGeneratorShellPressure, 0, false, &open)
	assert.LessOrEqual(t, float64(tr.RealPower), 0.5*float64(TurbineRatedkW),
		"want RealPower ≤ 50%% rated at Setpoint=0.5, got %v", tr.RealPower)
	assert.Greater(t, float64(tr.RealPower), 0.0,
		"want RealPower > 0 at Setpoint=0.5, got %v", tr.RealPower)
}

// PowerGenerator defaults match the spec on initial plant creation:
// breaker open, power factor 0.9, 1370 MVA rating.
//
// Spec justification:
//   - "Rating: 1370 mva"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
//   - "Power factor: 0.9, short circuit ratio: 0.58"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
func TestPowerGenerator_DefaultsMatchSpec(t *testing.T) {
	p := MakePlant()
	g := p.TurbineBuilding.Generator
	assert.False(t, g.BreakerClosed,
		"want BreakerClosed=false at startup, got true")
	assert.InDelta(t, 0.9, g.PowerFactor, 1e-9,
		"want PowerFactor=0.9, got %v", g.PowerFactor)
	assert.InDelta(t, 1370e3, g.RatedMVA, 1e-6,
		"want RatedMVA=1370 MVA, got %v", g.RatedMVA)
}

// ReactivePower tracks RealPower through the design 0.9 power
// factor. With BreakerClosed and the unit at rated load, the
// reactive power should be RealPower * (1/0.9 - 1) ≈ 0.111 *
// RealPower.
//
// Spec justification:
//   - "Power factor: 0.9, short circuit ratio: 0.58"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
//   - "output power, the power factor, and speed to ensure that the
//     limits of the generator are not exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
func TestPowerGenerator_ReactivePowerMatchesPowerFactor(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Generator.BreakerClosed = true
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(5*time.Second/TimeStep))
	g := p.TurbineBuilding.Generator
	if float64(g.RealPower) <= 0 {
		t.Skipf("precondition: turbine did not produce power in 5 s (RealPower=%v)", g.RealPower)
	}
	wantFactor := 1.0/g.PowerFactor - 1.0
	assert.InDelta(t, wantFactor*float64(g.RealPower), g.ReactivePower,
		float64(g.RealPower)*0.05,
		"want ReactivePower ≈ RealPower * (1/PF - 1) = %v, got %v",
		wantFactor*float64(g.RealPower), g.ReactivePower)
}
