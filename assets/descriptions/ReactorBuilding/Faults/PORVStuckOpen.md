Simulator construct with no plant control: a failure latch filed with the accident faults that jams the pressurizer PORV open. While latched, the relief valve ignores both its automatic reclose below the 2500 psia trip pressure and operator close writes to the PORV itself. The only remaining way to stop the inventory loss is the upstream block valve.

The transient it drives is the classic stuck-open-relief sequence from the training catalog: primary coolant flashes through the valve and vents to the pressurizer relief tank, depressurizing the RCS and draining inventory with nowhere to go but the tank and, if the rupture disc lifts, the containment. Isolate via the block valve ([Open](ReactorBuilding/BlockValve/Open)) - that isolation capability is exactly why the stop valve exists.

Model note: routes discharge into the PRT per the primary spec; the pwr_sim exercise's letdown-condenser routing is not modeled.

Source: specs/pwr_sim/chapter_4/page_049.txt:26-31
Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-34

Related controls: [BlockValve](ReactorBuilding/BlockValve/Open), [PORV](ReactorBuilding/PORV/Open)
Related sensors: [Pressure](ReactorBuilding/Pressurizer/Pressure)
