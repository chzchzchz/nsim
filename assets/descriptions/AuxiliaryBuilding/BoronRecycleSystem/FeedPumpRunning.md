Panel latch that starts a recycle evaporator feed pump, drawing borated liquid from the CVCS holdup tanks into the reclaim train: feed demineralizers, feed filter, stripping column, then the evaporator shell.

The BRS is manually operated from the boron recycle panel, so nothing runs until this latch is set. At the without-BTRS average evaporator duty the train processes about 10 gpm (38 l/min); heavy use of BTRS for load follow drops typical demand below 2 gpm (8 l/min). Watch [BRSHoldupLevel](AuxiliaryBuilding/ChemicalVolumeControlSystem/BRSHoldupLevel) while running: the pump draws it down at the [FeedFlow](AuxiliaryBuilding/BoronRecycleSystem/FeedFlow) rate, and letting it refill means diverting letdown again.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:1-3
Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:67-74

Related sensors: [FeedFlow](AuxiliaryBuilding/BoronRecycleSystem/FeedFlow), [BRSHoldupLevel](AuxiliaryBuilding/ChemicalVolumeControlSystem/BRSHoldupLevel)
