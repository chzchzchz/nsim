Effective auxiliary spray drive as a 0..1 fraction - what the pressurizer actually receives after the [aux spray valve](AuxiliaryBuilding/ChemicalVolumeControlSystem/AuxSprayValve/Open) position is gated by the charging pump latch.

The gate matters: auxiliary spray is just throttled charging flow routed to the pressurizer steam space, so with the charging pump secured the valve percentage reads promising while this value stays at zero. Use it to confirm that a manual pressure-lowering attempt with the reactor coolant pumps shut is really delivering spray, and how hard.

Model note: this is the simulator's computed product of valve position and pump availability, not a separate plant instrument.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104

Related controls: [Open](AuxiliaryBuilding/ChemicalVolumeControlSystem/AuxSprayValve/Open), [ChargingFlowDemand](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlowDemand)
Related sensors: [Pressure](ReactorBuilding/Pressurizer/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104
