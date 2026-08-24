Self-actuating ASME-code spring safety valve on the pressurizer - the final, unpowered stage of primary overpressure protection. If system pressure keeps rising past the PORV stage, the spring simply gets overcome and the valve opens; no signal, no power, no operator. Steam discharges to the [pressurizer relief tank](ReactorBuilding/PressurizedReliefTank/Level).

These valves reseat automatically when pressure falls back below setpoint, but each lift is an event worth understanding: sustained safety flow means the powered stages failed to hold the line. The bridge exposes aggregate position mostly for visibility - simulation lifts the valve automatically at the code setpoint regardless of the control's setting.

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-34

Related controls: [PORV](ReactorBuilding/PORV/Open), [PRTSprayValve](ReactorBuilding/PRTSprayValve/Open)
Related sensors: [PORV](ReactorBuilding/PORV/Open), [Level](ReactorBuilding/PressurizedReliefTank/Level)

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-34
