Engages automatic rod control. While latched, the controller owns the insertion demand: it integrates the error between measured Tavg and the programmed reference, moving rods briskly for large deviations and creeping as the error closes. The manual demand path is ignored while automatic is engaged.

This is one of the last steps in a startup sequence - the reference checklist places rod control in automatic alongside steam generator level control and the Tavg mode of steam dump once the plant is stable at low power, because from there the coordinated system handles load changes better than a hand on the rods.

Automatic control always operates under protection-system shadow: eight trip breakers sit in series/parallel with rod drive power and the RPS opens them on any trip, dropping rods regardless of what the controller wants.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
Source: specs/westinghouse_plant_operations/chapter_19/page_003.txt:15-18

Related controls: [RodControlAuto](PlantOperation/RodControlAuto), [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
Related sensors: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion), [ProgrammedTavg](ReactorBuilding/ReactorPressureVessel/ControlRods/ProgrammedTavg)

Source: $W/chapter_16/page_226.txt:24-27
Source: $W/chapter_7/page_133.txt:26-31
