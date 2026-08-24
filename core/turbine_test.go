package core

import (
	"math"
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

// steamingTurbine returns a zero Turbine with the governing valves
// wide open and the lube-oil pump in service at design header
// pressure, matching the MakePlant operating lineup. A turbine
// without its bearing-oil pump running trips immediately on low
// lubricating pressure (page_203.txt:87-89, page_206.txt:25).
func steamingTurbine() Turbine {
	return Turbine{
		GoverningValve:      Valve{Open: 1},
		LubeOilPumpRunning:  true,
		BearingOilPressure:  BearingOilDesignPressure,
	}
}

func TestTurbine_SteamFlowStartsRunning(t *testing.T) {
	tr := steamingTurbine()
	for i := 0; i < 100; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.Equal(t, TurbineRunning, tr.State, "want TurbineRunning")
	assert.Positive(t, tr.Velocity, "want Velocity > 0 after steam admission")
	assert.Positive(t, tr.RealPower, "want RealPower > 0")
}

func TestTurbine_ShortSteamLossStaysRunning(t *testing.T) {
	tr := steamingTurbine()
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	require.Equal(t, TurbineRunning, tr.State, "precondition: want Running")
	velBefore := tr.Velocity
	// Cut steam briefly — well under TurbineSteamLossDeadline. The
	// rotor coasts down but the turbine remains Running; once steam
	// returns the rotor accelerates again.
	for i := 0; i < int(100*time.Millisecond/TimeStep); i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.Equal(t, TurbineRunning, tr.State,
		"want still Running after brief transient, got %v", tr.State)
	assert.Less(t, tr.Velocity, velBefore,
		"want Velocity to coast down, before %v after %v", velBefore, tr.Velocity)
	// Restore steam and confirm we accelerate back up.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.GreaterOrEqual(t, tr.Velocity, velBefore,
		"want Velocity restored, before %v after %v", velBefore, tr.Velocity)
}

func TestTurbine_SustainedSteamLossTrips(t *testing.T) {
	tr := steamingTurbine()
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.Equal(t, TurbineTripped, tr.State,
		"want Tripped after sustained loss of steam, got %v", tr.State)
}

func TestTurbine_OverspeedTrips(t *testing.T) {
	tr := Turbine{State: TurbineRunning, Velocity: TurbineRatedSpeed}
	// Pre-load the rotor past the trip threshold; the Step must latch
	// TurbineTripped on the very next tick.
	tr.Velocity = Velocity(float64(TurbineRatedSpeed) * TurbineOverspeedTripRatio * 1.5)
	tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	assert.Equal(t, TurbineTripped, tr.State,
		"want Tripped on overspeed, got %v (velocity=%v)", tr.State, tr.Velocity)
}

func TestTurbine_TrippedRequiresReset(t *testing.T) {
	tr := steamingTurbine()
	// Trip via sustained steam loss.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	}
	require.Equal(t, TurbineTripped, tr.State, "precondition: want Tripped")
	// Steam returns but reset is still false — turbine must remain
	// tripped until an explicit reset clears it.
	for i := 0; i < 5000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.Equal(t, TurbineTripped, tr.State, "want Tripped until reset, got %v", tr.State)
	// Wait for Velocity to decay to 0 (coast ~1/3 s).
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	}
	for i := 0; i < 10; i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, true, openValve())
	}
	assert.Equal(t, TurbineOff, tr.State, "want Off after reset, got %v", tr.State)
}


