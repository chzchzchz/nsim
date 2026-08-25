package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

// Step advances the Emergency Feedwater System by one timestep.
// The step function implements four state machines that run in
// parallel for each of the two subsystems (A and B):
//
//   1. Tank depletion: the subsystem's primary tank level
//      decreases by exactly the flow its pumps deliver to the
//      steam generators (valve-scaled, gated by Running &&
//      DriveReady), so secondary-side inventory is conserved.
//   2. SG feedwater delivery: the total EFWS feedwater flow
//      equals the sum of all running pumps' rated flows, gated
//      by each pump's DriveReady.
//   3. Secured condition: once the primary system hot-leg
//      temperature falls to EFWSSecureTemp and the RHRS has
//      taken over, the EFWS is secured and all pump Running
//      flags are cleared.
//   4. DriveReady for the turbine-driven pump: the turbine
//      pump's DriveReady tracks SG pressure (set in
//      Plant.Step); the SteamAdmissionValve is fail-open when
//      SG pressure is present.
//
// Spec justification:
//   - Tank sizing language: "Each tank contains a quantity of
//     condensate quality water sufficient to allow the plant to
//     be maintained in hot standby for 13 hours then allow a
//     5-hour cooldown of the plant to 350°F (176.7°C). The
//     maximum permissible water temperature in the primary
//     emergency feedwater supply tank is 120°F (48.9°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-59
//   - Performance requirement: "supply at least the minimum
//     required flow, within one minute of the actuation signal,
//     to at least two effective steam generators [...], and to
//     continue this delivery for an indefinite period without
//     operator action."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:23-28
//   - EFWS secured: "the reactor coolant hot leg temperature
//     is reduced to 350°F (176.7°C) and the RHRS has been
//     activated, the EFWS is secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
//   - Long-form restatement of secured condition:
//     "the EFWS continues to supply feedwater to the steam
//     generators throughout the cooldown until the primary
//     system hot leg temperature is reduced to 350°F (176.7°C).
//     At this temperature, the Residual Heat Removal System
//     (RHRS) is activated and the EFWS is secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:91-96
//   - Flow-modulating valves full-open assumption: "it is
//     assumed that no operator action can be taken for 30
//     minutes, and for this period it is assumed that these
//     valves will be full open."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:36-39
func (e *EFWS) Step(
	rpvOutlet Temperature,
	rhrsActive bool,
) EFWS {
	ret := *e
	dt := TimeStep

	// Secured condition: when the RCS hot-leg temperature has
	// been reduced to EFWSSecureTemp and the RHRS has taken
	// over, the EFWS is no longer needed and is secured. The
	// spec names the system, not a start mode, so manually
	// started pumps secure exactly like autostarted ones.
	//
	// Spec justification:
	//   - "the reactor coolant hot leg temperature is reduced
	//     to 350°F (176.7°C) and the RHRS has been activated,
	//     the EFWS is secured."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
	if rpvOutlet.Float() <= EFWSSecureTemp && rhrsActive {
		ret.SubsystemA.MotorPump.Running = false
			ret.SubsystemA.TurbinePump.Running = false
			ret.SubsystemB.MotorPump.Running = false
			ret.SubsystemB.TurbinePump.Running = false
		ret.Active = false
		ret.AutoStartElapsed = 0
		ret.DeliveredFlow = Volume{Val: 0, Unit: u.Gallon}
		return ret
	}

	// Manual operator starts (the bridge MotorPumpRunning /
	// TurbinePumpRunning controls) deliver water exactly like an
	// autostart: tank depletion below runs whenever a pump is
	// running, Active or not. Active stays the auto-start latch.
	// The pumps are manually started with manually controlled
	// flowrates (OCR breaks this sentence across the page break;
	// paraphrased per plans/CHECK_SKILL.md citation rule).
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18

	// Tank depletion. The tank discharges exactly what its
	// pumps deliver to the SG header this step: rated flow
	// scaled by each pump's FlowModulatingValve position and
	// gated by Running && DriveReady, capped by the water left
	// in the tank. Undelivered pump discharge recirculates to
	// the tank through the orificed line, so net draw equals
	// delivery.
	//
	// Spec justification:
	//   - "In operation, the emergency feedwater pumps take
	//     suction from the primary emergency feedwater tanks and
	//     discharge the water into the main feedwater piping
	//     [...]. Each pump is provided with an orificed
	//     recirculation line leading back to the primary
	//     emergency feedwater tank."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:12-21
	//   - "Each tank contains a quantity of condensate quality
	//     water sufficient to allow the plant to be maintained
	//     in hot standby for 13 hours then allow a 5-hour
	//     cooldown of the plant to 350°F (176.7°C)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-56
	drawA, subA := drainEFWSTank(ret.SubsystemA, dt)
	ret.SubsystemA = subA
	drawB, subB := drainEFWSTank(ret.SubsystemB, dt)
	ret.SubsystemB = subB
	ret.DeliveredFlow = Volume{
		Val: (drawA + drawB) / dt.Seconds(), Unit: u.Gallon}

	if ret.Active {
		ret.AutoStartElapsed += dt
	}

	return ret
}

