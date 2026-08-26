Flow confirmation for containment ventilation circulation. The control is [Ventilation](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation); this sensor says the atmosphere is actually moving - the same demand-versus-delivery split every fan duty has.

Its purpose per the spec: circulating "the containment atmosphere to remove pockets of high hydrogen concentration after an accident." With it running, hydrogen generation anywhere in containment diffuses into the bulk instead of pooling under the dome; watch [HydrogenPercent](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent) respond as mixed volume reaches the recombiners.

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:15-17

Related controls: [Ventilation](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation)
Related sensors: [HydrogenPercent](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent), [RecombinerRunning](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/RecombinerRunning)
