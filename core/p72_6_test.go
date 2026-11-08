package core

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// p72_6 consolidated acceptance (plans/p72_6.txt): the slice ships
// no new physics and no new bridge entries (all sensors were
// published by p72_2-p72_5). What this file pins is the wiring
// contract: every bridge key resolves with correct values, steady
// state reads clean, the first-step suppression holds, and
// accumulated damage survives a JSON round-trip. RHR is excluded:
// p72_4 cut it (no modelled isolation state).

// p72_6 bridge keys, fully expanded: per-loop RCP keys iterate
// {A,B,C,D}, EFWS pump keys iterate {A,B} x {MotorPump,TurbinePump}.
// A table-row count is not a bridge-key count.
func p72_6BridgeKeys() []string {
	keys := []string{
		"ReactorBuilding/ReactorPressureVessel/CladdingFatigue",
		"ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm",
		"ReactorBuilding/ReactorPressureVessel/RodTravelSteps",
		"ReactorBuilding/SteamGenerator/FeedRingVoid",
		"ReactorBuilding/SteamGenerator/SleeveGap",
		"ReactorBuilding/SteamGenerator/HammerDamage",
		"ReactorBuilding/SteamGenerator/HammerAlarm",
		"ReactorBuilding/SafetyInjectionSystem/PumpHeatDamage",
		"ReactorBuilding/SafetyInjectionSystem/PumpOverheatAlarm",
		"TurbineBuilding/Turbine/BladeErosion",
		"ReactorBuilding/ContainmentBuilding/ThermalCycleCount",
		"ReactorBuilding/ContainmentBuilding/LinerFatigue",
		"ReactorBuilding/ContainmentBuilding/ChlorideSCCRisk",
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/CorrosionProductControlDegraded",
	}
	for _, loop := range []string{"A", "B", "C", "D"} {
		keys = append(keys,
			"ReactorBuilding/ReactorCoolantPumps/"+loop+"/SealHeatDamage",
			"ReactorBuilding/ReactorCoolantPumps/"+loop+"/SealOverheatAlarm",
		)
	}
	for _, sub := range []string{"A", "B"} {
		for _, pk := range []string{"MotorPump", "TurbinePump"} {
			keys = append(keys,
				"AuxiliaryBuilding/EmergencyFeedwaterSystem/Subsystem"+sub+"/"+pk+"/PumpHeatDamage",
				"AuxiliaryBuilding/EmergencyFeedwaterSystem/Subsystem"+sub+"/"+pk+"/PumpOverheatAlarm",
			)
		}
	}
	return keys
}

