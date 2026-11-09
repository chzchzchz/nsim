Axial flux difference in percent: top-half core power minus bottom-half, over total. Zero means the flux centroid sits at midplane, positive is top-heavy, and negative is bottom-heavy. The model axial offset is the same upper-to-lower half difference, so the detector projection is the identity. Follow [AxialOffset](ReactorBuilding/ReactorPressureVessel/AxialOffset) for the plant-level number behind it.

Rod insertion pushes it negative because rods enter from the top; xenon-135 oscillations drag it back over hours as the poison burns where flux concentrates. A large offset at high power pushes peak-to-average toward fuel limits even when [ExcorePowerRange](ReactorBuilding/ReactorPressureVessel/ExcorePowerRange) looks ordinary, and it eats into the [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin) (departure-from-nucleateboiling ratio margin) through the axial term. Keep it inside the operating band during maneuvers and correct with rod insertion.

Related sensors: [AxialOffset](ReactorBuilding/ReactorPressureVessel/AxialOffset), [DNBRMargin](ReactorBuilding/ReactorPressureVessel/DNBRMargin)

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:73-94
