Cumulative safety-injection volume delivered into the RCS hot legs, in gallons. It reads non-zero once the operator realigns to simultaneous cold- and hot-leg injection (see [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)).

Why hot legs at all: after a large break the core flow path can vapor-lock, and injection only through the cold legs leaves the upper internals steam-bound. The spec's long-term cooling guidance is that "a substantial portion of the flow is directed to the RCS hot legs" to strip the steam and keep boron moving everywhere. Watch this total alongside cold-leg delivery as the recirculation phase grinds on - it is the bookkeeping for anti-boron-buildup cooling.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:132-133

Related controls: [HotLegInjection](ReactorBuilding/SafetyInjectionSystem/HotLegInjection)
Related sensors: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)
