Derived flag that the BTRS demineralizer path is not in service, meaning the plant is not actively removing corrosion products from the letdown stream. It reads true whenever the BTRS recirculation pump is stopped, letdown is not flowing, or the path is diverted. A fresh startup lineup reads true until the BTRS is put in service.

The concern is slow chemistry, not an immediate trip: sustained operation without demineralization lets dissolved iron and crud build up in the primary, feeding CRUD and dose rate later. Check [BTRSPump](AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump) and [BoronMode](AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode) to see which leg of the predicate failed.

Model note: the bridge publishes the predicate as a boolean; the model does not simulate a corrosion inventory, so this is a service status, not a dose prediction.

Related controls: [BTRSPump](AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump)

Source: specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39
