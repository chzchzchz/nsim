Closes the train B diesel output breaker onto its engineered safety features bus - the moment backup power becomes actual power for this train's safety loads.

Closing requires the generator matched to bus voltage and frequency (its own regulator handles that within seconds of [ReadyToLoad](AuxiliaryBuilding/DieselGeneratorB/ReadyToLoad)), and the bus dead or separated from off-site sources so supplies never parallel unintentionally. With per-train machines, closing this breaker while [ESFBusB](Electrical/ESFBusB/Energized) is fed from the grid is exactly the wrong move; the interlocks exist so you never have to reason that fast.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:29-36

Related controls: [BreakerClosed](Electrical/ESFBusB/BreakerClosed), [StartSignal](AuxiliaryBuilding/DieselGeneratorB/StartSignal)
Related sensors: [Energized](Electrical/ESFBusB/Energized)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:29-36
