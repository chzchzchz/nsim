package core

import (
	"encoding/json"
	"strconv"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RCP running with seal injection secured: live alarm + accumulation;
// restoring injection clears the alarm but damage is retained.
// Wiring check (CHECK_SKILL.md §8): the exact InDelta before: values
// recompute from production's own derived constants, so they pin the
// accumulation math, not the physical rate.
func TestPumpOverheat_RCPSealLiveAlarm(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.DriveReady = true
	}
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false

	stepN(&p, 1000, DefaultTimeStep) // 1 s
	pu := p.ReactorBuilding.ReactorCoolantPumps.All()[0]
	assert.True(t, pu.SealOverheatAlarm, "alarm live while injection secured")
	assert.InDelta(t, SealHeatRatePerSec*1.0, pu.SealHeatDamageFrac, 1e-9,
		"one second of overheat: %v", pu.SealHeatDamageFrac)
	assert.Equal(t, float32(pu.SealHeatDamageFrac), pu.Damage)

	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	stepN(&p, 100, DefaultTimeStep)
	pu = p.ReactorBuilding.ReactorCoolantPumps.All()[0]
	assert.False(t, pu.SealOverheatAlarm, "alarm clears when injection restored")
	assert.InDelta(t, SealHeatRatePerSec*1.0, pu.SealHeatDamageFrac, 1e-9,
		"damage retained after alarm clears: %v", pu.SealHeatDamageFrac)
}

// No drive, no rotation, no accumulation even with injection secured.
// Plant.Step latches DecideReady from the ESF bus, so seed the buses
// de-energised via the plant-level drive path (same mechanism the
// RCP step reads).
func TestPumpOverheat_RCPSealNoDriveNoDamage(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	// Force plantDriveReady false: loss of offsite power, so
	// mirrorOffsiteAvailability (core/plant.go:2741-2750) drives
	// both offsite source voltages to zero before Electrical.Step.
	p.ReactorBuilding.PlantElectricalSystems.OffsitePowerAvailable = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	stepN(&p, 100, DefaultTimeStep)
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.False(t, pu.DriveReady, "precondition: no drive")
		assert.False(t, pu.SealOverheatAlarm, "no alarm without rotation")
		assert.Zero(t, pu.SealHeatDamageFrac, "no damage without rotation")
	}
}

// EFWS pump running with the orificed recirculation line isolated:
// live alarm + accumulation; restoring the line clears the alarm,
// damage retained. Normal lineup accumulates nothing.
// Wiring check (CHECK_SKILL.md §8): exact InDelta recomputes from
// production's own derived constant.
func TestPumpOverheat_EFWSDeadHead(t *testing.T) {
	t.Parallel()
	e := MakePlant().AuxiliaryBuilding.EmergencyFeedwaterSystem
	hot := Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}
	e.SubsystemA.MotorPump.Running = true
	e.SubsystemA.MotorPump.DriveReady = true
	e.SubsystemA.MotorPump.RecirculationValve.Open = 0
	e.SubsystemA.MotorPump.FlowModulatingValve.Open = 0
	e.SubsystemA.Tank.Level = Volume{Val: 100000, Unit: u.Gallon} // wet tank so drain path stays quiet
	e.SubsystemB.Tank.Level = Volume{Val: 100000, Unit: u.Gallon}

	ret := e.Step(DefaultTimeStep, hot, false, nil)
	mp := ret.SubsystemA.MotorPump
	assert.True(t, mp.PumpOverheatAlarm, "alarm live while recirculation isolated")
	assert.InDelta(t, DeadHeadHeatRatePerSec*DefaultTimeStep.Seconds(), mp.PumpHeatDamageFrac, 1e-12)
	assert.Equal(t, float32(mp.PumpHeatDamageFrac), mp.Damage)

	// Other pumps untouched.
	assert.Zero(t, ret.SubsystemB.MotorPump.PumpHeatDamageFrac)

	// Normal lineup: recirculation open.
	e2 := MakePlant().AuxiliaryBuilding.EmergencyFeedwaterSystem
	e2.SubsystemA.MotorPump.Running = true
	e2.SubsystemA.MotorPump.DriveReady = true
	e2.SubsystemA.MotorPump.RecirculationValve.Open = 1
	e2.SubsystemA.MotorPump.FlowModulatingValve.Open = 0
	e2.SubsystemA.Tank.Level = Volume{Val: 100000, Unit: u.Gallon}
	e2.SubsystemB.Tank.Level = Volume{Val: 100000, Unit: u.Gallon}
	ret2 := e2.Step(DefaultTimeStep, hot, false, nil)
	assert.False(t, ret2.SubsystemA.MotorPump.PumpOverheatAlarm)
	assert.Zero(t, ret2.SubsystemA.MotorPump.PumpHeatDamageFrac)

	// Restore line: alarm clears, damage retained.
	e3 := e
	e3.SubsystemA.MotorPump.PumpHeatDamageFrac = 0.1
	e3.SubsystemA.MotorPump.RecirculationValve.Open = 1
	ret3 := e3.Step(DefaultTimeStep, hot, false, nil)
	assert.False(t, ret3.SubsystemA.MotorPump.PumpOverheatAlarm)
	assert.InDelta(t, 0.1, ret3.SubsystemA.MotorPump.PumpHeatDamageFrac, 1e-12,
		"damage retained: %v", ret3.SubsystemA.MotorPump.PumpHeatDamageFrac)
}

