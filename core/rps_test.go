package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// At construction the reactor is not tripped, no manual trip
// has been pressed, no ESFAS output is active, and the trip
// signal count is zero.
//
// Spec justification:
//   - "The RPS provides for manual initiation of reactor trip by
//     operator action in the control room."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
func TestRPS_InitialStateHasNoTrip(t *testing.T) {
	p := MakePlant()
	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.False(t, rps.ReactorTripped,
		"want ReactorTripped=false at startup, got %v", rps.ReactorTripped)
	assert.False(t, rps.ManualTrip,
		"want ManualTrip=false at startup, got %v", rps.ManualTrip)
	assert.Zero(t, rps.TripSignalCount,
		"want TripSignalCount=0 at startup, got %v", rps.TripSignalCount)
	assert.False(t, rps.SafetyInjectionActuated,
		"want SafetyInjectionActuated=false at startup, got %v",
		rps.SafetyInjectionActuated)
	assert.False(t, rps.ContainmentSprayActuated,
		"want ContainmentSprayActuated=false at startup, got %v",
		rps.ContainmentSprayActuated)
	assert.False(t, rps.TurbineTripActuated,
		"want TurbineTripActuated=false at startup, got %v",
		rps.TurbineTripActuated)
}

// 2/4 voting: two simultaneous trip conditions trigger a
// reactor trip. Below the threshold the reactor is not tripped.
//
// Spec justification:
//   - "A 2/4 logic means that a trip occurs when at least 2
//     signals out of 4 indicate a trip condition."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
func TestRPS_TwoTripConditionsDeclareReactorTrip(t *testing.T) {
	r := ReactorProtectionSystem{}
	// One trip: low SG level (15%) alone should not trip.
	r1 := r.Step(SteamGeneratorShellPressure, 10, CoreReferenceInletF, 0)
	assert.False(t, r1.ReactorTripped,
		"want ReactorTripped=false with one trip condition, got %v",
		r1.ReactorTripped)
	assert.InDelta(t, 1, r1.TripSignalCount, 1e-9,
		"want TripSignalCount=1, got %v", r1.TripSignalCount)
	// Two trips: low SG level + low steamline pressure → trip.
	r2 := r.Step(100, 10, CoreReferenceInletF, 0)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true with two trip conditions, got %v",
		r2.ReactorTripped)
	assert.InDelta(t, 2, r2.TripSignalCount, 1e-9,
		"want TripSignalCount=2, got %v", r2.TripSignalCount)
}

// Manual trip is honoured regardless of process variable state.
//
// Spec justification:
//   - "The RPS provides for manual initiation of reactor trip by
//     operator action in the control room."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
func TestRPS_ManualTripOverridesVoting(t *testing.T) {
	r := ReactorProtectionSystem{ManualTrip: true}
	r2 := r.Step(SteamGeneratorShellPressure, 50,
		CoreReferenceInletF, 0)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true on manual trip, got %v",
		r2.ReactorTripped)
}

// Once tripped the reactor remains tripped.
//
// Spec justification:
//   - The simplified model assumes the trip breaker is a
//     mechanical latch; once opened it cannot re-close without
//     operator action.
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
func TestRPS_ReactorTripLatches(t *testing.T) {
	r := ReactorProtectionSystem{ReactorTripped: true}
	r2 := r.Step(SteamGeneratorShellPressure, 50,
		CoreReferenceInletF, 0)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true after latch, got %v", r2.ReactorTripped)
}

// High containment pressure triggers a reactor trip and
// actuates the corresponding ESFAS outputs. Driving two
// trip conditions (high containment + low steamline pressure)
// crosses the 2/4 voting threshold.
//
// Spec justification:
//   - High containment pressure is a Table 7-1 trigger for
//     Safety Injection, Steamline Isolation, Containment Spray,
//     and Containment Isolation Phase B.
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-71
//   - 2/4 voting: "A 2/4 logic means that a trip occurs when at
//     least 2 signals out of 4 indicate a trip condition."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
func TestRPS_HighContainmentPressureActuatesESFAS(t *testing.T) {
	r := ReactorProtectionSystem{}
	// Drive 2 trips: high containment pressure + low steamline
	// pressure.
	r2 := r.Step(100, 50, CoreReferenceInletF,
		RPSHighContainmentPressureTrip+1)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true, got %v", r2.ReactorTripped)
	assert.True(t, r2.SafetyInjectionActuated,
		"want SafetyInjectionActuated=true, got %v",
		r2.SafetyInjectionActuated)
	assert.True(t, r2.ContainmentSprayActuated,
		"want ContainmentSprayActuated=true, got %v",
		r2.ContainmentSprayActuated)
	assert.True(t, r2.ContainmentIsolationPhaseBActuated,
		"want ContainmentIsolationPhaseBActuated=true, got %v",
		r2.ContainmentIsolationPhaseBActuated)
}

// Turbine trip is actuated whenever reactor trip is declared.
//
// Spec justification:
//   - "The RPS initiates a turbine trip signal whenever reactor
//     trip occurs."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:10-11
func TestRPS_TurbineTripFollowsReactorTrip(t *testing.T) {
	r := ReactorProtectionSystem{}
	// Drive 2 trips: low steamline pressure + low SG level.
	r2 := r.Step(100, RPSLowSteamGeneratorLevelTrip-1,
		CoreReferenceInletF, 0)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true, got %v", r2.ReactorTripped)
	assert.True(t, r2.TurbineTripActuated,
		"want TurbineTripActuated=true after reactor trip, got %v",
		r2.TurbineTripActuated)
}

// Manual trip does not trigger containment spray (spray requires
// high containment pressure or manual; manual trip is a separate
// operator action).
func TestRPS_ManualTripDoesNotStartContainmentSpray(t *testing.T) {
	r := ReactorProtectionSystem{ManualTrip: true}
	r2 := r.Step(SteamGeneratorShellPressure, 50,
		CoreReferenceInletF, 0)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true on manual trip, got %v",
		r2.ReactorTripped)
	assert.False(t, r2.ContainmentSprayActuated,
		"want ContainmentSprayActuated=false on manual trip alone, "+
			"got %v", r2.ContainmentSprayActuated)
}

// Low SG level actuates the auxiliary feed pumps start signal.
//
// Spec justification:
//   - ESFAS Table 7-1: Start Auxiliary Feed Pumps triggers on
//     safety injection or low steam generator level.
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:42-44
func TestRPS_LowSGLevelStartsAuxFeedPumps(t *testing.T) {
	r := ReactorProtectionSystem{}
	r2 := r.Step(SteamGeneratorShellPressure,
		RPSLowSteamGeneratorLevelTrip-1,
		CoreReferenceInletF, 0)
	assert.True(t, r2.AuxiliaryFeedPumpsStartActuated,
		"want AuxiliaryFeedPumpsStartActuated=true with low SG "+
			"level, got %v", r2.AuxiliaryFeedPumpsStartActuated)
}

// When the plant wires RPS into Plant.Step, the
// ReactorPressureVessel.Trip flag is latched by the RPS so the
// RPV drops rods this same step.
func TestPlant_RPS_LatchesReactorPressureVesselTrip(t *testing.T) {
	p := MakePlant()
	// Drive two trip conditions: low steamline pressure and low
	// SG level.
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 0.05*p.ReactorBuilding.SteamGenerator.Volume.Float(), Unit: u.Gallon}
	p.Step()
	p.Update()
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"want RPV.Trip=true after RPS trips, got %v",
		p.ReactorBuilding.ReactorPressureVessel.Trip)
}
