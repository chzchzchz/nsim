Programmed reference average coolant temperature (Tref) for the present power demand. Automatic rod control drives measured Tavg to this value, and the steam generator level programs and steam dump references hang off the same signal, which is why it is the single most important number on the RPC screen.

The program rises with load - roughly 560F at no-load toward 600F+ at full power in typical programs - so that secondary steam conditions stay favorable for the turbine across the whole power range. Because Tref moves with the demanded setpoint rather than actual power, it leads the plant: after a load increase the rods begin inserting withdrawal demand immediately, warming the primary ahead of the steam flow that will need it.

After large load decreases the steam dump system augments rod control specifically to walk actual Tavg back down to this program smoothly instead of letting it overshoot and oscillate.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-33

Related sensors: [ProgrammedTavg](ReactorBuilding/ReactorPressureVessel/ControlRods/ProgrammedTavg), [PowerError](PlantOperation/PowerError)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-33
