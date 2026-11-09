Average reactor coolant temperature has fallen below the Tavg program by the deviation band, so the turbine loading control has held the load reference instead of raising it. The logic is a protection against loading the machine faster than the reactor can follow: Tavg will keep dropping while steam demand pulls heat away, so the digital electro-hydraulic controller stops adding load at the low deviation setpoint, either by manual action or automatically.

Expect this during any cooldown that outruns reactor heat, especially once a steam demand step (breaker closing, dump closing) lands on a falling Tavg. Confirm [power error](PlantOperation/PowerError) converging and [setpoint](TurbineBuilding/Turbine/Setpoint) holding rather than climbing; loading resumes on its own once Tavg recovers to the program. Repeated holds at steady Tavg point at a reactor-side shortfall, not a turbine fault.

Model note: the -2 F deviation threshold is simulator-derived; the reference names the low deviation signal without a magnitude.

Related controls: [Setpoint](TurbineBuilding/Turbine/Setpoint), [SpeedDemand](TurbineBuilding/Turbine/SpeedDemand)
Related sensors: [PowerError](PlantOperation/PowerError), [Velocity](TurbineBuilding/Turbine/Velocity)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:62-66
