Malfunction latch that one control cluster has been ejected from the core. Control groups are built symmetrical and worth about one percent of reactivity each, precisely so a hypothetical rod cluster ejection gives an acceptable transient. When this latch is set the missing group adds that worth to the core as a direct [EjectedReactivityPct](ReactorBuilding/ReactorPressureVessel/ControlRods/EjectedReactivityPct) term instead of a rod-position shift.

Set it to reproduce the dropped-bank and full-ejection transients; clear it to return the cluster to normal service. The prompt on the following step raises [EjectedEvent](ReactorBuilding/ReactorPressureVessel/ControlRods/EjectedEvent), which is what the protection system watches for the high-flux-rate trip. A reactor trip is the expected response once the flux rises. Rod position sensors keep reporting the cluster as if seated while the latch is set, so the position disagree is the diagnostic.

Related controls: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
Related sensors: [EjectedEvent](ReactorBuilding/ReactorPressureVessel/ControlRods/EjectedEvent)

Source: specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:28-33
