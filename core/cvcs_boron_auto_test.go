package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The boron trim reads rod-bank travel position: near full withdrawal
// the chain needs positive reactivity (dilute), under deep insertion
// negative (borate), and mid-band hands the mode back to neutral.
//
// Spec: the Power Control System adjusts power "through control rod
// motions, boron concentration changes, and control of the moderator
// temperature."
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_129.txt:112-115
func TestCVCSAutoBoronSelectsModeFromRodTravel(t *testing.T) {
	low := CVCS{AutoBoronControl: true}
	low.applyAutoBoronControl(CVCSAutoBoronLowInsertionFrac / 2)
	assert.Equal(t, CVCSDilution, low.Mode,
		"bank out of travel dilutes")
	assert.True(t, low.BTRSRecirculationPumpRunning,
		"dilution needs the BTRS pump")

	high := CVCS{AutoBoronControl: true}
	high.applyAutoBoronControl(CVCSAutoBoronHighInsertionFrac * 2)
	assert.Equal(t, CVCSBoration, high.Mode,
		"buried bank borates")
}

// Mid-band travel restores the neutral volume-control lineup and
// stops the BTRS pump.
func TestCVCSAutoBoronNeutralMidBand(t *testing.T) {
	mid := CVCS{
		Mode:                         CVCSBoration,
		BTRSRecirculationPumpRunning: true,
		AutoBoronControl:             true,
	}
	mid.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2)
	assert.Equal(t, CVCSVolumeControl, mid.Mode)
	assert.False(t, mid.BTRSRecirculationPumpRunning)
}

// With the latch off, operator-selected mode and pump state are never
// rewritten, matching the manual-lineup convention. Differential form:
// an identical twin with the latch engaged does respond, so the
// assertion proves gating rather than a vacuous no-op.
func TestCVCSAutoBoronManualOverride(t *testing.T) {
	manual := MakePlant()
	manual.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSVolumeControl
	// Rated lineup insertion (~1 percent, trimming the no-load
	// Doppler offset against the 571°F anchor) sits inside the low
	// trigger band, so the engaged twin selects dilution while the
	// latch-off twin must not move.

	auto := MakePlant()
	auto.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true

	stepN(&manual, 1)
	stepN(&auto, 1)

	assert.Equal(t, CVCSVolumeControl,
		manual.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode,
		"latch off: mode stays operator-owned")
	assert.Equal(t, CVCSDilution,
		auto.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode,
		"engaged twin responds at rated insertion")
}

// End-to-end at fresh-fuel chemistry: commanding a deep plateau inside
// the previously-infeasible band descends without collapsing because
// the boron trim dilutes as the rod bank runs out of withdrawal
// authority - the exact scenario family gap #8 blocked. Windows follow
// the 5-percent-per-minute internal program slew (probe-backed bands).
//
// Spec: "The Power Control System (Figure 7-7) automatically adjusts
// the reactor power [...]"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_129.txt:112-115
func TestCVCSAutoBoronUnlocksDeepPlateau(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true
	p.TurbineBuilding.Turbine.Setpoint = 0.35

	powerFrac := func() float64 {
		return float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
			float64(CoreRatedThermalPower)
	}

	stepN(&p, int(240*time.Second/TimeStep))
	descending := powerFrac()
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
	assert.Greater(t, descending, 0.6,
		"descent stays quasi-static this early")
	assert.Less(t, descending, 0.95, "power is leaving the rated cap")
	assert.Less(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.
		BoronConcentration, CVCSInitialBoron,
		"boron trim has diluted below the fresh-fuel value")

	stepN(&p, int(240*time.Second/TimeStep))
	final := powerFrac()
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"deep maneuver must not trip")
	assert.Greater(t, final, 0.4,
		"plateau holds far above the neutron-source floor")
	assert.Less(t, final, 0.65,
		"power followed the program into the deep-partial band")
	assert.Less(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.
		BoronConcentration, CVCSInitialBoron)
}
