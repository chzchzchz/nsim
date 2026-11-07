package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// At construction neither Phase A nor Phase B is latched, both
// manual flags are false, and the design-qualification flags
// (MissileProtected, SeismicClass1) are recorded as true.
func TestCIS_InitialStateHasNeitherPhaseLatched(t *testing.T) {
	p := MakePlant()
	cis := p.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	assert.False(t, cis.PhaseAIsolated,
		"want PhaseAIsolated=false at startup, got %v",
		cis.PhaseAIsolated)
	assert.False(t, cis.PhaseBIsolated,
		"want PhaseBIsolated=false at startup, got %v",
		cis.PhaseBIsolated)
	assert.False(t, cis.ManualPhaseA,
		"want ManualPhaseA=false at startup, got %v", cis.ManualPhaseA)
	assert.False(t, cis.ManualPhaseB,
		"want ManualPhaseB=false at startup, got %v", cis.ManualPhaseB)
	assert.True(t, cis.MissileProtected,
		"want MissileProtected=true at startup, got %v",
		cis.MissileProtected)
	assert.True(t, cis.SeismicClass1,
		"want SeismicClass1=true at startup, got %v", cis.SeismicClass1)
	assert.False(t, cis.IsIsolated(),
		"want IsIsolated=false at startup, got %v", cis.IsIsolated())
}

// Phase A latches when the safety-injection signal is true. The
// latch clears once BOTH the safety-injection signal is gone AND
// the operator has not commanded manual Phase A.
//
// Spec justification:
//   - "Containment Isolation Phase A: Safety injection, Manual."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
func TestCIS_PhaseALatchesOnSafetyInjection(t *testing.T) {
	cis := ContainmentIsolationSystem{}
	// First step with safety injection active.
	s1 := cis.Step(DefaultTimeStep, true, 0)
	assert.True(t, s1.PhaseAIsolated,
		"want PhaseAIsolated=true after safety injection, got %v",
		s1.PhaseAIsolated)
	// Subsequent step with safety injection cleared, no
	// manual → Phase A clears (both triggers gone).
	s2 := s1.Step(DefaultTimeStep, false, 0)
	assert.False(t, s2.PhaseAIsolated,
		"want PhaseAIsolated=false after SI clears, got %v",
		s2.PhaseAIsolated)
	// Now phase A latches again on SI, and stays latched
	// while SI persists.
	s3 := s2.Step(DefaultTimeStep, true, 0)
	assert.True(t, s3.PhaseAIsolated,
		"want PhaseAIsolated=true after SI returns, got %v",
		s3.PhaseAIsolated)
}

// Phase A also latches when the operator sets ManualPhaseA and
// stays latched until the operator clears the manual flag AND no
// safety-injection signal is present.
//
// Spec justification:
//   - "Containment Isolation Phase A: Safety injection, Manual."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
func TestCIS_PhaseALatchesOnManual(t *testing.T) {
	cis := ContainmentIsolationSystem{
		ManualPhaseA: true,
	}
	s1 := cis.Step(DefaultTimeStep, false, 0)
	assert.True(t, s1.PhaseAIsolated,
		"want PhaseAIsolated=true with ManualPhaseA set, got %v",
		s1.PhaseAIsolated)
	// Operator clears manual, no SI signal → Phase A clears.
	s2 := s1.Step(DefaultTimeStep, false, 0)
	s2.ManualPhaseA = false
	s3 := s2.Step(DefaultTimeStep, false, 0)
	assert.False(t, s3.PhaseAIsolated,
		"want PhaseAIsolated=false after manual reset, got %v",
		s3.PhaseAIsolated)
}

// Phase B latches when containment pressure exceeds the
// CISPhaseBSetpoint threshold and stays latched until pressure
// falls below the setpoint AND the operator clears ManualPhaseB.
//
// Spec justification:
//   - "Containment Isolation Phase B: High containment pressure,
//     Manual actuation of containment spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
func TestCIS_PhaseBLatchesOnHighPressure(t *testing.T) {
	cis := ContainmentIsolationSystem{}
	hi := CISPhaseBSetpoint + 1
	// Step with pressure above the setpoint.
	s1 := cis.Step(DefaultTimeStep, false, hi)
	assert.True(t, s1.PhaseBIsolated,
		"want PhaseBIsolated=true above setpoint, got %v",
		s1.PhaseBIsolated)
	// Step with pressure below the setpoint, no manual →
	// Phase B clears.
	s2 := s1.Step(DefaultTimeStep, false, CISPhaseBSetpoint-1)
	assert.False(t, s2.PhaseBIsolated,
		"want PhaseBIsolated=false below setpoint, got %v",
		s2.PhaseBIsolated)
}

// Phase B also latches on ManualPhaseB and only clears once the
// manual flag is reset AND pressure is below the setpoint.
//
// Spec justification:
//   - "Containment Isolation Phase B: High containment pressure,
//     Manual actuation of containment spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
func TestCIS_PhaseBLatchesOnManual(t *testing.T) {
	cis := ContainmentIsolationSystem{
		ManualPhaseB: true,
	}
	s1 := cis.Step(DefaultTimeStep, false, 0)
	assert.True(t, s1.PhaseBIsolated,
		"want PhaseBIsolated=true with ManualPhaseB set, got %v",
		s1.PhaseBIsolated)
	// Manual reset but pressure is fine — Phase B clears.
	s2 := s1.Step(DefaultTimeStep, false, 0)
	s2.ManualPhaseB = false
	s3 := s2.Step(DefaultTimeStep, false, 0)
	assert.False(t, s3.PhaseBIsolated,
		"want PhaseBIsolated=false after manual reset, got %v",
		s3.PhaseBIsolated)
}

// IsIsolated returns true when either Phase A or Phase B is
// latched.
//
// Spec justification:
//   - "The Containment Isolation System provides the means of
//     isolating the various fluid systems passing through the
//     containment walls as required to prevent the release of
//     radioactivity to the outside environment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:92-96
func TestCIS_IsIsolatedReportsEitherPhase(t *testing.T) {
	// Neither phase active.
	cis := ContainmentIsolationSystem{}
	assert.False(t, cis.IsIsolated(),
		"want IsIsolated=false with no phase latched, got %v",
		cis.IsIsolated())
	// Phase A only.
	cis.PhaseAIsolated = true
	assert.True(t, cis.IsIsolated(),
		"want IsIsolated=true with PhaseA latched, got %v",
		cis.IsIsolated())
	cis.PhaseAIsolated = false
	// Phase B only.
	cis.PhaseBIsolated = true
	assert.True(t, cis.IsIsolated(),
		"want IsIsolated=true with PhaseB latched, got %v",
		cis.IsIsolated())
}

// When the plant wires CIS into Plant.Step, the safety-injection
// signal latches Phase A and propagates into the
// ContainmentBuilding field.
func TestPlant_CIS_PhaseALatchesOnLOCA(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	p.Step(DefaultTimeStep)
	p.Update()
	cis := p.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	assert.True(t, cis.PhaseAIsolated,
		"want PhaseAIsolated=true after LOCA signal, got %v",
		cis.PhaseAIsolated)
}
