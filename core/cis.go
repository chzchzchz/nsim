package core

// Step advances the Containment Isolation System by one timestep.
// The step implements two phased latches with reset semantics:
//
//   - Phase A isolates when a safety-injection signal is present
//     OR the operator commands manual Phase A.
//   - Phase B isolates when containment pressure exceeds
//     CISPhaseBSetpoint OR the operator commands manual Phase B.
//
// Both phases latch: once set, they persist until BOTH the manual
// flag is reset AND the automatic trigger clears. The spec
// describes Phase A and Phase B as "initiated automatically" with
// no stated reset delay.
//
// Spec justification:
//   - Phase A actuation: "Containment Isolation Phase A: Safety
//     injection, Manual."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
//   - Phase B actuation: "Containment Isolation Phase B: High
//     containment pressure, Manual actuation of containment spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
//   - Purpose: "The Containment Isolation System provides the
//     means of isolating the various fluid systems passing
//     through the containment walls as required to prevent the
//     release of radioactivity to the outside environment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:92-96
func (s *ContainmentIsolationSystem) Step(
	safetyInjection bool,
	containmentPressure Pressure,
) ContainmentIsolationSystem {
	ret := *s

	// Phase A latch logic. Latches true on either trigger
	// (safety-injection signal or manual). Clears when both
	// the manual flag is reset and the safety-injection signal
	// is gone.
	if ret.PhaseAIsolated {
		if !ret.ManualPhaseA && !safetyInjection {
			ret.PhaseAIsolated = false
		}
	} else if ret.ManualPhaseA || safetyInjection {
		ret.PhaseAIsolated = true
	}

	// Phase B latch logic. Latches true on either trigger
	// (containment pressure above setpoint or manual). Clears
	// when both the manual flag is reset AND containment
	// pressure falls below the setpoint.
	if ret.PhaseBIsolated {
		if !ret.ManualPhaseB && containmentPressure < CISPhaseBSetpoint {
			ret.PhaseBIsolated = false
		}
	} else if ret.ManualPhaseB || containmentPressure > CISPhaseBSetpoint {
		ret.PhaseBIsolated = true
	}

	return ret
}

// IsIsolated reports the combined isolation state. A penetration
// is fully isolated when either Phase A or Phase B has latched.
//
// Spec justification:
//   - "Containment Isolation Phase A: Safety injection, Manual."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
//   - "Containment Isolation Phase B: High containment pressure,
//     Manual actuation of containment spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
//   - "The Containment Isolation System provides the means of
//     isolating the various fluid systems passing through the
//     containment walls as required to prevent the release of
//     radioactivity to the outside environment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:92-96
func (s *ContainmentIsolationSystem) IsIsolated() bool {
	return s.PhaseAIsolated || s.PhaseBIsolated
}
