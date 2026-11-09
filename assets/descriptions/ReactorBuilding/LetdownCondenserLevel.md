Water level in the letdown condenser, in gallons against a 2000 gallon capacity. The condenser is the letdown heat exchanger where hot primary bleed water gives up its heat before passing to purification; level here is the collected liquid inventory, not a flow.

A filling condenser with rising [Pressure](ReactorBuilding/Pressurizer/Pressure) points at relief routing or stuck-letdown inflow; the pwr_sim failed-valve case shows exactly this signature, where relief steam diverts into the letdown path instead of the relief tank. Check [ReliefRoutingToLetdownCondenser](ReactorBuilding/ReliefRoutingToLetdownCondenser) before blaming the condenser itself. A condenser near capacity with no relief routing set means letdown inflow is outrunning the purification draw, so bleed off rather than waiting for the level to self-clear.

Model note: the 2000 gallon capacity is simulator-derived; the reference gives no condenser size.

Related controls: [ReliefRoutingToLetdownCondenser](ReactorBuilding/ReliefRoutingToLetdownCondenser)

Source: specs/pwr_sim/chapter_2/page_027.txt:44-55
Source: specs/pwr_sim/chapter_4/page_048.txt:40-50
