Generator terminal voltage, held near nominal by the excitation system. The reference machines use static-type brushless excitation with field flashing for rapid voltage buildup after a start - important when you want to synchronize quickly following a blackout.

Terminal voltage is reactive power control in practice: raising field current exports VArs to support grid voltage, lowering imports them. Push too far either way and the machine walks into its capability limits, where the [MVA limit alarm](TurbineBuilding/Generator/MVALimitAlarm) starts talking. Voltage matching against the grid is also one of the three synchronization permissives checked before breaker closure.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:46-47
Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:1-10

Related sensors: [Voltage](ReactorBuilding/Grid/Voltage), [MVALimitAlarm](TurbineBuilding/Generator/MVALimitAlarm)

Source: $W/chapter_11/page_166.txt:46-47
Source: $W/chapter_15/page_208.txt:1-10
