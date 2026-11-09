Peak fuel cladding temperature in Fahrenheit. Covered clad tracks saturation at system pressure; an uncovered core climbs freely under decay heat and oxidation enthalpy, and a departure-from-nucleate-boiling (DNB) burnout excursion climbs even while still covered. This is the temperature the 10 CFR 50.46 peak-clad limit of 2200 F governs.

Cross-check it against [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac) and [OxidizedFrac](ReactorBuilding/ReactorPressureVessel/OxidizedFrac): the temperature is the driver, the fractions are the damage it leaves behind. At or above 3410 F with the core uncovered, the [VesselFailed](ReactorBuilding/ReactorPressureVessel/VesselFailed) latch sets, which is why clad temperature is the number to arrest first with injection. Re-covering the core pins clad back to saturation; the fractions remember the excursion. A rising clad temperature with a falling system pressure points at an uncovery that is consuming the heat sink, not at a power excursion.

Related sensors: [Pressure](ReactorBuilding/ReactorPressureVessel/Pressure), [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:51-53
Source: specs/westinghouse_tech_manual/chapter_2/page_0209.txt:25-30
