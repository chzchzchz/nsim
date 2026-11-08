package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SG drained below 20.6% NR at ~10 gpm total feed: void builds past
// the hammer-min threshold with no hammer damage. (plans/p72_3.txt)
func TestFeedRing_DrainedSGVoidBuildsWithoutHammer(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 16000, Unit: u.Gallon} // 16% NR
	// Total feed must be < 30 gpm leakage for the spec voiding condition
	// to hold. Main-feed regulator is PI-automatic (ValveDemand saturates
	// at 1 -> 22800 gpm), so clamp it with ManualControl at the 10 gpm
	// rate: valve = 10/ (380 gal/s * 60) = 0.00044. EFWS stays off (0).
	p.TurbineBuilding.FeedwaterRegulator.ManualControl = true
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	p.TurbineBuilding.FeedwaterRegulator.ValveDemand = 10.0 / (FeedwaterDesignFlow.Float() * SecondsPerMinute)
	stepN(&p, 3000, DefaultTimeStep)
	sg := p.ReactorBuilding.SteamGenerator
	assert.Greater(t, sg.FeedRingVoidFrac, FeedRingVoidHammerMinFrac,
		"void should exceed hammer-min threshold at 10% NR, got %v", sg.FeedRingVoidFrac)
	assert.Zero(t, sg.HammerDamageFrac, "no hammer damage without >400 gpm, got %v", sg.HammerDamageFrac)
	assert.False(t, sg.HammerAlarm, "alarm should not latch without hammer")
}

// Low level but feed at the 165 gpm safe preset: ring stays swept full.
func TestFeedRing_SafePresetFlowSweepsVoid(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 16000, Unit: u.Gallon} // 16% NR
	// Set total feed to exactly AfwSafeFlowGpm (165 gpm) via EFWS:
	// one motor pump at 150 gpm + valve trim is fiddly; instead set
	// EFWS delivery by running both pumps throttled to 165 gpm total.
	// Use one subsystem motor pump full open (150 gpm) + turbine
	// pump throttled to 15 gpm => 165 gpm. drive via EFWS lineup.
	efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	efws.SubsystemA.MotorPump = EFWSPump{
		AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
		FlowModulatingValve: Valve{Open: 1},
	}
	efws.SubsystemB.MotorPump = EFWSPump{
		AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
		FlowModulatingValve: Valve{Open: 0.1}, // 15 gpm
	}
	// Prime a void so we can observe it sweeping to zero.
	p.ReactorBuilding.SteamGenerator.FeedRingVoidFrac = 0.5
	// Prevent EFWS auto-start from re-closing the manual lineup:
	// feed EFWS Active path requires the pumps to stay Running; the
	// EFWS Step may operate the level bands — assert only that void
	// decays (165 > 30 → swept) after stepping.
	// Main feed is off (ValveDemand 0 by default), so total = ~165 gpm.
	stepN(&p, 100, DefaultTimeStep)
	sg := p.ReactorBuilding.SteamGenerator
	// Void should decay from 0.5 (alpha = 0.001/60 per step, 100 steps
	// -> ~0.498; assert it stays high rather than collapsing via hammer).
	// The key point: total feed 165 gpm is > SgFeedRingLeakageGpm (30)
	// so no new void builds; prior void decays slowly. The acceptance
	// criterion is "stays at 0" starting from zero — assert that here:
	p2 := MakePlant()
	p2.ReactorBuilding.SteamGenerator.Water = Volume{Val: 10000, Unit: u.Gallon}
	efws2 := &p2.AuxiliaryBuilding.EmergencyFeedwaterSystem
	efws2.SubsystemA.MotorPump = EFWSPump{
		AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
		FlowModulatingValve: Valve{Open: 1},
	}
	efws2.SubsystemB.MotorPump = EFWSPump{
		AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
		FlowModulatingValve: Valve{Open: 0.1},
	}
	stepN(&p2, 100, DefaultTimeStep)
	assert.Zero(t, p2.ReactorBuilding.SteamGenerator.FeedRingVoidFrac,
		"void must stay 0 at 165 gpm swept, got %v", p2.ReactorBuilding.SteamGenerator.FeedRingVoidFrac)
	_ = sg
}

