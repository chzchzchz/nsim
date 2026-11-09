Aligns subsystem A suction to the condensate storage tank as an alternate source. When this latch is set and the primary tank is empty, the subsystem draws from the shared CST instead of running dry. The CST (condensate storage tank) is the backup water supply the alternate-source rule requires.

Set it during the pre-trip lineup or after confirming the primary tank is low, and clear it once the primary source is restored. With the primary tank nonempty this latch changes nothing, which is why it is easy to forget until the swap is needed. Check the CST [Level](TurbineBuilding/CondensateStorageTank/Level) and [MotorPumpRunning](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning) together: an aligned latch with low CST level only delays the dry run.

Related controls: [MotorPumpRunning](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning)
Related sensors: [Level](TurbineBuilding/CondensateStorageTank/Level)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:74-78
Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
