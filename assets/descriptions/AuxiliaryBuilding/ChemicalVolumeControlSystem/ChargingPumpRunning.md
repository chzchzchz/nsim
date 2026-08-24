Runs a CVCS charging pump, taking water from the volume control tank and injecting it into the cold legs at high pressure. Charging is the RCS's circulation-and-inventory heart: it pushes water through letdown for cleanup, feeds RCP seals, and adds or removes primary mass on command.

During startup, speed is held to the minimum allowable RCP seal injection flow until the pressurizer bubble exists and level reaches no-load setpoint, then switched to automatic. From there the [pressurizer level control system](AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoLevelControl) adjusts charging continuously against programmed level. Charging versus letdown is the inventory seesaw: equal flows hold pressure constant in a solid plant; imbalance moves mass, and water being nearly incompressible, mass change is pressure change.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:23-27
Source: specs/westinghouse_plant_operations/chapter_19/page_004.txt:9-16

Related controls: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning), [AutoLevelControl](AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoLevelControl)
Related sensors: [ChargingFlow](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow), [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)

Source: $W/chapter_16/page_224.txt:23-27
Source: $W/chapter_16/page_226.txt:30-33
