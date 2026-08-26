Closes the main generator output breaker onto the switchyard, synchronizing the unit to the grid. Three things must match first: generator voltage to [grid voltage](Electrical/Grid/Voltage), frequency to [grid frequency](Electrical/Grid/Frequency), and phase - the synchroscope integrates all three into one go/no-go needle. Closing across a mismatch slams enormous torque through the rotor.

After synchronization, minimum load of 5-10 percent is established partly by dumping steam to the condenser so reactor power has somewhere to go, then load follows dispatcher demand upward through the governor. From this moment the unit is a power station rather than a very expensive heater.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-30
Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:17-19

Related controls: [SpeedDemand](TurbineBuilding/Turbine/SpeedDemand), [LoadDemand](Electrical/Grid/LoadDemand)
Related sensors: [Voltage](Electrical/Grid/Voltage), [Frequency](Electrical/Grid/Frequency)

Source: $W/chapter_15/page_216.txt:24-30
Source: $W/chapter_11/page_164.txt:17-19
