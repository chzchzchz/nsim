Discharge rate through the hot-leg break, gpm. The pwr_sim exercise that introduces this transient tells the operator to "Record the break flow in Table V" - tracking the leak rate is how a LOCA is followed. A full-open break reads 7200 gpm; partial break sizes scale linearly.

Read the trend, not just the number: the rate starts high and decays as RCS pressure falls toward containment pressure, so a rate that stops decaying says something upstream has changed. Every gallon gone here has become containment water - cross-check pressurizer level collapsing, subcooling vanishing, containment pressure rising. Injection ([HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)) fights the deficit but cannot cap it - only isolating or refilling the RCS does.

Model note: the backing field stores per-tick gallons; the bridge divides out the 1 ms tick and scales to minutes for the gpm presentation.

Source: specs/pwr_sim/chapter_4/page_053.txt:64-65

Related controls: [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)
Related sensors: [Water](ReactorBuilding/Pressurizer/Water)
