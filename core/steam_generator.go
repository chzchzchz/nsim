package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the steam generator by TimeStep. It implements the
type SteamGenerator struct {
	Part
	Pressure
	Water  Volume
	Volume Volume

	// SteamFlowRate is the design steam mass-flow the SG is sized
	// for, in volumetric units compatible with Water/Volume.
	// Sized to the spec's 480 kg/s full-load steam flow.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
	SteamFlowRate Volume

	// HeatInput is the primary-to-secondary heat flux in Watts.
	// Caller-supplied (the reactor step is out of scope; p7
	// candidate). When zero the SG does not generate steam on its
	// own; the fluid graph dominates the inventory.
	HeatInput Watts

	// MainSteamIsolationValve is the operator-controlled
	// isolation valve on the SG outlet steam line. Closing
	// it isolates the SG secondary side from the turbine and
	// steam dump. The bridge exposes this as the MSIV
	// control; closing it on a high-steam-line-break
	// signal is one of the reactor-protection-system
	// isolation paths.
	// "Each steam generator has isolation valves [...]
	// upstream of the main steam line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	MainSteamIsolationValve Valve

	// SafetyValve is the lumped ASME-Code spring-loaded
	// safety valve on the SG outlet steam line, upstream of
	// the MSIV. The valve self-actuates when SG pressure
	// exceeds SafetyValveSetpoint; the bridge exposes
	// Open for visibility.
	// "Safety valves in each steam generator outlet main
	// steam line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:42-45
	SafetyValve Valve

	// AtmosphericDumpValve is the power-operated atmospheric
	// dump valve (also called the "steam dump to
	// atmosphere") on the SG outlet steam line, upstream
	// of the MSIV. The operator (or the turbine-runback
	// interlock) opens it to dump excess steam to
	// atmosphere when the turbine cannot accept the SG
	// steam flow (e.g. turbine trip, load rejection).
	// "ASME Code safety valves and power operated relief
	// and atmospheric dump valves are located on each
	// steam line upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	AtmosphericDumpValve Valve
}

func (s *SteamGenerator) Update() SteamGenerator {
	ret := *s
	// watts := heatTransferred()
	return ret
}

func (s *SteamGenerator) heatTransferred() Watts {
	// Qdot = rate of heat transfer
	// U = heat transfer coefficient
	// A = area of heat transfer
	// deltaT = differential temperature across exchanger
	/*
		U := HeatTransferCoefficient
		A := AreaOfHeatTransfer
		Tavg := (Thot + Tcold) / 2.0
		deltaT := Tavg - Tstm
		Qdot := U*A*deltaT
	*/
	return 0
}
// SteamGenerator shell-side operating pressure and the latent heat of
// vaporisation used by the heat-transfer step.
//
// Spec justification:
//   - SteamGeneratorShellPressure — "Full load pressure, shell side:
//     920 psia (63 bar)" for the 2-loop plant from Table 3.4-1
//     (specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,
//     value lines 80-82).
//   - SteamGeneratorShellDesignPressure — design pressure on the
//     shell side is 1000 psia per the plan p6_1 trip-threshold
//     choice. (Table 3.4-1 quotes 1200 psia for the same row;
//     the plan uses 1000 as the trip point; the discrepancy is
//     noted in plans/x/x.txt verification.)
//   - LatentHeatOfSteamization — water at ~558 F to saturated steam
//     at 920 psia. The spec does not state a value; this is the
//     textbook number from standard steam tables.
const (
	SteamGeneratorShellPressure       Pressure = 920
	SteamGeneratorShellDesignPressure Pressure = 1000
	LatentHeatOfSteamization          Watts    = 2.26e6
)
// SteamGeneratorFullLoadSteamFlow is the per-SG steam flow at rated
// load, sized for 480 kg/s of saturated steam at 920 psia. The spec
// quotes 3,813,000 lb/hr (480 kg/s) per steam generator.
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//
// We convert kg/s -> lb/hr (× 2.2046 × 3600) and approximate the
// specific volume of saturated steam at 920 psia as ~0.47 ft^3/lb,
// giving a volumetric flow in ft^3/hr which we then express in
// gallons. The exact conversion is not load-bearing — the value
// drives the boil-rate integration only.
var SteamGeneratorFullLoadSteamFlow = Volume{Val: 480*2.2046*3600*0.47/7.4805, Unit: u.Gallon}

// SteamGeneratorFullLoadHeatInput is the primary-to-secondary heat
// flux that vaporises 480 kg/s of water at LatentHeatOfSteamization.
// Sized to match the spec's 480 kg/s full-load steam flow
// (specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56).
var SteamGeneratorFullLoadHeatInput Watts = Watts(480 * float64(LatentHeatOfSteamization))

// Step advances the steam generator by TimeStep. It implements the
// shell-side heat-transfer / boil-off step described in
// plans/p6_1.txt.
//
// Spec justification:
//   - "Pressurized water reactors utilize a closed-cycle system
//     design principle. […] The link connecting the two cycles is
//     the steam generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_064.txt:20-27
//   - "Steam flow per steam generator: 3,813,000 lb/hr (480 kg/sec)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//   - "Full load pressure, shell side: 920 psia (63 bar)" (Table 3.4-1)
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,80-82
func (s *SteamGenerator) Step() SteamGenerator {
	ret := *s
	dt := TimeStep

	// 1) Boil rate. HeatInput (Watts) -> mass rate via latent heat
	// of vaporisation; the result is then applied as a volume
	// decrement on the shell-side water inventory. With HeatInput
	// == 0 the SG does not generate steam on its own; the fluid
	// graph (see simulateFluidFlow) is then the only thing that
	// mutates Water.
	if ret.HeatInput > 0 {
		boilRateGPerSec := float64(ret.HeatInput) / float64(LatentHeatOfSteamization)
		boilVolume := boilRateGPerSec / 1000.0 // kg/s -> m^3/s assuming ~1 kg/L water
		newWater := ret.Water.Float() - boilVolume*dt.Seconds()
		if newWater < 0 {
			newWater = 0
		}
		if newWater > ret.Volume.Float() {
			newWater = ret.Volume.Float()
		}
		ret.Water = vol(ret.Water, newWater)
	}

	// 2) Pressure response. While HeatInput > 0 and there is water
	// to boil, Pressure relaxes toward SteamGeneratorShellPressure
	// at a rate proportional to the gap. With no HeatInput there
	// is no steam generation and Pressure stays where it is. With
	// Water depleted and HeatInput still > 0, Pressure decays
	// toward zero (no more steam to keep it up).
	pressure := float64(ret.Pressure)
	if ret.HeatInput > 0 {
		if ret.Water.Float() > 0 {
			gap := float64(SteamGeneratorShellPressure) - pressure
			pressure += gap * dt.Seconds() // relax on ~1 s timescale
		} else if pressure > 0 {
			pressure -= pressure * dt.Seconds() // decay toward zero
		}
	}
	if pressure < 0 {
		pressure = 0
	}

	// 3) Pressure trip. Cap at the design pressure. A real SG
	// would relieve via the SG safety valves (out of scope per
	// plans/p6_1.txt).
	if pressure > float64(SteamGeneratorShellDesignPressure) {
		pressure = float64(SteamGeneratorShellDesignPressure)
	}
	ret.Pressure = Pressure(pressure)

	return ret
}
