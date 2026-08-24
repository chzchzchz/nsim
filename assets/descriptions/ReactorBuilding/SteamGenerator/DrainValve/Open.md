Drains the steam generator secondary side. Fully open empties the generator for maintenance or lays it down dry; cracked open it lowers level when normal removal paths are unavailable.

A drain valve is a one-way street operationally: water leaving here does not return through it, and an SG drained below tube-sheet level uncovers the primary tubes' heat sink. Before opening, know where decay heat will go instead - [AFW](rps.afw_start) feeds the generator but only helps while there is water to boil. During wet layup evolutions this stays shut; the fill path is separate.

Model note: the simulator models drain flow as proportional to position; no drain-line time constant is carried.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122

Related controls: [Open](ReactorBuilding/SteamGenerator/SteamlineVent/Open), [WetLayupFillValve](ReactorBuilding/SteamGenerator/WetLayupFillValve/Open)
Related sensors: [Water](ReactorBuilding/SteamGenerator/Water), [Pressure](ReactorBuilding/SteamGenerator/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
