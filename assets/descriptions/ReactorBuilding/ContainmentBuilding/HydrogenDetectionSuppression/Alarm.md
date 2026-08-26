High-hydrogen alarm for the containment atmosphere. It latches when [HydrogenPercent](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent) crosses its setpoint, 2.0 volume percent in the model, so hydrogen management starts long before any flammability margin is threatened.

The spec requires "alarms at proper setpoints are provided" alongside continuous detection; the number itself is simulator-derived - the reference names no value. When it sounds, drive the response side: circulation ([Ventilation](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation)) to break up pockets, recombiners ([RecombinerRunning](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/RecombinerRunning)) to burn hydrogen back to water. Acknowledge through [AlarmReset](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/AlarmReset) once concentration recedes.

Model note: the 2.0 percent setpoint is a simulator constant.

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:10-11

Related controls: [AlarmReset](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/AlarmReset), [Ventilation](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation)
Related sensors: [HydrogenPercent](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent)
