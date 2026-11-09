Airborne activity in the containment atmosphere, in normalized units where 1.0 is rated-power equilibrium. Break discharge, relief accumulation, and dusting of the sump all feed this inventory, and containment spray scrubs iodine out of it into the sump. It is the inventory that escapes first if the containment fails.

Read it with [ContainmentSumpActivity](ReactorBuilding/ContainmentBuilding/ContainmentSumpActivity): spray success moves activity from air to sump without destroying it, so a falling air reading with a rising sump reading is scrubbing, not decay. The [PrimaryActivity](ReactorBuilding/ReactorPressureVessel/PrimaryActivity) inventory is the upstream source, and the [IodineActivity](ReactorBuilding/ReactorPressureVessel/IodineActivity) fraction is the part the spray can actually strip. A rising air reading with no spray running means a fresh break source, not a resuspension. Noble gas is not scrubbed at all, so the air inventory can hold up even while the spray pulls iodine out.

Related sensors: [ContainmentSumpActivity](ReactorBuilding/ContainmentBuilding/ContainmentSumpActivity)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:12-18
