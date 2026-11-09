Indicated departure-from-nucleate-boiling margin, signed percent around the derived reference. It reads 1.30 at steady full-load conditions (63 F core rise, no axial skew) and falls as the core rise widens or the axial offset grows; below 1.0 means the local flux exceeds critical heat flux and [DNBFireLive](ReactorBuilding/ReactorPressureVessel/DNBFireLive) is true. Negative readings are real and mean the margin is spent.

No trip is wired to this margin in this simulation, so it is a monitor, not a protection input: use it to steer rod insertion and flow away from the corner rather than waiting for an interlock. Watch it during power ascensions and during any flow coastdown, where a falling [TTFMFlowPercent](ReactorBuilding/ReactorPressureVessel/TTFMFlowPercent) and a widening delta-T converge.

Model note: the 1.30 reference and the 1.0 critical-heat-flux point are simulator-derived display anchors; the reference names no DNBR number and wires no trip to this margin.

Related sensors: [DNBFireLive](ReactorBuilding/ReactorPressureVessel/DNBFireLive), [AxialFluxDifference](ReactorBuilding/ReactorPressureVessel/AxialFluxDifference)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_126.txt:71-81
