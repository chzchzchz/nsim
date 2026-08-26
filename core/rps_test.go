package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	r1 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 10, CoreReferenceInletF, 0, 1, 1, 0,
		false)
	assert.False(t, r1.ReactorTripped,
		"want ReactorTripped=false with one trip condition, got %v",
		r1.ReactorTripped)
	assert.InDelta(t, 1, r1.TripSignalCount, 1e-9,
		"want TripSignalCount=1, got %v", r1.TripSignalCount)
	// Two trips: low SG level + low steamline pressure → trip.
	r2 := r.Step(100, PressurizerHeaterTarget, 10, CoreReferenceInletF, 0, 1, 1, 0,
		false)
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
	r2 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1, 1, 0,
		false)
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
	r2 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1, 1, 0,
		false)
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
	r2 := r.Step(100, PressurizerHeaterTarget, 50, CoreReferenceInletF,
		RPSHighContainmentPressureTrip+1, 1, 1, 0,
		false)
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
	r2 := r.Step(100, PressurizerHeaterTarget, RPSLowSteamGeneratorLevelTrip-1,
		CoreReferenceInletF, 0, 1, 1, 0,
		false)
	assert.True(t, r2.ReactorTripped,
		"want ReactorTripped=true, got %v", r2.ReactorTripped)
	assert.True(t, r2.TurbineTripActuated,
		"want TurbineTripActuated=true after reactor trip, got %v",
		r2.TurbineTripActuated)
}

// A turbine-generator trip above the at-power permissive initiates
// the reactor trip outright, in the same direct-initiation class as
// a manual trip or an SI signal. Below the permissive the machine
// can be rolling during hot-standby lineups, so its trip carries no
// vote. Like the other direct initiators it produces no ESFAS
// output on its own.
//
// Spec: "Turbine-generator trip - Trip of the turbinegenerator
// above a certain power level initiates a reactor trip"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:33-34
func TestRPS_TurbineTripInitiatesReactorTripAbovePermissive(t *testing.T) {
	idle := ReactorProtectionSystem{}
	belowPermissive := idle.Step(SteamGeneratorShellPressure,
		PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0, 1, 50,
		true)
	assert.False(t, belowPermissive.ReactorTripped,
		"a turbine trip below the permissive must not scram")

	atPower := idle.Step(SteamGeneratorShellPressure,
		PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0.5, 1, 50,
		true)
	assert.True(t, atPower.ReactorTripped,
		"want ReactorTripped=true on turbine trip above the "+
			"permissive, got %v", atPower.ReactorTripped)
	assert.False(t, atPower.SafetyInjectionActuated,
		"the turbine trip alone must not actuate safeguards")
	latched := atPower.Step(SteamGeneratorShellPressure,
		PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0.5, 1, 50,
		false)
	assert.True(t, latched.ReactorTripped,
		"the reactor trip must latch across subsequent steps")
}

// Manual trip does not trigger containment spray (spray requires
// high containment pressure or manual; manual trip is a separate
// operator action).
func TestRPS_ManualTripDoesNotStartContainmentSpray(t *testing.T) {
	r := ReactorProtectionSystem{ManualTrip: true}
	r2 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1, 1, 0,
		false)
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
	r2 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget,
		RPSLowSteamGeneratorLevelTrip-1,
		CoreReferenceInletF, 0, 1, 1, 0,
		false)
	assert.True(t, r2.AuxiliaryFeedPumpsStartActuated,
		"want AuxiliaryFeedPumpsStartActuated=true with low SG "+
			"level, got %v", r2.AuxiliaryFeedPumpsStartActuated)
}

