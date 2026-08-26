Pressure in the spent fuel pool cooling main supply header, psig. The system is designed to hold 55 to 75 psig here while a pump is serving the pool.

Pressure is this system's leak detector: a fall out of the normal band with the pump still running means flow is going somewhere other than back to the pool (see [Leakage](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Leakage)). The model starts the standby pump if the header drops below 50 psig; sustained low pressure with both pumps running says the hole is bigger than added pumping.

Model note: the 50 psig standby-pump start threshold is a simulator constant; the reference gives the operating band but names no start setpoint.

Source: specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-47

Related controls: [Leakage](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Leakage)
Related sensors: [PoolTemp](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PoolTemp)
