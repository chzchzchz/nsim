Indicates the reactor makeup sequence has been initiated - the demand signal that drives the makeup control valve through its opening stroke.

It is a momentary-style status, not a position: lit while an inventory demand is being satisfied, dark once level or pressure recovers and the sequence resets. Use it to distinguish "makeup running normally" from "valve drifted open with no demand," which is a leak-shaped signature worth a look at letdown and the VCT level trend before blaming the seals.

Model note: the simulator derives this flag from the makeup logic state machine; the reference describes the valve's makeup function without detailing initiation annunciation.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4

Related controls: [Auto](ReactorBuilding/ReactorMakeupControlValve/Auto)
Related sensors: [ChargingFlow](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
