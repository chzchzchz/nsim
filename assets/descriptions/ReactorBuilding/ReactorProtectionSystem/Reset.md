Resets the RPS trip latch and the engineered safety features actuation output latches after an actuation. Select Reset from the dropdown and apply.

Reset cannot defeat an active condition - that is the whole point of its design. While trip channels still satisfy 2/4 coincidence voting (two of the four protection channels agreeing a trip condition exists), the step function re-declares the trip immediately, so clearing the latch on a plant that still merits tripping simply re-trips it. Reset is for after: the transient has passed, variables are back inside limits, and the operator wants to restore rod drive power and re-arm the trip logic for the next startup.

Return to service after a genuine trip proceeds from hot standby: rods withdrawn back to criticality per the startup sequence, boron possibly adjusted for the new xenon state the transient left behind.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26

Related controls: [Trip](ReactorBuilding/ReactorPressureVessel/Trip)

Source: $W/chapter_7/page_138.txt:132-133
Source: $W/chapter_16/page_225.txt:24-26
