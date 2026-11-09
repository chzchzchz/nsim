Cumulative water-hammer damage to the feed nozzle, sleeve, and piping from void collapse at high emergency feedwater flow, as a percent of one damage unit. Each hammer event adds 5 percent, and the damage caps after 20 events, so the fraction and the [SleeveGap](ReactorBuilding/SteamGenerator/SleeveGap) both pin at their maxima once fully spent. Only this fraction feeds the part damage readout.

Read it with [HammerAlarm](ReactorBuilding/SteamGenerator/HammerAlarm) for the first event and [FeedRingVoid](ReactorBuilding/SteamGenerator/FeedRingVoid) for the next one: a void above 1 percent refills the precondition, so the next high-flow start can hammer again. Keeping auxiliary feedwater at the preset 165 gallons per minute per train sweeps the void without the collapse. No flow-limiting action reverses the damage, since the increment never decreases.

Related sensors: [HammerAlarm](ReactorBuilding/SteamGenerator/HammerAlarm), [SleeveGap](ReactorBuilding/SteamGenerator/SleeveGap)

Source: specs/westinghouse_tech_manual/chapter_5/page_0537.txt:39-41
Source: specs/westinghouse_tech_manual/chapter_5/page_0537.txt:34-36
