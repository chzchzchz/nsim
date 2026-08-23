package core

import (
	"fmt"

	"github.com/chzchzchz/nsim/bridge"
)

// GetControlMap returns a bridge.ControlMap populated with controls for the
// given plant. Each closure captures p and dereferences live state on every
// call so the map survives Plant.Step() and Plant.Update() cycles without
// pinning stale nextPlant addresses.
func GetControlMap(p *Plant) bridge.ControlMap {
	m := make(bridge.ControlMap)
	addPlantOperationControls(p, m)
	addReactorBuildingControls(p, m)
	addTurbineBuildingControls(p, m)
	addAuxiliaryBuildingControls(p, m)
	addOutsideControls(p, m)
	return m
}

// addPlantOperationControls registers the operator's plant-level mode
// selection. Mode changes are operator actions during startups and
// shutdowns (placing the unit in hot standby, entering cold shutdown),
// so the mode must be drivable at runtime, not only through a plant
// JSON edit.
//
// Spec justification:
//   - Mode definitions and the cold/hot distinction:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
//   - Hot standby as an operator-placed condition:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:24-29
func addPlantOperationControls(p *Plant, m bridge.ControlMap) {
	m["PlantOperation/Mode"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "Mode",
		Kind:  "enum",
		Enum: []string{
			"ColdShutdown",
			"HotStandby",
			"PowerOperation",
		},
		Get: func() string {
			return fmt.Sprintf("%d", p.PlantOperation.Mode)
		},
		Set: func(s string) error {
			switch s {
			case "0":
				p.PlantOperation.Mode = ColdShutdown
			case "1":
				p.PlantOperation.Mode = HotStandby
			case "2":
				p.PlantOperation.Mode = PowerOperation
			default:
				return fmt.Errorf("invalid Mode %q (want 0..2)", s)
			}
			return nil
		},
	}
}

// GetSensorMap returns a bridge.SensorMap populated with sensors for the
// given plant. Each Get closure calls bridge.FormatSensorValue according to
// Kind.
func GetSensorMap(p *Plant) bridge.SensorMap {
	m := make(bridge.SensorMap)
	addReactorBuildingSensors(p, m)
	addTurbineBuildingSensors(p, m)
	addAuxiliaryBuildingSensors(p, m)
	addOutsideSensors(p, m)
	return m
}

// GetControlMap returns a populated ControlMap for this plant.
func (p *Plant) GetControlMap() bridge.ControlMap {
	return GetControlMap(p)
}

// GetSensorMap returns a populated SensorMap for this plant.
func (p *Plant) GetSensorMap() bridge.SensorMap {
	return GetSensorMap(p)
}