// Low Tcold contributes no trip signal while the plant operates
// below the at-power permissive (P-7): cold-condition lineups run
// with low Tcold expected, so the condition neither votes toward
// a reactor trip nor actuates SI / steamline isolation until the
// plant exceeds the permissive.
//
// Spec justification:
//   - "When reactor power increases above 10%, verify the 'Nuclear
//     At-Power' Permissive P-10 light illuminates and the 'At-Power'
//     Permissive P-7 light extinguishes." NOTE: "The 'at-power'
//     reactor trips are now enabled."
//     specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-7
func TestRPS_LowTcoldBlockedBelowAtPowerPermissive(t *testing.T) {
	r := ReactorProtectionSystem{}
	coldTcold := 140.0
	belowPermissive := RPSAtPowerPermissiveFraction / 2

	// Below the permissive the low-Tcold condition is blocked.
	r1 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		coldTcold, 0, belowPermissive, 1, 0,
		false)
	assert.Equal(t, 0, r1.TripSignalCount,
		"want no trip signals with low Tcold blocked")
	assert.False(t, r1.ReactorTripped)
	assert.False(t, r1.SafetyInjectionActuated,
		"want SI blocked with low Tcold below P-7")
	assert.False(t, r1.SteamlineIsolationActuated)

	// Above the permissive low Tcold counts as one voted signal
	// (not enough to trip alone) and actuates SI, which then votes
	// the trip itself: "Actuation of the Safety Injection System
	// simultaneously initiates a reactor trip." The vote reads the
	// previous step's SI latch, so the trip lands one step later.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:38-40
	r2 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		coldTcold, 0, RPSAtPowerPermissiveFraction*2, 1, 0,
		false)
	assert.Equal(t, 1, r2.TripSignalCount,
		"want exactly the low-Tcold signal above the permissive")
	assert.True(t, r2.SafetyInjectionActuated)
	assert.False(t, r2.ReactorTripped,
		"the SI latch lands this step")
	r3 := r2.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		coldTcold, 0, RPSAtPowerPermissiveFraction*2, 1, 0,
		false)
	assert.True(t, r3.ReactorTripped,
		"SI actuation must initiate the reactor trip")
}

// When the plant wires RPS into Plant.Step, the
// ReactorPressureVessel.Trip flag is latched by the RPS so the
// RPV drops rods this same step.
func TestPlant_RPS_LatchesReactorPressureVesselTrip(t *testing.T) {
	p := MakePlant()
	// Drive two trip conditions: low steamline pressure and low
	// SG level.
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 0.05 * p.ReactorBuilding.SteamGenerator.Volume.Float(), Unit: u.Gallon}
	p.Step()
	p.Update()
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"want RPV.Trip=true after RPS trips, got %v",
		p.ReactorBuilding.ReactorPressureVessel.Trip)
}

// Operator post-trip reset clears the trip latch and the ESFAS
// outputs. The next Step re-declares the trip while trip channels
// still satisfy 2/4 voting, so reset cannot bypass an active trip.
//
// Spec justification:
//   - "The trip breakers are arranged to provide a 2/4 configuration."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
//   - Return to service proceeds from hot standby with the rods
//     withdrawn back to criticality.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26
func TestRPS_ResetClearsLatchesAndRevotes(t *testing.T) {
	r := ReactorProtectionSystem{}
	// Drive two channels (low steamline pressure + high
	// containment pressure) so the trip and its ESFAS outputs
	// latch.
	r = r.Step(100, PressurizerHeaterTarget, 50, CoreReferenceInletF,
		RPSHighContainmentPressureTrip+1, 1, 1, 0,
		false)
	assert.True(t, r.ReactorTripped, "precondition: want ReactorTripped=true")
	assert.True(t, r.SafetyInjectionActuated,
		"precondition: want SafetyInjectionActuated=true")
	assert.True(t, r.SteamlineIsolationActuated,
		"precondition: want SteamlineIsolationActuated=true")

	// Reset with every channel clear: nothing re-latches.
	r.Reset()
	assert.False(t, r.ReactorTripped)
	assert.False(t, r.ManualTrip)
	r2 := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 1, 1, 0,
		false)
	assert.False(t, r2.ReactorTripped,
		"want no re-trip after reset with clear channels")
	assert.False(t, r2.SafetyInjectionActuated)

	// Reset while a trip condition is still present: the next
	// Step re-declares the trip on its own.
	r3 := ReactorProtectionSystem{}
	r3 = r3.Step(100, PressurizerHeaterTarget, 50, CoreReferenceInletF,
		RPSHighContainmentPressureTrip+1, 1, 1, 0,
		false)

	require.True(t, r3.ReactorTripped)
	r3.Reset()
	r4 := r3.Step(100, PressurizerHeaterTarget, 50, CoreReferenceInletF,
		RPSHighContainmentPressureTrip+1, 1, 1, 0,
		false)
	assert.True(t, r4.ReactorTripped,
		"want re-trip while 2/4 voting still demands one")
}

