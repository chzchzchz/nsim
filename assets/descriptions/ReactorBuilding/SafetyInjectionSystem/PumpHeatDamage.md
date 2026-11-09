Cumulative heat damage to a safety injection pump running into a dead head, as a percent of one damage unit. In this model it tracks the high-head injection pump: after switchover the miniflow isolation valves close, so a still-running pump with no flow path accumulates heat at the dead-head rate. Normal miniflow-only operation before switchover is not dead-heading, because miniflow is still flowing.

Verify [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning) after switchover completes: a pump that stays latched on is feeding this fraction every second. Recovery means securing the pump, not cooling it; the fraction never decreases. A live [PumpOverheatAlarm](ReactorBuilding/SafetyInjectionSystem/PumpOverheatAlarm) is the condition while this damage is the ledger, so securing the pump clears the alarm but leaves the wear.

Model note: the dead-head heat rate is simulator-derived; the reference gives no pump damage rate.

Related controls: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:61-63
