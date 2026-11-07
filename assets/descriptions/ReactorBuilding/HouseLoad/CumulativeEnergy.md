Cumulative station-service energy consumed by the plant's own auxiliaries since the start of the simulation, in megawatt-hours. This is the time integral of house load demand: reactor coolant pumps, feedwater and condensate pumps, heaters, ESF bus trickle charge, HVAC, lighting, and controls.

Combined with the supply source selected at `HouseLoadSuppliedFrom` on the station transformer, this separates energy self-supplied through the UAT from energy imported from the offsite grid through the SAT. A high running total with the breaker open means the grid, not the unit, is carrying the plant.

Sim instrumentation, not a spec requirement: the reference describes auxiliary power flows and automatic transfer but specifies no cumulative counter.

Related sensors: [CumulativeEnergy](TurbineBuilding/Generator/CumulativeEnergy)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
