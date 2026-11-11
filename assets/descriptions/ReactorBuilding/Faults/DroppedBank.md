Simulator construct with no plant control: a malfunction latch filed with the faults that drops one rod control bank. While latched, the bank sits jammed fully into the core at 85 percent insertion, and the dropped-rod signal raises an automatic load runback on the first step - before any reactor trip - so the turbine sheds toward house loads.

Physically a dropped bank is gravity winning: a rod fails to hold and falls in. The immediate nuclear effect is negative reactivity concentrated in one region (watch [AxialOffset](ReactorBuilding/ReactorPressureVessel/AxialOffset) swing), and the protection response is economic rather than safety-critical: back power down to where the distorted core can hold it. The training catalog inserts this malfunction to exercise exactly that runback path.

Source: specs/pwr_sim/chapter_4/page_049.txt:61-67
Source: specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29

Related controls: [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
Related sensors: [AxialOffset](ReactorBuilding/ReactorPressureVessel/AxialOffset)
