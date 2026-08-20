package core

import (
	"math"

	u "github.com/bcicen/go-units"
)

// Step advances the Component Cooling Water System by one
// timestep. The simplified model tracks the lumped CCW inlet
// and outlet temperatures and the surge-tank level.
//
//   1. Inlet-temperature recovery. When the CCWS pumps are
//      running, the inlet temperature evolves as a first-order
//      lag toward CCWSInletSteadyStateTemp (105°F) with the
//      CCWSRecoveryTimeConstant (1 h) time constant. The spec
//      permits the inlet to rise to 120°F during cooldown but
//      requires it to return to 105°F within four hours.
//   2. Outlet temperature. When pumps are running, the outlet
//      temperature equals the inlet plus CCHXApproachTemp
//      (10°F). This lumped value is what the RHRS Step reads
//      as its ccwInletTemp argument, replacing the previous
//      RHRSCCWDesignInletTemp constant.
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
func (c *CCWS) Step() CCWS {
	ret := *c

	if !ret.PumpRunning {
		ret.OutletTemp = ret.InletTemp
		return ret
	}

	dt := TimeStep.Seconds()
	tau := CCWSRecoveryTimeConstant.Seconds()
	if tau <= 0 {
		tau = 1
	}
	ss := CCWSInletSteadyStateTemp
	gap := ss - ret.InletTemp.Float()
	ret.InletTemp = u.NewValue(
		ret.InletTemp.Float()+gap*(1.0-math.Exp(-dt/tau)),
		u.Fahrenheit,
	)
	ret.OutletTemp = u.NewValue(
		ret.InletTemp.Float()+CCHXApproachTemp.Float(),
		u.Fahrenheit,
	)

	return ret
}

