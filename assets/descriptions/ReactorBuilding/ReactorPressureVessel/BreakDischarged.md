Cumulative gallons discharged through the hot-leg break since the start of the evolution, in gallons. This ledger fills during a LOCA and drains slowly on injection credit, so it is the net inventory loss that drives the [Uncovery](ReactorBuilding/ReactorPressureVessel/Uncovery) fraction. The pwr_sim exercise tells the operator to record the break flow in Table V; this total is the same habit.

Read the level against [BreakFlow](ReactorBuilding/ReactorPressureVessel/BreakFlow): the flow is the instantaneous leak rate, this number is how much inventory is already gone. When [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning) credit exceeds the leak, the total stops climbing, which is the quantitative sign the inventory arrest is working.

Model note: the cumulative ledger is model-internal bookkeeping; the reference records only the per-step break flow, never a total.

Related sensors: [BreakFlow](ReactorBuilding/ReactorPressureVessel/BreakFlow)

Source: specs/pwr_sim/chapter_4/page_053.txt:64-65
