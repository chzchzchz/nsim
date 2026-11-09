Activity dissolved in the containment sump water, in normalized units where 1.0 is rated-power equilibrium. Containment spray scrubs iodine from the air into the sump, and long-term recirculation carries a fraction back to the reactor on hot-leg recirculation. The sump is the sink that keeps airborne activity down.

A rising sump reading with a falling [ContainmentAirActivity](ReactorBuilding/ContainmentBuilding/ContainmentAirActivity) is the spray doing its job. The sump returns slowly to the primary during recirculation, so expect the [PrimaryActivity](ReactorBuilding/ReactorPressureVessel/PrimaryActivity) inventory to hold up after switchover even as the containment air clears, and expect the scrub rate to track spray flow rather than reactor power once the pumps own the timeline. The sump never removes noble gas, since it stays airborne, so a rising sump with a flat air reading still leaves the containment atmosphere with its gas inventory.

Related sensors: [ContainmentAirActivity](ReactorBuilding/ContainmentBuilding/ContainmentAirActivity)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:12-18