// HHSI running with miniflow isolated at switchover: live alarm +
// accumulation. Normal miniflow operation accumulates nothing.
// Wiring check (CHECK_SKILL.md §8): exact InDelta recomputes from
// production's own derived constant.
func TestPumpOverheat_HHSIDeadHead(t *testing.T) {
	t.Parallel()
	s := MakePlant().ReactorBuilding.SafetyInjectionSystem
	s.HHSIPumpRunning = true
	s.HHSIDriveReady = true
	s.SwitchoverComplete = true
	// Keep suction/rwst scenarios out of the step's inventory path.
	s.RWSTLevel = Volume{Val: 50000, Unit: u.Gallon}
	ret := s.Step(DefaultTimeStep, 3000)
	assert.True(t, ret.PumpOverheatAlarm, "alarm live post-switchover")
	assert.InDelta(t, DeadHeadHeatRatePerSec*DefaultTimeStep.Seconds(), ret.PumpHeatDamageFrac, 1e-12)
	assert.Equal(t, float32(ret.PumpHeatDamageFrac), ret.Damage)

	s2 := MakePlant().ReactorBuilding.SafetyInjectionSystem
	s2.HHSIPumpRunning = true
	s2.HHSIDriveReady = true
	// SwitchoverComplete false: miniflow still flowing.
	ret2 := s2.Step(DefaultTimeStep, 3000)
	assert.False(t, ret2.PumpOverheatAlarm, "normal miniflow is not dead-heading")
	assert.Zero(t, ret2.PumpHeatDamageFrac)
}

// Steady operation with injection flowing and miniflow lineups normal:
// zero seal/pump damage, alarms false. First Step is safe too.
// MakePlant seeds RCPs at rated speed (core/plant.go:2806-2810), so
// "steady" must explicitly line up seal injection.
func TestPumpOverheat_SteadyZeroDamage(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	stepN(&p, 10, DefaultTimeStep)
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.False(t, pu.SealOverheatAlarm)
		assert.Zero(t, pu.SealHeatDamageFrac)
	}
	assert.False(t, p.ReactorBuilding.SafetyInjectionSystem.PumpOverheatAlarm)
	assert.Zero(t, p.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac)

	// First Step after MakePlant.
	p2 := MakePlant()
	stepN(&p2, 1, DefaultTimeStep)
	for _, pu := range p2.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.Zero(t, pu.SealHeatDamageFrac)
	}
}

