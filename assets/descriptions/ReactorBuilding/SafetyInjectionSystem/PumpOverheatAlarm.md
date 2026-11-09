Live flag that the high-head safety injection pump is dead-heading and heating up. It is true exactly while the pump is running, drive-ready, and past switchover (SwitchoverComplete) with miniflow isolated; before switchover, miniflow-only operation is normal and does not alarm. It clears when the pump is secured or the miniflow path reopens.

Unlike [PumpHeatDamage](ReactorBuilding/SafetyInjectionSystem/PumpHeatDamage), which never decreases, this alarm is a condition: a false reading after securing the pump is recovery, not healing. A sustained true reading while [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning) is latched is the prompt to secure the pump before the damage fraction turns into a pump failure. It is the acute end of the same dead-head transient, so each minute true feeds the damage ledger tick by tick.

Related controls: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)
Related sensors: [PumpHeatDamage](ReactorBuilding/SafetyInjectionSystem/PumpHeatDamage)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:61-63
