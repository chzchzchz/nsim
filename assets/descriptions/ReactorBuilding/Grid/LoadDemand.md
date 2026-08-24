Dispatcher load demand on the unit as a fraction of rated electrical output. The turbine load reference latches this value each step, and the governor drives the governing valves toward whatever steam flow delivers it.

The reference design computes control signals comparing turbine speed and first-stage pressure against reference values established by the dispatcher or initiated at the operator's panel. A remote dispatch interface guards this boundary in real plants: it feeds the network-dispatching computer the plant's computed power capability, but acts as a permissive network so that unsafe demands cannot be executed - operator dispatching permission and safety limits both gate what reaches the turbine.

Load demand is also the top of the causal chain the whole simulator serves: demand moves the turbine, turbine moves Tavg program, rods follow Tavg.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:10-18

Related controls: [Setpoint](TurbineBuilding/Turbine/Setpoint), [BreakerClosed](TurbineBuilding/Generator/BreakerClosed), [LeadLagMode](PlantOperation/LeadLagMode)

Source: $W/chapter_15/page_201.txt:37-41
Source: $W/chapter_7/page_135.txt:10-18
