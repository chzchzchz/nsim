Cumulative electrical energy produced by diesel generator A since the start of the simulation, in megawatt-hours. This is the time integral of diesel [RealPower](AuxiliaryBuilding/DieselGenerator/Generator/RealPower) while the engine is running.

Useful in station-blackout and loss-of-offsite-power scenarios for measuring how much of the engineered-safety-features load the backup supply actually carried. It complements, but does not replace, the fuel-tank level at [Fuel](AuxiliaryBuilding/DieselGenerator/Fuel): the same running time that adds energy here depletes fuel there.

Sim instrumentation, not a spec requirement: the reference requires monitoring of diesel output but specifies no cumulative counter.

Related sensors: [RealPower](AuxiliaryBuilding/DieselGenerator/Generator/RealPower), [Fuel](AuxiliaryBuilding/DieselGenerator/Fuel), [ReadyToLoad](AuxiliaryBuilding/DieselGenerator/ReadyToLoad)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:97-101
