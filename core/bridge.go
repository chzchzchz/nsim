package core

import (
	"github.com/chzchzchz/nsim/bridge"
)

// GetControlMap returns a bridge.ControlMap populated with controls for the
// given plant. Each closure captures p and dereferences live state on every
// call so the map survives Plant.Step() and Plant.Update() cycles without
// pinning stale nextPlant addresses.
func GetControlMap(p *Plant) bridge.ControlMap {
	m := make(bridge.ControlMap)
	addReactorBuildingControls(p, m)
	addTurbineBuildingControls(p, m)
	addAuxiliaryBuildingControls(p, m)
	addOutsideControls(p, m)
	return m
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
