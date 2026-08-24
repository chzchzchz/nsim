Opens the reactor makeup control valve, admitting makeup water from the chemical and volume control system into the RCS - how the simulator models influx of makeup water during inventory additions.

What arrives depends on the CVCS lineup: [charging pump state](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning) and [boron mode](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode) determine whether the water is borated, diluted, or plain primary-grade, while this valve meters quantity. In the reference plant, RCS inventory additions during shutdown operations happen through precisely this kind of balance - charging flow raised, letdown held, mass added, pressure rises because water is virtually incompressible.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
Source: specs/westinghouse_plant_operations/chapter_19/page_004.txt:9-13

Related controls: [BoronMode](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode)
Related sensors: [ChargingFlow](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
