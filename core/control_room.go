package core

// Step advances the Plant Control Room state machine by one
// timestep. The simplified model implements four per-step
// dynamics from chapter 8 of the spec:
//
//   - Display and control availability. The control room
//     requires electrical power from the plant electrical
//     system (p27). When power is available the PCC,
//     auxiliary panels, graphic and dedicated displays,
//     control inputs, and safety-system stations are all
//     operational; otherwise they are not.
//   - Annunciator alarm. A new alarm condition (any
//     monitored plant variable exceeding its setpoint)
//     latches AnnunciatorAlarm = true until the operator
//     acknowledges it. Once acknowledged the alarm clears;
//     a new alarm condition after acknowledgement
//     re-triggers it.
//   - Safety system station status. Each auxiliary panel
//     station displays the status of its assigned train
//     (ESF bus A/B + safety system train A/B).
//   - Plant state aggregation. The control room does not
//     modify plant physics; it only mirrors availability
//     and status flags from each system for display.
//
// Spec justification:
//   - Power dependency: "No single failure in the power
//     distribution system will result in degradation of the
//     control area functions, except for ancillary or
//     nonessential performance features whose failure would
//     not decrease plant availability."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_146.txt:11-16
//   - Alarm acknowledgement: "Turned off by operator
//     acknowledgement."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
//   - Auxiliary safety stations: "arranged in stations each
//     dedicated to a single train of safeguard systems."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:65-67
func (c *PlantControlRoom) Step(
	alarmCondition bool,
	esfBus1Available, esfBus2Available bool,
) PlantControlRoom {
	ret := *c

	// Power-gated availability. When the control room has
	// electrical power all six operator-interface subsystems
	// are operational; without power every display and
	// control is dark.
	if ret.PowerAvailable {
		ret.PCCAvailable = true
		ret.AuxiliaryPanelsAvailable = true
		ret.GraphicDisplaysAvailable = true
		ret.DedicatedDisplaysAvailable = true
		ret.ControlInputsAvailable = true
		ret.SafetySystemStationsAvailable = true
	} else {
		ret.PCCAvailable = false
		ret.AuxiliaryPanelsAvailable = false
		ret.GraphicDisplaysAvailable = false
		ret.DedicatedDisplaysAvailable = false
		ret.ControlInputsAvailable = false
		ret.SafetySystemStationsAvailable = false
	}

	// Annunciator alarm handling. The alarm latches when
	// the monitored condition is true and not yet
	// acknowledged; an acknowledgement clears the alarm
	// unless the condition persists.
	if alarmCondition {
		// A new alarm condition after acknowledgement
		// re-triggers the alarm (the operator must
		// re-acknowledge the new condition).
		if ret.AlarmAcknowledged {
			ret.AlarmAcknowledged = false
		}
		ret.AnnunciatorAlarm = true
	} else if ret.AlarmAcknowledged {
		// Operator acknowledged and the condition has
		// cleared.
		ret.AnnunciatorAlarm = false
	}

	// Safety system station status. The simplified model
	// tracks train-level availability from the corresponding
	// safety-system bus. The aggregated "available" state
	// is gated on the auxiliary panels being powered.
	ret.SafetySystemStationsAvailable =
		ret.AuxiliaryPanelsAvailable &&
			(esfBus1Available || esfBus2Available)

	return ret
}

// AcknowledgeAlarm is the operator-initiated entry point
// that latches the AnnunciatorAlarm field to false. The
// alarm will re-trigger on the next alarm condition.
func (c *PlantControlRoom) AcknowledgeAlarm() {
	c.AlarmAcknowledged = true
	c.AnnunciatorAlarm = false
}
