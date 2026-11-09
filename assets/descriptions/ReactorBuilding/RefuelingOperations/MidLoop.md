Latches reduced-inventory mid-loop operation: the reactor coolant system is lowered over about 4 minutes at 30 gallons per second to 35 percent of drainable inventory, which is loop mid-height, while the reactor coolant pumps keep running. The drain is a non-creditable ledger debit like cavity drain, and with the pumps still turning the core uncovers partway: dry-run emerges from [Uncovery](ReactorBuilding/ReactorPressureVessel/Uncovery) rather than a gate.

Latch it for steam-generator work that needs the loops drained, and clear it only after refilling, because the reduced inventory leaves no volume margin for a misstep. The [MidLoopLevelFrac](ReactorBuilding/RefuelingOperations/MidLoopLevelFrac) reading shows the inventory arriving at its target; a rising uncovery reading after that is the running pumps losing suction, which is the condition mid-loop surveillance exists to catch.

Model note: 35 percent of inventory and the 30 gal/s drain rate are simulator-derived; the reference names cooldown and refueling operations but gives no mid-loop numbers, so the drain numbers above come from the model, not the specification.

Related sensors: [MidLoopLevelFrac](ReactorBuilding/RefuelingOperations/MidLoopLevelFrac), [Speed](ReactorBuilding/ReactorCoolantPump/Speed)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:100-106
