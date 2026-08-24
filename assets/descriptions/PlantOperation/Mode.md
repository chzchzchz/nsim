Selects the plant operating mode: **ColdShutdown**, **HotStandby**, or **PowerOperation**. The modes matter because they gate which evolutions are allowed: a cold plant can be opened for maintenance, a hot standby plant can return to power quickly, and only PowerOperation exports electricity.

The reference startup walks through all three. From cold shutdown (MODE 5: Tavg below 200F, core keff under 0.99) the operators complete pre-startup checklists, establish the pressurizer steam bubble, start reactor coolant pumps, heat up to no-load temperature, dilute boron and withdraw rods to take the reactor critical, then roll and synchronize the turbine. Hot standby sits between criticality and power: subcritical or at zero load with Tavg held at program by steam dump, ready to resume loading within minutes. Above 200F primary temperature, Technical Specifications compliance is verified - containment integrity secured and auxiliary feedwater operable are two explicit examples.

Placing the mode is an operator action during startups and shutdowns rather than something the control system does on its own.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
Source: specs/westinghouse_plant_operations/chapter_19/page_004.txt:40-52
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:1-11

Related controls: [PowerSetpointTarget](PlantOperation/PowerSetpointTarget), [RodControlAuto](PlantOperation/RodControlAuto)
Related sensors: [Tref](PlantOperation/Tref)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_223.txt:20-27
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:1-11
