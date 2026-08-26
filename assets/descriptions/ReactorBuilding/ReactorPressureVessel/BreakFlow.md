Cumulative primary inventory discharged through the hot-leg break, in gallons. The pwr_sim exercise that introduces this transient tells the operator to "Record the break flow in Table V" - watching the leak rate and its integral is how a LOCA is tracked.

The number only grows while the breach is open; every gallon here is gone from the RCS and has become containment water. Cross-check it against what you see downstream: pressurizer level collapsing, subcooling vanishing, containment pressure rising. Injection ([HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)) fights the deficit but cannot cap it - only isolating or refilling the RCS does.

Source: specs/pwr_sim/chapter_4/page_053.txt:64-65

Related controls: [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)
Related sensors: [Water](ReactorBuilding/Pressurizer/Water)
