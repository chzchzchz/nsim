The simulator's own net reactivity in percent delta-k/k: rod worth, Doppler, moderator temperature coefficient, boron, xenon, and the ejection term summed. Positive means the chain would power up on its own, negative means it would decay without rods moving. This is the inverse of the operational problem: the operator never measures this directly, but infers it from rod position and boron concentration.

Xenon poisoning is the classic inference: after a power reduction the rods need to come out further than the last shutdown to hold criticality, and the difference is the xenon worth in [XenonWorth](ReactorBuilding/ReactorPressureVessel/XenonWorth). The reference has no instrument for xenon, so this estimate is how the simulator makes that inference visible.

Model note: the reference names the components but provides no computing instrument; the bridge exposes the simulator total for teaching.

Related sensors: [XenonWorth](ReactorBuilding/ReactorPressureVessel/XenonWorth), [StartupRate](ReactorBuilding/ReactorPressureVessel/StartupRate)

Source: specs/the_westinghouse_pressurized_water_react/glossary.md:140-140
