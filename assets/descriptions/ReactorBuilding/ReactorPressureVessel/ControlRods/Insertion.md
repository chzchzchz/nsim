Control rod insertion demand from fully withdrawn (0) to fully inserted (1). This is the operator's manual reactivity handle; actual rod motion is rate-limited by the drive speed so the core never sees a step change in reactivity.

Two constraints shape its use. First, the rod insertion limit: rods must stay above a minimum height during power operation so that a trip inserts enough negative reactivity - if rods run too deep, the margin a trip can add shrinks below the required shutdown margin. Second, criticality technique: after trip-reset the rods are withdrawn to achieve criticality, but the fine approach uses [boron dilution](ReactorBuilding/ReactorPressureVessel/BoronConcentration) so criticality lands with rods well above the limit, leaving rod motion available for power control afterward.

With [automatic control](ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl) engaged this demand is owned by the controller.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:155-156
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:12-20

Related controls: [AutoControl](ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl), [Trip](ReactorBuilding/ReactorPressureVessel/Trip)
Related sensors: [BoronConcentration](ReactorBuilding/ReactorPressureVessel/BoronConcentration)

Source: $W/chapter_16/page_225.txt:155-156
Source: $W/chapter_16/page_225.txt:12-20
