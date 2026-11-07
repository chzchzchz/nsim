Cold-leg temperature running low - the secondary is removing heat faster than the core produces it, over-cooling the primary. Tcold reflects heat removal rather than core heating, which makes this alarm a statement about the steam side even though it reads on the primary.

Over-cooling is a reactivity event as much as a thermal one: denser coolant moderates better, adding positive reactivity precisely when attention is elsewhere. Expect automatic rod insertion demands; consider [boration](ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint) if the cause persists or rods approach their insertion limit.

Usual suspects: excessive steam dump demand ([dump valves](TurbineBuilding/Turbine/DumpValve)), an atmospheric relief left open, or feedwater over-delivery. The [inlet temperature](ReactorBuilding/ReactorPressureVessel/Inlet) trend and [boron concentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration) identify which.

Model note: raised directly off [inlet temperature](ReactorBuilding/ReactorPressureVessel/Inlet) against the 530F RPS plate when the P-7 at-power permissive is satisfied.

Source: specs/pwr_sim/chapter_2/page_023.txt:32-35

Related controls: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion), [BoronSetpoint](ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint), [DumpValve](TurbineBuilding/Turbine/DumpValve)
Related sensors: [Inlet](ReactorBuilding/ReactorPressureVessel/Inlet), [BoronConcentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration)

Source: specs/pwr_sim/chapter_2/page_023.txt:32-35
