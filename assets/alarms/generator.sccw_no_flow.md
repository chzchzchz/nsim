Stator coil cooling water flow lost. The generator stator bars are hollow copper conductors with demineralized water flowing through them; without that flow, winding temperature climbs toward insulation damage in minutes at any meaningful load. This is one of the few generator conditions where load reduction comes before diagnosis - copper does not wait.

Response order: attempt flow restoration (pumps, valves, strainers), and simultaneously begin shedding load via the [turbine runback](TurbineBuilding/Turbine/LoadRunback) path or breaker if temperatures keep climbing. The machine can sit at low or zero output indefinitely; it cannot survive full-load loss of cooling.

Usually this alarm arrives via its precursor - [low flow](generator.sccw_low_flow) that went unaddressed - so post-event review should ask why the earlier warning did not produce action.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:1-10

Related controls: [BreakerClosed](TurbineBuilding/Generator/BreakerClosed), [LoadRunback](TurbineBuilding/Turbine/LoadRunback)
Related sensors: [RealPower](TurbineBuilding/Generator/RealPower), [MVALimitAlarm](TurbineBuilding/Generator/MVALimitAlarm)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:1-10