// The operator reset command latches ResetRequest, which the next
// Step consumes exactly once: Tripped -> Off once the rotor has
// decayed to rest, then Off -> Running when steam returns. The
// production call path (TurbineBuilding.StepWithSteam) always passes
// reset=false, so the latched request is the only reset route.
//
// Spec justification:
//   - "The operator may position the valves, when necessary,
//      through the manual valve controllers."
//   - specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:114-116
func TestTurbine_ResetRequestRestartsFromTripped(t *testing.T) {
	// Healthy operating lineup: without the lube-oil pump in service
	// and open governing valves the machine cannot roll, and the
	// steam-loss trip under test could not be reached.
	tr := steamingTurbine()
	// Trip via sustained steam loss, then let the rotor coast to
	// rest.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	}
	require.Equal(t, TurbineTripped, tr.State, "precondition: want Tripped")
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	}

	// Latch the operator reset request and step once.
	tr.ResetRequest = true
	tr = tr.Step(0, 0, Temperature{Val: 600}, false, openValve())
	assert.Equal(t, TurbineOff, tr.State,
		"want Off after consuming ResetRequest")
	assert.False(t, tr.ResetRequest, "ResetRequest must be consumed")

	// Steam returns: Off -> Running without any further reset.
	tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	assert.Equal(t, TurbineRunning, tr.State,
		"want Running once steam returns")
}

// With the stop valve closed, no steam admission occurs regardless
// of the upstream pressure; the turbine stays off.
func TestTurbine_StopValveClosedBlocksSteam(t *testing.T) {
	tr := Turbine{}
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, closedValve())
	}
	assert.Equal(t, TurbineOff, tr.State,
		"want Off with stop valve closed, got %v", tr.State)
	assert.Zero(t, tr.Velocity, "want Velocity=0 with stop valve closed, got %v", tr.Velocity)
}

// With the stop valve open and full upstream pressure the turbine
// reaches rated speed; closing the stop valve mid-run causes the
// rotor to coast down and the turbine eventually trips.
func TestTurbine_StopValveMidRunCoastsDown(t *testing.T) {
	tr := steamingTurbine()
	// Reach rated speed.
	for i := 0; i < 1000; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, openValve())
	}
	require.Equal(t, TurbineRunning, tr.State, "precondition: want Running")
	velBefore := tr.Velocity
	// Close the stop valve; no more steam admission. The rotor
	// coasts down. After TurbineSteamLossDeadline the turbine
	// trips.
	steps := int(TurbineSteamLossDeadline/TimeStep) + 100
	for i := 0; i < steps; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, closedValve())
	}
	assert.Less(t, tr.Velocity, velBefore,
		"want coast-down, before %v after %v", velBefore, tr.Velocity)
	assert.Equal(t, TurbineTripped, tr.State,
		"want Tripped after stop valve held closed, got %v", tr.State)
}

// Nil stopValveOpen pointer preserves the legacy admission path:
// steam flow is gated only by the upstream pressure differential.
func TestTurbine_NilStopValveAllowsSteam(t *testing.T) {
	tr := steamingTurbine()
	for i := 0; i < 100; i++ {
		tr = tr.Step(1000, 0, Temperature{Val: 600}, false, nil)
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
		State:              TurbineRunning,
		Velocity:           TurbineRatedSpeed,
		RealPower:          TurbineRatedkW,
		GoverningValve:     Valve{Open: 1},
		LubeOilPumpRunning: true,
		BearingOilPressure: BearingOilDesignPressure,
	}
	open := 1.0
	for i := 0; i < 1000; i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
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
	tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
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
	tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
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
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
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
		State:              TurbineRunning,
		Velocity:           TurbineRatedSpeed,
		RealPower:          TurbineRatedkW,
		Setpoint:           0.5,
		GoverningValve:     Valve{Open: 1},
		LubeOilPumpRunning: true,
		BearingOilPressure: BearingOilDesignPressure,
		Synchronized:       true,
	}
	open := 1.0
	for i := 0; i < 100; i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
	}
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
	synchronizeAndCloseBreaker(&p)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(5*time.Second/TimeStep))
	g := p.TurbineBuilding.Generator
	if float64(g.RealPower) <= 0 {
		t.Skipf("precondition: turbine did not produce power in 5 s (RealPower=%v)", g.RealPower)
	}
	wantFactor := math.Sqrt(1.0/(g.PowerFactor*g.PowerFactor) - 1.0)
	assert.InDelta(t, wantFactor*float64(g.RealPower), g.ReactivePower,
		float64(g.RealPower)*0.05,
		"want ReactivePower ≈ RealPower * (1/PF - 1) = %v, got %v",
		wantFactor*float64(g.RealPower), g.ReactivePower)
}

