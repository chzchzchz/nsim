Drains the refueling cavity so the vessel head can be reinstalled at the end of an outage. The sequence in the reference: "When the head is about 1 foot above the vessel flange, the refueling cavity is completely drained and the flange surface is cleaned."

The two cavity commands are mutually exclusive by construction - setting [CavityFlooding](ReactorBuilding/RefuelingOperations/CavityFlooding) clears this latch - because a cavity cannot fill and empty at once. Draining is the last step of disassembly done backwards: below the flange the reactor is no longer shielded for open handling, which is why the drain only happens once the head is nearly seated.

Source: specs/the_westinghouse_pressurized_water_react/chapter_13/page_186.txt:113-116

Related controls: [CavityFlooding](ReactorBuilding/RefuelingOperations/CavityFlooding)
