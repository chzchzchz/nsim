Startup rate in decades per minute (DPM), the control-room power-excitation indication derived from the source-range signal. Startup procedure caps the steady-state rate at 1.0 DPM when rod motion is not in progress: a planned approach to criticality sizes the final rod step so the predicted SUR stays inside that band.

A positive sustained reading with no rod motion, alongside increasing [source range counts](ReactorBuilding/ReactorPressureVessel/SourceRangeCounts), is the criticality evidence; a negative reading means the chain is decaying subcritical. The reading is signed, so decay after a re-insertion shows directly.

Model note: simulator-derived first-order filtered log derivative of fission power (5 s response); the reference mandates the indication but gives no instrument response time.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_121.txt:19-20
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:59-63

Related sensors: [SourceRangeCounts](ReactorBuilding/ReactorPressureVessel/SourceRangeCounts)
Related controls: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
