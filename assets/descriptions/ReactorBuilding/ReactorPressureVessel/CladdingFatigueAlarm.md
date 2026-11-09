One-way latch that cumulative cladding fatigue has reached 50 percent of its damage scale. It sets when [CladdingFatigue](ReactorBuilding/ReactorPressureVessel/CladdingFatigue) crosses 0.5 and never clears, so a true reading is permanent for the evolution. Treat it as an inspection prompt, not a trip.

A latched alarm means the core has spent half its cyclic life: plan the next outage around fuel inspection and avoid further large-amplitude power cycling until the fatigue is accounted for. New alarms in this family arrive late by design, since the threshold is half the total wear. It is the operational companion to severe-accident damage, which is tracked separately in the core damage fraction.

Model note: the 50 percent threshold is simulator-derived; the cited text is about rod motion and states no fatigue threshold.

Related sensors: [CladdingFatigue](ReactorBuilding/ReactorPressureVessel/CladdingFatigue)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:13-16
