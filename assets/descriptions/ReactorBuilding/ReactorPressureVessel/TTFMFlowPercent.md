Total reactor coolant flow as a percent of rated, combining pumped flow and natural-circulation flow. The reading is the simulator's own flow fraction: pumped flow scaled by reactor coolant pump speed plus the natural circulation fraction, capped at 100 percent. This is the flow the transit-time meter reports by timing N-16 decay from core to hot leg.

At full speed with zero natural circulation it reads 100 percent. After a pump trip it falls toward the natural circulation fraction, which is the cooling that keeps the core covered on thermal head alone. Pair it with [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin) (departure-from-nucleate-boiling ratio margin): low flow consumes margin before temperatures move. Two pumps at half speed can read above 100 percent when combined with natural circulation, since the cap applies to the total rather than to each source.

Related sensors: [Speed](ReactorBuilding/ReactorCoolantPump/Speed), [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_126.txt:71-81
