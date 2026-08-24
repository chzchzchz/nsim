Target RCS boron concentration in ppm handed to the CVCS boration/dilution logic. The write latches setpoint tracking: CVCS selects **Boration** or **Dilution**, runs the BTRS recirculation pump, and ramps the measured concentration toward the target on the flow-rate clock (about 0.8 ppm/s each way) until it lands inside the arrival band, then releases chemistry back to volume control. The read-back shows the measured concentration closing on the target, not the target itself; watch [BoronTarget](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronTarget) for what was commanded.

This setpoint is the operator-visible handle on the slow shim path. Fast transients belong to the rods, which move in seconds; chemistry moves in minutes as charging pumps trade letdown for boric-acid-bearing or pure water - startup dilution to the estimated critical concentration is paced by this ramp, not stepped. The full path runs through [CVCS mode](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode), the boron thermal regeneration beds, and the reactor water makeup system.

Model note: the ramp rate reuses the simulator's fixed BTRS exchange rates rather than modeling dilution batch flow explicitly.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:36-38

Related controls: [BoronMode](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode)
Related sensors: [BoronConcentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration), [BoronTarget](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronTarget)

Source: $W/chapter_7/page_135.txt:58-62