// Every slice sensor resolves via GetSensorMap and returns a
// non-empty value; fractions come back as percent (x100), not the
// raw fraction. Feeds each sensor via its bridge key against a
// known non-zero plant so percent-vs-raw is distinguishable.
func TestP72_6_BridgeKeysResolveWithCorrectValues(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.CladdingFatigueFrac = 0.42
	p.ReactorBuilding.ReactorPressureVessel.CladdingFatigueAlarm = true
	p.ReactorBuilding.ReactorPressureVessel.RodTravelSteps = 1234
	p.ReactorBuilding.SteamGenerator.FeedRingVoidFrac = 0.30
	p.ReactorBuilding.SteamGenerator.HammerDamageFrac = 0.25
	p.ReactorBuilding.SteamGenerator.HammerAlarm = true
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.SealHeatDamageFrac = 0.42
	p.ReactorBuilding.ReactorCoolantPumps.LoopB.SealHeatDamageFrac = 0.07
	p.ReactorBuilding.ReactorCoolantPumps.LoopC.SealOverheatAlarm = true
	p.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac = 0.13
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.PumpHeatDamageFrac = 0.29
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.PumpOverheatAlarm = true
	p.TurbineBuilding.Turbine.BladeErosionFrac = 0.55
	p.ReactorBuilding.ContainmentBuilding.ThermalCycleCount = 250
	p.ReactorBuilding.ContainmentBuilding.LinerFatigueFrac = 0.5
	p.AuxiliaryBuilding.ContainmentSpraySystem.ChlorideSCCRisk = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.CorrosionProductControlDegraded = true

	m := GetSensorMap(&p)
	for _, key := range p72_6BridgeKeys() {
		s, ok := m[key]
		require.Truef(t, ok, "bridge must expose %s", key)
		got, err := s.Get()
		require.NoErrorf(t, err, "Get %q", key)
		assert.NotEmptyf(t, got, "sensor %s must return a value", key)
	}
	// Percent keys (Kind "percent") come back x100; raw counts and
	// booleans come back unscaled.
	percentCases := []struct {
		key  string
		want float64
	}{
		{"ReactorBuilding/ReactorPressureVessel/CladdingFatigue", 42},
		{"ReactorBuilding/SteamGenerator/FeedRingVoid", 30},
		{"ReactorBuilding/SteamGenerator/HammerDamage", 25},
		{"ReactorBuilding/ReactorCoolantPumps/A/SealHeatDamage", 42},
		{"ReactorBuilding/ReactorCoolantPumps/B/SealHeatDamage", 7},
		{"ReactorBuilding/SafetyInjectionSystem/PumpHeatDamage", 13},
		{"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPump/PumpHeatDamage", 29},
		{"TurbineBuilding/Turbine/BladeErosion", 55},
		{"ReactorBuilding/ContainmentBuilding/LinerFatigue", 50},
	}
	for _, tc := range percentCases {
		got, err := m[tc.key].Get()
		require.NoError(t, err)
		gotF, err := strconv.ParseFloat(got, 64)
		require.NoErrorf(t, err, "Get %q numeric", tc.key)
		assert.InDeltaf(t, tc.want, gotF, 1e-9,
			"sensor %q must publish percent (x100), got %s", tc.key, got)
	}
	// Counters publish raw.
	rod, err := m["ReactorBuilding/ReactorPressureVessel/RodTravelSteps"].Get()
	require.NoError(t, err)
	assert.Equal(t, "1234", rod, "RodTravelSteps is a raw counter")
	tc, err := m["ReactorBuilding/ContainmentBuilding/ThermalCycleCount"].Get()
	require.NoError(t, err)
	assert.Equal(t, "250", tc, "ThermalCycleCount is a raw counter")
	// Booleans publish as 1/0.
	for _, key := range []string{
		"ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm",
		"ReactorBuilding/SteamGenerator/HammerAlarm",
		"ReactorBuilding/ReactorCoolantPumps/C/SealOverheatAlarm",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePump/PumpOverheatAlarm",
		"ReactorBuilding/ContainmentBuilding/ChlorideSCCRisk",
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/CorrosionProductControlDegraded",
	} {
		got, err := m[key].Get()
		require.NoError(t, err)
		assert.Equalf(t, "1", got, "sensor %q value", key)
	}
	// SleeveGap publishes inches (baseline 0.025 in from MakePlant).
	gap, err := m["ReactorBuilding/SteamGenerator/SleeveGap"].Get()
	require.NoError(t, err)
	assert.Contains(t, gap, "0.025", "gap must publish baseline inches, got %s", gap)
}

