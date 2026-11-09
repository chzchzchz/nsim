Peak linear power density in kilowatts per foot, normalized to the derived peak at reference conditions. Average power spreads over 193 assemblies times 264 rods times 12 feet of active length, then the peaking factor (peak can more than double the average) and a small axial term give the peak. It reads 1.0 at rated power with midplane flux, so a reading above 1.0 means the hottest spot is running above its derived reference.

Pair it with [AxialFluxDifference](ReactorBuilding/ReactorPressureVessel/AxialFluxDifference) and [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin): linear power density (LPD) is the fuel-side power peak, departure-from-nucleate-boiling ratio (DNBR) is the coolant-side margin against it. Axial skew raises LPD before it moves total power, which is why the offset belongs in the maneuvering limits.

Model note: the fuel geometry and the peaking factor make this a derived readout; the reference states the peaking principle but no kW/ft limit.

Related sensors: [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin), [AxialFluxDifference](ReactorBuilding/ReactorPressureVessel/AxialFluxDifference)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_132.txt:58-61
Source: specs/the_westinghouse_pressurized_water_react/chapter_2/page_029.txt:10-15
