Routes pressurizer relief and safety discharge to the letdown condenser instead of the pressurizer relief tank. The design sends steam to the relief tank; this latch models the failed-open relief valve that diverts that steam into the letdown path, the pwr_sim CV22 malfunction.

Set it to reproduce the diverted-relief transient, clear it to restore the design routing. While latched the PRT stays cold and the [LetdownCondenserLevel](ReactorBuilding/LetdownCondenserLevel) climbs, which is also the fastest way to confirm the routing changed. The discharge is the same mass either way; only the sink changes. The PRT (pressurizer relief tank) normally condenses and collects this discharge, so the diverted path overloads the small letdown condenser instead.

Related controls: [PORVStuckOpen](ReactorBuilding/Pressurizer/PORVStuckOpen)
Related sensors: [LetdownCondenserLevel](ReactorBuilding/LetdownCondenserLevel)

Source: specs/pwr_sim/chapter_4/page_049.txt:30-31
Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:31-34