// Steady full-power operation long run: all damage accumulators at
// zero and kind-(a)/(b) alarm sensors read false (guards against
// forgot-the-latch implementations that pass on accumulation
// alone). Kind-(c) derived-risk sensors are exempt from the
// false-read requirement: CorrosionProductControlDegraded reads
// TRUE at steady state by definition (p72_5 — volume-control
// lineup with no BTRS); ChlorideSCCRisk reads false outside LOCA.
func TestP72_6_SteadyStateAllZeroAndAlarmsFalse(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	// Steady full-power lineup: charging pump running so the RCP
	// seal-injection witness reads OK (core/plant.go:503); without
	// it the seal-overheat alarm fires on every running RCP.
	// EFWS pumps and HHSI stay secured.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	stepN(&p, 2000, DefaultTimeStep)

	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.Zero(t, rpv.CladdingFatigueFrac, "steady state accumulates no cladding fatigue")
	assert.Zero(t, rpv.RodTravelSteps, "steady state accumulates no rod travel")
	assert.False(t, rpv.CladdingFatigueAlarm)

	sg := p.ReactorBuilding.SteamGenerator
	assert.Zero(t, sg.FeedRingVoidFrac)
	assert.Zero(t, sg.HammerDamageFrac)
	assert.False(t, sg.HammerAlarm)

	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.Zero(t, pu.SealHeatDamageFrac, "steady state accumulates no seal damage")
		assert.False(t, pu.SealOverheatAlarm)
	}
	assert.Zero(t, p.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac)
	assert.False(t, p.ReactorBuilding.SafetyInjectionSystem.PumpOverheatAlarm)
	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	for _, pk := range []*EFWSPump{
		&efws.SubsystemA.MotorPump, &efws.SubsystemA.TurbinePump,
		&efws.SubsystemB.MotorPump, &efws.SubsystemB.TurbinePump,
	} {
		assert.Zero(t, pk.PumpHeatDamageFrac)
		assert.False(t, pk.PumpOverheatAlarm)
	}

	// Blade erosion at full load is bounded by "negligible":
	// a known steady-state drift below 1e-6 (see the dry
	// full-load guard for the physical reason).
	assert.InDelta(t, 0.0, p.TurbineBuilding.Turbine.BladeErosionFrac, 1e-6,
		"full-load steady state must not erode blades")

	cb := p.ReactorBuilding.ContainmentBuilding
	assert.Zero(t, cb.ThermalCycleCount, "steady state counts no thermal cycles")
	assert.Zero(t, cb.LinerFatigueFrac)

	m := GetSensorMap(&p)
	for _, loop := range []string{"A", "B", "C", "D"} {
		got, err := m["ReactorBuilding/ReactorCoolantPumps/"+loop+"/SealOverheatAlarm"].Get()
		require.NoError(t, err)
		assert.Equalf(t, "0", got, "live alarm must read false at steady state")
	}
	for _, sub := range []string{"A", "B"} {
		for _, pk := range []string{"MotorPump", "TurbinePump"} {
			got, err := m["AuxiliaryBuilding/EmergencyFeedwaterSystem/Subsystem"+sub+"/"+pk+"/PumpOverheatAlarm"].Get()
			require.NoError(t, err)
			assert.Equalf(t, "0", got, "live alarm must read false at steady state")
		}
	}
	got, err := m["ReactorBuilding/SafetyInjectionSystem/PumpOverheatAlarm"].Get()
	require.NoError(t, err)
	assert.Equal(t, "0", got)
	got, err = m["ReactorBuilding/ReactorPressureVessel/CladdingFatigueAlarm"].Get()
	require.NoError(t, err)
	assert.Equal(t, "0", got)
	got, err = m["ReactorBuilding/SteamGenerator/HammerAlarm"].Get()
	require.NoError(t, err)
	assert.Equal(t, "0", got)
	// Kind-(c) derived-risk sensors are exempt from the false-read
	// requirement: CorrosionProductControlDegraded reads TRUE at
	// steady state by definition (p72_5 — volume-control lineup
	// with no BTRS in service); ChlorideSCCRisk reads false
	// outside LOCA.
	assert.True(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.CorrosionProductControlDegraded,
		"derived-risk: steady volume-control lineup has no BTRS")
	assert.False(t, p.AuxiliaryBuilding.ContainmentSpraySystem.ChlorideSCCRisk,
		"no LOCA means no SCC risk")
}

// First Step after MakePlant produces no cladding-fatigue and no
// containment-cycle increment (both detectors suppress the
// zero-value first delta); accumulation begins on the second step.
func TestP72_6_FirstStepSuppressed(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.Zero(t, rpv.CladdingFatigueFrac, "first step suppresses cladding fatigue")
	cb := p.ReactorBuilding.ContainmentBuilding
	assert.Zero(t, cb.ThermalCycleCount, "first step suppresses containment cycle")
	assert.Zero(t, cb.LinerFatigueFrac)
}

