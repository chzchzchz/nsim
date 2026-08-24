Makeup flow through the reactor makeup control valve in gpm - the instantaneous readout of how much water the CVCS is injecting into the RCS right now.

Compare it against [letdown flow](AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownFlow) to read net inventory movement at a glance: charging exceeding letdown grows primary mass, which in a nearly incompressible water system shows up as rising [pressurizer level](ReactorBuilding/Pressurizer/Water) and slowly climbing pressure. The reference startup procedure exploits exactly this relationship, balancing the two flows to hold pressure between 320 and 400 psig while the plant is solid.

Source: specs/westinghouse_plant_operations/chapter_19/page_004.txt:9-16

Related controls: [ReactorMakeupControlValve](ReactorBuilding/ReactorMakeupControlValve/Open), [ChargingPumpRunning](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning)
