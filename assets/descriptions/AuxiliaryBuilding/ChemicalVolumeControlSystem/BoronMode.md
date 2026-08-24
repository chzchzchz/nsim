Selects the CVCS operating mode: **VolumeControl** (plain inventory matching), **Boration**, **Dilution**, or **Letdown**. Mode plus the boron setpoint drive the chemical shim path; the comparison of measured cold leg temperature with programmed Tc determines what change is needed.

The boron control system integrates BTRS and the reactor water makeup system to execute changes. Boration adds concentrated boric acid; dilution feeds primary-grade water; both move [RCS boron concentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration) slowly relative to rod motion.

Related controls: [BoronSetpoint](ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint), [BTRSPump](AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump)
Related sensors: [BoronConcentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-66
