package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scenario catalog feeds the web selection screen, so its
// identifiers must be unique and the numbering must track
// plans/x/scenarios.txt items 1-29 in order.
func TestScenarioCatalogIdentifiers(t *testing.T) {
	seen := make(map[string]bool)
	for i, s := range Scenarios() {
		require.False(t, seen[s.ID], "duplicate scenario id %q", s.ID)
		seen[s.ID] = true
		assert.Equal(t, i+1, s.Number, "catalog order must match item numbers")
		assert.NotEmpty(t, s.Title)
		assert.NotEmpty(t, s.Category)
		assert.NotEmpty(t, s.StartState)
		assert.NotEmpty(t, s.Objective)
	}
	assert.Len(t, Scenarios(), 29)

	s, ok := ScenarioByID("porv-stuck-open")
	require.True(t, ok)
	assert.Equal(t, 26, s.Number)

	_, ok = ScenarioByID("no-such-scenario")
	assert.False(t, ok)
}

// Every scenario override must land the plant in a state that steps
// cleanly from time zero; a short run catches wiring mistakes such as
// latches that instantly trip or panic on zero-valued fields.
func TestScenarioOverridesStepCleanly(t *testing.T) {
	for _, s := range Scenarios() {
		t.Run(s.ID, func(t *testing.T) {
			p := MakePlant()
			require.NotPanics(t, func() { s.Apply(&p) })
			stepN(&p, 200)
		})
	}
}

// Wiring checks: the full-power lineup must land synchronized from
// tick zero instead of relying on the first Step to reconcile
// contradictory seeds.
func TestFullPowerScenarioLineupIsSynchronized(t *testing.T) {
	p := MakePlant()
	Scenarios()[7].Apply(&p)

	assert.Equal(t, PowerOperation, p.PlantOperation.Mode)
	assert.True(t, p.ReactorBuilding.ReactorCoolantPumps.AnyRunning())
	assert.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State)
	assert.True(t, p.TurbineBuilding.Generator.BreakerClosed)
}

// Wiring check on the shared hot-standby fixture: source-level power,
// inserted rods, water-solid inventory, SI blocked per its citation.
func TestColdScenarioLineupStaysSubcriticalAndSolid(t *testing.T) {
	p := MakePlant()
	Scenarios()[0].Apply(&p)

	assert.Equal(t, CoreNeutronSourcePower,
		p.ReactorBuilding.ReactorPressureVessel.FissionPower)
	assert.Equal(t, 1.0, p.ReactorBuilding.ReactorPressureVessel.Insertion)
	assert.LessOrEqual(t, p.ReactorBuilding.Pressurizer.SteamBubble.Float(), 0.0,
		"pre-critical lineup opens water solid")
	assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionBlocked)
}

// Wiring check: the post-trip lineup must hold its latches through
// steps, not just at construction (RPS re-drives RPV.Trip).
func TestPostTripScenarioLineupHoldsTripState(t *testing.T) {
	p := MakePlant()
	Scenarios()[15].Apply(&p)
	stepN(&p, 100)

	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.True(t, rps.ReactorTripped)
	assert.Equal(t, 1.0, p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion)
	assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State)
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed)
}

// Wiring checks: each malfunction catalog entry schedules its trigger
// through a registered control at the shared pacing delay, so the
// transient starts from the healthy lineup instead of time zero.
func TestMalfunctionScenariosScheduleTheirTrigger(t *testing.T) {
	cases := []struct {
		id    string
		key   string
		value string
	}{
		{"loss-of-offsite-power", "Electrical/OffsiteSourceA/BreakerClosed", "0"},
		{"loss-of-feedwater", "TurbineBuilding/MainFeedwaterPump/Running", "0"},
		{"rcp-trip", "ReactorBuilding/ReactorCoolantPumps/A/Running", "0"},
		{"steam-line-break", "ReactorBuilding/SteamGenerator/MainSteamLineBreak/Open", "1"},
		{"porv-stuck-open", "ReactorBuilding/Pressurizer/PORVStuckOpen", "1"},
		{"dropped-rod-bank", "ReactorBuilding/ReactorPressureVessel/ControlRods/DroppedBank", "1"},
		{"si-recirculation", "ReactorBuilding/ReactorProtectionSystem/ManualSI", "1"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			s, ok := ScenarioByID(tc.id)
			require.True(t, ok)
			p := MakePlant()
			s.Apply(&p)
			cm := GetControlMap(&p)

			require.NotEmpty(t, s.Script, "no trigger scripted")
			found := false
			for _, cmd := range s.Script {
				if cmd.ControlKey == tc.key {
					found = true
					assert.Equal(t, tc.value, cmd.Value)
					assert.Equal(t, int64(scenarioTriggerDelayMs), cmd.AtMs)
					require.Contains(t, cm, cmd.ControlKey,
						"scripted control must be registered")
				}
			}
			assert.True(t, found, "trigger %q not scripted", tc.key)
		})
	}
}

