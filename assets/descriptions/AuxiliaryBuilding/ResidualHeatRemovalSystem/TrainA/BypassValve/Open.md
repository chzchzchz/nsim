RHR train A bypass valve position. Modulating flow around the RHR heat exchanger controls how much heat each pass removes - this is the cooldown-rate handle, since cooldown rate is bounded by the same thermal limits that cap heatup at 50F/hr, and by brittle-fracture pressure-temperature curves early in vessel life.

Full bypass sends minimal flow through the exchanger (slow cooldown); closing bypass maximizes transfer. Performance ceiling comes from [component cooling water](AuxiliaryBuilding/ComponentCoolingWaterSystem/OutletTemp) supply temperature - the exchanger can only cool primary down toward what CCW brings in.

Related controls: [PumpRunning](AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning)
Related sensors: [OutletTemp](AuxiliaryBuilding/ComponentCoolingWaterSystem/OutletTemp), [PumpRunning](AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:1-14
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:25-27