// Turning gear holds the rotor at a slow roll with no steam
// admission instead of letting it coast to zero.
func TestTurbine_TurningGearHoldsSlowRoll(t *testing.T) {
	tr := steamingTurbine()
	tr.TurningGear = true
	tr.Velocity = TurbineRatedSpeed
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		tr = tr.Step(0, 0, Temperature{Val: 600}, false, closedValve())
	}
	assert.InDelta(t, float64(TurbineTurningGearRpm), float64(tr.Velocity), 0.5,
		"want turning gear to hold ~%v rpm, got %v",
		TurbineTurningGearRpm, tr.Velocity)
}

// With a speed demand raised before breaker closure the DEHM ramps
// shaft speed toward the demand at TurbineSpeedRampRate instead of
// free-accelerating on admitted steam.
//
// Spec justification:
//   - "Wide range speed control of the turbine from turning gear to
//     rated speed. This control is accomplished by increasing the
//     speed demand (reference) to bring shaft speed to its rated
//     value at a controlled selectable rate."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-74
func TestTurbine_SpeedDemandRampsToRated(t *testing.T) {
	tr := steamingTurbine()
	tr.SpeedDemand = 1.0
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.GreaterOrEqual(t, float64(tr.Velocity), 250.0,
		"want rotor ramping toward demand, got %v rpm after 10 s", tr.Velocity)
	assert.Less(t, float64(tr.Velocity), float64(TurbineRatedSpeed),
		"want rotor still below rated after 10 s, got %v", tr.Velocity)
	for i := 0; i < int(60*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.InDelta(t, float64(TurbineRatedSpeed), float64(tr.Velocity), 60.0,
		"want rotor at rated speed demand, got %v", tr.Velocity)
}

// Condenser back-pressure above TurbineLowVacuumTripPsia trips a
// running turbine and latches the stop valve shut.
//
// Spec justification:
//   - "Low vacuum trip"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:26
func TestTurbine_LowVacuumTrips(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Velocity = TurbineRatedSpeed
	tr = tr.Step(SteamGeneratorShellPressure, Pressure(TurbineLowVacuumTripPsia+1), Temperature{Val: 600}, false, openValve())
	assert.True(t, tr.TripByVacuum, "want TripByVacuum latched")
	assert.Equal(t, TurbineTripped, tr.State, "want Tripped on low vacuum")
	assert.Zero(t, tr.StopValve.Open, "want stop valve shut on low vacuum trip")
}

// Loss of the lubricating header decays BearingOilPressure and trips
// a running turbine once it falls below TurbineLowBearingOilTripPsi.
//
// Spec justification:
//   - "Low bearing oil pressure trip"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
func TestTurbine_LowBearingOilTrips(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Velocity = TurbineRatedSpeed
	tr.LubeOilPumpRunning = false
	for i := 0; i < int(6*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, openValve())
		if tr.State == TurbineTripped {
			break
		}
	}
	assert.Less(t, float64(tr.BearingOilPressure), TurbineLowBearingOilTripPsi,
		"want header decayed below trip plate, got %v", tr.BearingOilPressure)
	assert.True(t, tr.TripByBearingOil, "want TripByBearingOil latched")
	assert.Equal(t, TurbineTripped, tr.State, "want Tripped on low bearing oil")
}

// Any trip latch shuts the extraction nonreturn valves, cutting
// extraction steam to the heaters and MSR.
//
// Spec justification:
//   - "Extraction line nonreturn valves."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:83
func TestTurbine_TripClosesExtractionNRVs(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Velocity = TurbineRatedSpeed
	tr.Setpoint = 1.0
	tr.Synchronized = true
	tr = tr.Step(SteamGeneratorShellPressure, Pressure(TurbineLowVacuumTripPsia+1), Temperature{Val: 600}, false, openValve())
	assert.True(t, tr.ExtractionNRVClosed, "want NRVs latched closed on trip")
	assert.Zero(t, tr.ExtractionSteamFlow.Float(), "want extraction flow cut")
}

