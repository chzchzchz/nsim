Fault insert for a refueling-canal crud burst: a disturbance of the fuel-assembly surface deposits throws corrosion products into the pool water, spiking [Turbidity](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Turbidity).

The model adds a 100 percent turbidity slug when this latches, which is the design burden the cleanup side is sized against. Recovery is a purification problem, not a cooling one: run purification ([RWPUPurifying](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/RWPUPurifying)) until clarity returns. The operational stakes come from the spec's own concern - regaining canal water clarity after a crud burst - because operators rely on visual access to the fuel during handling.

Source: specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-90

Related sensors: [Turbidity](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Turbidity)
Related controls: [RWPUPurifying](AuxiliaryBuilding/SpentFuelPoolCoolingSystem/RWPUPurifying)
