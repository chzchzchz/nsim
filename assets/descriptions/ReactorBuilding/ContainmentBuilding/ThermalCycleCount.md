Number of full thermal cycles the containment liner has seen, counting only temperature swings of at least 30 F that land on a trend reversal. A swing is counted once when the temperature reaches a peak or trough and the trend turns; jitter inside the band never counts. Startup and shutdown cycles dominate because they are the only operations that swing the liner through its full range.

Read it with [LinerFatigue](ReactorBuilding/ContainmentBuilding/LinerFatigue), which normalizes this count against the 500-cycle budget. Rapid heatup-cooldown cycles during recovery operations add cycles fast, so the count is the operational wear ledger behind the fatigue percentage.

Model note: the cycle detector and band are simulator-derived; the underlying cycle budgets are spec-given.

Related sensors: [LinerFatigue](ReactorBuilding/ContainmentBuilding/LinerFatigue)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_112.txt:130-135
