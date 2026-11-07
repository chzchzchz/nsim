Cumulative net electrical energy delivered to the grid by the main generator since the start of the simulation, in megawatt-hours. This is the time integral of net generator [RealPower](TurbineBuilding/Generator/RealPower), that is, gross turbine output minus station house load, while the generator breaker is closed.

Sim instrumentation, not a spec requirement: the reference describes output power and monitoring but specifies no cumulative counter. Model note: the total counts only energy exported to the grid; station-service energy drawn through the UAT is excluded here and tracked at [HouseLoad](ReactorBuilding/HouseLoad/CumulativeEnergy).

Related sensors: [RealPower](TurbineBuilding/Generator/RealPower), [CumulativeEnergy](TurbineBuilding/Turbine/CumulativeEnergy), [HouseLoad](ReactorBuilding/HouseLoad/CumulativeEnergy)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
