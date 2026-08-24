Signed difference between current reactor power and the [demanded power setpoint](PlantOperation/DemandedPowerSetpoint). It is the quantity automatic rod control works to drive to zero, and a persistent nonzero error tells the operator the plant is not tracking its demand.

Large positive error (demand above actual) produces withdrawal demand; negative error produces insertion. In the simulator the error is computed exactly as the reference describes: current reactor power minus demanded power setpoint.

See also: [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint), [RodControlAuto](PlantOperation/RodControlAuto)

Source: specs/pwr_sim/chapter_2/page_024.txt:17-18
