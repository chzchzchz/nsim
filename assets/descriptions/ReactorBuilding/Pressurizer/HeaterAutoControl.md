Arms automatic pressurizer pressure regulation: heaters then cycle inside the auto deadband of the pressure setpoint, and the manual [HeatersOn](ReactorBuilding/Pressurizer/HeatersOn) latch becomes an override rather than the primary control.

"Pressurizer pressure will recover as pressurizer heaters automatically energize" - that is the trip-response behavior this switch provides in steady operation too. With it armed you can treat heater state as an indication (heaters on means pressure below band); with it off, every pressure excursion needs a manual heater decision at exactly the moment attention is scarce.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:95-98

Related controls: [HeatersOn](ReactorBuilding/Pressurizer/HeatersOn), [PressureSetpoint](ReactorBuilding/Pressurizer/PressureSetpoint), [SprayOpen](ReactorBuilding/Pressurizer/SprayOpen)
Related sensors: [Pressure](ReactorBuilding/Pressurizer/Pressure)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:95-98
