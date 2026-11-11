Simulator construct with no plant control: a failure input filed with the faults that the turbine thrust bearing is worn. Protective device 4 in the turbine trip scheme is the thrust bearing trip, but the reference prints no setpoint for it, so the cause arrives as this externally written latch and the turbine step consumes it into the shared emergency-trip path. While latched and running, the machine trips on it exactly like the vacuum and bearing-oil trips: governing valve and stop valve close, extraction nonreturn valves shut, and the unit goes to tripped.

This is hardware damage, not a trip latch: resetting the turbine does not clear it, and a restarted machine re-trips while it stands. The fix is maintenance at turbine outage, which in this simulation means clearing the latch and restarting. Watch [State](TurbineBuilding/Turbine/State) for the trip and [Velocity](TurbineBuilding/Turbine/Velocity) for the coastdown.

Related sensors: [State](TurbineBuilding/Turbine/State), [Velocity](TurbineBuilding/Turbine/Velocity)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:34