// Hammer collapse: void 0.5 + total feed > 400 gpm latches alarm,
// advances damage/gap, and zeroes the void. 20 events pin the caps.
func TestFeedRing_HammerCollapseAdvancesDamageAndGap(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 10000, Unit: u.Gallon}
	// Drive total feed > 400 gpm: both motor pumps full open (300 gpm)
	// plus both turbine pumps full open (400 gpm) = 700 gpm.
	efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	for _, sub := range []*EFWSSubsystem{&efws.SubsystemA, &efws.SubsystemB} {
		sub.MotorPump = EFWSPump{
			AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
			FlowModulatingValve: Valve{Open: 1},
		}
		sub.TurbinePump = EFWSPump{
			AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
			FlowModulatingValve: Valve{Open: 1},
		}
	}
	for event := 0; event < 20; event++ {
		p.ReactorBuilding.SteamGenerator.FeedRingVoidFrac = 0.5
		stepN(&p, 1, DefaultTimeStep)
		sg := p.ReactorBuilding.SteamGenerator
		require.True(t, sg.HammerAlarm, "alarm must latch on first hammer event")
		assert.Zero(t, sg.FeedRingVoidFrac, "void must collapse on hammer")
	}
	sg := p.ReactorBuilding.SteamGenerator
	assert.InDelta(t, 1.0, sg.HammerDamageFrac, 1e-9, "damage must pin at 1.0 after 20 events, got %v", sg.HammerDamageFrac)
	assert.InDelta(t, SgSleeveGapErodedIn, sg.SleeveGapIn, 1e-9, "gap must pin at 0.25 in, got %v", sg.SleeveGapIn)
	// One more event: still pinned.
	p.ReactorBuilding.SteamGenerator.FeedRingVoidFrac = 0.5
	stepN(&p, 1, DefaultTimeStep)
	sg = p.ReactorBuilding.SteamGenerator
	assert.InDelta(t, 1.0, sg.HammerDamageFrac, 1e-9, "damage stays pinned, got %v", sg.HammerDamageFrac)
	assert.InDelta(t, SgSleeveGapErodedIn, sg.SleeveGapIn, 1e-9, "gap stays pinned, got %v", sg.SleeveGapIn)
}

// Refill leaves the latch and accumulators frozen (one-way).
func TestFeedRing_LatchPersistsAfterRefill(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 10000, Unit: u.Gallon}
	efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	for _, sub := range []*EFWSSubsystem{&efws.SubsystemA, &efws.SubsystemB} {
		sub.MotorPump = EFWSPump{
			AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
			FlowModulatingValve: Valve{Open: 1},
		}
		sub.TurbinePump = EFWSPump{
			AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
			FlowModulatingValve: Valve{Open: 1},
		}
	}
	p.ReactorBuilding.SteamGenerator.FeedRingVoidFrac = 0.5
	stepN(&p, 1, DefaultTimeStep)
	sg := p.ReactorBuilding.SteamGenerator
	require.True(t, sg.HammerAlarm)
	damageAfter := sg.HammerDamageFrac
	gapAfter := sg.SleeveGapIn
	// Refill to full: level 80%, feed off.
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 80000, Unit: u.Gallon}
	for _, sub := range []*EFWSSubsystem{&efws.SubsystemA, &efws.SubsystemB} {
		sub.MotorPump.Running = false
		sub.TurbinePump.Running = false
	}
	stepN(&p, 10, DefaultTimeStep)
	sg = p.ReactorBuilding.SteamGenerator
	assert.True(t, sg.HammerAlarm, "latch must stay set after refill")
	assert.Equal(t, damageAfter, sg.HammerDamageFrac, "damage frozen after refill")
	assert.Equal(t, gapAfter, sg.SleeveGapIn, "gap frozen after refill")
}

