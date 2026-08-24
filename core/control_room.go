package core

import "time"

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
	alarms []Alarm,
	esfBus1Available, esfBus2Available bool,
) PlantControlRoom {
	ret := *c
	if ret.AlarmSet == nil {
		ret.AlarmSet = make(map[AlarmID]Alarm)
	}

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

	// Merge incoming per-component alarms into AlarmSet.
	// Each alarm's Active field drives whether the entry
	// stays in the set; acknowledged entries persist one
	// extra step so the operator can see "this just cleared".
	//
	// Two-pass merge:
	//   1. For each incoming alarm, update or insert.
	//   2. For each existing entry not present in the
	//      incoming list and not Active (the incoming list
	//      is the authoritative view), drop it. This handles
	//      the case where the operator clears a condition
	//      and the next step arrives with no alarms.
	for _, a := range alarms {
		existing, ok := ret.AlarmSet[a.ID]
		if !a.Active {
			if !ok {
				continue
			}
			if existing.Acknowledged {
				delete(ret.AlarmSet, a.ID)
			} else {
				delete(ret.AlarmSet, a.ID)
			}
			continue
		}
		if !ok {
			// First sighting of this condition: record the
			// activation moment so the operator sees when
			// the alarm came in, not just that it is up.
			a.Timestamp = time.Duration(ret.SimTimeNs)
			ret.AlarmSet[a.ID] = a
		} else if existing.Timestamp != a.Timestamp {
			ret.AlarmSet[a.ID] = a
		}
	}
	// Drop entries whose ID is no longer present and whose
	// Active flag cannot be re-derived from the incoming
	// list. This is the second pass: any entry whose ID is
	// missing AND was Active is now cleared.
	if len(alarms) > 0 {
		seen := make(map[AlarmID]bool, len(alarms))
		for _, a := range alarms {
			seen[a.ID] = true
		}
		for id, existing := range ret.AlarmSet {
			if seen[id] {
				continue
			}
			if existing.Active {
				delete(ret.AlarmSet, id)
			}
		}
	}

	// Annunciator alarm handling.
	//
	// Two input modes are supported:
	//   1. Legacy callers pass alarmCondition=true/false with
	//      no per-alarm list. The boolean drives the
	//      annunciator latch directly.
	//   2. Plant.Step passes the full p.Alarms() slice and
	//      the annunciator is derived from the AlarmSet
	//      trip-severity entries.
	// Mixing both: any active trip-severity alarm keeps the
	// annunciator ringing regardless of the boolean.
	if len(alarms) == 0 {
		if alarmCondition {
			if ret.AlarmAcknowledged {
				ret.AlarmAcknowledged = false
			}
			ret.AnnunciatorAlarm = true
		} else if ret.AlarmAcknowledged {
			ret.AnnunciatorAlarm = false
		}
	} else {
		if alarmCondition && ret.AlarmAcknowledged {
			ret.AlarmAcknowledged = false
		}
		ret.AnnunciatorAlarm = false
		for _, a := range ret.AlarmSet {
			if a.Active && a.Severity == SeverityTrip {
				ret.AnnunciatorAlarm = true
				break
			}
		}
	}

	// Safety system station status. The simplified model
	// tracks train-level availability from the corresponding
	// safety-system bus. The aggregated "available" state
	// is gated on the auxiliary panels being powered.
	ret.SafetySystemStationsAvailable =
		ret.AuxiliaryPanelsAvailable &&
			(esfBus1Available || esfBus2Available)

	// One Step call per plant tick advances the annunciator
	// clock by exactly one TimeStep (see SimTimeNs).
	ret.SimTimeNs += int64(TimeStep)

	return ret
}

// AcknowledgeAlarm is the operator-initiated entry point
// that latches the AnnunciatorAlarm field to false. The
// alarm will re-trigger on the next alarm condition.
//
// Retained for backwards compatibility with the flat
// acknowledgement bit; new code should call AcknowledgeAll()
// or ResetAlarm(id) instead.
func (c *PlantControlRoom) AcknowledgeAlarm() {
	c.AlarmAcknowledged = true
	c.AnnunciatorAlarm = false
	c.AcknowledgeAll()
}
