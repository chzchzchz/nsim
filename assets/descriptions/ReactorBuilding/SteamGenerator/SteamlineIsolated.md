One-way-in-practice latch that the steam-line radiation monitor has auto-isolated the steam path: the main steam isolation valve is closed and main feedwater is secured. It sets when the [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation) monitor crosses its trip level and clears only when the leak stops and the reading decays below the release level, which is the trip and release hysteresis.

A latched isolation during a tube rupture is the protection working, not a valve failure: the plant is holding the primary water inside the secondary boundary. Closing the loop means confirming the [SGTubeRuptureFlow](ReactorBuilding/SteamGenerator/SGTubeRuptureFlow) stopped before expecting the latch to clear. The [HotwellRadiation](TurbineBuilding/Condenser/HotwellRadiation) monitor is diagnostic only and never drives this latch. Reopening the steam path before the leak is isolated re-admits the primary leak to the turbine plant, which the latch exists to prevent.

Related sensors: [SteamlineRadiation](ReactorBuilding/SteamGenerator/SteamlineRadiation), [SGTubeRuptureFlow](ReactorBuilding/SteamGenerator/SGTubeRuptureFlow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:88-92
