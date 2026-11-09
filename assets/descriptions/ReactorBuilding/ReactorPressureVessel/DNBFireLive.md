Live flag that departure from nucleate boiling is currently happening at power. It is true exactly while the indicated DNBR margin is below the physical critical-heat-flux point of 1.0 and power is at or above the 10 percent at-power permissive, and it clears the moment the margin recovers. Unlike [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac), it is not a latch, so it does not claim damage that already happened.

Treat a true reading as the clad heating alarm: film boiling has stopped rejecting local heat, so [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp) climbs until power or flow restores margin. Rising [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin) through 1.0 is the recovery evidence.

Model note: the 1.0 threshold and the 10 percent at-power permissive gate are simulator-derived; the reference names departure from nucleate boiling but gives no combined live-flag law.

Related sensors: [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin), [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_126.txt:71-81
