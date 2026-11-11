Auto-divert status: true while the high distillate radiation signal has product routed back to the recycle evaporator feed demineralizer instead of on to the reactor makeup water storage tank.

The sequence it reflects: the scintillation counter in the distillate line sees [DistillateRadiation](AuxiliaryBuilding/BoronRecycleSystem/DistillateRadiation) above its trip level, the divert latches, and everything evaporated recirculates for reprocessing until the monitor decays back below its release level. While diverted, [DistillateDelivered](AuxiliaryBuilding/BoronRecycleSystem/DistillateDelivered) stops accumulating and [Recirculating](AuxiliaryBuilding/BoronRecycleSystem/Recirculating) inventory grows; that inventory is contaminated water the train must re-evaporate before makeup delivery resumes.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:28-30

Related controls: [Carryover](AuxiliaryBuilding/Faults/EvaporatorCarryover)
Related sensors: [DistillateRadiation](AuxiliaryBuilding/BoronRecycleSystem/DistillateRadiation), [Recirculating](AuxiliaryBuilding/BoronRecycleSystem/Recirculating)
