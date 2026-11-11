Distillate volume, in gallons, held inside the reclaim train while the auto-divert is latched. This is water that already evaporated but failed its radiation check at the storage-tank line, so it loops back through the feed demineralizer for another pass.

It is the backlog you wait on. The divert releases when the monitor clears, then the train reprocesses this inventory before clean delivery to the reactor makeup water storage tank resumes and [DistillateDelivered](AuxiliaryBuilding/BoronRecycleSystem/DistillateDelivered) climbs again. A large Recirculating value after a [Carryover](AuxiliaryBuilding/Faults/EvaporatorCarryover) event means a long tail of re-evaporation ahead.

Model note: routing and accumulation are simulator-derived bookkeeping; the reference describes the divert path but not the holdup volume.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:28-30

Related sensors: [DistillateDiverted](AuxiliaryBuilding/BoronRecycleSystem/DistillateDiverted), [DistillateRadiation](AuxiliaryBuilding/BoronRecycleSystem/DistillateRadiation), [DistillateDelivered](AuxiliaryBuilding/BoronRecycleSystem/DistillateDelivered)
