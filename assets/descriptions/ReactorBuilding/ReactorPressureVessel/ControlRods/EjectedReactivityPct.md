Reactivity worth of the ejected cluster in percent delta-k/k, normalized. It reads 1.0 percent while [Ejected](ReactorBuilding/ReactorPressureVessel/ControlRods/Ejected) is latched and zero when the cluster is seated; the worth enters the core reactivity as a direct additive term rather than a position shift. That one percent is roughly one control group's worth, deliberately sized to keep the ejection transient acceptable.

A sudden 1 percent step moves power faster than any rod drive can chase, which is why the flux-rate trip exists. Expect [StartupRate](ReactorBuilding/ReactorPressureVessel/StartupRate) to rise within the step and the reactor to trip if the RPS is armed. The term clears only when the latch clears, so a post-trip restart with the latch still set immediately reproduces the ejection reactivity in the restart transient.

Related sensors: [EjectedEvent](ReactorBuilding/ReactorPressureVessel/ControlRods/EjectedEvent), [StartupRate](ReactorBuilding/ReactorPressureVessel/StartupRate)

Source: specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:28-33
