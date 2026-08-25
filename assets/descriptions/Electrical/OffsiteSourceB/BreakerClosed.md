Output breaker for offsite source B, the preferred supply to engineered safety features bus B. Opening it strips that train's grid feed; with both offsite breakers open and the unit off the line, the sensed dead-bus condition auto-starts the emergency [diesels](AuxiliaryBuilding/DieselGeneratorB/StartSignal) and sequences the safety buses onto them.

Each power source - diesel generator and offsite - is physically and electrically independent up to its connection to the ESF buses, so train B's supply path never shares a breaker with train B.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:29-36
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-109

Related controls: [BreakerClosed](Electrical/OffsiteSourceB/BreakerClosed), [BreakerClosed](Electrical/ESFBusB/BreakerClosed)
Related sensors: [Energized](Electrical/ESFBusB/Energized)
