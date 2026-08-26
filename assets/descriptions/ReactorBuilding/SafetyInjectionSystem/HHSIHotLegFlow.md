Share of high-head safety injection delivered into the RCS hot legs, gpm. It reads zero until the operator realigns to simultaneous cold- and hot-leg injection (see [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)), then tracks the split the realignment establishes.

Why hot legs at all: after a large break the core flow path can vapor-lock, and injection only through the cold legs leaves the upper internals steam-bound. The spec's long-term cooling guidance is that "a substantial portion of the flow is directed to the RCS hot legs" to strip steam and keep boron moving everywhere. Watch this rate alongside cold-leg delivery during recirculation - it is the anti-boron-buildup cooling path.

Model note: the backing field is stored in gal/s; the bridge scales by 60 for the gpm presentation.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:132-133

Related controls: [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)
Related sensors: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)
