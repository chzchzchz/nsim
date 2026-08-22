package core

import (
	"math"

	u "github.com/bcicen/go-units"
)

// Step advances the reactor pressure vessel by TimeStep. It
// implements the primary-side thermal model and the minimal
// reactivity model described in plans/p7_1.txt and plans/p8_1.txt.
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
//   - Moderator temperature coefficient: varies with boron
//     concentration; positive at beginning-of-life, negative at
//     end-of-life.
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:54-55
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
//   - Decay heat removal: "The system is designed to reduce the RCS
//     temperature to 140°F (60°C) within 20 hours following reactor
//     shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:104-105
//   - EFWS purpose: "The EFWS thereby maintains the capability of
//     the steam generators to remove plant stored heat and core
//     decay heat by converting the emergency feedwater to steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:28-31
//   - RCS design temperatures: "Reactor coolant inlet temperature:
//     621°F (327°C); outlet: 558°F (292°C)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
//   - Design full-load average temperature: "about 571°F (300°C)"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:115-116
//   - RPV design pressure: "designed for full system pressure of
//     2500 psia (172.3 bar a)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_054.txt:20
//   - RCP design capacity / head: "Design capacity 97,600 gpm
//     (6.15 m3/sec) Design head 280 ft (85.3 m)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:124-130
//     (cited for context; not directly used in the model).
func (r *ReactorPressureVessel) Step(rcpSpeed Velocity) ReactorPressureVessel {
	ret := *r
	dt := TimeStep

	// 1) Time-since-trip accumulation.
	if ret.Trip {
		ret.TimeSinceTrip += dt
	} else {
		ret.TimeSinceTrip = 0
	}

	// 2) Primary-side thermal model.
	powerFrac := float64(ret.ThermalPower) / float64(CoreRatedThermalPower)
	if powerFrac < 0 {
		powerFrac = 0
	}
	refDeltaF := CoreReferenceOutletF - CoreReferenceInletF
	targetOutlet := CoreReferenceInletF + powerFrac*refDeltaF

	outletF := ret.Outlet.Float()
	outletF += (targetOutlet - outletF) * (dt.Seconds() / PrimaryThermalTimeConstant)
	if outletF < 0 {
		outletF = 0
	}
	ret.Outlet = u.NewValue(outletF, u.Fahrenheit)

	// The cold-leg / downcomer inlet temperature only blends
	// with the outlet when the RCP is actually spinning. The
	// gate is the actual rotor speed, not the operator latch:
	// if the ESF breaker opens the pump coasts down to zero
	// speed even though Running=true is latched, and there is
	// no flow to drive the inlet-outlet blend.
	rcpFlowFrac := float64(rcpSpeed) / float64(RCPRatedSpeed)
	if rcpFlowFrac < 0 {
		rcpFlowFrac = 0
	}
	if rcpFlowFrac > 0 {
		blend := CoreInletBlendFraction * powerFrac * rcpFlowFrac
		targetInlet := CoreReferenceInletF*(1-blend) + outletF*blend
		inletF := ret.Inlet.Float()
		inletF += (targetInlet - inletF) * (dt.Seconds() / PrimaryInletTimeConstant)
		if inletF < 0 {
			inletF = 0
		}
		ret.Inlet = u.NewValue(inletF, u.Fahrenheit)
	}

	// 3) Pressure relaxes toward the RPV design pressure. The
	// pressurizer (p5) sets the actual RCS pressure; this is the
	// design baseline the RPV itself sits at in steady state.
	pressure := float64(ret.Pressure)
	pressure += (float64(RPVDesignPressure) - pressure) *
		(dt.Seconds() / PrimaryPressureTimeConstant)
	if pressure < 0 {
		pressure = 0
	}
	ret.Pressure = Pressure(pressure)

	// 4) Reactivity contributions. The rod-bank term follows the
	// ABCD rod-worth curve (plans/p7_1.txt). The Doppler term
	// captures fuel-temperature feedback. The boron term
	// converts the CVCS boron concentration to reactivity
	// using a linear worth model centred on CVCSInitialBoron;
	// positive ΔBoron decreases reactivity (boron is a
	// neutron absorber).
	//
	// Spec: "the moderator temperature coefficient is positive
	// at low power (BOL), and [...] the boron concentration
	// changes significantly with fuel depletion."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
	rodReactivity := (1 - float32(ret.ControlRods.Position)) * CoreRodWorthABCD
	dopplerDeltaF := outletF - float64(CoreDopplerReferenceTempF)
	dopplerPcm := float32(dopplerDeltaF * CoreDopplerPcmPerDegF)
	boronDelta := (ret.ControlRods.BoronConcentration - CVCSInitialBoron) / 100.0
	boronPcm := float32(boronDelta) * CoreBoronWorthPer100ppm
	ret.Reactivity = rodReactivity + dopplerPcm + boronPcm

	// 5) Thermal power output. At-power: rated scaled by the
	// reactivity power-defect correction. After trip: decay-heat
	// curve from p7 (unchanged).
	powerFrac32 := float32(powerFrac)
	if ret.Trip {
		t := ret.TimeSinceTrip.Seconds()
		if t < CoreDecayHeatFastTimeConstant.Seconds() {
			fast := float32(CoreDecayHeatShutdownFraction +
				(1.0-CoreDecayHeatShutdownFraction)*
					math.Exp(-float64(t)/CoreDecayHeatFastTimeConstant.Seconds()))
			powerFrac32 = fast
		} else {
			tSlow := t - CoreDecayHeatFastTimeConstant.Seconds()
			slow := float32(CoreDecayHeatMinFraction +
				(CoreDecayHeatShutdownFraction-CoreDecayHeatMinFraction)*
					math.Exp(-tSlow/CoreDecayHeatTimeConstant.Seconds()))
			powerFrac32 = slow
		}
	}
	powerCorrection := 1.0 + float64(ret.Reactivity)/100.0
	if powerCorrection < 0 {
		powerCorrection = 0
	}
	ret.ThermalPower = Watts(float64(CoreRatedThermalPower) * float64(powerFrac32) * powerCorrection)
	if ret.ThermalPower < 0 {
		ret.ThermalPower = 0
	}
	// RCP shaft-loss heat enters the primary coolant (chapter
	// 16 temperature balance). Added after the reactivity-based
	// power so the SG HeatInput sees the total primary-side
	// flux when the ReactorBuilding wiring copies
	// ret.ThermalPower into SteamGenerator.HeatInput.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:108-115
	ret.ThermalPower += ret.PumpHeatInput

	return ret
}
