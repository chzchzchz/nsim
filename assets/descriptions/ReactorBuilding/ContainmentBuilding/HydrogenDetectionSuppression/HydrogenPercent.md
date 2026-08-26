Continuous readout of hydrogen concentration in the containment atmosphere, volume percent. The spec requires "a hydrogen detection system that can detect and report in a continuous manner the hydrogen content in the containment atmosphere" - hydrogen accumulates after an accident from zirconium-water reactions on uncovered fuel, and it burns.

Watch it against the [Alarm](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Alarm) setpoint of 2.0 volume percent. Rising concentration means generation outruns removal; drive [Ventilation](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation) to mix out pockets and rely on the recombiners, which consume hydrogen back into water.

Model note: the model lumps the atmosphere into a single well-stirred percent, so no stratification or pocketing is simulated despite ventilation's pocket-removal purpose.

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:7-9

Related controls: [Ventilation](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation)
Related sensors: [Alarm](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Alarm), [RecombinerRunning](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/RecombinerRunning)