// Bridge read-back (CHECK_SKILL.md §5): every new stored field has a
// registered sensor whose Get returns the live value.
func TestPumpOverheat_BridgeSensorsPublish(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.SealHeatDamageFrac = 0.42
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.SealOverheatAlarm = true
	p.ReactorBuilding.ReactorCoolantPumps.LoopB.SealHeatDamageFrac = 0.07
	p.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac = 0.13
	p.ReactorBuilding.SafetyInjectionSystem.PumpOverheatAlarm = true
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.PumpHeatDamageFrac = 0.29
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.PumpOverheatAlarm = true

	m := GetSensorMap(&p)
	// Bridge "bool" kind formats as "1"/"0" (bridge/convert.go:90-98);
	// "percent" is the raw fraction ×100 with float formatting.
	cases := []struct {
		key  string
		want string
	}{
		{"ReactorBuilding/ReactorCoolantPumps/A/SealHeatDamage", "42"},
		{"ReactorBuilding/ReactorCoolantPumps/A/SealOverheatAlarm", "1"},
		{"ReactorBuilding/ReactorCoolantPumps/B/SealHeatDamage", "7"},
		{"ReactorBuilding/SafetyInjectionSystem/PumpHeatDamage", "13"},
		{"ReactorBuilding/SafetyInjectionSystem/PumpOverheatAlarm", "1"},
		{"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPump/PumpHeatDamage", "29"},
		{"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePump/PumpOverheatAlarm", "1"},
	}
	for _, tc := range cases {
		s, ok := m[tc.key]
		require.Truef(t, ok, "sensor %q registered", tc.key)
		got, err := s.Get()
		require.NoErrorf(t, err, "Get %q", tc.key)
		if s.Kind == "bool" {
			assert.Equalf(t, tc.want, got, "sensor %q value", tc.key)
		} else {
			// Percent values are raw float formatting; compare numerically
			// so float representation (e.g. 28.999999999999996) does not pin.
			wantF, err := strconv.ParseFloat(tc.want, 64)
			require.NoError(t, err)
			gotF, err := strconv.ParseFloat(got, 64)
			require.NoErrorf(t, err, "Get %q numeric", tc.key)
			assert.InDeltaf(t, wantF, gotF, 1e-9, "sensor %q value", tc.key)
		}
	}
}

// JSON round-trip (CHECK_SKILL.md §1): accumulated damage must survive
// serialization. Deep-compares the whole plant (condenser_test.go
// pattern) so no field can silently drop.
func TestPumpOverheat_DamageRoundTripsThroughJSON(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.LoopC.SealHeatDamageFrac = 0.33
	p.ReactorBuilding.ReactorCoolantPumps.LoopC.SealOverheatAlarm = true
	p.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac = 0.21
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.PumpHeatDamageFrac = 0.18
	p.nextPlant = nil
	b, err := json.Marshal(p)
	require.NoError(t, err)
	var p2 Plant
	require.NoError(t, json.Unmarshal(b, &p2))
	assert.InDelta(t, 0.33, p2.ReactorBuilding.ReactorCoolantPumps.LoopC.SealHeatDamageFrac, 1e-12)
	assert.True(t, p2.ReactorBuilding.ReactorCoolantPumps.LoopC.SealOverheatAlarm)
	assert.InDelta(t, 0.21, p2.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac, 1e-12)
	assert.InDelta(t, 0.18, p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.PumpHeatDamageFrac, 1e-12)
	assert.Equal(t, p, p2, "every plant field must survive the round trip")
}

// RHR is cut in p72_4: steady RHR operation contributes no damage.
func TestPumpOverheat_RHRCutGuard(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	for _, tr := range []*RHRSTrain{&p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA, &p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB} {
		tr.Pump.Running = true
		tr.DriveReady = true
	}
	damageBefore := p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Damage
	stepN(&p, 10, DefaultTimeStep)
	assert.Equal(t, damageBefore, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Damage,
		"no RHR accumulator exists in p72_4")
	require.Zero(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Damage)
}