// Resetting the RPS must also release the containment-isolation
// demand: Plant.Step mirrors the RPS Phase A/B ESFAS outputs into the
// CIS manual flags each step, and the CIS unlatches a phase once its
// manual flag is reset and the underlying condition is gone.
//
// Spec justification:
//   - Containment Isolation Phase A triggers: "Safety injection,
//     Manual"; Phase B: "High containment pressure, Manual actuation
//     of containment spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:68-80
func TestPlant_RPSResetReleasesContainmentIsolation(t *testing.T) {
	p := MakePlant()
	// Latch SI and both CIS phases via their Table 7-1 triggers:
	// high containment pressure drives SI, spray, and Phase B;
	// SI in turn drives Phase A.
	p.ReactorBuilding.ContainmentBuilding.Pressure =
		RPSHighContainmentPressureTrip + AtmosphericPressure + 1
	p.Step()
	p.Update()
	cis := p.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	require.True(t, cis.PhaseAIsolated, "precondition: want Phase A isolated")
	require.True(t, cis.PhaseBIsolated, "precondition: want Phase B isolated")

	// Operator restores conditions (containment back to
	// atmosphere, steamline at no-load pressure — MakePlant leaves
	// it at 0 psia, which is itself a live low-steamline SI
	// trigger), resets the protection system, and stands down the
	// ESFAS-initiated emergency-diesel start demand the actuation
	// latched ("The onsite emergency diesel generators are started",
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:113-114;
	// clearing the latch stays an explicit operator action). The
	// manual flags clear, then both phases unlatch.
	p.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartNone
	p.AuxiliaryBuilding.DieselGeneratorB.StartSignal = DieselStartNone
	p.ReactorBuilding.ReactorProtectionSystem.Reset()
	for i := 0; i < 10; i++ {
		p.Step()
		p.Update()
	}
	cis = p.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	assert.False(t, cis.ManualPhaseA, "want manual Phase A demand released")
	assert.False(t, cis.ManualPhaseB, "want manual Phase B demand released")
	assert.False(t, cis.PhaseAIsolated, "want Phase A unlatched after reset")
	assert.False(t, cis.PhaseBIsolated, "want Phase B unlatched after reset")
}

// The low-steamline channel votes only above the at-power permissive:
// cold lineups hold the header near atmospheric by design (cooldown
// ends with the dumps closed as pressure approaches zero psig), so the
// healthy reading is sub-setpoint below P-7 and must not stand as a
// permanent vote. Above the permissive the channel votes exactly as
// before, and the SI trigger stays live below it when unblocked (its
// own block governs that output - covered by
// TestRPS_ChannelTestSuppressesTestedChannel's in-service case).
//
// Spec: at-power trips enable above P-7/P-10
// (specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-7);
// planned descents arm the blocks before crossing
// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110).
func TestRPS_LowSteamlineVoteFollowsAtPowerPermissive(t *testing.T) {
	idle := ReactorProtectionSystem{}
	cold := idle.Step(100, PressurizerHeaterTarget,
		50, CoreReferenceInletF, 0, 0, 1, 0,
		false)
	assert.Zero(t, cold.TripSignalCount,
		"a sub-setpoint header votes nothing below the permissive")
	assert.False(t, cold.ReactorTripped,
		"the standing cold-lineup state alone must not scram")

	atPower := idle.Step(100, PressurizerHeaterTarget,
		50, CoreReferenceInletF, 0, 0.5, 1, 0,
		false)
	assert.Equal(t, 1, atPower.TripSignalCount,
		"above the permissive the same state votes")
}

// The high-pressurizer-level channel votes only above the at-power
// permissive: cold lineups sit water-solid at 100 percent level by
// design (chapter_16/page_222.txt:62-67), so the healthy reading is
// past any plausible setpoint below P-7. Above the permissive, a level
// past the declared setpoint votes.
//
// Spec: "Pressurizer trip signals - High pressure, low pressure, and
// high-level trip signals initiate a reactor trip"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:19-21
func TestRPS_HighPzrLevelVoteFollowsAtPowerPermissive(t *testing.T) {
	idle := ReactorProtectionSystem{}
	waterSolidCold := idle.Step(SteamGeneratorShellPressure,
		PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0, 1, 100,
		false)
	assert.Zero(t, waterSolidCold.TripSignalCount,
		"the designed water-solid cold lineup must not vote")

	atPower := idle.Step(SteamGeneratorShellPressure,
		PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0.5, 1, 85,
		false)
	assert.Equal(t, 1, atPower.TripSignalCount,
		"an over-filled pressurizer votes above the permissive")

	inBand := idle.Step(SteamGeneratorShellPressure,
		PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0.5, 1, 50,
		false)
	assert.Zero(t, inBand.TripSignalCount,
		"level inside the program band votes nothing")
}
