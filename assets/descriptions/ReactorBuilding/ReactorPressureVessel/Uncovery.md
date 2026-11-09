Net uncovered core fraction as a percent: cumulative break discharge (net of credited injection, but floored at the non-creditable losses) over drainable inventory. Zero means the core is fully covered; one means fully uncovered. It is the fraction of the core the clad-temperature integrator treats as steam-cooled instead of water-cooled.

This is the number the severe-accident thresholds gate on: oxidation and core damage only advance while it is above zero, and vessel failure needs both uncovery and 3410 F clad. Watch [BreakDischarged](ReactorBuilding/ReactorPressureVessel/BreakDischarged) to see the inventory behind it, and [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning) to see the credit fighting it. Small values matter because the clad integrator splits covered from steam-exposed share; even a few percent uncovered starts the oxidation clock at full rate on that share.

Related sensors: [BreakDischarged](ReactorBuilding/ReactorPressureVessel/BreakDischarged), [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:51-53
