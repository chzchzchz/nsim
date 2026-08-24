Requested ramp rate toward the [power setpoint target](PlantOperation/PowerSetpointTarget), expressed as a fraction of rated power per second. The integrator clamps it to the maximum of 0.8 percent of full power per second.

Rate limiting is what turns a setpoint change into a controlled maneuver: the DEMANDED POWER SETPOINT advances incrementally as current power plus rate times program cycle time. During startup the same philosophy appears as a bounded SUR - a positive startup rate that the power defect then pulls back toward zero as power levels off in the power range.

Related controls: [PowerSetpointTarget](PlantOperation/PowerSetpointTarget)
Related sensors: [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint)

Source: specs/pwr_sim/chapter_2/page_024.txt:3-8