// ApplyModulatingValveLineup positions the four discharge flow
// modulating valves for the current actuation phase:
//
//  1. Inside the assumed no-operator-action window after an
//     automatic start the valves are full open.
//  2. Afterwards they hold the steam generator level inside a
//     narrow band below the high-level trip (on-off with
//     hysteresis; between bands the previous position stands).
//  3. Manual starts and the secured state leave the valves at
//     their operator-positioned values.
//
// "When operator action can be taken (after an assumed 30-minute
// delay), the emergency feedwater flow is adjusted by positioning
// the flow modulating valves to restore and maintain the steam
// generator water levels within the narrow control range." ...
// "it is assumed that no operator action can be taken for 30
// minutes, and for this period it is assumed that these valves
// will be full open."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:29-39
func (e *EFWS) ApplyModulatingValveLineup(sgLevelPct float64) {
	valves := []*Valve{
		&e.SubsystemA.MotorPump.FlowModulatingValve,
		&e.SubsystemA.TurbinePump.FlowModulatingValve,
		&e.SubsystemB.MotorPump.FlowModulatingValve,
		&e.SubsystemB.TurbinePump.FlowModulatingValve,
	}
	switch {
	case e.Active && e.AutoStartElapsed < EFWSFullOpenPeriod:
		for _, v := range valves {
			v.Open = 1.0
		}
	case e.Active:
		demand := 1.0
		if sgLevelPct >= EFWSLevelControlClose {
			demand = 0.0
		} else if sgLevelPct > EFWSLevelControlReopen {
			return
		}
		for _, v := range valves {
			v.Open = demand
		}
	}
}

// drainEFWSTank debits the subsystem's primary tank by the flow
// its pumps actually deliver to the SG header this step: rated
// discharge scaled by the flow-modulating valve position, gated
// by Running && DriveReady, and capped by the water remaining in
// the tank. It returns the volume drawn so the caller can report
// it as the SG-side delivery rate.
//
// Spec justification:
//   - Suction/discharge/recirculation path: "the emergency
//     feedwater pumps take suction from the primary emergency
//     feedwater tanks and discharge the water into the main
//     feedwater piping [...] Each pump is provided with an
//     orificed recirculation line leading back to the primary
//     emergency feedwater tank. This line provides recirculation
//     flow any time the pumps are operating."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:12-21
//   - Operator-set rates: "the pumps are manually started and
//     the flowrates are manually [controlled by positioning the
//     flow modulating valves.]"
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
func drainEFWSTank(sub EFWSSubsystem, dt time.Duration) (float64, EFWSSubsystem) {
	draw := min(sub.EFWSSubsystemFlow()*dt.Seconds(),
		sub.Tank.Level.Float())
	if draw > 0 {
		sub.Tank.Level = Volume{
			Val: sub.Tank.Level.Float() - draw, Unit: u.Gallon}
	}
	return draw, sub
}

// EFWSSubsystemFlow returns the SG-side feedwater flow rate
// for one EFWS subsystem (gal/s). The flow is the sum of all
// running pumps' rated flows, gated by DriveReady. The motor-
// driven pump is gated by the ESF bus (DieselGenA.Energized
// path through the existing AFWPump.DriveReady wiring in
// Plant.Step); the turbine-driven pump is gated by SG pressure
// via its SteamAdmissionValve.
//
// Spec justification:
//   - "Normally open, fail-open, air-operated, flow-modulating
//     valves are located in each pump discharge line."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:1-2
//   - "These valves will be normally full open when the system
//     is activated."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:2-3
//   - Fail-open steam admission valve: "arranged to fail-open
//     on loss of air or electrical power."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:46-47
func (sub *EFWSSubsystem) EFWSSubsystemFlow() float64 {
	var total float64
	if sub.MotorPump.Running && sub.MotorPump.DriveReady {
		total += EFWSMotorPumpFlow.Float() *
			sub.MotorPump.FlowModulatingValve.Open
	}
	if sub.TurbinePump.Running && sub.TurbinePump.DriveReady {
		total += EFWSTurbinePumpFlow.Float() *
			sub.TurbinePump.FlowModulatingValve.Open
	}
	return total
}

// EFWSFeedwaterFlow reports the total SG-side feedwater flow
// rate integrated by the most recent Step (gal/s). The value is
// the same number that debited the primary tanks, so crediting
// it to the SG inventory conserves secondary-side mass; a dry
// tank reports zero. Manual operator starts count: only the
// pump Running + DriveReady states and the modulating-valve
// positions gate the flow, not the autostart latch.
//
// Spec justification:
//   - "supply at least the minimum required flow, within one
//     minute of the actuation signal, to at least two effective
//     steam generators"
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:23-26
func (e *EFWS) EFWSFeedwaterFlow() Volume {
	return e.DeliveredFlow
}
