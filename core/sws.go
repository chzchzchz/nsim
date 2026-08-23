package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the Service Water System by one timestep. The
// step function implements three state machines:
//
//   1. Pump running state. While PumpRunning is true, the lumped
//      flow equals ServiceWaterPumpFlow × number of running
//      pumps (capped at ServiceWaterEssentialPumpCount). The
//      simplified model collapses the three physical pumps into
//      a single boolean.
//   2. Outlet temperature. The mixed service water outlet
//      temperature rises above the source by the heat load
//      divided by the per-second water mass flow, capped at the
//      design rise to prevent instability at large timesteps.
//   3. Nonessential isolation. The NonessentialIsolated flag is
//      set by the operator during accident conditions to
//      conserve water for safety-grade loads.
//
// Spec justification:
//   - "The Service Water System supplies all the cooling water
//     for the plant, including that required for emergency
//     shutdown. Consequently, high reliability and redundancy
//     is required."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
//   - "The three essential service water pumps for each unit
//     are typically rated at approximately 20,000 gpm."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
//   - "The nonessential equipment coolers are fed from separate
//     branch headers that can be isolated from the main headers
//     under accident conditions to conserve water."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:23-26
func (s *ServiceWaterSystem) Step(heatLoad Watts) ServiceWaterSystem {
	ret := *s

	// DriveReady reflects whether the 6600V ESF bus that
	// feeds the SWS pump motors is energised. Without bus
	// power the pumps spin down to zero flow even if
	// PumpRunning was latched by the safety-injection auto-
	// start; the latch is preserved so the operator can
	// recover flow once power returns.
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
	if !ret.PumpRunning || !ret.DriveReady {
		ret.Flow = 0
		ret.OutletTemp = ret.InletTemp
		return ret
	}

	ret.Flow = ServiceWaterPumpFlow *
		float64(ServiceWaterEssentialPumpCount)
	ret.Flow = min(ret.Flow, ServiceWaterDesignFlow)

	// Outlet temperature: source temperature plus the heat-
	// load-driven rise. Mass flow in kg/s = gpm × 0.0631.
	massKgPerS := ret.Flow * 0.0631
	cpWater := 4186.0
	if massKgPerS > 0 && float64(heatLoad) > 0 {
		deltaF := float64(heatLoad) / (massKgPerS * cpWater) *
			9.0 / 5.0
		deltaF = max(deltaF, 0)
		ret.OutletTemp = Temperature{Val: ret.InletTemp.Float()+deltaF, Unit: u.Fahrenheit}
	} else {
		ret.OutletTemp = ret.InletTemp
	}

	return ret
}

// Step advances the Spent Fuel Pool Cooling System by one
// timestep. The step function implements three state
// machines:
//
//   1. Header pressure. With two pumps running the header
//      pressure is in the 55-75 psig normal range. The
//      simplified model holds the header pressure constant
//      at SFPCPressureNormalMin during normal operation.
//   2. Standby pump auto-start. When the header pressure drops
//      below SFPCStandbyStartPressure (50 psig), the standby
//      pump starts.
//   3. Pool temperature. The pool temperature rises with
//      residual decay heat and drops with SFPCS cooling. The
//      simplified model holds PoolTemp at the design value
//      while both pumps are running.
//
// Spec justification:
//   - "Normal operation requires two pumps with the third pump
//     serving as a standby."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
//   - "The system pressure is maintained at 55 to 75 psig (4.8
//     to 6.2 bar) in the main supply header and the third pump
//     automatically starts at a header pressure of 50 psig."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-48
//   - "passing approximately 100 gpm (23 m3/hr) of each loop's
//     flow through a filter and demineralizer."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
//
// driveReady is the upstream-power availability signal plumbed
// in by Plant.Step. When false (480 V MCC de-energised), the
// pumps produce zero flow even if PumpRunning was latched by
// the standby auto-start; the latch is preserved so flow
// recovers when power returns. See plans/x/uat_sat_gating.md.
func (s *SpentFuelPoolCoolingSystem) Step(driveReady bool) SpentFuelPoolCoolingSystem {
	ret := *s

	if !ret.PumpRunning || !driveReady {
		ret.StandbyRunning = false
		ret.PurificationFlow = 0
		return ret
	}

	// Standby pump auto-start: latch true once header pressure
	// drops below the standby setpoint.
	//
	// Spec justification:
	//   - "the third pump automatically starts at a header
	//     pressure of 50 psig (4.5 bar)"
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:47-48
	if ret.HeaderPressure < SFPCStandbyStartPressure {
		ret.StandbyRunning = true
	}

	// Purification flow: ~100 gpm through filter + demineralizer.
	//
	// Spec justification:
	//   - "passing approximately 100 gpm (23 m3/hr) of each
	//     loop's flow through a filter and demineralizer."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	ret.PurificationFlow = SFPCPurificationFlow

	return ret
}
