package core

// Step advances the Reactor Protection System by one timestep.
// The RPS evaluates trip conditions from the supplied process
// variables, applies 2/4 voting logic to decide whether to
// declare a reactor trip, and computes the ESFAS outputs per
// Table 7-1.
//
// 2/4 voting: each monitored variable is treated as one of four
// redundant channels. A trip is declared when the trip count
// reaches RPSTripVotingThreshold (2).
//
//   "The protection system normally uses 2/4 logic but reverts to
//    a safe 2/3 logic when part of the system is being tested
//    or is involved in maintenance. A 2/4 logic means that a
//    trip occurs when at least 2 signals out of 4 indicate a
//    trip condition."
//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:116-121
//
// ESFAS outputs are commanded per Table 7-1:
//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81
//
// Spec justification:
//   - RPS functions: "Monitors the plant for abnormal
//     conditions and alerts the operator to take appropriate
//     action; Automatically provides reactor trip (shutdown)
//     signals and engineered safeguards actuation signals when
//     plant conditions [...] reach the plant safety limits."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:55-72
//   - Trip purpose: "Whenever a direct process or calculated
//     variable exceeds a setpoint, the reactor will be shut
//     down."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:1-3
//   - Turbine trip interlock: "The RPS initiates a turbine trip
//     signal whenever reactor trip occurs."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:10-11
func (r *ReactorProtectionSystem) Step(
	steamlinePressure Pressure,
	sgLevelPct float64,
	tcoldF float64,
	containmentPressureGauge Pressure,
	powerFrac float64,
) ReactorProtectionSystem {
	ret := *r

	// Count trip conditions. Each boolean contributes 1 to
	// the trip count; the trip is declared when the count
	// reaches RPSTripVotingThreshold (2/4 logic).
	//
	// Note: the simplified model omits the pressurizer-pressure
	// trip and the high-pressurizer-level trip. The
	// pressurizer pressure in this fidelity band is
	// dominated by surge-line interactions and does not
	// reliably reflect a real low-pressure event; the
	// high-level trip requires a stable level measurement
	// that the simplified model does not yet provide. Both
	// are deferred to a higher fidelity band as called out
	// in the plan's "Assumptions" section.
	//
	// "A 2/4 logic means that a trip occurs when at least 2
	// signals out of 4 indicate a trip condition."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
	// P-7 at-power trip permissive: below the at-power level
	// the "at-power" reactor trips are not enabled, so low Tcold
	// — expected throughout cold-condition lineup and startup —
	// contributes no trip signal and no SI/steamline-isolation
	// trigger until the plant exceeds the permissive.
	//
	// "When reactor power increases above 10%, verify the 'Nuclear
	// At-Power' Permissive P-10 light illuminates and the
	// 'At-Power' Permissive P-7 light extinguishes." NOTE: "The
	// 'at-power' reactor trips are now enabled."
	// specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-7
	atPowerTripsEnabled := powerFrac >= RPSAtPowerPermissiveFraction

	var trips int
	if sgLevelPct < RPSLowSteamGeneratorLevelTrip {
		trips++
	}
	if sgLevelPct > RPSHighSteamGeneratorLevelTrip {
		trips++
	}
	if steamlinePressure < RPSLowSteamlinePressureTrip {
		trips++
	}
	if tcoldF < RPSLowTcoldTrip && atPowerTripsEnabled {
		trips++
	}
	if containmentPressureGauge > RPSHighContainmentPressureTrip {
		trips++
	}
	ret.TripSignalCount = trips

	// Declare reactor trip when 2/4 voting threshold is met
	// OR when the operator has pressed manual trip.
	// "The RPS provides for manual initiation of reactor trip
	// by operator action in the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
	if ret.ReactorTripped {
		// Latched; stays tripped until manually reset.
	} else if ret.TripSignalCount >= RPSTripVotingThreshold ||
		ret.ManualTrip {
		ret.ReactorTripped = true
	}

	// ESFAS outputs per Table 7-1. Each output latches on the
	// first step the triggering condition is met and stays
	// active; operator reset clears them.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81

	// Safety Injection: high containment pressure, low
	// steamline pressure, low Tcold, manual SI. (Low
	// pressurizer pressure is deferred — see Step preamble.)
	// The operator block arms during planned cooldowns before
	// these variables reach their actuation setpoints
	// (specs/the_westinghouse_pressurized_water_react/
	// chapter_16/page_227.txt:108-110).
	if !ret.SafetyInjectionActuated && !ret.SafetyInjectionBlocked {
		if containmentPressureGauge > RPSHighContainmentPressureTrip ||
			steamlinePressure < RPSLowSteamlinePressureTrip ||
			(tcoldF < RPSLowTcoldTrip && atPowerTripsEnabled) ||
			ret.ManualTrip {
			ret.SafetyInjectionActuated = true
		}
	}

	// Steamline Isolation: same triggers plus high
	// steam-pressure rate of change (not modelled at this
	// fidelity).
	if !ret.SteamlineIsolationActuated {
		if containmentPressureGauge > RPSHighContainmentPressureTrip ||
			steamlinePressure < RPSLowSteamlinePressureTrip ||
			(tcoldF < RPSLowTcoldTrip && atPowerTripsEnabled) ||
			ret.ManualTrip {
			ret.SteamlineIsolationActuated = true
		}
	}

	// Containment Spray: high containment pressure (manual
	// containment spray is operator action on the spray
	// system, not an RPS output).
	if !ret.ContainmentSprayActuated {
		if containmentPressureGauge > RPSHighContainmentPressureTrip {
			ret.ContainmentSprayActuated = true
		}
	}

	// Containment Isolation Phase A: safety injection or
	// manual.
	if !ret.ContainmentIsolationPhaseAActuated {
		if ret.SafetyInjectionActuated || ret.ManualTrip {
			ret.ContainmentIsolationPhaseAActuated = true
		}
	}

	// Containment Isolation Phase B: high containment
	// pressure (manual containment spray / Phase B is
	// operator action on the spray system, not an RPS
	// output).
	if !ret.ContainmentIsolationPhaseBActuated {
		if containmentPressureGauge > RPSHighContainmentPressureTrip {
			ret.ContainmentIsolationPhaseBActuated = true
		}
	}

	// Auxiliary Feed Pumps start: safety injection or low SG
	// level.
	if !ret.AuxiliaryFeedPumpsStartActuated {
		if ret.SafetyInjectionActuated ||
			sgLevelPct < RPSLowSteamGeneratorLevelTrip {
			ret.AuxiliaryFeedPumpsStartActuated = true
		}
	}

	// Feedwater Isolation: high SG level, safety injection, or
	// coincidence of reactor trip and low Tcold.
	if !ret.FeedwaterIsolationActuated {
		if sgLevelPct > RPSHighSteamGeneratorLevelTrip ||
			ret.SafetyInjectionActuated ||
			(ret.ReactorTripped && tcoldF < RPSLowTcoldTrip) {
			ret.FeedwaterIsolationActuated = true
		}
	}

	// Turbine Trip: reactor trip, high SG level, or safety
	// injection.
	// "The RPS initiates a turbine trip signal whenever
	// reactor trip occurs."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:10-11
	if !ret.TurbineTripActuated {
		if ret.ReactorTripped ||
			sgLevelPct > RPSHighSteamGeneratorLevelTrip ||
			ret.SafetyInjectionActuated {
			ret.TurbineTripActuated = true
		}
	}

	return ret
}

// Reset clears the reactor-trip latch and the ESFAS output latches
// once the operator has addressed the initiating conditions. The
// next Step re-declares a trip on its own if enough trip channels
// are still demanding one, so reset cannot bypass an active trip:
//   "The trip breakers are arranged to provide a 2/4 configuration."
//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
// Return to operation after a shutdown proceeds from hot standby,
// withdrawing the rods back to criticality:
//   "Startup from Hot Standby: The shutdown banks of the reactor
//    control rods are withdrawn."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26
func (r *ReactorProtectionSystem) Reset() {
	r.ReactorTripped = false
	r.ManualTrip = false
	r.SafetyInjectionActuated = false
	r.SteamlineIsolationActuated = false
	r.ContainmentSprayActuated = false
	r.ContainmentIsolationPhaseAActuated = false
	r.ContainmentIsolationPhaseBActuated = false
	r.AuxiliaryFeedPumpsStartActuated = false
	r.FeedwaterIsolationActuated = false
	r.TurbineTripActuated = false
}
