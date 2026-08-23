package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the moisture separator-reheater by TimeStep. The
// caller supplies the inlet steam conditions (pressure, quality,
// temperature) and the reheat-steam supply (flow + pressure). The
// MSR drains the liquid fraction of the wet inlet steam to the
// feedwater system and reheats the dried steam with the supplied
// reheat steam, producing a superheated outlet temperature.
//
// Spec justification:
//   - "Combination moisture separator-reheater assemblies are
//     provided between the high- and low-pressure elements to dry
//     and superheat the steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_198.txt:43-45
//   - "Typical combined moisture separator-reheaters [...] remove
//     the moisture in the wet steam exhausting from the high-
//     pressure element exhaust and reheat the steam to over 100°F
//     (55°C) superheat."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:54-55
//   - "The wet steam enters the moisture removal section and rises
//     through chevron-type moisture-separators where the water is
//     removed and drained to the feedwater system."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:60-62
func (m *MoistureSeparatorReheater) Step(
	inletPressure Pressure,
	inletQuality float64,
	inletTemp Temperature,
	reheatSteamFlow Volume,
	reheatSteamPressure Pressure,
) MoistureSeparatorReheater {
	ret := *m
	dt := TimeStep

	// Outlet pressure drops slightly across the separator and
	// reheater bundles.
	ret.OutletPressure = inletPressure * 0.98
	ret.InletPressure = inletPressure
	ret.InletQuality = inletQuality

	// Moisture removal: the liquid water fraction leaves as
	// DrainFlow, condensed by the reheater steam. The chevron
	// separators remove most of the bulk liquid.
	liquidFraction := 1.0 - inletQuality
	if liquidFraction < 0 {
		liquidFraction = 0
	}
	drainGalPerStep := liquidFraction * reheatSteamFlow.Float() * dt.Seconds() * MSRMoistureRemovalEfficiency
	ret.DrainFlow = Volume{Val: drainGalPerStep, Unit: u.Gallon}

	// Reheat temperature: simplified scaling of the >100°F
	// superheat target by the extraction steam fraction. The MSR
	// receives MSRDesignExtractionFraction of the HP steam flow,
	// so normalise by that fraction so the design point reaches
	// the full MSRReheatTarget.
	reheatFraction := reheatSteamFlow.Float() /
		(FeedwaterDesignFlow.Float() * MSRDesignExtractionFraction)
	if reheatFraction > 1.0 {
		reheatFraction = 1.0
	}
	ret.OutletTemperature = Temperature{Val: inletTemp.Float()+MSRReheatTarget*reheatFraction, Unit: u.Fahrenheit}

	// Reheat steam pressure is passed through for state tracking.
	ret.ReheatSteamPressure = reheatSteamPressure
	ret.ReheatSteamFlow = reheatSteamFlow

	return ret
}
