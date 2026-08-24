Chooses the coordination philosophy of the integrated control system: **TurbineLead** lets turbine demand drive reactor power, while **ReactorLead** makes the turbine follow reactor power. The RPC screen shows which mode is active and allows changing it.

In turbine lead the dispatcher's load demand moves the turbine governor first, and rods follow to match steam production to the new draw. That gives the grid tight frequency support but asks the core to chase every load change. Reactor lead reverses the causality: the operator sets reactor power and the secondary system tracks the steam the core actually makes, which is gentler on fuel and xenon during slow maneuvers. The choice matters most during load-follow operation when coupling between Tavg, rod motion, and steam dump is strongest and a poorly matched mode shows up as oscillation rather than smooth tracking.

Source: specs/pwr_sim/chapter_2/page_023.txt:27-28
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-31

Related controls: [LoadDemand](ReactorBuilding/Grid/LoadDemand)
Related sensors: [PowerError](PlantOperation/PowerError)

Source: specs/pwr_sim/chapter_2/page_023.txt:27-28
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-31
