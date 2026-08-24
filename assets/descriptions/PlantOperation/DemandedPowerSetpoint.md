The current demanded power setpoint after ramp limiting - the value the rod controller actually steers toward right now. Displayed alongside actual power so the operator sees both the destination and the gap.

It advances incrementally whenever the operator changes the [power setpoint target](PlantOperation/PowerSetpointTarget), moving at the demanded rate. It also anchors the temperature program: Tref is computed as a function of this setpoint, so raising demanded power immediately raises the average coolant temperature the rods must hold, even minutes before the power itself arrives. Watching this value move explains otherwise puzzling rod motion - the rods are tracking the setpoint's ramp, not waiting for it to finish.

Source: specs/pwr_sim/chapter_2/page_024.txt:7-9

Related controls: [PowerSetpointTarget](PlantOperation/PowerSetpointTarget)
Related sensors: [Tref](PlantOperation/Tref), [PowerError](PlantOperation/PowerError)

Source: specs/pwr_sim/chapter_2/page_024.txt:7-9
