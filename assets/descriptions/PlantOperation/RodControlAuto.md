Engages automatic gray (control) rod control. While latched, the controller integrates the error between measured Tavg - the average coolant temperature across reactor coolant loops - and its programmed reference into an insertion demand each step, replacing the manual demand input.

The distinction between manual and automatic is rate behavior. In manual mode rods move at a preset fixed speed the operator chose, so correcting a large temperature error takes many strokes; in automatic the rod speed varies with error size, moving briskly after a load step and creeping as the error closes. Group sequencing follows a preset reversible program - withdrawal order mirrors insertion order - so bank overlaps stay consistent regardless of who or what is driving.

Automatic control always operates under the protection system's shadow: eight trip breakers sit in series/parallel with the rod drive power supply and the RPS opens them on any trip signal, dropping the rods regardless of controller state.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:1-9
Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:26-31

Related controls: [AutoControl](ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl), [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
Related sensors: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:1-9
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-28
