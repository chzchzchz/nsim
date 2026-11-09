Steam-line radiation monitor reading, normalized so 1 is the trip level. The detector sits on the steam line and watches radiation through the pipe wall; the reading is driven by leaked primary water that boiled into the steam header. It is a first-order lag toward trip while the tube leak flows and decays after the leak stops.

Crossing 1.0 sets the [SteamlineIsolated](ReactorBuilding/SteamGenerator/SteamlineIsolated) auto-isolate latch; staying just below it with a running tube rupture means the leak is real but small. The companion [HotwellRadiation](TurbineBuilding/Condenser/HotwellRadiation) reading watches the condenser side and is diagnostic only. Normal power operation shows a low reading near 0.1, since normal steam activity is only weakly radioactive, so a reading above 0.5 already points at a tube leak.

Related sensors: [SteamlineIsolated](ReactorBuilding/SteamGenerator/SteamlineIsolated), [SGTubeRuptureFlow](ReactorBuilding/SteamGenerator/SGTubeRuptureFlow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:88-92
