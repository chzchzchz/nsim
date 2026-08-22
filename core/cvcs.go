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
//   - Letdown: VCT level has exceeded VCTHighLevel; the level
//     controller has diverted BTRS outflow to the BRS holdup
//     tanks and charging/letdown are halted so the VCT level
//     can stabilise.
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
//     control system and depletes by (charging flow + BTRS
//     recirculation). The letdown path returns coolant to the
//     VCT so it does not directly drain VCTLevel.
//   - ChargingFlow = CVCSDesignChargingFlow when
//     ChargingPumpRunning, else 0.
//   - LetdownFlow = CVCSDesignLetdownFlow when LetdownValveOpen
//     and the mode is not CVCSLetdown, else 0.
//   - In CVCSBoration / CVCSDilution modes the
//     BTRSRecirculationPump must be running (set by operator);
//     BorationRate / DilutionRate are populated accordingly.
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
		ret.ChargingFlow = u.NewValue(0, u.Gallon)
	}

	// Letdown flow depends on valve state.
	if ret.LetdownValveOpen && ret.Mode != CVCSLetdown {
		ret.LetdownFlow = CVCSDesignLetdownFlow
	} else {
		ret.LetdownFlow = u.NewValue(0, u.Gallon)
	}

	// VCT level accumulation: makeup enters, charging-pump
	// suction leaves, BTRS recirculation drains when active.
	// Per-second rates; multiply by dt.Seconds() for per-step
	// delta.
	delta := 0.0
	delta += makeupFlow.Float() * dt.Seconds()
	delta -= ret.ChargingFlow.Float() * dt.Seconds()
	if ret.BTRSRecirculationPumpRunning {
		delta -= VCTBTRSCirculation * dt.Seconds()
	}
	newVCT := ret.VCTLevel.Float() + delta
	if newVCT < 0 {
		newVCT = 0
	}
	ret.VCTLevel = u.NewValue(newVCT, u.Gallon)

	// VCT high-level condition: divert BTRS outflow to BRS
	// holdup tanks. Charging and letdown are halted so VCT
	// level can stabilise.
	//
	// "the volume control tank level controller automatically
	// diverts the outflow from the BTRS into the BRS holdup
	// tanks."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
	if newVCT > VCTHighLevel {
		ret.Mode = CVCSLetdown
		ret.ChargingFlow = u.NewValue(0, u.Gallon)
		ret.LetdownFlow = u.NewValue(0, u.Gallon)
		return ret
	}

	// Otherwise: act on the current mode.
	switch ret.Mode {
	case CVCSBoration:
		if ret.BTRSRecirculationPumpRunning {
			ret.BorationRate = CVCSBorationRate
			ret.BoronConcentration += CVCSBorationRate * dt.Seconds()
		}
	case CVCSDilution:
		if ret.BTRSRecirculationPumpRunning {
			ret.DilutionRate = CVCSDilutionRate
			ret.BoronConcentration -= CVCSDilutionRate * dt.Seconds()
			if ret.BoronConcentration < 0 {
				ret.BoronConcentration = 0
			}
		}
	case CVCSLetdown:
		// Operator must correct VCT level (e.g. manual
		// blowdown or BRS recycle); the CVCS does not exit
		// this mode on its own.
		ret.ChargingFlow = u.NewValue(0, u.Gallon)
		ret.LetdownFlow = u.NewValue(0, u.Gallon)
	case CVCSVolumeControl:
		// Charging/letdown balance to maintain pressurizer
		// level; the imbalance is the makeup that reaches the
		// VCT level controller (already accumulated above).
		// No boron concentration change.
	}

	return ret
}
