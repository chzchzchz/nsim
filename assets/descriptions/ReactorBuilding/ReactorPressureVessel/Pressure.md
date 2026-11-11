Reactor coolant system pressure in psig - full-power operating point 2250 psia with design pressure 2500 psia. Water at these temperatures wants to boil, and the only thing keeping it liquid is this pressure; the pressurizer's steam cushion exists to hold the line through transients.

Protection stages above the normal control band run in order: [spray](ReactorBuilding/Pressurizer/SprayOpen) condenses steam, then the [PORV](ReactorBuilding/PORV/Open) relieves below design pressure, then the self-actuating [code safety valves](ReactorBuilding/SafetyValve/Open). Below band, low pressure feeds SI actuation logic. Pressure plus temperature together also gate operations: RHR may only be connected below about 400 psig, and cooldown rate is limited by pressure-temperature (brittle fracture) curves early in life.

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:45

Related controls: [PORV](ReactorBuilding/PORV/Open), [PumpRunning](AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning)
Related sensors: [Pressure](ReactorBuilding/Pressurizer/Pressure), [PORV](ReactorBuilding/PORV/Open), [PumpRunning](AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning)

Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:45
Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:1-7
