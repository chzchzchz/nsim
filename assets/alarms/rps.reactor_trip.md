The reactor protection system has tripped: trip breakers opened, rod drive power died, and the control rods dropped into the core by gravity within about two seconds. Fission stops immediately; what remains is decay heat - initially several percent of full power, decaying slowly over hours - and that, not fission, is what the operator manages next.

Trips fire on engineered signals as well as pushbuttons: low RCS pressure, high neutron flux, low-low steam generator level, low steamline pressure, high containment pressure, and their siblings. Every other annunciator sounding during a trip transient is usually downstream of this one, which is why sequence-of-events reading starts here.

Immediate response is verification, not action: confirm rod bottom on the position indication, confirm heat removal - [steam dump](TurbineBuilding/Turbine/DumpValve) or [atmospheric relief](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open) carrying decay heat with feedwater replacing what boils off - and stabilize pressure with heaters and spray. Expect a xenon transient afterward; the reference notes boron changes may be required because of it before restart.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:30-34

Related controls: [Reset](ReactorBuilding/ReactorProtectionSystem/Reset), [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion), [AtmosphericDumpValve](ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open)
Related sensors: [PowerError](PlantOperation/PowerError), [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint), [Pressure](ReactorBuilding/Pressurizer/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:12-15
