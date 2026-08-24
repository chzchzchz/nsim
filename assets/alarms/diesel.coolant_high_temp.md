Diesel generator jacket coolant at or above 220F. Under rated load the engine rejects enormous heat through its jacket water, which rejects in turn through a cooler served by [service water](Outside/ServiceWaterSystem/PumpRunning) - so this alarm usually means SWS capacity or supply degraded, not engine trouble.

The stakes exceed the diesel itself: an overheated machine cannot be trusted for its 30-second-start mission later, which silently downgrades the plant's entire backup power train while the alarm is lit. Check the SWS chain first ([flow](Outside/ServiceWaterSystem/Flow), [outlet temperatures](Outside/ServiceWaterSystem/OutletTemp)), then jacket valve lineups; reduce load if temperatures keep climbing.

Standby diesels are kept warm deliberately so a start from cold never thermal-shocks the block - coolant drifting low at standby means heat tracing failed and needs attention before the next start test. At or above 220F while running, the engine also trips itself: service-water loss must take the engine out rather than cook it.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:10-18
Source: specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19

Related controls: [PumpRunning](Outside/ServiceWaterSystem/PumpRunning)
Related sensors: [CoolantTemp](AuxiliaryBuilding/DieselGenerator/CoolantTemp), [OutletTemp](Outside/ServiceWaterSystem/OutletTemp), [ReadyToLoad](AuxiliaryBuilding/DieselGenerator/ReadyToLoad)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:10-18
