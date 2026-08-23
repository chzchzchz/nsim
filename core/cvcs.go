package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the CVCS by one timestep. The CVCS handles four
// operating modes per the spec:
//
//   - VolumeControl: charging and letdown balance to maintain
//     pressurizer level. No boron concentration change.
//   - Boration: BTRS demineralizers release boron into the
//     charging stream; RCS boron concentration rises.
//   - Dilution: BTRS demineralizers absorb boron from the
//     charging stream; RCS boron concentration falls.
//   - Diverting: VCT level has exceeded VCTHighLevel; letdown
//     continues at design rate but the level controller diverts it
//     to the BRS holdup tanks until VCTLevel falls back to
//     VCTDesignLevel. The boron mode keeps running while
//     diverting.
//
// Spec justification:
//   - Mode selection: "The operating mode (boration or dilution)
//     and the magnitude of the boron concentration change
//     required is determined by comparison of the measured Tc
//     (cold leg temperature) with the programmed (function of
//     power) Tc."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
//   - Charging/letdown balance: "The pressurizer level is
//     controlled by balancing the charging flow to the letdown
//     flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
//   - Boration path: "for boration, an increase in the boric
//     acid concentration of the reactor coolant is accomplished
//     by sending the flow from the VCT, at relatively high
//     temperatures (140°F) (60°C), to the thermal regeneration
//     demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:16-19
//   - Dilution path: "for dilution, a decrease in the boric acid
//     concentration in the reactor coolant is accomplished by
//     sending the flow from the VCT, at relatively low
//     temperatures (50°F) (10°C), to the thermal regeneration
//     demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:26-30
//   - BTRS recirculation pump: "The BTRS recirculation pump
//     provides the motive force to route flow from the VCT
//     through the demineralizers during boration and dilution
//     modes."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39
//   - VCT level controller overflow: "the volume control tank
//     level controller automatically diverts the outflow from
//     the BTRS into the BRS holdup tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
//   - BRS collects CVCS letdown: "The BRS normally collects water
//     from the RCS through the Chemical and Volume Control
//     System (CVCS) letdown line."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:32-35
//   - Solid-plant primary pressure control: "Solid plant primary
//     pressure control utilizes a pressure control valve between
//     the RHR and CVC systems which is set to maintain a
//     selected pressure while charging makes up for any coolant
//     contraction and losses."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:27-32
//   - Manual charging/letdown during startup: "The charging
//     pump flow and low-pressure letdown valve are controlled
//     manually during plant startup and cooldown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
//
// Per-step dynamics:
//
//   - VCTLevel accumulates makeup from the reactor makeup
//     control system, depletes by (charging flow + BTRS
//     recirculation), and gains the letdown return path. While
//     diverting (VCT high-level condition), letdown lands in
//     BRSHoldupLevel instead of returning to the VCT.
//   - ChargingFlow = CVCSDesignChargingFlow when
//     ChargingPumpRunning, else 0.
//   - LetdownFlow = CVCSDesignLetdownFlow when LetdownValveOpen,
//     else 0. While diverting, the flow accumulates into
//     BRSHoldupLevel rather than the VCT.
//   - In CVCSBoration / CVCSDilution modes the
//     BTRSRecirculationPump must be running (set by operator);
//     BorationRate / DilutionRate are populated accordingly and
//     BTRSBedSaturation advances toward 1, after which boron
//     changes halt.
func (c *CVCS) Step(
	makeupFlow Volume,
) CVCS {
	ret := *c
	dt := TimeStep

	// Reset per-step rates.
	ret.BorationRate = 0
	ret.DilutionRate = 0

	// Charging flow depends on the charging-pump latch (not
	// the reactor coolant pumps — CVCS charging is an
	// independent path). The pumps are fed from the
	// engineered safety features bus during accident
	// conditions; see
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80.
	if ret.ChargingPumpRunning {
		ret.ChargingFlow = CVCSDesignChargingFlow
	} else {
		ret.ChargingFlow = Volume{Val: 0, Unit: u.Gallon}
	}

	// Letdown flow depends on valve state only; where the flow
	// lands (VCT return vs BRS diversion) is decided below.
	if ret.LetdownValveOpen {
		ret.LetdownFlow = CVCSDesignLetdownFlow
	} else {
		ret.LetdownFlow = Volume{Val: 0, Unit: u.Gallon}
	}

	// VCT level accumulation: makeup enters, charging-pump
	// suction leaves, BTRS recirculation drains when active.
	// Per-second rates; multiply by dt.Seconds() for per-step
	// delta. The letdown return path normally credits the VCT;
	// while diverting it lands in the BRS holdup tanks instead.
	delta := 0.0
	delta += makeupFlow.Float() * dt.Seconds()
	delta -= ret.ChargingFlow.Float() * dt.Seconds()
	if ret.BTRSRecirculationPumpRunning {
		delta -= VCTBTRSCirculation * dt.Seconds()
	}
	newVCT := ret.VCTLevel.Float() + delta

	// VCT high-level condition: divert letdown to the BRS holdup
	// tanks until the level returns to its design band. The spec
	// reroutes letdown; it does not stop it, and the BTRS keeps
	// cycling through its operator-selected boron mode while
	// diverting.
	//
	// "If the volume control tank is full, a high-water level
	// signal diverts letdown flow to the Boron Recycle"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:19-21
	// "the volume control tank level controller automatically
	// diverts the outflow from the BTRS into the BRS holdup
	// tanks."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
	diverting := ret.Diverting || newVCT > VCTHighLevel
	if diverting {
		ret.Diverting = true
		ret.BRSHoldupLevel = Volume{Val: ret.BRSHoldupLevel.Float() +
			ret.LetdownFlow.Float()*dt.Seconds(), Unit: u.Gallon}
	} else {
		newVCT += ret.LetdownFlow.Float() * dt.Seconds()
	}
	newVCT = max(newVCT, 0)
	ret.VCTLevel = Volume{Val: newVCT, Unit: u.Gallon}

	switch ret.Mode {
	case CVCSBoration:
		if ret.BTRSRecirculationPumpRunning && ret.BTRSBedSaturation < 1 {
			ret.BTRSBedSaturation = min(
				ret.BTRSBedSaturation+dt.Seconds()/BTRSBedLifeSeconds, 1)
			ret.BorationRate = CVCSBorationRate
			ret.BoronConcentration += CVCSBorationRate * dt.Seconds()
		}
	case CVCSDilution:
		if ret.BTRSRecirculationPumpRunning && ret.BTRSBedSaturation < 1 {
			ret.BTRSBedSaturation = min(
				ret.BTRSBedSaturation+dt.Seconds()/BTRSBedLifeSeconds, 1)
			ret.DilutionRate = CVCSDilutionRate
			ret.BoronConcentration -= CVCSDilutionRate * dt.Seconds()
			ret.BoronConcentration = max(ret.BoronConcentration, 0)
		}
	case CVCSLetdown:
		// Legacy operator-selectable mode; the automatic divert
		// uses the Diverting latch instead so the boron mode
		// survives a transient VCT overflow.
	case CVCSVolumeControl:
		// Charging/letdown balance to maintain pressurizer
		// level; the imbalance is the makeup that reaches the
		// VCT level controller (already accumulated above).
		// No boron concentration change.
	}

	// Clear the divert latch once the VCT is back in its design
	// band; letdown then returns to the VCT again.
	if ret.Diverting && newVCT <= VCTDesignLevel {
		ret.Diverting = false
	}

	return ret
}
