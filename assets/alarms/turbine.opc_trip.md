The overspeed permissive control has tripped the turbine at 110 percent of rated speed (1980 rpm): stop valves slammed, steam admission cut, and the machine is coasting down under supervision instead of flying apart. The trip exists because blading endurance margins are finite - overspeed beyond design stresses rotors in ways inspection cannot always find later.

Consequences cascade exactly like any turbine trip: reactor power cannot follow instantly, so dump burden lands on the condenser via the [dump valves](TurbineBuilding/Turbine/DumpValve), Tavg walks off program until rods bring it back, and the operator works the standard post-trip sequence - verify heat removal, stabilize temperature and pressure, then diagnose.

The diagnosis target is specific here: what accelerated the shaft past the permissive? Load rejection is benign and recoverable; governor malfunction or valve sticking means finding the fault before any [reset](TurbineBuilding/Turbine/Reset) attempt, because resetting into the same condition simply retrips with less margin.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:12-18
Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-73

Related controls: [Reset](TurbineBuilding/Turbine/Reset), [StopValve](TurbineBuilding/Turbine/StopValve)
Related sensors: [Velocity](TurbineBuilding/Turbine/Velocity), [State](TurbineBuilding/Turbine/State), [Pressure](ReactorBuilding/SteamGenerator/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:12-18
