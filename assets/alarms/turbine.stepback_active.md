A reactor stepback is active: the RPC detected conditions requiring power reduction - a turbine trip is the classic trigger - and rapidly inserted control rods to cut neutron power quickly toward 60 percent, a controlled ramp instead of a full scram. Setback status reads YES while it runs.

The design intent matters for response: the stepback is saving the plant a scram by matching reactor power to sudden steam-demand loss. Do not fight it; let rods settle where the RPC put them, address the triggering condition (usually the turbine side - check [stop valve](TurbineBuilding/Turbine/StopValve) and [breaker](TurbineBuilding/Generator/BreakerClosed) state), and confirm [power error](PlantOperation/PowerError) converging back toward zero.

Operators can also initiate setbacks manually from the same screen when judgment says a controlled reduction beats riding one out to a trip.

Source: specs/pwr_sim/chapter_2/page_023.txt:32-35
Source: specs/pwr_sim/chapter_3/page_041.txt:28-29

Related controls: [RodControlAuto](PlantOperation/RodControlAuto), [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
Related sensors: [PowerError](PlantOperation/PowerError), [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint)

Source: specs/pwr_sim/chapter_2/page_023.txt:32-35
