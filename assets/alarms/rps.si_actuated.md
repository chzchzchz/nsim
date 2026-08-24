Safety injection has actuated - the RPS decided the primary needs emergency borated water, on signals such as low RCS pressure or high pressurizer level, or on a manual call. Injection pumps started with their discharge paths aligned.

This alarm means an accident sequence or serious malfunction is underway, and verification replaces complacency: pumps running against a still-pressurized RCS are on [miniflow](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning) until pressure falls below shutoff head - running is not injecting yet. Confirm the reactor tripped, confirm charging/HHSI flow when pressure allows, and track injection source inventory ahead of the eventual swap to containment sump recirculation.

Everything else happening simultaneously - alarms, isolations, starts - is downstream choreography designed around this moment.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:26-36

Related controls: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning)
Related sensors: [HHSIPumpRunning](ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning), [Pressure](ReactorBuilding/Pressurizer/Pressure), [Pressure](ReactorBuilding/ReactorPressureVessel/Pressure), [PowerError](PlantOperation/PowerError)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:26-36
