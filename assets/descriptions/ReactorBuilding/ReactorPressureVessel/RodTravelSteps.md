Cumulative rod travel in CRDM (control rod drive mechanism) steps, counting the absolute [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion) movement each step. At 100 steps per full insertion travel, the counter tracks drive-mechanism wear the way the preset reversible drive program counts it. It advances only when rods actually move; a steady plant at power accumulates nothing.

Read it during startup and load-following: hundreds of steps per maneuver is normal, thousands per week is a wear budget being spent. The control-rod drive program minimizes rod movement during random load variations precisely to protect this counter. A sudden jump with no [InsertionLimit](ReactorBuilding/ReactorPressureVessel/ControlRods/InsertionLimit) change points at a dropped-bank event, not normal maneuvering.

Model note: steps per travel and the pre-trip wear law are derived; the reference names the drive program but gives no steps-per-travel figure.

Related controls: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion), [InsertionLimit](ReactorBuilding/ReactorPressureVessel/ControlRods/InsertionLimit)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_133.txt:1-11
