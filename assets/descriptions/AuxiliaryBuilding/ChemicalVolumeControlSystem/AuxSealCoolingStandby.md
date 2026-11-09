Standby seal-cooling latch for the reactor coolant pump seals. While seal injection is off, the seal water heat exchanger cooled by the component cooling water system provides the second heat sink for the seals. It only counts when CCWS is running and drive-ready, so a latched standby with dead CCWS does not protect the seals.

Latch it before securing seal injection or before a pump restart, and release it when normal seal injection is restored. Uncooled seals cook in minutes, so standby cooling is the bridge between normal injection and shutdown cooling. Watch [ChargingPumpRunning](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning) and the [PumpRunning](AuxiliaryBuilding/ComponentCoolingWaterSystem/PumpRunning) state on the CCWS train; the seal water heat exchanger only cools while CCWS delivers flows, so a latched standby on a dead train is a label, not a heat sink.

Related controls: [ChargingPumpRunning](AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:76-77
Source: specs/westinghouse_tech_manual/chapter_4/page_0380.txt:27-29
