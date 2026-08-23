package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances one RCFC unit by one timestep. The Step
// function implements the three state machines described in the
// spec:
//
//   1. Running state. The fan motor either runs (in either
//      high or low speed) or is stopped. When stopped, the unit
//      rejects no heat; the backdraft damper may close to
//      protect against reverse flow.
//   2. Speed switchover. When the LOCA signal arrives the fan
//      motor drops from normal speed to low speed; the air-flow
//      path is also switched (normal dampers → emergency-flow
//      dampers, HEPA filters, moisture separators). The
//      simplified model collapses this to a single LowSpeed
//      boolean.
//   3. Heat transfer. While running, the unit rejects heat from
//      the containment atmosphere to the service water at a
//      rate proportional to the live ΔT (containment air
//      temperature minus coil inlet temperature) scaled against
//      the design ΔT. The coil inlet is the live essential
//      service water supply temperature; with no service water
//      flow the coils run dry and reject no heat.
//
// Spec justification:
//   - RCFC purpose: "The Reactor Containment Fan Cooler System
//     (RCFC) is designed to remove heat from the containment
//     building during both normal operation and in the event of
//     a loss-of-coolant accident (LOCA). The RCFC is an
//     engineered safeguard system."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:79-83
//   - Low-speed switchover: "When a LOCA is sensed, the fan
//     motors are automatically switched to low speed to provide
//     proper flow of the steam-air mixture."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-57
//   - Service water supply: "Cooling water is supplied by the
//     essential service water system."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
//   - Backdraft damper: "A gravity actuated backdraft damper is
//     installed in the ventilation system discharge ductwork of
//     each fan. These dampers serve to isolate units from the
//     ventilation system when the fan is not in use and to
//     protect each unit from damage due to reverse flow during
//     a LOCA pressure transient."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:71-77
func (r *RCFCUnit) Step(
	locaSensed bool,
	containmentTemp Temperature,
	swSupplyTemp Temperature,
	swFlowAvail bool,
) RCFCUnit {
	ret := *r
	ret.CoilInletTemp = swSupplyTemp

	if !ret.Running {
		ret.HeatTransferRate = 0
		ret.CoilOutletTemp = ret.CoilInletTemp
		ret.BackdraftDamperOpen = false
		return ret
	}

	// Speed switchover. When the LOCA signal arrives, drop to
	// low speed. The spec describes this as automatic with no
	// operator delay.
	//
	// Spec justification:
	//   - "When a LOCA is sensed, the fan motors are
	//     automatically switched to low speed."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-56
	ret.LowSpeed = locaSensed
	ret.BackdraftDamperOpen = true

	// Dry coils: without service water flow there is no heat
	// sink for the containment atmosphere.
	// "Cooling water is supplied by the essential
	// service water system."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
	if !swFlowAvail {
		ret.HeatTransferRate = 0
		ret.CoilOutletTemp = ret.CoilInletTemp
		return ret
	}

	// Heat transfer: scale the design rating by the live
	// (containment air − coil inlet) ΔT over the design ΔT.
	// Clamp the effective ΔT at 0 so a cold containment does
	// not produce a negative heat-rejection term.
	airT := containmentTemp.Float()
	coilIn := ret.CoilInletTemp.Float()
	deltaT := airT - coilIn
	if deltaT < RCFCCoilApproachTemp {
		deltaT = 0
	}
	eff := deltaT / RCFCDesignDeltaT
	eff = min(eff, 1.0)
	ret.HeatTransferRate = RCFCDesignHeatRejection * eff

	// Coil outlet: rise above the inlet by the same temperature
	// that the air drops by, weighted by the design rating.
	coilOut := coilIn + deltaT*eff
	coilOut = min(coilOut, airT)
	ret.CoilOutletTemp = Temperature{Val: coilOut, Unit: u.Fahrenheit}

	return ret
}

// RCFCUnitsRunning returns the count of running RCFC units.
// Used by ContainmentBuilding.Step to verify the safeguard
// minimum is met.
//
// Spec justification:
//   - "A minimum of two units must function to satisfy design
//     requirements."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:20-22
func RCFCUnitsRunning(units *[RCFCUnitCount]RCFCUnit) int {
	n := 0
	for i := 0; i < RCFCUnitCount; i++ {
		if units[i].Running {
			n++
		}
	}
	return n
}

// Containment atmosphere properties used by the lumped
// ideal-gas model. Air density 1.2 kg/m³ is representative of
// containment-atmosphere conditions; the spec states no value
// (derived sizing assumption).
const (
	ft3ToM3              = 0.0283168
	paPerPsi             = 6894.76
	airSpecificGasConst   = 287.0 // J/kg·K
	containmentAirDensity = 1.2   // kg/m³
	airCp                = 1005.0 // J/kg·K
)

// Step advances the ContainmentBuilding by one timestep. The
// step integrates the lumped atmosphere temperature with the
// net heat input minus the RCFC heat sink, then derives
// absolute pressure via the ideal-gas law.
//
// heatIn is signed: Plant.Step passes the negative of the CSS
// spray-condensation enthalpy removal. RCS break enthalpy
// release (the DBA source: "The minimum design pressure and
// temperature of the containment are equal to the peak pressure
// and temperature occurring as the result of any rupture of the
// RCS") is deferred until a break-flow state exists.
// specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:14-21
//
// Spec justification:
//   - Containment purpose: "This chapter discusses the design of
//     the Reactor Containment Building (hereafter referred as
//     the containment) that will contain and control any
//     release of radioactivity to the environment under normal,
//     upset, or emergency conditions."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:43-51
//   - Design pressure: "Transients resulting from the DBA
//     serve as the basis for a containment design pressure of
//     60 psig (4.46 bar)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
//   - Free volume: "The internal net free volume approximates
//     2,000,000 cubic feet (56,600 m³)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_109.txt:11-12
func (c *ContainmentBuilding) Step(
	heatIn Watts,
	heatOutRCFC Watts,
) ContainmentBuilding {
	ret := *c
	dt := TimeStep.Seconds()

	netW := float64(heatIn) - float64(heatOutRCFC)
	volumeM3 := ret.FreeVolume * ft3ToM3
	massKg := volumeM3 * containmentAirDensity
	deltaK := netW / (massKg * airCp) * dt
	newTempF := ret.Temperature.Float() + deltaK*9.0/5.0
	newTempF = max(newTempF, -459.67)
	ret.Temperature = Temperature{Val: newTempF, Unit: u.Fahrenheit}

	kelvin := (newTempF + 459.67) * 5.0 / 9.0
	pPa := massKg * airSpecificGasConst * kelvin / volumeM3
	ret.Pressure = Pressure(pPa / paPerPsi)

	return ret
}
