Simulator construct with no plant control: a stuck-open malfunction latch filed with the faults. The physical valve is a self-actuating ASME-code spring safety valve on the pressurizer - the final, unpowered stage of primary overpressure protection. If system pressure keeps rising past the PORV stage, the spring gets overcome and the valve opens; steam discharges to the [pressurizer relief tank](ReactorBuilding/PressurizedReliefTank/Level).

With the latch set, the valve holds open regardless of pressure: relief discharge drains through the common PORV/safety-valve line-up gated by the block valve (max(PORV, safety valve) * BlockValve) into the PRT, so one BlockValve == 0 isolates both paths. Set the latch back to 0 to clear it - the simulator has no other way to reseat a mechanically stuck valve. When the latch is clear the valve follows the pressure law alone (open above the model's derived 2,600 psia setpoint, closed below it).

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-34

Related controls: [PORV](ReactorBuilding/PORV/Open), [BlockValve](ReactorBuilding/BlockValve/Open), [PRTSprayValve](ReactorBuilding/PRTSprayValve/Open)
Related sensors: [PORV](ReactorBuilding/PORV/Open), [Level](ReactorBuilding/PressurizedReliefTank/Level)
