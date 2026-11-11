Simulator construct with no plant control: a stuck-open malfunction latch filed with the faults. The physical valve is a self-actuating ASME spring-loaded safety valve in each SG outlet main steam line - secondary-side overpressure protection that needs neither power nor operator action. It lifts when steam pressure overcomes the spring, vents to atmosphere, and reseats when pressure falls back.

With the latch set, the valve holds open regardless of steam pressure, venting to atmosphere at the modeled rate (0.5 of rated steam flow). Set the latch back to 0 to clear it - the simulator has no other way to reseat a mechanically stuck valve. When the latch is clear the valve follows the pressure law alone (pops at the model's assumed 985 psia setpoint - the spec names the valves but quotes no value; reseats about 3 percent below it, hysteresis held in between).

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:42-45

Related controls: [MainSteamIsolationValve](ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open)
Related sensors: [MainSteamIsolationValve](ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open), [Pressure](ReactorBuilding/SteamGenerator/Pressure)
