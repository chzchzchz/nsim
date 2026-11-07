Cumulative mechanical energy delivered by the turbine to the generator shaft since the start of the simulation, in megawatt-hours. This is the time integral of turbine [RealPower](TurbineBuilding/Turbine/RealPower): every second of shaft power adds to the total.

Sim instrumentation, not a spec requirement: the reference describes turbine output power but specifies no cumulative counter. Model note: the total is exact only as a simulation bookkeeping device; a real plant has no single shaft-energy totalizer.

Related sensors: [RealPower](TurbineBuilding/Turbine/RealPower), [CumulativeEnergy](TurbineBuilding/Generator/CumulativeEnergy)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
