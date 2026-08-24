Requested ramp rate toward the [power setpoint target](PlantOperation/PowerSetpointTarget), expressed as a fraction of rated power per second. The integrator clamps it to the maximum of 0.8 percent of full power per second.

Rate limiting is what turns a setpoint change into a controlled maneuver instead of a thermal transient. The demanded setpoint advances incrementally as current power plus rate times the program cycle time, so a jump from 50 to 90 percent takes several minutes even if the operator types the new number at once. During startup the same philosophy appears as a bounded SUR (startup rate in decades per minute): a positive startup rate heats the core until the power defect - the loss of reactivity that comes with hotter, less dense fuel and coolant - pulls the rate back toward zero and power levels off.

Source: specs/pwr_sim/chapter_2/page_024.txt:3-8

Related controls: [PowerSetpointTarget](PlantOperation/PowerSetpointTarget)
Related sensors: [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint)

Source: specs/pwr_sim/chapter_2/page_024.txt:3-8
