Condenser hotwell radiation monitor reading, normalized so 1 is the trip level. A steam generator tube leak carries primary water into the secondary steam that condenses in the hotwell, so the hotwell sample is the condenser-side witness to the rupture. It lags the [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation) monitor with a slower time constant and decays more slowly after the leak stops.

It is diagnostic only: it never drives the steam-line auto-isolate latch, so a high hotwell reading alone does not secure the steam path. Use it to confirm a tube rupture after the steam-line monitor has already alarmed; a rising hotwell with a steady steam-line reading means the leak is still flowing through the turbine path. The hotwell side reads about three times lower than the steam line at the same leak, so it confirms late but then decays late as well.

Related sensors: [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation), [SGTubeRuptureFlow](ReactorBuilding/SteamGenerator/SGTubeRuptureFlow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-20
