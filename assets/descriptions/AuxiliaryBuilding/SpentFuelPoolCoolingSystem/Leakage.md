Fault insert that opens a leak in the spent fuel pool cooling supply header. Real leakage announces itself the same way the model plays it: installed pressure, temperature, and flow instrumentation detects it, and motor-operated valves isolate the affected section.

Watch [HeaderPressure](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/HeaderPressure) sag out of its 55 to 75 psig normal band while the pumps keep running - flow is escaping through the leak instead of returning to the pool. If pressure falls below 50 psig the standby pump start logic engages. Isolating the leak restores the pressure balance; leaving it latched keeps the system fighting a hole.

Source: specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:75-83

Related sensors: [HeaderPressure](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/HeaderPressure), [PoolTemp](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PoolTemp)
