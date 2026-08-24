The current demanded power setpoint after ramp limiting - the value the rod controller actually steers toward. Displayed alongside actual power so the operator can see both the target and the gap.

It advances incrementally from current reactor power at the demanded rate whenever the operator changes the target, and it feeds directly into the Tref program: reference average coolant temperature is a programmed function of this setpoint, so raising power raises the temperature the rods must hold.

See also: [PowerSetpointTarget](PlantOperation/PowerSetpointTarget), [Tref](PlantOperation/Tref), [PowerError](PlantOperation/PowerError)

Source: specs/pwr_sim/chapter_2/page_024.txt:7-9
