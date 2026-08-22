package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the Circulating Water System state machine by
// one timestep. The simplified model implements the spec's
// lumped flow-source + approach-temperature architecture
// from chapter 12.1:
//
//   - When a CW pump is running the system delivers
//     PumpsInService × CirculatingWaterPumpFlowGPM total flow
//     to the condenser. With no pumps online, FlowGPM is
//     zero and the condenser has no heat sink.
//   - The inlet temperature is the source temperature
//     (river / lake / ocean) directly, or — when
//     SourceIsCoolingTower is true and the tower fan is
//     running — source + CoolingTowerApproachF (the cold-
//     basin temperature relative to ambient wet-bulb).
//   - The outlet temperature is computed by the condenser
//     Step based on actual heat duty; this method
//     initialises OutletTemp to InletTemp so the struct is
//     always in a self-consistent state.
//   - When driveReady is false (the plant distribution bus
//     feeding the CW pump motors is de-energised) the system
//     produces zero flow even if pumpRunning was latched.
//     The latch is preserved so flow recovers automatically
//     when power returns. See plans/x/uat_sat_gating.md.
//
// Spec justification:
//   - CWS mission: "major heat sink for the power plant by
//     providing large amounts of cooling water for the main
//     condensers."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
//   - Pump type and flow path: "vertical, mixed flow, volute
//     type circulating water pumps. The pumps take suction
//     from their individual intake bays downstream of the
//     traveling screens, and discharge water through
//     motor-operated valves into individual discharge lines
//     which combine into a common discharge line from the
//     main condenser."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:56-64
//   - Source diversity: "Depending on the plant, the source
//     of circulating water may be a river, lake, ocean, or
//     cooling tower."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
func (c *CirculatingWaterSystem) Step(
	pumpRunning bool,
	driveReady bool,
	numPumps int,
	sourceTemp Temperature,
	coolingTower *CoolingTower,
) CirculatingWaterSystem {
	ret := *c

	if pumpRunning && driveReady && numPumps > 0 {
		ret.PumpsInService = numPumps
		ret.FlowGPM = float64(numPumps) * CirculatingWaterPumpFlowGPM
	} else {
		ret.PumpsInService = 0
		ret.FlowGPM = 0
	}

	ret.InletTemp = sourceTemp
	if ret.SourceIsCoolingTower &&
		coolingTower != nil &&
		coolingTower.FanRunning {
		// Cooling tower returns water at
		// (wet-bulb + approach). The simplified model
		// treats the "source temp" passed to this Step
		// as the ambient wet-bulb.
		approach := coolingTower.ApproachF
		if approach <= 0 {
			approach = CoolingTowerApproachF
		}
		ret.InletTemp = u.NewValue(
			sourceTemp.Float()+approach, u.Fahrenheit,
		)
	}

	// The condenser Step will overwrite OutletTemp with the
	// actual heat-balance result; initialise here so the
	// struct is always in a self-consistent state when read
	// before the condenser has run.
	ret.OutletTemp = ret.InletTemp

	return ret
}

// Step advances the CoolingTower state machine by one
// timestep. The simplified model tracks only the fan
// running flag; fill hydraulics, fan curves, and drift loss
// are not modelled. The tower fan shares the same upstream
// power bus as the CW pump motors, so driveReady is the same
// readiness signal (plans/x/uat_sat_gating.md).
//
// Spec justification:
//   - Cooling tower is one of the four CW sources listed in
//     the spec: "river, lake, ocean, or cooling tower."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
func (ct *CoolingTower) Step(fanRunning, driveReady bool) CoolingTower {
	ret := *ct
	ret.FanRunning = fanRunning && driveReady
	if ret.ApproachF <= 0 {
		ret.ApproachF = CoolingTowerApproachF
	}
	return ret
}
