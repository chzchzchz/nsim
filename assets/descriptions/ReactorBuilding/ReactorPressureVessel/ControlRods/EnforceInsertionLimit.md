Latch that makes the rod insertion limit an enforced drive clamp rather than a procedural caution. While on, the rod drive refuses any demand deeper than [InsertionLimit](ReactorBuilding/ReactorPressureVessel/ControlRods/InsertionLimit), keeping resting bank positions inside the shutdown-margin envelope through manual withdrawal and automatic rod control alike.

The zero (off) value leaves the demand operator-owned, which is the historical lineup; evolutions that want the guard latch it together with the limit before starting rod motion. A reactor trip supersedes the clamp: full insertion is a scram action, not a position limit.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:30-34

Related controls: [InsertionLimit](ReactorBuilding/ReactorPressureVessel/ControlRods/InsertionLimit)
Related sensors: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
