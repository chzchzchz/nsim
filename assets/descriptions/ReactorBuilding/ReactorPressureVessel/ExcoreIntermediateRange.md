Excore intermediate-range detector current in percent of full power. It reads percent of rated inside its window from 1 to 25 percent power: zero below (source range covers it) and saturated at 25 above (power range takes over). The handover is continuous because both ranges use the same percent-of-full-power law; at the crosspoint both read 25.

Use it as the primary power meter during low-power physics: boron worth, rod worth, and xenon transients all ride on it. Below 1 percent fall back to [SourceRangeCounts](ReactorBuilding/ReactorPressureVessel/SourceRangeCounts), above 25 use [ExcorePowerRange](ReactorBuilding/ReactorPressureVessel/ExcorePowerRange). It shares the same ion chambers as the power range, so a disagreement between the two at the 25 percent crosspoint points at the detector rather than at the core.

Model note: the 1 to 25 percent window is simulator-derived; the reference names the intermediate range but gives no window edges.

Related sensors: [ExcorePowerRange](ReactorBuilding/ReactorPressureVessel/ExcorePowerRange), [SourceRangeCounts](ReactorBuilding/ReactorPressureVessel/SourceRangeCounts)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_119.txt:74-78
