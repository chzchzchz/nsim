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
		"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/DriveReady",
		"ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning",
		"Outside/CirculatingWaterSystem/PumpRunning",
		"Outside/CirculatingWaterPump/DriveReady",
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

// Pressure-relief / steam-side / RHR isolation valves are
// spec-justified in core/bridge.go. These tests verify each
// entry exists and round-trips through Set/Get.
func TestBridge_ReliefAndIsolationValveSensors(t *testing.T) {
	p := MakePlant()
	m := GetSensorMap(&p)
	for _, id := range []string{
		"ReactorBuilding/PORV/Open",
		"ReactorBuilding/SafetyValve/Open",
		"ReactorBuilding/BlockValve/Open",
		"ReactorBuilding/PressurizedReliefTank/Level",
		"ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open",
		"ReactorBuilding/SteamGenerator/SafetyValve/Open",
		"ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open",
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow",
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownFlow",
		"ReactorBuilding/ReactorMakeupControlValve/Flow",
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion",
	} {
		s, ok := m[id]
		require.Truef(t, ok, "want sensor %q registered", id)
		out, err := s.Get()
		require.NoErrorf(t, err, "Get %q", id)
		assert.NotEmptyf(t, out, "want non-empty value from sensor %q", id)
	}
}

func TestBridge_ReliefAndIsolationValveControls(t *testing.T) {
	p := MakePlant()
	m := GetControlMap(&p)
	for _, id := range []string{
		"ReactorBuilding/PORV/Open",
		"ReactorBuilding/SafetyValve/Open",
		"ReactorBuilding/BlockValve/Open",
		"ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open",
		"ReactorBuilding/SteamGenerator/SafetyValve/Open",
		"ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/SuctionValve1/Open",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/SuctionValve2/Open",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/SuctionValve1/Open",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/SuctionValve2/Open",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/BypassValve/Open",
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/BypassValve/Open",
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
