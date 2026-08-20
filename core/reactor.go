package core

import (
	"math"

	u "github.com/bcicen/go-units"
)

// Step advances the reactor pressure vessel by TimeStep. It
// implements the minimal reactivity model and the two-branch decay
// heat curve described in plans/p7_1.txt.
//
// Spec justification for the underlying physics:
//   - Rod cluster control: "Rod cluster control (RCC) assemblies are
//     used for reactor startup or shutdown, to follow load changes,
//     and to control small transient changes in reactivity."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:90-93
//     "Full length RCCs provide operational reactivity control and
//     can shut the reactor down at all times, even with the most
//     reactive RCC stuck out of the core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:108-114
//   - Doppler defect: "This defect is due to an increase in width of
//     uranium-238 resonance peaks in the epithermal energy range.
//     […] the effective neutron temperature increases by about
//     1000°F (540°C) as the average physical pellet temperature is
//     increased with heat generation."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:89-96
//   - Power defect: "The negative reactivity associated with an
//     increase in reactor core power due to temperature feedback
//     effects from an increase in fuel and moderator temperature."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
//   - Decay heat removal: "The system is designed to reduce the RCS
//     temperature to 140°F (60°C) within 20 hours following reactor
//     shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:104-105
//   - EFWS purpose: "The EFWS thereby maintains the capability of
//     the steam generators to remove plant stored heat and core
//     decay heat by converting the emergency feedwater to steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:28-31
func (r *ReactorPressureVessel) Step() ReactorPressureVessel {
	ret := *r
	dt := TimeStep

	// 1) Time-since-trip accumulation.
	if ret.Trip {
		ret.TimeSinceTrip += dt
	} else {
		ret.TimeSinceTrip = 0
	}

	// 2) Control-rod reactivity contribution. Position=0 (fully
	// inserted) gives most negative reactivity (shutdown).
	// Position=1 (fully withdrawn) gives zero rod contribution.
	rodReactivity := float32(ret.ControlRods.Position-1) * CoreRodWorthABCD

	// 3) Doppler / temperature feedback. Proxy: coolant outlet
	// temperature deviation from the design full-load reference.
	// The spec ties this to the broader "power defect":
	// "The sum of the above three effects, Doppler, variable average
	// moderator temperature, and redistribution is termed the power
	// defect."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:100-104
	var outletF float32
	if ret.Outlet.Temperature.Unit().Name == "" {
		outletF = float32(ret.Outlet.Temperature.Float())
	} else {
		toF, err := ret.Outlet.Temperature.Convert(u.Fahrenheit)
		if err != nil {
			outletF = float32(ret.Outlet.Temperature.Float())
		} else {
			outletF = float32(toF.Float())
		}
	}
	dopplerDelta := outletF - CoreDopplerReferenceTempF
	ret.Reactivity = rodReactivity + dopplerDelta*CoreDopplerPcmPerDegF

	// 4) Thermal power output. At-power: rated. After trip:
	// two-branch decay-heat curve.
	powerFrac := float32(1.0)
	if ret.Trip {
		t := ret.TimeSinceTrip.Seconds()
		if t < CoreDecayHeatFastTimeConstant.Seconds() {
			fast := float32(CoreDecayHeatShutdownFraction +
				(1.0-CoreDecayHeatShutdownFraction)*
					math.Exp(-t/CoreDecayHeatFastTimeConstant.Seconds()))
			powerFrac = fast
		} else {
			tSlow := t - CoreDecayHeatFastTimeConstant.Seconds()
			slow := float32(CoreDecayHeatMinFraction +
				(CoreDecayHeatShutdownFraction-CoreDecayHeatMinFraction)*
					math.Exp(-tSlow/CoreDecayHeatTimeConstant.Seconds()))
			powerFrac = slow
		}
	}
	ret.ThermalPower = Watts(float64(CoreRatedThermalPower) * float64(powerFrac))
	if ret.ThermalPower < 0 {
		ret.ThermalPower = 0
	}

	return ret
}
