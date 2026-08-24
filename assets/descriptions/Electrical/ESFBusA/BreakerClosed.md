Feeder breaker for engineered safety features bus A. A source - off-site transformer or diesel generator - can deliver voltage to the train only with this breaker closed, so it gates every downstream load pickup on that train.

Power sources feeding the ESF buses are physically and electrically independent up to the point of connection, each through its own breaker logic and its own [diesel](AuxiliaryBuilding/DieselGenerator/Generator/BreakerClosed). Bus pickup logic requires the breaker closed before any source can energize the train - an open breaker silently converts a healthy power system into a dead one.

Source: /chapter_11/page_165.txt:29-36

Related controls: [BreakerClosed](AuxiliaryBuilding/DieselGenerator/Generator/BreakerClosed)
Related sensors: [Energized](Electrical/ESFBusA/Energized), [Voltage](ReactorBuilding/Grid/Voltage)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:29-36