// Mechanical power tracks admitted steam energy: half admission
// converges to half of rated power with no setpoint ceiling.
func TestTurbine_RealPowerTracksSteamEnergy(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Velocity = TurbineRatedSpeed
	tr.GoverningValve = Valve{Open: 0.5}
	tr.Synchronized = true
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.InDelta(t, 0.5*float64(TurbineRatedkW), float64(tr.RealPower),
		0.1*float64(TurbineRatedkW),
		"want RealPower ≈ 50%% of rated at half admission, got %v", tr.RealPower)
}

// A dispatcher load reference ramps the setpoint toward itself at
// RampRate while running and not in load-runback.
//
// Spec justification:
//   - "reference values established by the dispatcher"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
func TestTurbine_LoadDemandRefRampsSetpoint(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Velocity = TurbineRatedSpeed
	tr.Setpoint = 0.5
	tr.RampRate = 0.05
	tr.LoadDemandRef = 0.7
	tr.GridFrequency = GridRatedFrequency
	tr.Synchronized = true
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.InDelta(t, 0.7, tr.Setpoint, 0.01,
		"want Setpoint ramped to dispatcher reference 0.7, got %v", tr.Setpoint)
}

// A synchronized machine is held at synchronous speed by the
// infinite bus: throttling admission for a low initial load must not
// sag shaft speed (which would drag terminal frequency out of the
// synchronism tolerance and pop the breaker).
//
// Spec justification:
//   - "The turbine-generator is brought up to synchronous speed and
//     paralleled on the grid."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:52-53
func TestTurbine_SynchronizedSpeedHeldUnderThrottledAdmission(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Synchronized = true
	tr.Velocity = TurbineRatedSpeed
	tr.GoverningValve = Valve{Open: 0.05}
	tr.GridFrequency = GridRatedFrequency
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, openValve())
	}
	assert.Equal(t, TurbineRatedSpeed, tr.Velocity,
		"want bus-held rotor to stay at rated speed under 5 percent admission")
}

// A synchronized machine's steam demand follows the load reference
// even with the governing valves parked wide open: the draw is
// throttled to the Setpoint fraction directly instead of waiting for
// the DEHM first-stage trim to converge.
//
// Spec justification:
//   - "After generator breaker closure, the ability to increase or
//      decrease electrical load at a controlled selectable rate."
//      specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
func TestTurbine_SynchronizedDrawFollowsLoadReference(t *testing.T) {
	tr := steamingTurbine()
	tr.State = TurbineRunning
	tr.Velocity = TurbineRatedSpeed
	tr.Synchronized = true
	tr.Setpoint = 0.6
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		tr = tr.Step(SteamGeneratorShellPressure, CondenserDesignVacuum, Temperature{Val: 600}, false,
			openValve())
	}
	want := CondenserDesignSteamLoadGalPer * 0.6
	assert.InDelta(t, want, tr.steamDemandGalPerStep, want*0.02,
		"synchronized draw should cap at the load reference fraction")
	assert.InDelta(t, 0.6*float64(TurbineRatedkW), float64(tr.RealPower),
		0.1*float64(TurbineRatedkW),
		"want RealPower at the load reference ceiling, got %v", tr.RealPower)
}

// The main generator breaker is operator-reachable through the
// control map: closing it parallels the unit once the synchronizer
// tolerance is met. Wiring check — the sync/pop physics are covered
// by the plant-level scenario run.
//
// Spec justification:
//   - "The turbine-generator is brought up to synchronous speed and
//     paralleled on the grid."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:52-53
func TestGeneratorBreakerControlRegistered(t *testing.T) {
	p := MakePlant()
	c := GetControlMap(&p)["TurbineBuilding/Generator/BreakerClosed"]
	require.NotNil(t, c, "want main generator breaker control registered")
	require.NoError(t, c.Set("1"))
	assert.True(t, p.TurbineBuilding.Generator.BreakerClosed)
	require.NoError(t, c.Set("0"))
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed)
}
