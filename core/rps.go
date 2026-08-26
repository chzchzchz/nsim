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
	pzrPressure Pressure,
	sgLevelPct float64,
	tcoldF float64,
	containmentPressureGauge Pressure,
	powerFrac float64,
	flowFrac float64,
	pzrLevelPct float64,
	turbineTripped bool,
) ReactorProtectionSystem {
	ret := *r

	// Count trip conditions. Each boolean contributes 1 to
	// the trip count; the trip is declared when the count
	// reaches RPSTripVotingThreshold (2/4 logic).
	//
	// The high-pressurizer-level channel rides the same at-power
	// permissive as its siblings: cold lineups sit water-solid at
	// 100 percent level by design, so the healthy reading is past
	// any plausible setpoint below P-7. It consumes the level
	// fraction computed each step for the CVCS level controller -
	// the measurement an earlier omission note called missing.
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

	// Channel-under-test isolation: the monitored input being
	// calibrated is forced away from its trip setpoints before
	// any voting or ESFAS trigger evaluates, so a stimulus
	// injected into that one channel cannot actuate protections
	// while the crew verifies the instrumentation. Forcing the
	// value at this single point covers every downstream consumer
	// at once.
	// "The protection system normally uses 2/4 logic but reverts
	// to a safe 2/3 logic when part of the system is being tested
	// or is involved in maintenance."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
	switch ret.ChannelTest {
	case RPSSteamlineChannelInTest:
		steamlinePressure = max(steamlinePressure,
			RPSLowSteamlinePressureTrip+1)
	case RPSSGLevelChannelInTest:
		sgLevelPct = (RPSLowSteamGeneratorLevelTrip +
			RPSHighSteamGeneratorLevelTrip) / 2
	case RPSTcoldChannelInTest:
		tcoldF = max(tcoldF, RPSLowTcoldTrip+1)
	case RPSContainmentChannelInTest:
		containmentPressureGauge = min(containmentPressureGauge,
			RPSHighContainmentPressureTrip-1)
	case RPSPressurizerChannelInTest:
		pzrPressure = max(pzrPressure,
			RPSLowPressurizerPressureTrip+1)
	case RPSFlowChannelInTest:
		flowFrac = max(flowFrac, RPSLowFlowTripFraction+0.01)
	}

	var trips int
	// The SG level channels join low Tcold / low pressurizer
	// pressure behind the at-power permissive: the cooldown
	// procedure deliberately fills the SG steam sides to the vents
	// for wet layup once the reactor coolant reaches 200 F, so a
	// shell level above the hi-hi setpoint is normal below P-7.
	// "When the reactor coolant reaches 200°F (93°C), the steam
	//  sides of the steam generators are filled to the vents for
	//  wet layup."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:53-55
	// specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-7
	if sgLevelPct < RPSLowSteamGeneratorLevelTrip && atPowerTripsEnabled {
		trips++
	}
	if sgLevelPct > RPSHighSteamGeneratorLevelTrip && atPowerTripsEnabled {
		trips++
	}
	// The low-steamline channel completes the at-power family: cold
	// lineups hold the header near atmospheric by design (cooldown
	// ends with the dumps closed as pressure approaches zero psig),
	// and the procedure arms the SI/steamline blocks before planned
	// descents cross the shared setpoints - so below P-7 the healthy
	// reading is sub-setpoint and contributes no vote.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	// specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-7
	if steamlinePressure < RPSLowSteamlinePressureTrip && atPowerTripsEnabled {
		trips++
	}
	if tcoldF < RPSLowTcoldTrip && atPowerTripsEnabled {
		trips++
	}
	if pzrPressure < RPSLowPressurizerPressureTrip && atPowerTripsEnabled {
		trips++
	}
	if pzrLevelPct > RPSHighPressurizerLevelTrip && atPowerTripsEnabled {
		trips++
	}
	if containmentPressureGauge > RPSHighContainmentPressureTrip {
		trips++
	}
	ret.TripSignalCount = trips

	// Low forced-coolant-flow reactor trip. The signal votes the trip
	// directly rather than through the collapsed vote count: the
	// simplified model collapses each monitored variable's redundant
	// channels into a single integer, so no lone variable could ever
	// reach the 2/4 threshold by itself, while the loss-of-flow
	// evolution requires exactly that single-signal response ("the low
	// coolant flow in that loop will exceed the trip setpoint and will
	// scram the reactor", specs/pwr_sim/chapter_4/page_051.txt:46-47).
	// It is one of the independent primary-coolant trip functions
	// ("low flow, and reactor coolant pump under-speed trip signals
	// provide trips", specs/the_westinghouse_pressurized_water_react/
	// chapter_7/page_138.txt:80-87) and carries no ESFAS output:
	// Table 7-1 lists no flow-driven safeguard
	// (chapter_7/page_140.txt:6-81). Arming follows the same at-power
	// permissive as the other at-power trips: reduced-flow lineups are
	// normal below it (sequential pump starts, hot standby).
	// specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-5
	// The FlowEstablished latch scopes the signal to a DEGRADED
	// flow: it sets once measured flow has reached the setpoint and
	// stays set, so the trip answers a loss from an established
	// lineup, never a pump startup ramp passing through the band on
	// its way up. The latch reads the previous step's state,
	// matching the one-step telemetry lag used elsewhere in this
	// step. A fully secured lineup (flow exactly zero) is natural
	// circulation - the same boundary at which the RPV inlet blend
	// switches off.
	lowFlowTrip := ret.FlowEstablished &&
		flowFrac < RPSLowFlowTripFraction && atPowerTripsEnabled
	if flowFrac >= RPSLowFlowTripFraction {
		ret.FlowEstablished = true
	}

	// High-neutron-flux reactor trip. This votes the trip directly
	// rather than through the collapsed vote count, for the same
	// single-signal reason as the low-flow channel: an overpower
	// transient demands exactly that immediate response ("High flux
	// rate trips protect the reactor in case of rod ejection or
	// accidental rod drop"). It arms at every power level - the flux
	// channels exist for "protection during startup, full power, and
	// shutdown operations" - so no permissive gates it, and Table 7-1
	// lists no flux-driven safeguard.
	//   "Neutron flux trip signals - Source, intermediate, low power
	//    range, and high power range signals are provided for
	//    protection during startup, full power, and shutdown
	//    operations."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:56-63
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81
	highFluxTrip := powerFrac > RPSHighNeutronFluxTripFraction

	// High-pressurizer-pressure reactor trip. Like the flux channel
	// it declares outright: the collapsed single-integer vote cannot
	// reach the 2/4 threshold alone, and unlike low pressurizer
	// pressure this channel has no SI echo to back-trip the reactor
	// (Table 7-1 lists no safeguard on high pressurizer pressure).
	// It arms at every power level - boundary protection is
	// unconditional, and cold overpressurization happens below P-7.
	//   "Pressurizer trip signals - High pressure, low pressure,
	//    and high-level trip signals initiate a reactor trip"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:19-21
	highPzrPressureTrip := pzrPressure > RPSHighPressurizerPressureTrip

	// Declare reactor trip when 2/4 voting threshold is met
	// OR when the operator has pressed manual trip.
	// "The RPS provides for manual initiation of reactor trip
	// by operator action in the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
	if ret.ReactorTripped {
		// Latched; stays tripped until manually reset.
	} else if ret.TripSignalCount >= RPSTripVotingThreshold ||
		ret.ManualTrip ||
		lowFlowTrip ||
		ret.SafetyInjectionActuated ||
		(turbineTripped && atPowerTripsEnabled) ||
		highFluxTrip ||
		highPzrPressureTrip {
		// An SI signal votes the trip directly: "Safety injection
		// signal - Actuation of the Safety Injection System
		// simultaneously initiates a reactor trip". The latch read
		// here is the previous step's, so the trip lands within one
		// timestep of the actuation.
		// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:38-40
		// A turbine-generator trip above the permissive initiates
		// the reactor trip outright - same direct-initiation class.
		// With the condenser gone as a heat sink, only the dump
		// banks remain for full-power production.
		//   "Turbine-generator trip - Trip of the turbinegenerator
		//    above a certain power level initiates a reactor trip"
		//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:33-34
		ret.ReactorTripped = true
	}

	// ESFAS outputs per Table 7-1. Each output latches on the
	// first step the triggering condition is met and stays
	// active; operator reset clears them.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81

	// Safety Injection: low pressurizer pressure, high
	// containment pressure, low steamline pressure, low Tcold,
	// manual SI - the Table 7-1 order. Like low Tcold, the
	// low-pressurizer-pressure channel sits behind the at-power
	// permissive so planned cooldowns and cold-condition lineups
	// do not actuate ESFAS through it (the operator block arms
	// before reaching setpoints during deliberate cooldowns:
	// specs/the_westinghouse_pressurized_water_react/
	// chapter_16/page_227.txt:108-110).
	// The manual entry is the dedicated manual-SI pushbutton of
	// Table 7-1, not the manual reactor trip: a plain reactor trip
	// "avoids unnecessary actuation of engineered safety features"
	// (specs/the_westinghouse_pressurized_water_react/
	// chapter_7/page_138.txt:11-13).
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-15
	if !ret.SafetyInjectionActuated && !ret.SafetyInjectionBlocked {
		if pzrPressure < RPSLowPressurizerPressureTrip && atPowerTripsEnabled ||
			containmentPressureGauge > RPSHighContainmentPressureTrip ||
			steamlinePressure < RPSLowSteamlinePressureTrip ||
			(tcoldF < RPSLowTcoldTrip && atPowerTripsEnabled) ||
			ret.ManualSI {
			ret.SafetyInjectionActuated = true
		}
	}

	// Steamline Isolation: same triggers plus high
	// steam-pressure rate of change (not modelled at this
	// fidelity); "Low pressurizer pressure" heads this Table
	// 7-1 list as well (chapter_7/page_140.txt:25-31). The
	// low-steamline trigger carries its own planned-cooldown
	// block, matching the SI block: an intentional
	// atmospheric-dump cooldown legitimately carries the header
	// through the shared setpoint. Containment-pressure and
	// manual triggers stay live while the block is armed — a
	// block gates its own consumer only.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:107-110
	if !ret.SteamlineIsolationActuated {
		lowSteamline := steamlinePressure < RPSLowSteamlinePressureTrip &&
			!ret.SteamlineIsolationBlocked
		lowPzr := pzrPressure < RPSLowPressurizerPressureTrip &&
			atPowerTripsEnabled
		if lowPzr ||
			containmentPressureGauge > RPSHighContainmentPressureTrip ||
			lowSteamline ||
			(tcoldF < RPSLowTcoldTrip && atPowerTripsEnabled) ||
			ret.ManualSI {
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
	// manual. No dedicated Phase-A pushbutton is modeled; the
	// manual-ESFAS input carries its Table 7-1 "Manual" entry.
	if !ret.ContainmentIsolationPhaseAActuated {
		if ret.SafetyInjectionActuated || ret.ManualSI {
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
	// level. The low-level start stays live below the at-power
	// permissive: auxiliary feedwater carries decay heat removal
	// in every mode, unlike the at-power protection channels.
	if !ret.AuxiliaryFeedPumpsStartActuated {
		if ret.SafetyInjectionActuated ||
			sgLevelPct < RPSLowSteamGeneratorLevelTrip {
			ret.AuxiliaryFeedPumpsStartActuated = true
		}
	}

	// Feedwater Isolation: high SG level, safety injection, or
	// coincidence of reactor trip and low Tcold. The high-level
	// trigger sits behind the at-power permissive like its voting
	// channel: wet layup fills the shells to the vents below it
	// (specs/the_westinghouse_pressurized_water_react/
	// chapter_16/page_228.txt:53-55).
	if !ret.FeedwaterIsolationActuated {
		if sgLevelPct > RPSHighSteamGeneratorLevelTrip && atPowerTripsEnabled ||
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
	// The high-level trigger follows its voting channel behind the
	// at-power permissive for the same wet-layup reason.
	if !ret.TurbineTripActuated {
		if ret.ReactorTripped ||
			(sgLevelPct > RPSHighSteamGeneratorLevelTrip && atPowerTripsEnabled) ||
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
	r.ManualSI = false
	r.SafetyInjectionActuated = false
	r.SteamlineIsolationActuated = false
	r.ContainmentSprayActuated = false
	r.ContainmentIsolationPhaseAActuated = false
	r.ContainmentIsolationPhaseBActuated = false
	r.AuxiliaryFeedPumpsStartActuated = false
	r.FeedwaterIsolationActuated = false
	r.TurbineTripActuated = false
}
