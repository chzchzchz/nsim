Starts or stops the reactor coolant pumps (RCPs) - the four massive machines (each motor on the order of 7 MW) that force roughly 100,000 gpm of primary water around its loop. Without them, heat transfer depends on natural circulation alone, which works but moves far less water.

Starting them is a procedure, not a switch flip: RCS pressure must be at least 320 psig to provide net positive suction head for the pumps and enough differential pressure to lift the number one shaft seals, and [seal injection](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning) from the CVCS must be flowing to cool and lubricate the bearings and seal packages. During heatup the pumps are started sequentially and their heat input is deliberately used - RCP work plus decay heat is what warms the plant to no-load temperature before criticality.

At shutdown the mirror rule applies: at least one RCP keeps running for uniform loop cooling until temperature falls below 160F (71C), and in hot standby a maximum of two of four may be secured.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:37-40
Source: specs/westinghouse_plant_operations/chapter_19/page_005.txt:1-14

Related controls: [PumpRunning](AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning), [HeatersOn](ReactorBuilding/Pressurizer/HeatersOn)
Related sensors: [Speed](ReactorBuilding/ReactorCoolantPump/Speed), [PumpRunning](AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:37-40
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:24-29
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:31-33
