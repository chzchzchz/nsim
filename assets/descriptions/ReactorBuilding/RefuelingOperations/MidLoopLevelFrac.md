Remaining vessel inventory as a fraction of drainable inventory, in normalized units. The mid-loop target is 0.35, loop mid-height; one means the vessel is full. It is computed from the non-creditable break ledger, so relief, tube-rupture, cavity, and mid-loop drains all draw it down, while credited injection does not refill it.

Watch it during [MidLoop](ReactorBuilding/RefuelingOperations/MidLoop) evolutions for the arrival at target, and during any LOCA for the [Uncovery](ReactorBuilding/ReactorPressureVessel/Uncovery) consequence. A reading below 0.35 with pumps running is the reduced-inventory hazard the mid-loop procedure controls. Because credited injection never refills this ledger, recovery after a large break leaves the reading depressed even when the core is re-covered, so read it together with the uncovery fraction rather than as a standalone water-level gauge.

Model note: the 0.35 target is simulator-derived; the reference gives no mid-loop level number and its cavity-depth figure is not used here.

Related sensors: [Uncovery](ReactorBuilding/ReactorPressureVessel/Uncovery)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:100-106
