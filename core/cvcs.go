package core

import (
	"math"

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
//   - With AutoLevelControl false (manual lineup),
//     ChargingFlow = CVCSDesignChargingFlow when
//     ChargingPumpRunning, else 0, and LetdownFlow =
//     CVCSDesignLetdownFlow when LetdownValveOpen, else 0.
//   - With AutoLevelControl true, the Tavg-referenced program
//     error splits a net-flow demand between charging and
//     letdown, clamped to the design rates; the pump and valve
//     latches still gate their paths. The programmed level is
//     computed every step regardless of mode for display.
//   - In CVCSBoration / CVCSDilution modes the
//     BTRSRecirculationPump must be running (set by operator);
//     BorationRate / DilutionRate are populated accordingly and
//     BTRSBedSaturation advances toward 1, after which boron
//     changes halt.
func (c *CVCS) Step(
	makeupFlow Volume,
	tavg Temperature,
	pzrLevelFraction float64,
	pzrPressure Pressure,
) CVCS {
	ret := *c
	dt := TimeStep

	ret.ProgrammedLevel = programmedPzrLevel(tavg)

	// Reset per-step rates.
	ret.BorationRate = 0
	ret.DilutionRate = 0

	// Charging is an independent path gated by its own latch,
	// not the reactor coolant pumps; the pumps are fed from the
	// engineered safety features bus during accident conditions,
	// see
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80.
	if ret.AutoLevelControl {
		ret.applyLevelControl(pzrLevelFraction)
	} else if ret.ChargingPumpRunning {
		// Zero demand means unthrottled so the zero value keeps the
		// historical full-design behavior; shutting charging is the
		// pump latch's job.
		demand := ret.ChargingFlowDemand
		if demand > 0 {
			demand = min(demand, 1)
			ret.ChargingFlow = Volume{
				Val: CVCSDesignChargingFlow.Float() * demand, Unit: u.Gallon}
		} else {
			ret.ChargingFlow = CVCSDesignChargingFlow
		}
	} else {
		ret.ChargingFlow = Volume{Val: 0, Unit: u.Gallon}
	}
	if !ret.AutoLevelControl {
		// Letdown flow depends on valve state and the available
		// differential pressure across the letdown orifices; where
		// the flow lands (VCT return vs BRS diversion) is decided
		// below.
		if ret.LetdownValveOpen {
			ret.LetdownFlow = letdownAtPressure(pzrPressure)
		} else {
			ret.LetdownFlow = Volume{Val: 0, Unit: u.Gallon}
		}
	} else if ret.LetdownFlow.Float() > 0 {
		// The orifice dP limit is a property of the flow path, not
		// the controller, so the auto controller's letdown demand
		// passes through the same scaling.
		ret.LetdownFlow = letdownAtPressure(pzrPressure)
	}

	// Auxiliary spray drive: the valve position gated by the
	// charging pump that supplies it. With the pump secured the
	// valve has no flow behind it and delivers no drive.
	// "Charging flow is throttled to control pressurizer cooldown
	// rate and raise pressurizer level using auxiliary spray since
	// normal spray is not available when reactor coolant pumps have
	// been secured."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104
	ret.AuxSprayDrive = 0
	if ret.AuxSprayValveOpen > 0 && ret.ChargingPumpRunning {
		ret.AuxSprayDrive = min(ret.AuxSprayValveOpen, 1)
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

// letdownAtPressure scales design letdown by the square root of the
// available primary-system differential (orifice flow follows sqrt(dP)):
// at low RCS pressure very little letdown is possible through the normal
// path. A non-positive pressure reads as design so fixtures without a
// pressurizer keep historical behavior (the same zero-value convention
// ChargingFlowDemand uses).
// "Very little letdown flow is possible through the normal letdown flow
// path at low primary system pressures due to the low differential
// pressure across the CVCS letdown orifices."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:135-139
func letdownAtPressure(pzrPressure Pressure) Volume {
	if pzrPressure <= 0 {
		return CVCSDesignLetdownFlow
	}
	frac := math.Sqrt(min(float64(pzrPressure)/CVCSLetdownDesignPsia, 1))
	return Volume{Val: CVCSDesignLetdownFlow.Float() * frac, Unit: u.Gallon}
}

// applyLevelControl balances charging against letdown from the
// pressurizer level error, per the level-control function of the
// reactor pressurizer control system:
//
//	"it must maintain the RCS mass inventory, that is,
//	the pressurizer level, by balancing the charging
//	flow to the letdown flow."
//	specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:9-12
func (c *CVCS) applyLevelControl(pzrLevelFraction float64) {
	errPct := c.ProgrammedLevel - 100*pzrLevelFraction
	demand := PzrLevelCtrlGainGalPerSecPerPct * errPct
	charge := min(max(demand, 0), CVCSDesignChargingFlow.Float())
	letdown := min(max(-demand, 0), CVCSDesignLetdownFlow.Float())
	if !c.ChargingPumpRunning {
		charge = 0
	}
	if !c.LetdownValveOpen {
		letdown = 0
	}
	c.ChargingFlow = Volume{Val: charge, Unit: u.Gallon}
	c.LetdownFlow = Volume{Val: letdown, Unit: u.Gallon}
}

// programmedPzrLevel maps RCS average coolant temperature onto the
// pressurizer level program, anchored at no-load (25 percent at
// 557°F, chapter_16/page_222.txt:48-66) and full load (60 percent at
// the rated inlet/outlet midpoint, chapter_3/page_068.txt:40-43), and
// clamped flat outside that band so shutdown and overload hold the
// nearest anchor.
func programmedPzrLevel(tavg Temperature) float64 {
	fullLoadTavgF := RCSFullLoadTavgF
	slope := (PzrLevelFullLoadPct - PzrLevelNoLoadPct) /
		(fullLoadTavgF - RCSNoLoadTavgF)
	pct := PzrLevelNoLoadPct + (tavg.Float()-RCSNoLoadTavgF)*slope
	return min(max(pct, PzrLevelNoLoadPct), PzrLevelFullLoadPct)
}

// applyAutoBoronControl is the slow third of the Power Control
// System's actuators: "automatically adjusts the reactor power [...]
// through control rod motions, boron concentration changes, and
// control of the moderator temperature"
// (specs/the_westinghouse_pressurized_water_react/chapter_7/page_129.txt:112-115).
// Rods are fast and chemistry slow, so when the rod bank approaches a
// travel limit while holding the program, chemistry relieves it:
// near full withdrawal the chain needs positive reactivity (dilute),
// under deep insertion negative (borate). The existing fixed BTRS
// rates and bed dynamics are the rate limiter. While engaged this
// owns Mode and the BTRS pump; the latch lives on AutoBoronControl.
func (c *CVCS) applyAutoBoronControl(rodInsertion float64) {
	switch {
	case rodInsertion < CVCSAutoBoronLowInsertionFrac:
		c.Mode = CVCSDilution
		c.BTRSRecirculationPumpRunning = true
	case rodInsertion > CVCSAutoBoronHighInsertionFrac:
		c.Mode = CVCSBoration
		c.BTRSRecirculationPumpRunning = true
	default:
		c.Mode = CVCSVolumeControl
		c.BTRSRecirculationPumpRunning = false
	}
}
