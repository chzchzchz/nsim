Initiates the emergency diesel generator start signal. Options: **None**, **Manual**, **Safety Injection**, **Loss of Offsite Power**, or **Test** - the automatic starts mirror the two events that strand the ESF buses from their normal off-site supply.

Each diesel is designed to carry its assigned ESF bus load, reaching rated load within 30 seconds of a start signal. Controls for governor, voltage regulator, breaker, and synchronization live on physically separated board sections so one control-room event cannot disable both trains.

Related controls: [RunningFraction](AuxiliaryBuilding/DieselGenerator/RunningFraction)
Related sensors: [ReadyToLoad](AuxiliaryBuilding/DieselGenerator/ReadyToLoad), [Energized](Electrical/ESFBusA/Energized)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:133-136
Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8
