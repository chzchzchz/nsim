Steamline pressure low. Two families of causes with very different consequences: demand outrunning production (dump valves open, fast turbine loading) recovers when the load stabilizes; a steam line break depressurizes continuously, drags SG levels down with it, and is the case this alarm exists to catch early.

Discriminate by trend: recovering pressure points at load, continuing decline points at breach. A confirmed break calls for [steamline isolation](rps.steamline_isolation) - closing the MSIVs starves the leak while [atmospheric dump valves](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open) upstream of the isolation retain a controlled decay heat path.

Secondary side inventory leaves with escaping steam, so track [SG water](ReactorBuilding/SteamGenerator/Water) alongside pressure; feedwater must replace what the break takes.

Model note: raised directly off [steam pressure](ReactorBuilding/SteamGenerator/Pressure) against the 600 psig RPS plate when the P-7 at-power permissive is satisfied.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10

Related controls: [MainSteamIsolationValve](ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open), [AtmosphericDumpValve](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open)
Related sensors: [Pressure](ReactorBuilding/SteamGenerator/Pressure), [MainSteamIsolationValve](ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
