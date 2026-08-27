Latches when sufficient steam pressure exists in the main steam line to admit steam to the turbine. High drive-ready means the upstream steam supply is live and the steam admission valve is open; the pump will turn when the operator commands [Running](Outside/TurbineAFWPump/Running).

The steam admission valve fails open on loss of air or electrical power, so once [SG pressure](ReactorBuilding/SteamGenerator/Pressure) is established after reactor trip, DriveReady stays high. If the steam generators depressurize after an MSIV isolation without dump flow, DriveReady can drop even though the operator has not commanded stop.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:130-134

Related controls: [Running](Outside/TurbineAFWPump/Running)