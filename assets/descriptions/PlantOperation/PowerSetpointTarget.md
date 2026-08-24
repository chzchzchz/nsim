Operator-entered target for reactor power as a fraction of rated full power. The demanded power setpoint ramps toward this target at the requested rate rather than stepping instantly, which keeps rod motion, fuel temperature gradients, and xenon perturbations bounded during maneuvering.

The power control system turns this target into coordinated action automatically: rod motion handles fast reactivity, moderator temperature responds through the fuel-coolant feedback loop, and boron concentration changes handle the slow drifts. A remote dispatch interface exists in the reference design - it transmits computed power capability upward to the network dispatcher's computer, but acts as a permissive network so an unsafe dispatch demand cannot simply be executed against the plant.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:14-19
Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:10-18

Related controls: [PowerRateDemand](PlantOperation/PowerRateDemand)
Related sensors: [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:14-19
Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:10-18
