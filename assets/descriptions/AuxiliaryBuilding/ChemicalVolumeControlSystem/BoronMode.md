Selects the CVCS operating mode: **VolumeControl** (plain inventory matching), **Boration**, **Dilution**, or **Letdown**. Mode plus the boron setpoint drive the chemical shim path - the comparison of measured cold leg temperature with the programmed Tc-for-power value determines what the reactivity balance needs.

Boration injects concentrated boric acid solution from the boric acid tanks; dilution feeds primary-grade water, diluting what is already dissolved. Both work through the same pumps and heat exchangers as everyday cleanup - chemistry changes ride along the normal flow path rather than needing dedicated plumbing. The boron control system integrates BTRS beds and the reactor water makeup system to execute changes automatically.

Changes land slowly - minutes of flow time versus seconds of rod motion - which is exactly why they are trusted with slow drifts (burnup, xenon decay) and never asked to fight transients.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-66

Related controls: [BoronSetpoint](ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint), [BTRSPump](AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump)
Related sensors: [BoronConcentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-66
