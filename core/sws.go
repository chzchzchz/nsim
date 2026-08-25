package core

import (
	"math"

	u "github.com/bcicen/go-units"
)

// Step advances the Service Water System by one timestep. The
// step function implements four state machines:
//
//   1. Inlet temperature. The inlet tracks Outside.RiverTemp
//      every step; the service water pumps share the river
//      intake house with the circulating water pumps.
//   2. Pump running state. While PumpRunning is true, the lumped
//      flow equals ServiceWaterPumpFlow × number of running
//      pumps (capped at ServiceWaterEssentialPumpCount). The
//      simplified model collapses the three physical pumps into
//      a single boolean.
//   3. Outlet temperature. The mixed service water outlet
//      temperature rises above the source by the heat load
//      divided by the per-second water mass flow, capped at the
//      design rise to prevent instability at large timesteps.
//   4. Nonessential isolation. The NonessentialIsolated flag is
//      set by the operator during accident conditions to
//      conserve water for safety-grade loads.
//
// Spec justification:
//   - Shared river intake: "serves one circulating water pump
//     and one service water pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:37-38
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
func (s *ServiceWaterSystem) Step(
	riverTemp Temperature,
	heatLoad Watts,
) ServiceWaterSystem {
	ret := *s
	ret.InletTemp = riverTemp

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
	// load-driven rise. Mass flow in kg/s = gpm × KilogramsPerGPM.
	massKgPerS := ret.Flow * KilogramsPerGPM
	cpWater := SpecificHeatWater
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
// timestep. The step function implements four state machines:
//
//   1. Header pressure. The supply header relaxes toward the
//      running-pump target: the 55 psig band floor with the two
//      normal pumps running, the 65 psig mid-band with the standby
//      added, and depressurized with no pumps. A latched leak
//      depresses the header below the standby auto-start setpoint.
//   2. Standby pump auto-start. While the normal coolers run, a
//      header pressure below SFPCStandbyStartPressure (50 psig)
//      latches the third pump; it stays running.
//   3. Purification and turbidity. Running loops pass the ~100 gpm
//      filter/demineralizer sidestream; an aligned refueling water
//      purification pump adds a parallel stream. Turbidity decays
//      in proportion to total active purification flow.
//   4. Pool temperature. Running loops relax the pool toward the
//      design temperature; without them the stored fuel's decay
//      heat warms the pool toward boiling.
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
//   - Leakage detected through pressure instrumentation and isolated
//     by motor-operated valves:
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:75-83
//   - Refueling water purification beside the pool demineralizer:
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-94
//
// driveReady is the upstream-power availability signal plumbed
// in by Plant.Step. When false (480 V MCC de-energised), the
// pumps produce zero flow even if PumpRunning was latched by
// the standby auto-start; the latch is preserved so flow
// recovers when power returns. See plans/x/uat_sat_gating.md.
func (s *SpentFuelPoolCoolingSystem) Step(driveReady bool) SpentFuelPoolCoolingSystem {
	ret := *s
	dt := TimeStep.Seconds()
	cooling := ret.PumpRunning && driveReady

	// Standby pump auto-start: latch true once header pressure
	// drops below the standby setpoint while the coolers run.
	// Securing the coolers clears it; a standby-only lineup cannot
	// persist because PumpRunning is the lumped normal-loops flag.
	//
	// Spec justification:
	//   - "the third pump automatically starts at a header
	//     pressure of 50 psig (4.5 bar)"
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:47-48
	if cooling {
		if ret.HeaderPressure < SFPCStandbyStartPressure {
			ret.StandbyRunning = true
		}
	} else {
		ret.StandbyRunning = false
	}

	pumps := 0
	if cooling {
		pumps += 2
	}
	if ret.StandbyRunning {
		pumps++
	}
	target := 0.0
	switch pumps {
	case 3:
		target = SFPCPressureStandbyTarget
	case 2:
		target = SFPCPressureNormalMin
	}
	if ret.Leakage && pumps > 0 {
		target -= SFPCLeakPressureDrop
	}
	tau := max(SFPCPressureTimeConstant.Seconds(), 1)
	ret.HeaderPressure += (target - ret.HeaderPressure) *
		(1.0 - math.Exp(-dt/tau))

	// Purification streams. The loop sidestream rides on cooler
	// operation; the refueling water purification pump works
	// separately, so it only needs its drive power.
	sfpcFlow := 0.0
	if cooling {
		sfpcFlow = SFPCPurificationFlow
	}
	rwpuFlow := 0.0
	if ret.RWPUPurifying && driveReady {
		rwpuFlow = SFPRWPUPurificationFlow
	}
	ret.PurificationFlow = sfpcFlow

	// Turbidity cleanup scales with total active purification flow;
	// with no stream running the water just stays cloudy.
	cleanupTau := SFPCPurificationCleanupTimeConstant.Seconds() *
		SFPCPurificationFlow / (sfpcFlow + rwpuFlow)
	ret.Turbidity *= math.Exp(-dt / cleanupTau)

	// Pool heat balance: cooled loops relax toward the design
	// temperature, otherwise stored-fuel decay heat soaks the pool
	// up to the atmospheric boiling point but never past it.
	if cooling {
		coolTau := SFPCCoolingTimeConstant.Seconds()
		gap := SFPCPoolDesignTemp.Float() - ret.PoolTemp.Float()
		ret.PoolTemp = Temperature{Val: ret.PoolTemp.Float() +
			gap*(1.0-math.Exp(-dt/coolTau)), Unit: u.Fahrenheit}
	} else {
		massKg := SFPCPoolVolume.Float() * KilogramsPerGallon
		riseF := float64(SFPStoredFuelDecayHeat) /
			(massKg * SpecificHeatWater) * dt * 9.0 / 5.0
		ret.PoolTemp = Temperature{
			Val: min(ret.PoolTemp.Float()+riseF, SFPCPoolBoilingTemp),
			Unit: u.Fahrenheit,
		}
	}

	return ret
}