// JSON round-trip: a plant carrying non-zero accumulated damage on
// every mechanism survives serialize/deserialize bit-for-bit.
// Mechanism-level round-trips already exist (p pump_overheat
// test, feedring test); this extends coverage to every mechanism
// in one plant so no field can silently drop.
func TestP72_6_AccumulatedDamageRoundTripsThroughJSON(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.CladdingFatigueFrac = 0.42
	rpv.CladdingFatigueAlarm = true
	rpv.RodTravelSteps = 1234
	rpv.FatiguePeakW = 1e8
	rpv.FatigueTroughW = 9e7
	rpv.FatigueTrendSteps = 5000
	rpv.FatigueTrendStartW = 9.5e7
	rpv.PrevPowerSlope = 5e6
	rpv.PrevFissionPowerW = Watts(1e8)
	rpv.PrevInsertion = 0.3
	sg := &p.ReactorBuilding.SteamGenerator
	sg.FeedRingVoidFrac = 0.30
	sg.HammerDamageFrac = 0.25
	sg.Damage = 0.25
	sg.SleeveGapIn = 0.1
	sg.HammerAlarm = true
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.SealHeatDamageFrac = 0.42
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.Damage = 0.42
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.SealOverheatAlarm = true
	p.ReactorBuilding.ReactorCoolantPumps.LoopB.SealHeatDamageFrac = 0.07
	p.ReactorBuilding.ReactorCoolantPumps.LoopB.Damage = 0.07
	p.ReactorBuilding.ReactorCoolantPumps.LoopC.SealHeatDamageFrac = 0.33
	p.ReactorBuilding.ReactorCoolantPumps.LoopC.Damage = 0.33
	p.ReactorBuilding.ReactorCoolantPumps.LoopD.SealHeatDamageFrac = 0.11
	p.ReactorBuilding.ReactorCoolantPumps.LoopD.Damage = 0.11
	p.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac = 0.13
	p.ReactorBuilding.SafetyInjectionSystem.Damage = 0.13
	p.ReactorBuilding.SafetyInjectionSystem.PumpOverheatAlarm = true
	efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	efws.SubsystemA.MotorPump.PumpHeatDamageFrac = 0.29
	efws.SubsystemA.MotorPump.Damage = 0.29
	efws.SubsystemA.TurbinePump.PumpHeatDamageFrac = 0.18
	efws.SubsystemA.TurbinePump.Damage = 0.18
	efws.SubsystemB.MotorPump.PumpHeatDamageFrac = 0.05
	efws.SubsystemB.MotorPump.Damage = 0.05
	efws.SubsystemB.TurbinePump.PumpHeatDamageFrac = 0.09
	efws.SubsystemB.TurbinePump.Damage = 0.09
	efws.SubsystemB.TurbinePump.PumpOverheatAlarm = true
	p.TurbineBuilding.Turbine.BladeErosionFrac = 0.55
	p.TurbineBuilding.Turbine.Damage = 0.55
	cb := &p.ReactorBuilding.ContainmentBuilding
	cb.ThermalCycleCount = 250
	cb.LinerFatigueFrac = 0.5
	cb.PeakF = 160
	cb.TroughF = 120
	cb.TrendSteps = 5000
	cb.TrendStartF = 130
	cb.PrevTrendF = 30
	cb.TrendSeeded = true
	p.AuxiliaryBuilding.ContainmentSpraySystem.ChlorideSCCRisk = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.CorrosionProductControlDegraded = true
	p.nextPlant = nil
	b, err := json.Marshal(p)
	require.NoError(t, err)
	// Grep the serialized output for the damage keys that must
	// exist (CHECK_SKILL §1): byte-equality alone lets a lossy
	// cache round-trip invisibly; a missing key == a dropped
	// field.
	for _, key := range []string{
		`"CladdingFatigueFrac":0.42`,
		`"RodTravelSteps":1234`,
		`"FeedRingVoidFrac":0.3`,
		`"HammerDamageFrac":0.25`,
		`"SleeveGapIn":0.1`,
		`"BladeErosionFrac":0.55`,
		`"ThermalCycleCount":250`,
		`"LinerFatigueFrac":0.5`,
		`"TrendSeeded":true`,
		`"PeakF":160`,
		`"TroughF":120`,
		`"CladdingFatigueAlarm":true`,
		`"HammerAlarm":true`,
		`"ChlorideSCCRisk":true`,
		`"CorrosionProductControlDegraded":true`,
	} {
		assert.Containsf(t, string(b), key,
			"serialized plant must carry %s", key)
	}
	var p2 Plant
	require.NoError(t, json.Unmarshal(b, &p2))
	rpv2 := p2.ReactorBuilding.ReactorPressureVessel
	assert.InDelta(t, 0.42, rpv2.CladdingFatigueFrac, 1e-12)
	assert.True(t, rpv2.CladdingFatigueAlarm)
	assert.InDelta(t, 1234, rpv2.RodTravelSteps, 1e-12)
	assert.Equal(t, 5000, rpv2.FatigueTrendSteps)
	sg2 := p2.ReactorBuilding.SteamGenerator
	assert.InDelta(t, 0.30, sg2.FeedRingVoidFrac, 1e-12)
	assert.InDelta(t, 0.25, sg2.HammerDamageFrac, 1e-12)
	assert.InDelta(t, 0.1, sg2.SleeveGapIn, 1e-12)
	assert.True(t, sg2.HammerAlarm)
	assert.InDelta(t, 0.42, p2.ReactorBuilding.ReactorCoolantPumps.LoopA.SealHeatDamageFrac, 1e-12)
	assert.True(t, p2.ReactorBuilding.ReactorCoolantPumps.LoopA.SealOverheatAlarm)
	assert.InDelta(t, 0.13, p2.ReactorBuilding.SafetyInjectionSystem.PumpHeatDamageFrac, 1e-12)
	assert.InDelta(t, 0.29,
		p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.PumpHeatDamageFrac, 1e-12)
	assert.InDelta(t, 0.55, p2.TurbineBuilding.Turbine.BladeErosionFrac, 1e-12)
	cb2 := p2.ReactorBuilding.ContainmentBuilding
	assert.InDelta(t, 250, cb2.ThermalCycleCount, 1e-12)
	assert.InDelta(t, 0.5, cb2.LinerFatigueFrac, 1e-12)
	assert.True(t, cb2.TrendSeeded)
	assert.True(t, p2.AuxiliaryBuilding.ContainmentSpraySystem.ChlorideSCCRisk)
	assert.True(t, p2.AuxiliaryBuilding.ChemicalVolumeControlSystem.CorrosionProductControlDegraded)
	assert.Equal(t, p, p2, "every plant field must survive the round trip")
	// Re-marshaled plant is byte-identical.
	b2, err := json.Marshal(&p2)
	require.NoError(t, err)
	require.Equal(t, b, b2, "re-marshaled plant must be byte-identical")

	// Part.Damage mirror (CHECK_SKILL §5): every accumulator the
	// plan wires to ret.Damage=max(...) must round-trip through
	// Part.Damage itself, or a missing wire reads as invisible
	// damage to generic consumers.
	assert.InDelta(t, 0.25, sg2.Damage, 1e-12,
		"hammer damage must mirror Part.Damage")
	for _, pu := range p2.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.InDeltaf(t, pu.SealHeatDamageFrac, float64(pu.Damage), 1e-6,
			"seal damage must mirror Part.Damage")
	}
	assert.InDelta(t, 0.13, float64(p2.ReactorBuilding.SafetyInjectionSystem.Damage), 1e-6,
		"HHSI damage must mirror Part.Damage")
	for _, tk := range []*EFWSPump{
		&p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump,
		&p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump,
		&p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPump,
		&p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump,
	} {
		assert.InDeltaf(t, tk.PumpHeatDamageFrac, float64(tk.Damage), 1e-6,
			"aux-pump damage must mirror Part.Damage")
	}
	assert.InDelta(t, 0.55, float64(p2.TurbineBuilding.Turbine.Damage), 1e-6,
		"blade erosion must mirror Part.Damage")
	// Cladding fatigue is the deliberate exception: it never feeds
	// Part.Damage (mirrors CoreDamageFrac instead).
	assert.Zero(t, float64(rpv2.Damage),
		"cladding fatigue must never feed Part.Damage")
}

