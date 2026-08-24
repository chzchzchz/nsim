Maximum allowed rod insertion fraction (0 to 1) while enforcement is latched. With [EnforceInsertionLimit](ReactorBuilding/ReactorPressureVessel/ControlRods/EnforceInsertionLimit) on, the rod drive clamps every demand - manual or automatic - to this ceiling, so resting positions stay withdrawn enough for a trip to add at least the prescribed negative reactivity: rods initially too far in leave too little shutdown margin.

At zero power the criticality evolution holds resting positions about 95 percent withdrawn (insertion 0.05 or less); deeper limits apply as power falls. A reactor trip bypasses the clamp and drives full insertion regardless.

Model note: the limit value is operator-entered per lineup; the reference defines the mechanism and its purpose but publishes no single number (limits vary with thermal power).

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:30-34

Related controls: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion), [AutoControl](ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl)
Related sensors: [ControlBankA](ReactorBuilding/ReactorPressureVessel/ControlRods/ControlBankA)
