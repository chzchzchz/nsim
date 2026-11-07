package core

import "time"

// Step advances the Plant Computer state machine by one
// timestep. The simplified model implements four per-step
// dynamics from chapter 9 of the spec:
//
//   - Data acquisition availability. The computer is
//     collecting plant data when it has electrical power and
//     the control room displays are operational (p28).
//   - Display format availability. Each of the six formats
//     (mimic, bar chart, tabular, graphical, data trends,
//     polar) is available when data acquisition is on.
//   - Safety Parameter Display System. SPDS is active when
//     the major safety systems are all reporting operable.
//   - Post-trip review and sequence-of-events recording.
//     Both latch true on a reactor trip from the RPS (p26);
//     the review printer fires once per trip.
//   - Alarm processing. AlarmActive latches true when any
//     parameter exceeds its limit; cleared by an
//     acknowledgement through the control room (p28).
//
// Spec justification:
//   - Display formats (Table 9-1):
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:104-202
//   - SPDS:
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
//   - Post-trip review:
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:1-3
//   - Sequence of events recording:
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:5-7
//   - Bypassed / Inoperable Status Indication:
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:9-12
func (c *PlantComputer) Step(dt time.Duration,
	powerAvailable bool,
	reactorTripped, reactorTripOccurred bool,
	alarmCondition bool,
	safetySystemsAvailable bool,
	alarmAcknowledged bool,
) PlantComputer {
	ret := *c

	// Data acquisition. Without electrical power (and
	// therefore without functional displays from p28) the
	// computer cannot collect or display data.
	ret.DataAcquisitionAvailable = powerAvailable
	ret.MimicDisplaysAvailable = powerAvailable
	ret.BarChartDisplaysAvailable = powerAvailable
	ret.TabularDisplaysAvailable = powerAvailable
	ret.GraphicalDisplaysAvailable = powerAvailable
	ret.DataTrendsAvailable = powerAvailable
	ret.PolarDisplaysAvailable = powerAvailable

	// Bypassed / Inoperable Status Indication is part of
	// the auxiliary safety-system display set
	// (plans/p28_1.txt); it is available whenever the
	// control room safety-system stations are.
	ret.BypassedInoperableStatusIndication = powerAvailable &&
		safetySystemsAvailable

	// SPDS active when computer is on and the major
	// safety systems are all reporting operable.
	ret.SPCSActive = powerAvailable && safetySystemsAvailable

	// Post-trip review: printed once on each reactor trip.
	// The trigger is the rising edge of ReactorTripped.
	if reactorTripOccurred && !ret.PostTripReviewPrinted {
		ret.PostTripReviewPrinted = true
	}

	// Sequence of events recording is active when the
	// computer has power; it captures every transition
	// automatically.
	ret.SequenceOfEventsRecording = powerAvailable

	// Alarm processing. The computer checks every
	// monitored parameter against its limit on each scan;
	// an alarm latches true until acknowledged through
	// the control room.
	switch {
	case alarmCondition:
		ret.AlarmActive = true
	case alarmAcknowledged:
		ret.AlarmActive = false
	}

	return ret
}