// Mid-window JSON reload with TrendSeeded lost: the containment
// detector must re-seed from the current temperature rather than
// count a phantom cycle from stale (or zeroed) peak/trough fields
// (CHECK_SKILL §5 — contradictory seeds resolve on first Step).
// Simulates the save/load path where the latch is cleared while
// its extrema are mid-window values.
func TestP72_6_ContainmentDetectorReseedsMidWindow(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	// Drive 10k steps into a window with warmed extrema.
	stepN(&p, 10000, DefaultTimeStep)
	cb := &p.ReactorBuilding.ContainmentBuilding
	require.True(t, cb.TrendSeeded, "10k steps must seed the detector")
	// Mid-window reload: TrendSeeded lost, extrema preserved.
	cb.TrendSeeded = false
	stepBefore := cb.ThermalCycleCount
	// One step at the current temperature: must re-seed, not count.
	p.Step(DefaultTimeStep)
	p.Update()
	cb2 := p.ReactorBuilding.ContainmentBuilding
	assert.Equal(t, stepBefore, cb2.ThermalCycleCount,
		"re-seed step must not count a phantom cycle")
	assert.True(t, cb2.TrendSeeded, "detector must re-seed")
	assert.InDelta(t, cb2.Temperature.Float(), cb2.PeakF, 1e-9,
		"re-seed peaks at current temperature")
	assert.InDelta(t, cb2.Temperature.Float(), cb2.TroughF, 1e-9,
		"re-seed troughs at current temperature")
}
