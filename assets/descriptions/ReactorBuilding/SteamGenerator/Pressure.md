Steam generator secondary steam pressure in psig, nominally about 1000 psig at full power, falling toward roughly 800 psig at no-load as Tavg rises along the program - the inversion happens because hotter primary water bakes harder steam out of a given inventory.

Pressure is the handshake between primary and secondary sides: primary heat arriving versus steam being taken away sets it, instantly and continuously. During hot standby the plant deliberately holds steam pressure at saturation corresponding to no-load Tavg - that specific number is part of what "hot standby" means. Sudden pressure drops mean steam demand jumped (turbine loading, dump opening); sudden spikes mean demand vanished (trip, isolation) and protection stages are about to earn their keep.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:6-11

Related controls: [AtmosphericDumpValve](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open)
Related sensors: [HeatInput](ReactorBuilding/SteamGenerator/HeatInput), [AtmosphericDumpValve](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open)

Source: $W/chapter_16/page_225.txt:6-11
