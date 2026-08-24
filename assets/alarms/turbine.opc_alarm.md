Turbine overspeed permissive control has alarmed - shaft speed exceeded 103 percent of rated speed (1854 rpm on the 1800 rpm machine), the OPC alarm threshold. Understanding the layering matters: governor control holds speed under normal perturbations, OPC acts first at a permissive threshold, and only mechanical overspeed trip sits behind it as the last resort. An OPC alarm means the outer layers were tested but held.

The usual driver is sudden load rejection with the generator breaker open or opening: steam flow that was becoming megawatts has nowhere to go, and the shaft accelerates until governors and dump valves catch it. On a grid absorbing random load swings of about ±5 percent through droop alone, steady-state operation should never see this alarm - repeated occurrences during stable operation point at governing valve trouble rather than grid behavior.

Watch [shaft speed](TurbineBuilding/Turbine/Velocity) recover and settle back to rated; if speed oscillates instead, suspect the governing valves before blaming the grid.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_202.txt:11-14
Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-73
Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:10-14

Related controls: [Setpoint](TurbineBuilding/Turbine/Setpoint), [SpeedDemand](TurbineBuilding/Turbine/SpeedDemand)
Related sensors: [Velocity](TurbineBuilding/Turbine/Velocity), [State](TurbineBuilding/Turbine/State)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_202.txt:11-14
