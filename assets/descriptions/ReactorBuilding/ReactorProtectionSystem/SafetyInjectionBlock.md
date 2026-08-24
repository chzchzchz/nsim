Operator block on the safety injection initiation signals, armed before a planned cooldown depressurizes toward the automatic actuation setpoints. Without the block, deliberately pressing RCS pressure down through the SI setpoint during shutdown would spuriously start injection pumps and open injection valves on a healthy plant.

The reference shutdown sequence blocks SI initiation prior to reaching the automatic setpoints during cooldown and depressurization, paired with isolating the [accumulators](ReactorBuilding/SafetyInjectionSystem/AccumulatorIsolation/Shut). Both actions trade automatic protection for deliberate control while the plant is in a known, controlled state - and both must be remembered to restore before returning the plant to power.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110

Related controls: [AccumulatorIsolation](ReactorBuilding/SafetyInjectionSystem/AccumulatorIsolation/Shut), [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)
Related sensors: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)

Source: $W/chapter_16/page_227.txt:108-110
