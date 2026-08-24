Initiates the emergency diesel generator start signal. Options: **None**, **Manual**, **Safety Injection**, **Loss of Offsite Power**, or **Test** - the automatic starts mirror exactly the two events that strand the ESF buses from their normal off-site supply.

The diesel exists because safe shutdown loads cannot tolerate an outage: on a valid start it must reach rated speed and voltage and accept load within 30 seconds. That requirement drives everything about its standby posture - jacket water kept warm, air receivers charged, fuel day tank full - because there is no time to fix anything after the demand arrives.

Controls for governor, voltage regulator, breaker, and synchronization sit on physically separated board sections per train, so one control-room mishap cannot disable both machines.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:133-136
Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8

Related controls: [RunningFraction](AuxiliaryBuilding/DieselGenerator/RunningFraction)
Related sensors: [ReadyToLoad](AuxiliaryBuilding/DieselGenerator/ReadyToLoad), [Energized](Electrical/ESFBusA/Energized)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:133-136
Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8
