Cumulative steel-liner fatigue from thermal cycling, as a percent of the 500-cycle startup and shutdown budget. Each full swing of 30 F or more in containment temperature consumes a proportional share; flat operation consumes nothing. The outdoor 40-cycle and single DBA cycle budgets sit outside this counter, which is why the integrated number reads about 8 percent light.

Use it with [ThermalCycleCount](ReactorBuilding/ContainmentBuilding/ThermalCycleCount) to judge remaining life: the fatigue fraction divided by the count is the average cycle severity. Aggressive heatup and cooldown rates burn liner life even when no pressure limit is threatened. It is one-way, so a calm day never restores liner, and reaching 100 percent means the budget is spent rather than the liner failed.

Model note: the budget and band are simulator-derived; the cycle budgets themselves are spec-given.

Related sensors: [ThermalCycleCount](ReactorBuilding/ContainmentBuilding/ThermalCycleCount)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_112.txt:130-135
