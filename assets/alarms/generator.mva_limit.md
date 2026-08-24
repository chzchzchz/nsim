Generator MVA capability limit exceeded. The machine is operating outside its capability curve - almost always reactive burden rather than megawatts - and winding temperatures are trading insulation life for the excess, with damage measured in months of remaining life per hour of abuse.

The two control handles act on different axes: excitation reshapes VAr exchange (watch [terminal voltage](TurbineBuilding/Generator/Voltage) respond), turbine load reshapes real power. Standard recovery trims whichever axis is driving the violation - usually reduce VAr export by lowering voltage setpoint, or run back real load via [LoadRunback](TurbineBuilding/Turbine/LoadRunback) if voltage support obligations pin the reactive side.

Sustained alarm means the operating point is parked outside the curve; transient excursions during grid events are tolerated briefly by design, then not at all.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:1-10

Related controls: [LoadRunback](TurbineBuilding/Turbine/LoadRunback)
Related sensors: [Voltage](TurbineBuilding/Generator/Voltage), [Frequency](TurbineBuilding/Generator/Frequency), [RealPower](TurbineBuilding/Generator/RealPower)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:1-10
