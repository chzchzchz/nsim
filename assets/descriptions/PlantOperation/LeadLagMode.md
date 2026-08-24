Chooses the coordination philosophy of the integrated control system: **TurbineLead** lets turbine demand drive reactor power, while **ReactorLead** makes the turbine follow reactor power. The RPC screen shows which mode is active and allows changing it.

In turbine lead the load dispatcher's demand moves the turbine first and rods follow to match steam production, giving tight grid response; in reactor lead the operator sets reactor power and secondary systems track it, which is gentler on the core during maneuvering. Lead/lag selection matters most during load-follow operation when coupling between Tavg, steam dump, and rod motion is strongest.

See also: [PowerError](PlantOperation/PowerError), [LoadDemand](ReactorBuilding/Grid/LoadDemand)

Source: specs/pwr_sim/chapter_2/page_023.txt:27-28
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-31
