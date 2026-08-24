Shutdown-bank position indication (0 fully withdrawn, 1 fully inserted), decomposed from the aggregate rod path. The two shutdown banks travel first during withdrawal: they complete their stroke before any control-bank motion starts, which is why the procedure verifies all shutdown banks fully withdrawn within 15 minutes of withdrawing control banks.

On a reactor trip every bank - shutdown and control - drives to the fully inserted position; the shutdown banks carry the first block of negative reactivity that guarantees shutdown margin even with the most reactive control bank stuck out.

Model note: the bridge exposes one aggregate rod demand; this reading maps it onto the physical bank sequence for visibility, using a derived 30 percent worth share for the shutdown group.

Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:25-26
Source: specs/westinghouse_plant_operations/chapter_19/page_017.txt:9-10

Related sensors: [ControlBankA](ReactorBuilding/ReactorPressureVessel/ControlRods/ControlBankA), [Insertion](ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion)
