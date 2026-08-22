package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The bridge exposes one sensor + one control for every pump whose
// running state is spec-justified. These tests verify each entry
// exists, returns the underlying plant state, and the control round-
// trips through Set back into the field.
func TestBridge_PumpSensorsExist(t *testing.T) {
	p := MakePlant()
	m := GetSensorMap(&p)
	want := []string{
		// spec justifications are recorded next to each bridge
		// registration in core/bridge.go.
		"AuxiliaryBuilding/ContainmentSpraySystem/PumpRunning",
		"AuxiliaryBuilding/ComponentCoolingWaterSystem/PumpRunning",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/PumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePumpRunning",
		"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PumpRunning",
		"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/StandbyRunning",
		"ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning",
		"Outside/CirculatingWaterSystem/PumpRunning",
	}
	for _, id := range want {
		s, ok := m[id]
		require.Truef(t, ok, "want sensor %q registered", id)
		out, err := s.Get()
		require.NoErrorf(t, err, "Get %q", id)
		assert.NotEmptyf(t, out, "want non-empty value from sensor %q", id)
	}
}

// Operator controls for the same pumps must round-trip: Set writes
// the field, Get returns it.
func TestBridge_PumpControlsRoundTrip(t *testing.T) {
	p := MakePlant()
	m := GetControlMap(&p)
	for _, id := range []string{
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/PumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePumpRunning",
		"ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning",
		"Outside/CirculatingWaterSystem/PumpRunning",
	} {
		c, ok := m[id]
		require.Truef(t, ok, "want control %q registered", id)
		require.NoErrorf(t, c.Set("1"), "Set %q=1", id)
		assert.Equalf(t, "1", c.Get(),
			"want Get %q == 1 after Set, got %q", id, c.Get())
		require.NoErrorf(t, c.Set("0"), "Set %q=0", id)
		assert.Equalf(t, "0", c.Get(),
			"want Get %q == 0 after Set, got %q", id, c.Get())
	}
}
