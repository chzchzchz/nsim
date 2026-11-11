Readback of the pressurizer ASME-code safety valves' aggregate position as the model reports it; exposed mostly for visibility. The aggregate is the logical OR of the pressure law (open above the model's derived 2,600 psia setpoint, closed below it) and the stuck-open latch — a bridge write to the fault injector ([PressurizerSafetyValve](ReactorBuilding/Faults/PressurizerSafetyValve)) holds it open like the PORV-stuck path until cleared.

Related controls: [PressurizerSafetyValve](ReactorBuilding/Faults/PressurizerSafetyValve), [PORV](ReactorBuilding/PORV/Open)
Related sensors: [Level](ReactorBuilding/PressurizedReliefTank/Level)

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-34
