package core

import (
	u "github.com/bcicen/go-units"
	"time"
)

// Step advances the Containment Spray System by one timestep.
// The step function implements the four state machines described
// in the spec:
//
//  1. CSST / SAT depletion. During the injection phase the
//     CSST drains at the design flow rate when the spray
//     header is open, or at a reduced rate when the header is
//     closed (miniflow recirc). The SAT drains proportionally
//     with the CSST so the spray additive ratio is preserved.
//  2. Sump pH adjustment. When CSSTLevel falls below the
//     "Lo-Lo" setpoint, SumpPHAdjusted latches true; the long-
//     term containment sump pH is adequate for the
//     recirculation phase.
//  3. Recirculation switchover. When CSSTLevel drops below the
//     Lo-Lo setpoint the operator switches to sump suction
//     (SuctionSump = true). Additive is no longer injected in
//     this mode.
//
// The auto-start decision (turning Running on, opening the spray
// header) is made by the caller in Plant.Step, where the
// containment pressure and the diesel state are available. This
// step assumes Running has already been set by the caller.
//
// Spec justification:
//   - Actuation: "Initial Injection Mode – Containment spray is
//     initiated automatically in response to a containment high
//     pressure signal. This signal starts the spray pumps,
//     opens the spray header isolation valves "A", and opens
//     the SAT isolation valves "B"."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
//   - CSST + SAT suction: "Suction is provided to the spray
//     pump from the CSST, with the SAT providing sodium
//     hydroxide solution. The SAT drains into each spray pump's
//     suction header from the CSST."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:61-64
//   - Additive duration: "(T)he storage tanks are sized to
//     provide approximately 30 minutes of spray with spray
//     additive."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-18
//   - Switchover: "when the CSST is drained to the "Lo-Lo"
//     setpoint, the spray pumps are manually switched to the
//     recirculation mode of spray operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:18-21
//   - Sump pH adjustment: "An additional function which is
//     performed by the CSS during its normal operation is the
//     pH adjustment of the containment sump. The sodium
//     hydroxide (spray additive) that is delivered through the
//     injection phase is sufficient to raise the long-term
//     sump pH to a level adequate for inhibiting chloride
//     stress corrosion/cracking of stainless steel components,
//     and for keeping the iodine entrained in the water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:37-49
//   - Pump miniflow line: "A pump miniflow line, containing a
//     heat exchanger and flow limiting orifice."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:20-22
func (c *ContainmentSpraySystem) Step(dt time.Duration) ContainmentSpraySystem {
	ret := *c
	dtSec := dt.Seconds()

	// Chloride SCC risk: kind (c) derived risk condition,
	// recomputed at the TOP of Step, before the early return, so
	// the sensor cannot go stale when the spray pumps are
	// secured (Running=false) or the bus is dead (DriveReady=false).
	// plans/p72_5.txt Chemistry drivers.
	ret.ChlorideSCCRisk = locaPressure(&ret) && !(ret.SATIsolationValveBOpen && ret.SATLevel.Float() > 0)

	// DriveReady reflects whether the ESF bus that feeds
	// the spray pumps is energised. Without bus power the
	// pumps cannot deliver spray even if Running was latched
	// by the high-pressure auto-start.
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
	if !ret.Running || !ret.DriveReady {
		ret.SprayFlow = 0
		return ret
	}

	// Compute the net CSST drain rate. When the spray header is
	// open the full design flow exits to the containment; when
	// closed the miniflow line recirculates a fraction back to
	// the CSST so the net loss is reduced (the pump does not
	// deadhead).
	flowGpm := CSSDesignFlow
	if !ret.SprayHeaderOpen {
		flowGpm *= 1.0 - CSSMiniflowFraction
	}

	// Drain the CSST (and SAT proportionally) at the chosen
	// rate. After switchover to sump suction, the CSST no
	// longer drains (the pumps draw from the containment sump).
	if !ret.SuctionSump {
		dV := flowGpm * (dtSec / 60.0)
		newCSST := ret.CSSTLevel.Float() - dV
		newCSST = max(newCSST, 0)
		ret.CSSTLevel = Volume{Val: newCSST, Unit: u.Gallon}
		// The SAT only feeds the pump suction headers through
		// the SAT isolation valves "B"; with them closed the
		// pumps spray borated water without additive.
		// "opens the SAT isolation valves "B""
		// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:60-61
		if ret.SATIsolationValveBOpen {
			ratio := SATCapacity.Float() / CSSTCapacity.Float()
			newSAT := ret.SATLevel.Float() - dV*ratio
			newSAT = max(newSAT, 0)
			ret.SATLevel = Volume{Val: newSAT, Unit: u.Gallon}
		}

		// Sump pH adjustment latches once the CSST drains below
		// the Lo-Lo setpoint. The simplified model only tracks
		// the boolean; the actual pH value is not modelled.
		//
		// Spec justification:
		//   - "the sodium hydroxide (spray additive) that is
		//     delivered through the injection phase is sufficient
		//     to raise the long-term sump pH to a level adequate
		//     for inhibiting chloride stress corrosion/cracking
		//     of stainless steel components, and for keeping the
		//     iodine entrained in the water."
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:40-49
		// The pH credit is tied to additive actually delivered:
		// with the SAT isolated by valves "B" no sodium hydroxide
		// enters the spray, so the consequence cannot latch.
		if ret.CSSTLevel.Float() < CSSLowLevelSetpoint &&
			ret.SATIsolationValveBOpen {
			ret.SumpPHAdjusted = true
		}
		// Recirculation switchover is deliberately NOT automatic:
		// "the spray pumps are manually switched to the recirculation
		// mode of spray operation" (chapter_6/page_115.txt:18-21),
		// via sump isolation valves C open and suction isolation
		// valves D closed (page_115.txt:63-70). The operator performs
		// it through the SuctionSump control once the Lo-Lo cue
		// arrives; injection continues until then.
	}

	// SprayFlow tracks the per-second delivery rate for
	// downstream consumers (containment atmosphere, if a
	// ContainmentBuilding model exists). Zero when the header
	// is closed or pumps are off; CSSDesignFlow otherwise. In
	// recirculation mode the header is still open (the spray
	// pumps deliver from the sump to the ring headers), so
	// SprayFlow remains at the design rate.
	if ret.SprayHeaderOpen {
		ret.SprayFlow = CSSDesignFlow / 60.0
	} else {
		ret.SprayFlow = 0
	}

	return ret
}

// containmentPressure reports the absolute containment pressure
// feeding the LOCA predicate. ContainmentSpraySystem does not carry
// containment pressure; the CSS step reads it from the ambient
// field synced by Plant.Step (ContainmentPressure). The field is
// injected via NextContainmentPressure before the CSS step.
func (c *ContainmentSpraySystem) containmentPressure() Pressure {
	return c.NextContainmentPressure
}

// locaPressure reports whether containment pressure is above the
// CSS auto-start threshold (AtmosphericPressure +
// CSSContainmentPressureSetpoint), i.e. a LOCA exists. Shared by
// the early-return guard and the end-of-Step recompute so the two
// paths cannot diverge.
func locaPressure(c *ContainmentSpraySystem) bool {
	return c.containmentPressure() > AtmosphericPressure+CSSContainmentPressureSetpoint
}