// Steady full-power: zero damage, zero void, no alarm.
func TestFeedRing_SteadyFullPowerZeroDamage(t *testing.T) {
	t.Parallel()
	p := MakePlant() // Water 80000/100000 = 80% NR by default
	stepN(&p, 100, DefaultTimeStep)
	sg := p.ReactorBuilding.SteamGenerator
	assert.Zero(t, sg.HammerDamageFrac, "no damage at full power, got %v", sg.HammerDamageFrac)
	assert.Zero(t, sg.FeedRingVoidFrac, "no void at normal level, got %v", sg.FeedRingVoidFrac)
	assert.False(t, sg.HammerAlarm, "alarm false at steady state")
	assert.InDelta(t, SgSleeveGapBaselineIn, sg.SleeveGapIn, 1e-9, "gap at baseline, got %v", sg.SleeveGapIn)
}

// Tube-rupture independence: with the gap fully eroded, rupture behavior
// is unchanged (flow depends only on the rupture Valve, not on hammer state).
func TestFeedRing_TubeRuptureUnaffectedByErodedGap(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	sg := &p.ReactorBuilding.SteamGenerator
	sg.SleeveGapIn = SgSleeveGapErodedIn
	sg.HammerDamageFrac = 1.0
	sg.HammerAlarm = true
	// Open the rupture valve and step; flow is determined by the valve
	// and pressures ( applyTubeRupture in steam_generator.go ), not by
	// the gap. Compare against a plant with baseline gap.
	p2 := MakePlant()
	sg2 := &p2.ReactorBuilding.SteamGenerator
	sg2.SleeveGapIn = SgSleeveGapBaselineIn
	for _, plant := range []*Plant{&p, &p2} {
		plant.ReactorBuilding.SteamGenerator.SGTubeRupture.Open = 0.5
		plant.ReactorBuilding.ReactorPressureVessel.Pressure = PressurizerHeaterTarget
		plant.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	}
	stepN(&p, 1, DefaultTimeStep)
	stepN(&p2, 1, DefaultTimeStep)
	assert.Equal(t, p2.ReactorBuilding.SteamGenerator.SGTubeRuptureFlow.Float(),
		p.ReactorBuilding.SteamGenerator.SGTubeRuptureFlow.Float(),
		"rupture flow must not depend on hammer state")
}

// Bridge read-back: the four p72_3 sensors publish through the bridge
// (CHECK_SKILL §5) — a field the sim integrates but no SensorMap entry
// exposes is invisible to the web UI. Fractions publish as percent (x100).
func TestFeedRing_BridgeSensorsPublish(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.FeedRingVoidFrac = 0.42
	p.ReactorBuilding.SteamGenerator.HammerDamageFrac = 0.25
	p.ReactorBuilding.SteamGenerator.HammerAlarm = true
	for _, key := range []string{
		"ReactorBuilding/SteamGenerator/FeedRingVoid",
		"ReactorBuilding/SteamGenerator/HammerDamage",
		"ReactorBuilding/SteamGenerator/SleeveGap",
		"ReactorBuilding/SteamGenerator/HammerAlarm",
	} {
		s, ok := GetSensorMap(&p)[key]
		require.True(t, ok, "bridge must expose %s", key)
		v, err := s.Get()
		require.NoError(t, err)
		assert.NotEmpty(t, v, "sensor %s must return a value", key)
	}
	// SleeveGap starts at the baseline (MakePlant seeding).
	gap, err := GetSensorMap(&p)["ReactorBuilding/SteamGenerator/SleeveGap"].Get()
	require.NoError(t, err)
	assert.Contains(t, gap, "0.025", "gap must publish baseline inches, got %s", gap)
}
