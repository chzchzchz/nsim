Activity in the liquid waste holdup tanks waiting for monitored discharge, in normalized units where 1.0 is rated-power equilibrium. It decays with the tritium half-life and drains to the river only while the [LiquidMonitor](Outside/LiquidMonitor) is below its trip level.

A rising holdup with a held monitor is normal batch processing; a rising holdup with the monitor permitting flow means inflow is outrunning the drain rate. Tritium decay barely dents it (12.3 year half-life), so batch transfer, not decay, is the only way the inventory falls. The cumulative [LiquidReleasedCumulative](Outside/LiquidReleasedCumulative) counter shows how much has left so far. A holdup near zero with no release means the tank was already discharged and the monitors are idle, not a failure.

Model note: the bridge shows normalized activity; conversion to becquerels happens inside the plant-step pathway routing.

Related sensors: [LiquidMonitor](Outside/LiquidMonitor)

Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:77-83
