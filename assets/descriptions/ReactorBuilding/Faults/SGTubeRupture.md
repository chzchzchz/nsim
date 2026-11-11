Simulator construct with no plant control: a normalized rupture-size latch for a steam generator tube rupture, filed with the accident faults. 0 is intact; 1 inserts the double-ended-equivalent full break of the largest tube. The leak it drives lands in [SGTubeRuptureFlow](ReactorBuilding/SteamGenerator/SGTubeRuptureFlow), which debits the primary and credits the secondary; the radiation monitor on the steam line and the hotwell both watch the boiled-off fraction.

This is not a main steam line break: it does not vent the header to containment and the main steam isolation valve cannot stop it. It terminates only when reactor pressure falls below shell pressure, usually through a reactor trip. Set it low for a pinhole leak, high for the full rupture, and clear it to end the leak.

Model note: the normalized size scale and the termination condition are model simplifications; the cited reference text is an auto-generated figure description that labels the tube sheet only.

Related sensors: [SGTubeRuptureFlow](ReactorBuilding/SteamGenerator/SGTubeRuptureFlow), [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation)

Source: specs/the_westinghouse_pressurized_water_react/imgs/023.md:4-4
