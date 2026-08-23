package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 6 (low-power ascension): the operational mode loaded from a
// start-state file must survive stepping. The step used to read the
// mode out of the zero-valued nextPlant copy, collapsing any loaded
// Mode to ColdShutdown on the first tick.
func TestScenario6_ModeSurvivesFirstStep(t *testing.T) {
	p := MakePlant()
	p.PlantOperation.Mode = PowerOperation

	stepFor(&p, 10*time.Millisecond)

	assert.Equal(t, PowerOperation, p.PlantOperation.Mode,
		"loaded operational mode must survive the first Step")
}

// The aux-to-main feedwater indicator gates on both the operational
// mode and the steam-flow fraction: it clears only in PowerOperation
// once per-SG steam flow reaches the AFW capacity threshold.
//
// "Before reaching the flow capacity of the auxiliary feedwater
// system, equivalent to about 3.5-percent steam flow, the steam
// generators feedwater supply is switched to the main feedwater
// system."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:45-48
func TestScenario6_AuxToMainFeedwaterGating(t *testing.T) {
	temp := Temperature{Val: 558, Unit: u.Fahrenheit}

	cases := []struct {
		name     string
		mode     PlantOperationalMode
		steamFrc float64
		want     bool
	}{
		{"hot standby keeps AFW in use", HotStandby, 1.0, true},
		{"power operation below threshold keeps AFW", PowerOperation, OpAuxToMainFeedwaterThreshold / 2, true},
		{"power operation at threshold switches to MFW", PowerOperation, OpAuxToMainFeedwaterThreshold, false},
	}
	for _, tc := range cases {
		po := PlantOperation{}
		got := po.Step(tc.mode, temp, 2250, 0, CoreRatedThermalPower,
			false, 1, tc.steamFrc, 0)
		assert.Equal(t, tc.want, got.AuxFeedwaterInUse, tc.name)
	}
}

// Wiring check (not independent physics): with the surfaced condensate
// and main-feedwater pump controls running, the CST -> drain tank ->
// SG graph path delivers inventory into the steam generator.
//
// "Conventional condensate and feedwater systems startup procedures
//  would be followed."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:175-177
func TestScenario6_MainFeedwaterControlsDeliverInventory(t *testing.T) {
	p := MakePlant()
	sg := &p.ReactorBuilding.SteamGenerator
	// Start below the 50-percent level program: the feedwater
	// regulator only opens the main feedwater control valve when
	// the level error asks for inventory (the valve gates the
	// CST -> drain tank -> SG graph edge), so a fixture parked
	// exactly on program would assert a no-op.
	sg.Water = Volume{Val: sg.Volume.Float() * 2 / 5, Unit: u.Gallon}
	start := sg.Water.Float()

	setControl(t, &p, "TurbineBuilding/CondensatePump/Running", "1")
	setControl(t, &p, "TurbineBuilding/MainFeedwaterPump/Running", "1")
	stepFor(&p, 5*time.Second)

	assert.Greater(t, sg.Water.Float(), start,
		"running main feedwater pumps must deliver SG inventory")
	require.True(t, p.TurbineBuilding.MainFeedwaterPump.Running)
}
