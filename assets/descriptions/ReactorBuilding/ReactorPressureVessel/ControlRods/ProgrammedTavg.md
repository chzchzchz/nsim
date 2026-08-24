Programmed average coolant temperature target for the current load - the reference line the rod control system chases. Reading it next to actual Tavg gives the instantaneous control error without arithmetic.

The program exists because the plant wants specific secondary conditions at each power level: steam pressure and temperature that the turbine was designed around. Raising load raises the program, warming the primary so SG steam pressure stays in band; the rods deliver that warm-up by withdrawing. The same signal feeds steam dump: after a large load decrease, dump valves open under Tavg-error control specifically to bring actual temperature back to this program without overshoot.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-33

Related controls: [AutoControl](ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl), [DumpValve](TurbineBuilding/Turbine/DumpValve)
Related sensors: [Tref](PlantOperation/Tref)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-33
