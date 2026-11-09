Xenon-135 reactivity worth in percent delta-k/k, computed from the normalized xenon inventory against its rated-power equilibrium. Negative means xenon is poisoning the core beyond equilibrium, positive means the inventory is below equilibrium and adding reactivity. The worth is flux-weighted across the core halves, so the [AxialOffset](ReactorBuilding/ReactorPressureVessel/AxialOffset) shifts where the poison acts.

This is how the operator infers poisoning without a detector, which the reference confirms does not exist: after a power cut the rods must withdraw further for the same power, and the extra travel is this worth. It is also the reason a post-trip restart waits hours, because the inventory peaks after shutdown and starting into the peak demands more rod withdrawal than the [InsertionLimit](ReactorBuilding/ReactorPressureVessel/ControlRods/InsertionLimit) allows. Watch [ReactivityEstimate](ReactorBuilding/ReactorPressureVessel/ReactivityEstimate) for the net effect.

Related sensors: [ReactivityEstimate](ReactorBuilding/ReactorPressureVessel/ReactivityEstimate), [AxialOffset](ReactorBuilding/ReactorPressureVessel/AxialOffset)

Source: specs/the_westinghouse_pressurized_water_react/glossary.md:176-176
