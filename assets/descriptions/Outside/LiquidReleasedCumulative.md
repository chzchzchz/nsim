One-way cumulative activity discharged to the river through the monitored liquid line, in becquerels. Like its gas counterpart it never decreases, so it is the release ledger for the liquid pathway. Subtracting two readings gives the released activity over any interval, which is how the cumulative total becomes an interval release record.

It only advances while the [LiquidMonitor](Outside/LiquidMonitor) permits discharge, so a flat counter with a full [LiquidHoldup](Outside/LiquidHoldup) means the monitor is holding the tank, not that the activity vanished. Compare against the gas cumulative to see which pathway actually carried the release; both ledgers blank until the holdups start draining, then the faster one names the dominant path. During a long outage the liquid ledger moves only while the holdup tanks drain, so a day of still water shows a flat line even when the gas pathway is active.

Related sensors: [LiquidMonitor](Outside/LiquidMonitor)

Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:77-83
