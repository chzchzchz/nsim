Selected pressure the solid-plant control valve maintains while the primary is water-solid during RHR cooldown - the target the valve trims toward by bleeding RHR pump discharge into the CVCS letdown path.

With the pressurizer water-solid there is no steam bubble to absorb expansion, so any heatup presses directly on the cold overpressure limits. The setpoint gives that situation an ordinary control loop instead of a relief event: charging makes up contraction, the valve relieves expansion. Zero disables the path entirely until you set it, so after selecting it confirm [SolidPlantReliefFlow](AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantReliefFlow) sits near zero and [SolidPlantPressureValve](AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureValve/Open) is modulating.

Model note: the simulator stores this as psia and applies no default; the reference passage names the scheme without quoting a number.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:117-122

Related controls: [Open](AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureValve/Open)
Related sensors: [SolidPlantReliefFlow](AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantReliefFlow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:117-122
