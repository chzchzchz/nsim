Cumulative cyclic-fatigue damage to the fuel cladding from power cycling, as a percent of one full damage unit. A fatigue cycle is a power excursion that reverses direction with a peak-to-trough amplitude of at least 5 percent of rated power, a band borrowed from the rod control reference as a stand-in. Each reversal consumes damage in proportion to its amplitude; steady operation consumes nothing, because the failure mode is cycling, not hours at power.

Watch it during load-following evolutions and xenon transients: repeated 5 percent swings in either direction add up over days. It is deliberately separate from severe-accident damage, which lives in [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac), so operational wear never masquerades as a core damage readout. The [CladdingFatigueAlarm](ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm) latches at 50 percent of this scale.

Model note: the cited text is about rod motion on random load variations and not about cladding fatigue at all; the 5 percent dead-band repurposes that figure as a threshold that the simulator chose.

Related sensors: [CladdingFatigueAlarm](ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:13-16
