Aligns subsystem B suction to the condensate storage tank as an alternate source. When this latch is set and the primary tank is empty, the subsystem draws from the shared CST instead of running dry. The CST (condensate storage tank) is the backup water supply the alternate-source rule requires.

Set it during the pre-trip lineup or after confirming the primary tank is low, and clear it once the primary source is restored. The alignment only matters while the primary tank is empty: a set latch with a full primary tank idles, while a cleared latch with an empty primary tank runs the subsystem dry on suction. Subsystem A has its own independent [CSTAligned](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/CSTAligned), so verify both trains rather than assuming one covers the plant. Check the CST [Level](TurbineBuilding/CondensateStorageTank/Level) and [MotorPumpRunning](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning) together, since the CST alone can carry the alternate source for roughly one hour of operation.

Related controls: [MotorPumpRunning](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning), [CSTAligned](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/CSTAligned)
Related sensors: [Level](TurbineBuilding/CondensateStorageTank/Level)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:74-78
Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
