package core

import (
	"math"

	u "github.com/bcicen/go-units"
)

// Step advances the Component Cooling Water System by one timestep.
// The simplified model tracks the lumped CCW inlet and outlet
// temperatures, the surge-tank level, and the heat rate rejected
// to the service water across the component cooling heat
// exchangers.
//
//   1. Inlet-temperature target. The component cooling heat
//      exchanger can only cool the loop toward the service water
//      supply temperature plus the HX approach, so the inlet
//      relaxes toward max(CCWSInletSteadyStateTemp, supply +
//      CCHXApproachTemp). At nominal river temperature this is
//      the spec's 105°F design value; a hotter river lifts it.
//      Without service water flow there is no heat sink and the
//      loop soaks toward CCWSInletMaxCooldownTemp with the same
//      first-order lag (CCWSRecoveryTimeConstant).
//   2. Outlet temperature. When pumps are running, the outlet
//      temperature equals the inlet plus CCHXApproachTemp
//      (10°F). This lumped value is what the RHRS Step reads
//      as its ccwInletTemp argument.
//   3. Surge-tank level. Initialised full at MakePlant and not
//      drawn down in the simplified fidelity band (leakage is
//      omitted; passive-failure isolation is deferred).
//
// Spec justification:
//   - "The component cooling water inlet temperature to various
//     components during normal cooldown is permitted to increase
//     to 120°F (49°C), however it must return to 105°F (40°C)
//     after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
//   - Intermediate barrier to the service water: "it forms an
//     intermediate barrier between these potentially radioactive
//     systems and the Service Water System"
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:26-33
//   - "Component cooling heat exchangers" are service water
//     loads.
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:81
//   - "During plant normal cooldown, the standby train of the
//     Safeguards subsystem is placed in operation at approximately
//     four hours after reactor shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:102-104
//   - "The component cooling water flows from the pump, through
//     the shell side of the component cooling heat exchangers,
//     through the components being cooled, and back to the pump.
//     The surge tank is connected to the suction side of the
//     component cooling pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:51-56
//   - "They provide a surge volume to accommodate thermal
//     expansion and contraction of system volume during
//     transients, and collects water that may leak into the
//     system from components being cooled."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:60-64
func (c *CCWS) Step(
	swSupplyTemp Temperature,
	swFlowAvail bool,
) CCWS {
	ret := *c

	// DriveReady reflects whether the ESF bus that feeds
	// the CCWS pumps is energised. Without bus power the
	// pumps cannot deliver flow even if PumpRunning was
	// latched by the safeguards auto-start.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:135-138
	if !ret.PumpRunning || !ret.DriveReady {
		ret.OutletTemp = ret.InletTemp
		ret.HXHeatDuty = 0
		return ret
	}

	dt := TimeStep.Seconds()
	tau := CCWSRecoveryTimeConstant.Seconds()
	if tau <= 0 {
		tau = 1
	}
	// Cold-side limit: the HX cannot cool the loop below what
	// the service water supply absorbs. The spec's 105°F value
	// governs while the river is cool enough to reach it.
	target := max(CCWSInletSteadyStateTemp,
		swSupplyTemp.Float()+CCHXApproachTemp.Float())
	if !swFlowAvail {
		// Loss of the ultimate heat sink: no cooling water
		// across the HX, so the loop soaks up to the maximum
		// permitted cooldown inlet.
		// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
		target = CCWSInletMaxCooldownTemp
	}
	ss := target
	gap := ss - ret.InletTemp.Float()
	ret.InletTemp = Temperature{Val: ret.InletTemp.Float()+gap*(1.0-math.Exp(-dt/tau)), Unit: u.Fahrenheit}
	ret.OutletTemp = Temperature{Val: ret.InletTemp.Float()+CCHXApproachTemp.Float(), Unit: u.Fahrenheit}

	// Lumped duty across the HX from the CCW-side temperature
	// drop: mass flow × cp × approach. This is the heat handed
	// to the service water header each step; without service
	// water flow nothing is rejected across the HX.
	if !swFlowAvail {
		ret.HXHeatDuty = 0
		return ret
	}
	massKgPerS := float64(CCWSDesignFlow.Float()) * 60.0 * 0.0631
	dutyK := CCHXApproachTemp.Float() * 5.0 / 9.0
	ret.HXHeatDuty = Watts(massKgPerS * 4186.0 * dutyK)

	return ret
}

