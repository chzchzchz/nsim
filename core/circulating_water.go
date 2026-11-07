package core

import (
	u "github.com/bcicen/go-units"
	"time"
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
func (c *CirculatingWaterSystem) Step(dt time.Duration,
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
	if ret.SourceIsCoolingTower && coolingTower != nil {
		// The CW pumps draw from the tower basin, so the
		// condenser inlet is the live cold-basin temperature
		// regardless of fan state; fans change how fast the
		// basin cools (CoolingTower.Step), not the suction
		// temperature.
		// "The circulating water pumps circulate the water
		// from the cooling tower basin through the main
		// condenser and back to the cooling tower."
		// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:1-3
		ret.InletTemp = coolingTower.ColdBasinTemp
		if ret.FlowGPM > 0 &&
			coolingTower.Basin.Level.Float() < CoolingTowerMinSuctionGal {
			ret.PumpsInService = 0 // lost suction: basin drained
			ret.FlowGPM = 0
		}
	}

	// The condenser Step will overwrite OutletTemp with the
	// actual heat-balance result; initialise here so the
	// struct is always in a self-consistent state when read
	// before the condenser has run.
	ret.OutletTemp = ret.InletTemp

	return ret
}

// Step advances the CoolingTower state machine by one
// timestep. The model tracks fan state, basin inventory, and a
// bulk cold-basin temperature:
//
//   - With fans running, the basin relaxes toward
//     ambient wet-bulb + CoolingTowerApproachF; without them it
//     soaks toward the hot return water. The wet-bulb is the
//     evaporative lower bound: "effectively the lowest
//     temperature that may be achieved by evaporative cooling".
//     specs/wiki/Wet-bulb_temperature.txt:775
//   - Evaporation scales with circulating flow times the range
//     (cp·ΔT/h_fg fraction); continuous blowdown bleeds a fixed
//     fraction of flow. Both deplete the basin.
//     "This inventory loss, combined with losses due to
//     evaporation and drift, requires two cooling tower makeup
//     pumps [...]"
//     specs/westinghouse_tech_manual/chapter_14/page_1044.txt:12-17
//   - The makeup valve refills the basin at
//     CoolingTowerMakeupFlowGPM when open.
//
// cwFlowGPM is the previous tick's circulating flow (a 1 ms lag
// consistent with the copy-into-nextPlant ordering).
func (ct *CoolingTower) Step(dt time.Duration, fansRunning, driveReady bool,
	cwFlowGPM float64, hotInlet, wetBulb Temperature) CoolingTower {
	ret := *ct

	ret.FanRunning = fansRunning && driveReady
	ret.HotInletTemp = hotInlet

	target := hotInlet.Float()
	if ret.ColdBasinTemp.Float() <= 0 {
		ret.ColdBasinTemp = Temperature{Val: target, Unit: u.Fahrenheit}
	}
	approach := ret.ApproachF
	if approach <= 0 {
		approach = CoolingTowerApproachF
	}
	if ret.FanRunning {
		target = wetBulb.Float() + approach
	}
	k := dt.Seconds() / CoolingTowerTimeConstant.Seconds()
	cold := ret.ColdBasinTemp.Float() + k*(target-ret.ColdBasinTemp.Float())
	ret.ColdBasinTemp = Temperature{Val: cold, Unit: u.Fahrenheit}

	rangeF := max(hotInlet.Float()-ret.ColdBasinTemp.Float(), 0)
	level := ret.Basin.Level.Float()
	if ret.Basin.Level.UnitType().Name == "" {
		ret.Basin.Level = vol(ret.Basin.Level, level)
	}
	makeup := ret.MakeupValve.Open * CoolingTowerMakeupFlowGPM / 60.0
	if cwFlowGPM > 0 {
		evapGPM := cwFlowGPM * CoolingTowerEvapFractionPerDegF * rangeF
		ret.EvaporationFlow = Volume{Val: evapGPM / 60.0, Unit: u.Gallon}
		blowdown := cwFlowGPM * CoolingTowerBlowdownFraction / 60.0 *
			ret.BlowdownValve.Open
		level += (makeup - evapGPM/60.0 - blowdown) * dt.Seconds()
	} else {
		ret.EvaporationFlow = Volume{}
		level += makeup * dt.Seconds()
	}
	level = min(max(level, 0), CoolingTowerBasinCapacityGal)
	ret.Basin.Level = vol(ret.Basin.Level, level)

	return ret
}
