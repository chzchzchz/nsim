One-way latch that a hydrogen deflagration has already burned the containment mixture. It fires once when the volume-weighted hydrogen concentration reaches the 4 percent explosive limit, releasing a single calibrated heat pulse into the containment that the structure, fan coolers, and spray then shape. Sustained concentrations above 4 percent are the recombiner-burning regime, not repeated deflagrations.

A true reading means the burn already happened: check the pressure spike it left in [ContainmentPressure](ReactorBuilding/ContainmentPressure) and whether the [ContainmentFailed](ReactorBuilding/ContainmentBuilding/ContainmentFailed) latch followed. Prevention lives in the [HydrogenDetectionSuppression](ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent) controls, which remove hydrogen before the mixture can reach the limit.

Model note: the 4 percent explosive limit and the one-shot calibrated heat pulse are simulator-derived; the reference names hydrogen release and explosive-mixture control but gives no limit, and the model does not simulate repeated burns.

Related sensors: [ContainmentPressure](ReactorBuilding/ContainmentPressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-34
Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:54-56
