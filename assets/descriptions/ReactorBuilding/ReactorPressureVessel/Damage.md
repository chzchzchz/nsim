Part damage readout for the reactor pressure vessel, mirroring [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac). Zero means intact clad, one means the 2200 F peak-clad limit is fully reached. Generic damage consumers read this field, which is why it tracks the severe-accident fraction and not operational wear like [CladdingFatigue](ReactorBuilding/ReactorPressureVessel/CladdingFatigue).

It ratchets and never recovers inside an evolution: reflood and cooling freeze the value where it stands but never lower it, so a falling [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp) with a held damage reading is the evidence that burning already happened. The percent display lives in the core-damage fraction above; generic consumers that average over parts see this number, which is why fatigue wear stays invisible to them. A stable zero across a long accident means the 2200 F limit was never reached, not that the cladding never heated.

Related sensors: [CoreDamageFrac](ReactorBuilding/ReactorPressureVessel/CoreDamageFrac), [VesselFailed](ReactorBuilding/ReactorPressureVessel/VesselFailed)

Source: specs/westinghouse_tech_manual/chapter_2/page_0209.txt:25-30
