Mean core-exit temperature from the core-exit thermocouple aggregate, in Fahrenheit. The mean is the bulk mixed-mean outlet temperature the simulator carries; nominal design outlet is 558 F at full power, rising along the average coolant temperature (Tavg) program at low load. It is the temperature the hottest assembly's delta-T is hung from. The word core-exit is not in the reference table; the label comes from the bridge's thermocouple aggregate naming, not from the specification text.

Read it with [CoreExitDeltaT](ReactorBuilding/ReactorPressureVessel/CoreExitDeltaT) and [Inlet](ReactorBuilding/ReactorPressureVessel/Inlet): the exit mean pins the top of the core, the delta-T pins the local peak. A rising exit mean with a steady inlet means heat removal is degrading; both rising together means power is up or flow is down.

Related sensors: [Inlet](ReactorBuilding/ReactorPressureVessel/Inlet), [CoreExitDeltaT](ReactorBuilding/ReactorPressureVessel/CoreExitDeltaT)

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:55-62
