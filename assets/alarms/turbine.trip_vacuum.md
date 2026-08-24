Turbine tripped on low condenser vacuum - backpressure above the 8.0 psia trip plate while running. Backpressure is not just an efficiency number - last-stage blading tolerates only limited exhaust pressure before overheating and eroding, so the trip protects the machine mechanically from conditions that would destroy it economically anyway.

Vacuum loss has a short cause list worth walking in order: circulating water problems ([flow](Outside/CirculatingWaterSystem/Flow), [pumps](Outside/CirculatingWaterSystem/PumpRunning)), air in-leakage overwhelming the [vacuum pumps](TurbineBuilding/Condenser/VacuumPump), or condenser tube degradation dumping cooling water into the hotwell. Each leaves different fingerprints in downstream temperatures and hotwell chemistry.

Restore vacuum before attempting a [reset](TurbineBuilding/Turbine/Reset) and roll - restarting against poor vacuum simply retrips while thermally cycling the rotor for nothing.

Model note: the 8.0 psia plate is simulator-derived, placed between the spec's emergency LP exhaust limit (about 3.9 psia, quoted as eight inches HgA) and the lumped condenser's full-load ceiling, so load never trips spuriously but loss of heat sink does.

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25-26
Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_199.txt:94

Related controls: [VacuumPump](TurbineBuilding/Condenser/VacuumPump), [PumpRunning](Outside/CirculatingWaterSystem/PumpRunning), [Reset](TurbineBuilding/Turbine/Reset)
Related sensors: [Pressure](TurbineBuilding/Condenser/Pressure), [Flow](Outside/CirculatingWaterSystem/Flow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:20-27
