Water inventory in the pressurizer in gallons, displayed against a program that rises with Tavg - hotter coolant occupies more volume, so the program asks for a larger water fraction as power climbs. The CVCS maintains actual level against this program automatically, adjusting charging versus letdown.

The band matters both ways. Too little water uncovers the electric heaters, which burn out in air, and shrinks the cushion that softens outsurges. Too much leaves no room for an insurge and floods the spray nozzle's ability to reach steam. During startup the operator raises [letdown flow](AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownFlow) specifically to pull level down to roughly 25 percent as the steam bubble forms, creating expansion room before heatup begins.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:19-23
Source: specs/westinghouse_plant_operations/chapter_19/page_005.txt:24-27

Related controls: [AutoLevelControl](AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoLevelControl)
Related sensors: [ProgrammedLevel](AuxiliaryBuilding/ChemicalVolumeControlSystem/ProgrammedLevel), [Pressure](ReactorBuilding/Pressurizer/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:19-23
Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:20-24
