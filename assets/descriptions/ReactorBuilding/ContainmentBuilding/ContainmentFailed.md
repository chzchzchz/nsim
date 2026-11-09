One-way latch that the containment has failed structurally. It sets when containment pressure exceeds 150 psig, which is 2.5 times the 60 psig design pressure of the Surry-style vessel. Once set it never clears, and any remaining airborne activity drains at the open-penetration leakage rate instead of the engineered isolation rate.

This is the end of the containment function rather than a setpoint to steer around: recovery plans go to managing the release path, not to rebuilding the pressure boundary. Watch the [ContainmentPressure](ReactorBuilding/ContainmentPressure) trend approaching the threshold; the spray system and fan coolers decide whether it ever gets there, and the [VesselFailed](ReactorBuilding/ReactorPressureVessel/VesselFailed) latch is a separate core-side failure that can co-exist.

Model note: the failure pressure is a derived fragility factor applied to the spec design pressure; no sourced failure point exists in the cited texts.

Related sensors: [ContainmentPressure](ReactorBuilding/ContainmentPressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
