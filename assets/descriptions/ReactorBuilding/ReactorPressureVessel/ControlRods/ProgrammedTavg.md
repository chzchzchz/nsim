Programmed average coolant temperature target for the current load - the reference line the rod control system chases. Reading it next to actual Tavg gives the instantaneous control error without arithmetic.

The program rises with load so that secondary steam conditions stay favorable; after large load decreases steam dump augments rod control specifically to walk Tavg back to this program smoothly rather than oscillating around it.

Related controls: [AutoControl](ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl), [DumpValve](TurbineBuilding/Turbine/DumpValve)
Related sensors: [Tref](PlantOperation/Tref)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-33
