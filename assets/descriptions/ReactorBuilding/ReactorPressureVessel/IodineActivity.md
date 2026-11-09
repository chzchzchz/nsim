Lumped iodine coolant activity in the primary, in normalized units where 1.0 is rated-power equilibrium. Iodine shares production with noble gas (proportional to fission power) but has a different removal path: containment spray scrubs it into the sump instead of the charcoal delay carrying it to the vent. Its effective half-life is the short I-135 value, so it tracks power changes faster than Xe-133 noble gas.

A rising iodine reading with a falling [PrimaryActivity](ReactorBuilding/ReactorPressureVessel/PrimaryActivity) points at a clad leak adding fresh release beyond fission production. It is also the fraction that spray can actually remove: [ContainmentSumpActivity](ReactorBuilding/ContainmentBuilding/ContainmentSumpActivity) climbs as this inventory gets scrubbed. The lumped primary inventory and the spray-to-sump path are model simplifications.

Related sensors: [PrimaryActivity](ReactorBuilding/ReactorPressureVessel/PrimaryActivity), [ContainmentSumpActivity](ReactorBuilding/ContainmentBuilding/ContainmentSumpActivity)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:170-173
Source: specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:70-73
