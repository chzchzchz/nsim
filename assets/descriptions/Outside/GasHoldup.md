Activity accumulated in the waste gas surge and holdup volume waiting for the charcoal delay, in normalized units where 1.0 is rated-power equilibrium. Noble gas held here decays with the Xe-133 half-life and vents to the stack only after the delay, so the holdup is the buffer between primary and atmosphere.

Watch it fall as it vents through the stack: the delayed fraction reaching the [PlantVentActivity](Outside/PlantVentActivity) monitor is this inventory times the vent fraction each tick. The hydrogen carrier gas is not delayed, so a rising holdup with a steady vent reading means carrier flow is holding the charcoal near its limit.

Model note: the first-order lag replaces a bookkeeping queue; the bridge shows normalized activity, not becquerels.

Related sensors: [PlantVentActivity](Outside/PlantVentActivity)

Source: specs/the_westinghouse_pressurized_water_react/chapter_14/page_190.txt:68-71
Source: specs/the_westinghouse_pressurized_water_react/chapter_14/page_190.txt:102-109
Source: specs/the_westinghouse_pressurized_water_react/chapter_14/page_190.txt:123-125
