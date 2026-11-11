Pressurizer steam pressure in psig - effectively RCS pressure, since the pressurizer connects to the hot leg and its steam cushion dominates system response. Full-power operation sits near 2250 psia against a 2500 psia design pressure; heaters and spray hold that line through insurges and outsurges in accordance with the applicable temperature-pressure curves.

Reading it well means reading it alongside [level](ReactorBuilding/Pressurizer/Water): level is programmed against Tavg, so a level swing with steady temperature usually signals a pressure transient underway. Pressure also sequences the protection stages above normal band - [spray](ReactorBuilding/Pressurizer/SprayOpen), then the [PORV](ReactorBuilding/PORV/Open), then self-actuating [code safety valves](ReactorBuilding/SafetyValve/Open) - and feeds SI logic below band. During cooldown, pressure plus temperature together gate RHR connection and brittle-fracture limits.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
Source: specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:23-34

Related controls: [PORV](ReactorBuilding/PORV/Open), [SafetyValve](ReactorBuilding/Faults/PressurizerSafetyValve)
Related sensors: [PORV](ReactorBuilding/PORV/Open), [SafetyValve](ReactorBuilding/SafetyValve/Open), [Water](ReactorBuilding/Pressurizer/Water)
