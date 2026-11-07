package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 2 (RCS pressurization and steam bubble formation) walks a
// water-solid cold pressurizer up to the no-load pressure and drains it
// to the no-load level with the pressure held:
//
//   - "All pressurizer heaters are energized to start the heatup of the
//     pressurizer necessary to form the steam bubble."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-17
//   - "Flow in the letdown line is increased by the operator to lower
//     pressurizer level to the normal range of 20 to 80 percent in
//     preparation for the plant heatup."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:20-23
//   - "RCS pressure ... is maintained by adjustment of the pressurizer
//     heaters and/or pressurizer spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:28-29
//   - No-load level 25 percent full:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:62-67
//   - Cold condition below 140 F, system held by charging/letdown:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-51,
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:94-97

func scenario2Plant() *Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Trip = true
	rpv.ControlRods.Insertion = 1
	rpv.ControlRods.DemandInsertion = 1
	rpv.FissionPower = 0
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0
	rpv.Inlet = Temperature{Val: 135, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 135, Unit: u.Fahrenheit}

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pzr.Water = Volume{Val: 1500, Unit: u.Gallon}
	pzr.SteamBubble = Volume{}
	pzr.Pressure = PressurizerBasePressure
	pzr.HeatersOn = false

	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.AutoLevelControl = false
	cvcs.ChargingPumpRunning = true
	cvcs.LetdownValveOpen = false
	return &p
}

func heatToTarget(t *testing.T, p *Plant) {
	t.Helper()
	setControl(t, p, "ReactorBuilding/Pressurizer/HeatersOn", "1")
	for i := 0; i < int(20*time.Second/time.Millisecond); i++ {
		stepN(p, 1, DefaultTimeStep)
		if float64(p.ReactorBuilding.Pressurizer.Pressure) >=
			float64(PressurizerHeaterTarget) {
			return
		}
	}
	require.FailNow(t, "pressurizer never reached the heater target")
}

// Energizing all heaters on a water-solid cold pressurizer forms the
// steam bubble and drives pressure to the saturation proxy
// (PressurizerHeaterTarget) inside the E1 band.
func TestScenario2_BubbleFormationFromWaterSolid(t *testing.T) {
	p := scenario2Plant()

	heatToTarget(t, p)

	pzr := &p.ReactorBuilding.Pressurizer
	assert.GreaterOrEqual(t, float64(pzr.Pressure), 2240.0,
		"pressure must reach the [2240, 2260] psia band, got %v", pzr.Pressure)
	assert.LessOrEqual(t, float64(pzr.Pressure), 2260.0)
	assert.Greater(t, pzr.SteamBubble.Float(), 0.0,
		"boiloff must leave a steam bubble behind")
	assert.Less(t, pzr.Water.Float(), pzr.Volume.Float(),
		"the pressurizer must leave the water-solid state")
}

// Opening letdown with heaters available drains the level from the
// post-bubble state into the 20-80 percent band down to no-load while
// the heater/spray loop holds 2250 +/- 50 psia, without lifting the
// PORV and without adding heat to the cold RCS.
func TestScenario2_LetdownDrainToNoLoadHoldsPressure(t *testing.T) {
	p := scenario2Plant()
	heatToTarget(t, p)

	setControl(t, p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")

	const (
		holdLowPsia   = 2200.0 // 2250 +/- 50 psia hold (page_224.txt:28-29)
		holdHighPsia  = 2300.0
		noLoadGal     = 380.0 // stop draining just past the 25 percent mark
		noLoadLowGal  = 337.0 // 25 +/- 5 percent of 1500 gal (page_222.txt:62-67)
		noLoadHighGal = 412.0
		maxDrainSteps = int(60 * time.Second / time.Millisecond)
	)
	minP, maxP := holdHighPsia, holdLowPsia
	minLevelPct, maxLevelPct := 100.0, 0.0
	maxPORV, maxOutletF := 0.0, 0.0
	reachedNoLoad := false
	for i := 0; i < maxDrainSteps && !reachedNoLoad; i++ {
		stepN(p, 1, DefaultTimeStep)
		pzr := &p.ReactorBuilding.Pressurizer
		levelPct := 100 * pzr.Water.Float() / pzr.Volume.Float()
		minLevelPct = min(minLevelPct, levelPct)
		maxLevelPct = max(maxLevelPct, levelPct)
		minP = min(minP, float64(pzr.Pressure))
		maxP = max(maxP, float64(pzr.Pressure))
		maxPORV = max(maxPORV, p.ReactorBuilding.PORV.Open)
		maxOutletF = max(maxOutletF,
			p.ReactorBuilding.ReactorPressureVessel.Outlet.Float())
		reachedNoLoad = pzr.Water.Float() <= noLoadGal
	}
	require.True(t, reachedNoLoad,
		"letdown never drained the pressurizer to the no-load mark")

	setControl(t, p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "0")
	setControl(t, p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning", "0")
	stepFor(p, 2*time.Second, DefaultTimeStep)

	final := &p.ReactorBuilding.Pressurizer
	assert.GreaterOrEqual(t, minLevelPct, 20.0,
		"level must stay inside the 20-80 percent band during the drain")
	assert.LessOrEqual(t, maxLevelPct, 80.0)
	assert.GreaterOrEqual(t, minP, holdLowPsia,
		"pressure must stay within 2250 +/- 50 psia through the drain")
	assert.LessOrEqual(t, maxP, holdHighPsia)
	assert.Equal(t, 0.0, maxPORV, "the PORV must never lift")
	assert.Less(t, maxOutletF, 140.0,
		"no scenario action may add heat to the cold RCS")
	assert.GreaterOrEqual(t, final.Water.Float(), noLoadLowGal,
		"final level must land at no-load 25 +/- 5 percent, got %v gal",
		final.Water.Float())
	assert.LessOrEqual(t, final.Water.Float(), noLoadHighGal)
	assert.GreaterOrEqual(t, float64(final.Pressure), holdLowPsia,
		"the heater hold must survive the lineup change")
	assert.LessOrEqual(t, float64(final.Pressure), holdHighPsia)
	assert.Greater(t, final.SteamBubble.Float(), 0.0)
}
