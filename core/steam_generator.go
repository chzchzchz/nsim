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
	return ret
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

	// SteamGallonWaterM3 is the liquid-water volume (m^3) the SG
	// inventory loses per gallon of steam drawn off, matching the
	// ~1 kg/L convention used by the boil-rate integration.
	SteamGallonWaterM3 = 3.785 / 1000.0
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
// shell-side heat-transfer / boil-off step described in plans/p6_1.txt,
// the steam offtake drawn by turbine admission and the condenser dump
// (passed in as a fraction of rated steam flow), and the atmospheric
// dump read from this struct's own valves.
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
//   - Inertial heat leaves through the dump valves when turbine demand
//     falls short of production:
//     "This heat is rejected to the condenser through the steam dump
//     valves and to the atmosphere for the initial phase of large load
//     reductions."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:6-8
func (s *SteamGenerator) Step(steamOutflowFraction float64) SteamGenerator {
	ret := *s
	dt := TimeStep

	// Rated boil-off capacity in liquid-equivalent m^3/s, the design
	// reference the offtake fraction is measured against. At rated
	// heat input this vaporises 480 kg/s of water.
	ratedBoilM3PerSec := float64(SteamGeneratorFullLoadHeatInput) /
		float64(LatentHeatOfSteamization) / 1000.0

	boilRateM3PerSec := 0.0
	if ret.HeatInput > 0 {
		boilRateGPerSec := float64(ret.HeatInput) / float64(LatentHeatOfSteamization)
		boilRateM3PerSec = boilRateGPerSec / 1000.0 // kg/s -> m^3/s at ~1 kg/L
	}

	// Steam offtake drawn from the shell side this step (turbine
	// admission + condenser dump), referenced to rated full-load
	// steam flow and converted to the same liquid-equivalent volume
	// convention as the boil-off. The atmospheric valves are read
	// below because they live on this struct.
	offtakeM3 := steamOutflowFraction * ratedBoilM3PerSec * dt.Seconds()

	// Mass balance: boil-off production minus steam offtake.
	newWater := ret.Water.Float() - offtakeM3
	if ret.HeatInput > 0 {
		newWater -= boilRateM3PerSec * dt.Seconds()
	}
	newWater = min(max(newWater, 0), ret.Volume.Float())
	ret.Water = vol(ret.Water, newWater)

	// Atmospheric dump debit. Dumping to atmosphere spends secondary
	// inventory; only the condenser dump returns it ("During plant
	// cooldown, steam is dumped to the condensers [...] rather than to
	// atmosphere through the atmospheric dump valves. This conserves
	// condensate in the system.",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:33-37).
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam line
	// upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	advFraction := 0.0
	if ret.AtmosphericDumpValve.Open > 0 && ret.Pressure > 0 {
		advFraction = ret.AtmosphericDumpValve.Open * AtmosphericDumpFraction

		dump := advFraction *
			SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
			dt.Seconds()
		newWater := max(ret.Water.Float()-dump, 0)
		ret.Water = vol(ret.Water, newWater)
	}

	// 2) Pressure response. While HeatInput > 0 and there is water
	// to boil, Pressure relaxes toward the shell pressure scaled by
	// how much of the boil-off capacity is left after the offtake:
	// drawing faster than the SG can raise steam sags the header
	// pressure. Atmospheric relief draws from the same header as the
	// turbine and condenser dump ("This heat is rejected to the
	// condenser through the steam dump valves and to the atmosphere",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:6-7),
	// so it participates in that draw. With no HeatInput there is no
	// steam generation and Pressure decays toward zero.
	pressure := float64(ret.Pressure)
	if ret.HeatInput > 0 && ret.Water.Float() > 0 {
		outflow := steamOutflowFraction + advFraction
		ratio := 1.0
		if outflow > 1.0 {
			ratio = 1.0 / outflow
		}
		// Boiling beyond what the outlets take lifts the header
		// above nominal: with the turbine shut and the dumps
		// closed this is the only path to the steam-dump
		// pressure-mode setpoint during startup.
		// "These valves are also utilized in the pressure control
		//  mode during plant startup and plant cooldown."
		// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-26
		excess := boilRateM3PerSec/ratedBoilM3PerSec - outflow
		surplusTarget := float64(SteamGeneratorShellPressure) *
			min(1.0+excess,
				float64(SteamGeneratorShellDesignPressure)/float64(SteamGeneratorShellPressure))
		target := float64(SteamGeneratorShellPressure) * ratio
		if surplusTarget > target {
			target = surplusTarget
		}
		gap := target - pressure
		pressure += gap * dt.Seconds()
	} else if ret.HeatInput > 0 && pressure > 0 {
		// Water depleted while still boiling: no more steam to
		// keep the header up.
		pressure -= pressure * dt.Seconds()
	}
	pressure = max(pressure, 0)

	// 3) Pressure trip. Cap at the design pressure. A real SG
	// would relieve via the SG safety valves (out of scope per
	// plans/p6_1.txt).
	pressure = min(pressure, float64(SteamGeneratorShellDesignPressure))
	ret.Pressure = Pressure(pressure)

	return ret
}
