One-step pulse raised on the transition of [Ejected](ReactorBuilding/Faults/Ejected) from false to true. The reactor protection system reads this pulse to fire the high flux-rate trip without waiting for the flux to climb on its own. It clears on the following step, so it is a momentary signal, not a state.

Its purpose is speed: the ejection reactivity arrives in the step the cluster leaves, and the one-step event lets protection act on the cause rather than on the consequence. A mechanical rod drive cannot exceed 0.05 of full power per second, so a rise faster than that is declared an ejection outright. Watch [StartupRate](ReactorBuilding/ReactorPressureVessel/StartupRate) and [EjectedReactivityPct](ReactorBuilding/ReactorPressureVessel/ControlRods/EjectedReactivityPct) for the transient it announces. A missed pulse in a long tick still trips, since the protection reads the latch transition rather than waiting for a sampled pulse.

Related sensors: [EjectedReactivityPct](ReactorBuilding/ReactorPressureVessel/ControlRods/EjectedReactivityPct), [StartupRate](ReactorBuilding/ReactorPressureVessel/StartupRate)

Source: specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:28-33
