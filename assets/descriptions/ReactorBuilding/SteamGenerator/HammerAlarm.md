One-way latch that a feed-ring water hammer has occurred. It sets on the first event where emergency feedwater flow exceeds 400 gallons per minute in a generator with a void already present (void fraction at least 1 percent). The latch never clears inside an evolution, so a true reading is permanent proof the hammer happened.

A latched alarm means the [SleeveGap](ReactorBuilding/SteamGenerator/SleeveGap) has already grown and [HammerDamage](ReactorBuilding/SteamGenerator/HammerDamage) has already advanced by 5 percent on that event. Prevention is the preset auxiliary feedwater control valves at about 165 gallons per minute per train, which sweep the void gradually instead of collapsing it.

Model note: the alarm setpoint is simulator-derived; the reference gives the 400 gpm hammer mechanism but no alarm.

Related sensors: [HammerDamage](ReactorBuilding/SteamGenerator/HammerDamage), [FeedRingVoid](ReactorBuilding/SteamGenerator/FeedRingVoid)

Source: specs/westinghouse_tech_manual/chapter_5/page_0537.txt:39-41
