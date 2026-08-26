Injects an evaporator carryover fault: entrained borated liquid contaminates the distillate instead of staying behind in the evaporator shell, so the radiation monitor downstream sees activity it must not see.

This is a modeling device for the protection the spec implies. The reference only says the reactor makeup water storage tank "will not become contaminated" and therefore carries a scintillation counter in the distillate line; something has to make that monitor matter. With carryover latched, [DistillateRadiation](AuxiliaryBuilding/BoronRecycleSystem/DistillateRadiation) ramps toward its trip level, the auto-divert routes distillate back to the feed demineralizer ([DistillateDiverted](AuxiliaryBuilding/BoronRecycleSystem/DistillateDiverted)), and clean makeup delivery stops accumulating. Clearing the latch lets the monitor decay and the divert release.

Model note: the carryover source, monitor response (30 s time constant), and trip level are simulator-derived; the reference names no cause or number.

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:25-28

Related sensors: [DistillateRadiation](AuxiliaryBuilding/BoronRecycleSystem/DistillateRadiation), [DistillateDiverted](AuxiliaryBuilding/BoronRecycleSystem/DistillateDiverted), [DistillateDelivered](AuxiliaryBuilding/BoronRecycleSystem/DistillateDelivered)