// ApplyScript consumes exactly the commands whose offset has been
// reached and leaves the rest pending.
func TestApplyScriptFiresOnlyDueCommands(t *testing.T) {
	script := []ScriptCommand{
		{AtMs: 1000, ControlKey: "PlantOperation/RodControlAuto", Value: "true"},
		{AtMs: 5000, ControlKey: "PlantOperation/RodControlAuto", Value: "false"},
	}
	p := MakePlant()
	cm := GetControlMap(&p)

	ApplyScript(&script, 999*int64(time.Millisecond), cm)
	assert.Len(t, script, 2, "nothing due yet")

	ApplyScript(&script, 1000*int64(time.Millisecond), cm)
	assert.Len(t, script, 1, "only the due command fires")
	assert.True(t, p.PlantOperation.RodControlAuto)

	ApplyScript(&script, 5000*int64(time.Millisecond), cm)
	assert.Empty(t, script)
	assert.False(t, p.PlantOperation.RodControlAuto)
}

func TestRefuelingScenarioStartsCavityFlood(t *testing.T) {
	p := MakePlant()
	Scenarios()[21].Apply(&p)

	refuel := p.ReactorBuilding.RefuelingOperations
	assert.Equal(t, RefuelingPhasePreparation, refuel.Phase)
	assert.True(t, refuel.CavityFlooding)
}

// Every catalog entry must carry end-state bounds, and no evolution
// may start with its objective already satisfied.
func TestScenarioChecksCoverCatalogAndStartUnmet(t *testing.T) {
	catalog := Scenarios()
	require.Len(t, scenarioChecks, len(catalog),
		"check table must cover the catalog exactly")
	for _, s := range catalog {
		t.Run(s.ID, func(t *testing.T) {
			_, registered := scenarioChecks[s.ID]
			require.True(t, registered, "no check registered for %q", s.ID)
			p := MakePlant()
			s.Apply(&p)
			res := s.Check(&p)
			assert.False(t, res.Done,
				"%q must not start complete", s.ID)
			assert.NotEmpty(t, res.Detail,
				"pending state must name the unmet bounds")
		})
	}
}

// Representative satisfaction cases across the lineup families: each
// seeds the checked end state directly and expects Done.
func TestScenarioChecksFlipDoneAtTheirEndStates(t *testing.T) {
	cases := []struct {
		id    string
		reach func(*Plant)
	}{
		{"rcs-pressurization", func(p *Plant) {
			pzr := &p.ReactorBuilding.Pressurizer
			pzr.Water = Volume{Val: 600, Unit: u.Gallon}
			pzr.SteamBubble = Volume{Val: 900, Unit: u.Gallon}
			pzr.Pressure = 440
			pzr.HeatersOn = true
		}},
		{"grid-synchronization", func(p *Plant) {
			p.TurbineBuilding.Generator.BreakerClosed = true
			p.TurbineBuilding.Generator.RealPower = 1e6
			p.TurbineBuilding.Turbine.State = TurbineRunning
			p.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
		}},
		{"coordinated-cooldown", func(p *Plant) {
			p.ReactorBuilding.ReactorPressureVessel.Outlet =
				Temperature{Val: RHRSTempThreshold - 10, Unit: u.Fahrenheit}
			p.ReactorBuilding.ReactorPressureVessel.Pressure = 300
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.HasTakenOver = true
		}},
		{"containment-spray", func(p *Plant) {
			css := &p.AuxiliaryBuilding.ContainmentSpraySystem
			css.CSSTLevel = Volume{Val: css.CSSTLowLevelSetpoint.Float() - 1,
				Unit: u.Gallon}
			css.SuctionSump = true
			css.Running = true
		}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			s, ok := ScenarioByID(tc.id)
			require.True(t, ok)
			p := MakePlant()
			s.Apply(&p)
			tc.reach(&p)
			res := s.Check(&p)
			assert.True(t, res.Done,
				"end state must satisfy the bounds: %s", res.Detail)
		})
	}
}
