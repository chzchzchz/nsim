Core damage progression from 0 to 1 as a percent of clad failure: 0 at or below the ballooning onset of 1520 F, saturating to 1 at the 2200 F peak-clad limit of 10 CFR 50.46. It is a one-way ratchet: once clad has burned, re-covering the core and cooling it does not heal the damage, so the fraction never decreases. This is the damage number behind [Damage](ReactorBuilding/ReactorPressureVessel/Damage) and the part generic plant consumers read.

Read it with [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp) to separate driver from result: temperature can fall while the fraction stays where the excursion left it. The fraction also drives the fuel release term into [PrimaryActivity](ReactorBuilding/ReactorPressureVessel/PrimaryActivity), so a rising core damage fraction eventually shows up in the containment air inventory.

Model note: the damage curve is a lumped temperature readout; the reference gives the limit points but no progression law.

Related sensors: [CladTemp](ReactorBuilding/ReactorPressureVessel/CladTemp), [OxidizedFrac](ReactorBuilding/ReactorPressureVessel/OxidizedFrac)

Source: specs/westinghouse_tech_manual/chapter_2/page_0209.txt:25-30
