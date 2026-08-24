Signed difference between current reactor power and the [demanded power setpoint](PlantOperation/DemandedPowerSetpoint). It is the quantity automatic rod control works to drive to zero, and a persistent nonzero value tells the operator the plant is not tracking its demand - usually because rate limiting is mid-ramp, or because something (xenon, boron, a stuck rod) is fighting the controller.

Sign convention matters for reading the screen: positive error means demand exceeds actual power, producing rod withdrawal demand; negative error means the plant is over-power relative to demand, producing insertion. In the simulator the error is computed exactly as the reference describes: current reactor power minus demanded power setpoint.

Source: specs/pwr_sim/chapter_2/page_024.txt:17-18

Related controls: [RodControlAuto](PlantOperation/RodControlAuto)
Related sensors: [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint)

Source: specs/pwr_sim/chapter_2/page_024.txt:17-18
