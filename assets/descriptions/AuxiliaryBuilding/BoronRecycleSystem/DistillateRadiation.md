Distillate radiation monitor at the line into the reactor makeup water storage tank, normalized so 1.0 is the trip level. A scintillation counter sits in this line specifically so the storage tank "will not become contaminated".

The number to remember is 0.95: above it the high-radiation signal fires and [DistillateDiverted](AuxiliaryBuilding/BoronRecycleSystem/DistillateDiverted) routes distillate back to the recycle evaporator feed demineralizer instead of the tank. The divert releases only after the reading decays below 0.05, a hysteresis gap that stops divert chatter around the setpoint.

Model note: both thresholds and the monitor's 30 s response time are simulator-derived; the reference describes the monitor's function and routing action but states no numbers. Readings rise when evaporator [Carryover](AuxiliaryBuilding/Faults/EvaporatorCarryover) is latched and fall slowly after it clears.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:25-30

Related controls: [Carryover](AuxiliaryBuilding/Faults/EvaporatorCarryover)
Related sensors: [DistillateDiverted](AuxiliaryBuilding/BoronRecycleSystem/DistillateDiverted), [Recirculating](AuxiliaryBuilding/BoronRecycleSystem/Recirculating)
