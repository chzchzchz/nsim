Steam pressure downstream of the governing valves in psig - effectively an instantaneous, fast load meter, since pressure there scales almost linearly with steam mass flow into the machine.

The DEHC uses it alongside speed as feedback in its control computations - the reference describes signals comparing turbine speed AND first-stage pressure with reference values. It reacts faster than generator output to governor movement (no electrical dynamics in the way), making it the right place to watch when diagnosing valve problems versus electrical problems.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41

Related sensors: [GoverningValve](TurbineBuilding/Turbine/GoverningValve), [RealPower](TurbineBuilding/Turbine/RealPower)

Source: $W/chapter_15/page_201.txt:37-41
