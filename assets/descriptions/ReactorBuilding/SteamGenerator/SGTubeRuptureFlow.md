Leak rate through the steam generator tube rupture, in gallons per minute. The same call debits the reactor inventory and credits the secondary shell, then hands the flow to the [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation) and hotwell monitors. A tube rupture is primary water leaking through the tube sheet into the secondary side: unlike a main steam line break it cannot be isolated by the main steam isolation valve and ends only when the RCS depressurizes below shell pressure.

Trending matters more than the number: a slow, steady rate with a climbing shell level is a pinhole; a rate that escalates as the RCS pulls the level down is the larger rupture sizes this control feeds. Cross-check [Pressure](ReactorBuilding/SteamGenerator/Pressure) and [Water](ReactorBuilding/SteamGenerator/Water) to separate leak from normal inventory accounting.

Model note: the leak law (linear in pressure differential) and the termination condition are model simplifications; the cited reference text is an auto-generated figure description that labels the tube sheet only.

Related sensors: [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation), [Pressure](ReactorBuilding/SteamGenerator/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/imgs/023.md:4-4
