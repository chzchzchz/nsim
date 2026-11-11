Simulator construct with no plant control: a normalized break-size latch filed with the accident faults. Nothing on any control board can open a pipe; 1 inserts the full "RC hot leg #1 LOCA break" from the pwr_sim malfunction catalog, and partial values scale the breach area.

A hot-leg break blows down in the worst direction: coolant leaves on the suction side of the RCS, so with the reactor coolant pumps running it can drain the vessel toward voiding even as injection fights in ("For large pipe ruptures, the RCS would be rapidly depressurized and voided of coolant"). The transient this drives - depressurization, reactor trip, safety injection actuation, sump recirculation - is the design accident the ECCS exists for; watch [BreakFlow](ReactorBuilding/ReactorPressureVessel/BreakFlow) accumulate the lost inventory. The alarms and the SI lineup are the operator's response, not the injector.

Related controls: [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)
Related sensors: [BreakFlow](ReactorBuilding/ReactorPressureVessel/BreakFlow), [Pressure](ReactorBuilding/ReactorPressureVessel/Pressure)

Source: specs/pwr_sim/chapter_4/page_053.txt:55-56
Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
