package core

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
