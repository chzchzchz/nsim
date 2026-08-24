Opens the pressurizer spray valve, admitting relatively cool reactor coolant from a cold leg into the steam space at the top of the pressurizer. The cold water condenses steam directly, dropping pressure within seconds - spray is the fast, controllable downward actuator, the mirror image of the [heaters](ReactorBuilding/Pressurizer/HeatersOn).

The pressure control system uses spray whenever insurge transients push pressure above band: load rejections drive primary water up into the pressurizer, compressing the bubble, and spray is what stops the compression from becoming an overpressure event. If spray alone cannot hold the line during a big transient, the next stage arms itself - the [PORV](ReactorBuilding/PORV/Open) opens automatically below design pressure.

Spray effectiveness depends on there being a bubble to condense into; in solid-water operation (no bubble) spray does nothing, which is why cold overpressure protection uses different valves entirely.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:23-30
Source: specs/westinghouse_plant_operations/chapter_19/page_005.txt:28-31

Related controls: [HeatersOn](ReactorBuilding/Pressurizer/HeatersOn), [PORV](ReactorBuilding/PORV/Open)
Related sensors: [PORV](ReactorBuilding/PORV/Open), [Pressure](ReactorBuilding/Pressurizer/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:23-30
