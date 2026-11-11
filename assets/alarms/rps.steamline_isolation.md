Steamline isolation has actuated: main steam isolation valves closed, cutting each affected generator off from the turbine and dump systems. The trigger is a steam line breach indication or an operator command - both mean the secondary steam system is being sealed to stop uncontrolled release.

Isolation stops the leak but also stops that path's heat removal while decay heat continues, so immediately verify alternate paths: [atmospheric dump valves](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open) sit upstream of the isolation and remain available, and SG [safety valves](ReactorBuilding/SteamGenerator/SafetyValve/Open) still guard overpressure. Track [SG pressure](ReactorBuilding/SteamGenerator/Pressure) climbing as proof the isolated generator needs its relief path working.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10

Related controls: [AtmosphericDumpValve](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open)
Related sensors: [MainSteamIsolationValve](ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open), [Pressure](ReactorBuilding/SteamGenerator/Pressure), [Water](ReactorBuilding/SteamGenerator/Water)
