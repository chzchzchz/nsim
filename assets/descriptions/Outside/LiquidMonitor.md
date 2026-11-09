On-line liquid effluent monitor reading for the monitored discharge line to the river, in becquerels. Sample water passes through a sample volume watched by a radiation detector; above the trip level the model holds the discharge, below it the holdup drains at the design rate. This is the same hysteresis logic as the BRS auto-divert.

Watch it alongside [LiquidHoldup](Outside/LiquidHoldup): a high holdup with a tripped monitor is a stored-release problem, not yet a river release. The cumulative counter [LiquidReleasedCumulative](Outside/LiquidReleasedCumulative) only advances while the monitor permits discharge, so the two together tell you whether activity is sitting in the tank or already gone. The monitor reads holdup concentration normalized to its trip level, so at 0.5 the tank is half to trip; near 1.0 the discharge is already held and only decay lowers it.

Related sensors: [LiquidReleasedCumulative](Outside/LiquidReleasedCumulative)

Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:77-83
