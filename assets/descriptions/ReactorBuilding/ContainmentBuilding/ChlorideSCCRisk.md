Derived risk flag that chloride stress-corrosion cracking threatens the stainless containment and piping under LOCA conditions. It reads true while containment pressure is above the spray setpoint and the spray tank has no valid inventory behind it, meaning chloride-laden spray water can sit on hot stainless steel.

The fix is the spray tank: open the SAT isolation valve with water in the tank and the risk clears. Track [Level](AuxiliaryBuilding/ContainmentSpraySystem/SAT/Level) and the containment pressure together; the risk is highest in the window before spray starts wetting surfaces. It is a risk flag, not a crack alarm, so treat a sustained true reading as a prompt to align the tank rather than an inspection finding.

Model note: simulator-derived predicate; the reference gives no chloride SCC source, and the bridge mirrors the spray-system flag as a containment sensor for visibility.

Related controls: [SATIsolationValveB](AuxiliaryBuilding/ContainmentSpraySystem/SATIsolationValveBOpen)
Related sensors: [ContainmentPressure](ReactorBuilding/ContainmentPressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
