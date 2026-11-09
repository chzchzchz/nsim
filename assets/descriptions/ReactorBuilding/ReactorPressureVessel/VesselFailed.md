One-way latch that the vessel has failed: uncovered clad past the 3410 F zirconium melting regime. Severe-accident state does not recover, so no trip, reset, or reflood clears it. Once true the core is in the meltdown regime and the reflood inventory calculations stop being a damage-limitation exercise.

The path here is narrow by design: the clad must be both uncovered ([Uncovery](ReactorBuilding/ReactorPressureVessel/Uncovery) above zero) and past 3410 F, which requires sustained failure of injection under a large break. Read [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp) approaching the threshold; the [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac) saturates long before this latch, so vessel failure is well past the 2200 F limit.

Model note: 3410 F is a derived structural-integrity anchor; covered clad can never reach it because it is pinned at saturation.

Related sensors: [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp), [Uncovery](ReactorBuilding/ReactorPressureVessel/Uncovery)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:51-53
