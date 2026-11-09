Instantaneous activity in the plant vent stack discharge, in becquerels from the on-line gas monitor at the vent. It is the operator-alert input for routine gaseous releases: the monitor watches the pipe carrying the waste gas to the atmosphere and alarms when the release runs high.

Read it with the cumulative [GasReleasedCumulative](Outside/GasReleasedCumulative) counter to separate a brief spike from a sustained release. Normal operation shows a low steady reading that decays as the charcoal delay holds back noble gas; a rising trend with falling [GasHoldup](Outside/GasHoldup) means the holdup is draining toward the stack. Containment failure bypasses this path entirely. It never shows the liquid pathway, so a zero reading does not mean the river is clean.

Related sensors: [GasReleasedCumulative](Outside/GasReleasedCumulative)

Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:66-68
Source: specs/the_westinghouse_pressurized_water_react/chapter_14/page_190.txt:123-125
