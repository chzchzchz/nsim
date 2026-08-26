Processing rate of the recycle evaporator package in gpm, drawn from the CVCS holdup tanks while a feed pump runs. It is the drain rate for [BRSHoldupLevel](AuxiliaryBuilding/ChemicalVolumeControlSystem/BRSHoldupLevel).

The reference quotes an average evaporator duty of about 10 gpm (38 l/min) without BTRS, falling below 2 gpm (8 l/min) when BTRS carries most boron changes; the simulator processes at the 10 gpm figure because the reclamation scenario runs precisely when boron changes exceed what BTRS can absorb. Feed passes through demineralizers that strip lithium and fission products, then a stripping column that removes hydrogen and residual gases before the liquid enters the evaporator shell.

Model note: the simulator holds feed flow flat at its constant instead of modulating it with plant state.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:71-74
Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:76-80

Related controls: [FeedPumpRunning](AuxiliaryBuilding/BoronRecycleSystem/FeedPumpRunning)
Related sensors: [BoricAcidProduced](AuxiliaryBuilding/BoronRecycleSystem/BoricAcidProduced), [DistillateDelivered](AuxiliaryBuilding/BoronRecycleSystem/DistillateDelivered)
