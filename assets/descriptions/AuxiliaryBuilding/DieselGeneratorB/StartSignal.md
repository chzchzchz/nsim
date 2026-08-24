Initiates the train B emergency diesel start signal. Options: **None**, **Manual**, **Safety Injection**, **Loss of Offsite Power**, or **Test** - the automatic starts mirror exactly the two events that strand this train's ESF bus from its off-site supply.

Each train's diesel exists because its safe-shutdown loads cannot tolerate an outage: on a valid start it must reach rated conditions and accept load within 30 seconds. Standby posture - warm jacket water, charged air receivers, full day tank - is what makes that deadline reachable after the demand arrives, not before it.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:133-136
Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8

Related controls: [RunningFraction](AuxiliaryBuilding/DieselGeneratorB/RunningFraction), [StartSignal](AuxiliaryBuilding/DieselGenerator/StartSignal)
Related sensors: [Energized](Electrical/ESFBusB/Energized)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8
