package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	low.applyAutoBoronControl(CVCSAutoBoronLowInsertionFrac/2, 0, 0)
	assert.Equal(t, CVCSDilution, low.Mode,
		"bank out of travel dilutes")
	assert.True(t, low.BTRSRecirculationPumpRunning,
		"dilution needs the BTRS pump")

	high := CVCS{AutoBoronControl: true}
	high.applyAutoBoronControl(CVCSAutoBoronHighInsertionFrac*2, 0, 0)
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
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2, 0, 0)
	assert.Equal(t, CVCSVolumeControl, mid.Mode)
	assert.False(t, mid.BTRSRecirculationPumpRunning)
}

// A bottom-skewed axial offset past the target band borates: negative
// chemistry lets the power loop withdraw the bank, and the withdrawn
// bank lifts the offset back toward the band. A top-skewed offset
// mirrors as dilution. Band excursions outrank travel position.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:120-123
func TestCVCSAutoBoronAnswersAxialOffsetBand(t *testing.T) {
	bottomSkew := CVCS{AutoBoronControl: true}
	bottomSkew.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2,
		-CoreAFDTargetBandPct-1, 0)
	assert.Equal(t, CVCSBoration, bottomSkew.Mode,
		"bottom-skewed offset borates")

	topSkew := CVCS{AutoBoronControl: true}
	topSkew.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2,
		CoreAFDTargetBandPct+1, 0)
	assert.Equal(t, CVCSDilution, topSkew.Mode,
		"top-skewed offset dilutes")

	insideBand := CVCS{AutoBoronControl: true}
	insideBand.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2,
		-CoreAFDTargetBandPct/2, 0)
	assert.Equal(t, CVCSVolumeControl, insideBand.Mode,
		"in-band offset stays neutral")

	// Combined extremes where the inputs DISAGREE: the band wins,
	// because a running spatial-xenon oscillation is the active
	// instability while travel-limit relief can wait for it to settle.
	// Bottom skew with a withdrawn bank: the travel rule would dilute,
	// the band rule borates - boration is pinned.
	skewVsBuried := CVCS{AutoBoronControl: true}
	skewVsBuried.applyAutoBoronControl(
		CVCSAutoBoronLowInsertionFrac/2, -CoreAFDTargetBandPct-1, 0)
	assert.Equal(t, CVCSBoration, skewVsBuried.Mode,
		"bottom skew outranks withdrawal-limit dilution")

	// Top skew with a buried bank: the travel rule would borate, the
	// band rule dilutes - dilution is pinned.
	skewVsBuried2 := CVCS{AutoBoronControl: true}
	skewVsBuried2.applyAutoBoronControl(
		CVCSAutoBoronHighInsertionFrac*2, CoreAFDTargetBandPct+1, 0)
	assert.Equal(t, CVCSDilution, skewVsBuried2.Mode,
		"top skew outranks burial-limit boration")
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
	// Width note: the descent is a program walk over the anchored
	// fission update (verified by
	// TestReactorFissionPower_WidthInvariance) plus fixed-rate boron
	// dilution selected by applyAutoBoronControl on rod position and
	// integrated linearly in dt; the assertions are wide bands around
	// the walked plateau, so the widened tick reproduces the
	// trajectory at one tenth the step count. Must not run in
	// parallel: overrideTimeStep mutates global simulation state.
	overrideTimeStep(t, 10*time.Millisecond)
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

// The demanded-minus-achieved power error adds a selection tier above
// the position window while the RPC cascade stands engaged: a plant
// missing its walked target dilutes even with the bank mid-window
// (where the position rule would stand neutral), an overswinging plant
// borates, and inside the deadband the old position rules govern.
//
// Spec: mode and magnitude of the boron change are "determined by
// comparison" against a total error; "if this total error is large
// enough, the BTRS is switched to the proper mode".
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-65
func TestCVCSAutoBoronAnswersPowerError(t *testing.T) {
	midInsertion := (CVCSAutoBoronLowInsertionFrac +
		CVCSAutoBoronHighInsertionFrac) / 2

	underDemand := CVCS{AutoBoronControl: true}
	underDemand.applyAutoBoronControl(midInsertion, 0,
		CVCSAutoBoronPowerErrBandFrac*3)
	assert.Equal(t, CVCSDilution, underDemand.Mode,
		"a standing deficit dilutes even with the bank mid-window")
	assert.True(t, underDemand.BTRSRecirculationPumpRunning)

	overSwing := CVCS{AutoBoronControl: true}
	overSwing.applyAutoBoronControl(midInsertion, 0,
		-CVCSAutoBoronPowerErrBandFrac*3)
	assert.Equal(t, CVCSBoration, overSwing.Mode,
		"an achieved-power overswing borates")

	onTarget := CVCS{AutoBoronControl: true}
	onTarget.applyAutoBoronControl(midInsertion, 0,
		CVCSAutoBoronPowerErrBandFrac/2)
	assert.Equal(t, CVCSVolumeControl, onTarget.Mode,
		"inside the deadband the mid-band position rule governs")

	// The tier sits below the axial-offset band: a running spatial
	// instability outranks load-error relief.
	skewWins := CVCS{AutoBoronControl: true}
	skewWins.applyAutoBoronControl(midInsertion,
		CoreAFDTargetBandPct+1, CVCSAutoBoronPowerErrBandFrac*3)
	assert.Equal(t, CVCSDilution, skewWins.Mode,
		"band excursion and deficit agree on dilution")
	skewOpposes := CVCS{AutoBoronControl: true}
	skewOpposes.applyAutoBoronControl(midInsertion,
		-CoreAFDTargetBandPct-1, CVCSAutoBoronPowerErrBandFrac*3)
	assert.Equal(t, CVCSBoration, skewOpposes.Mode,
		"the axial-offset band outranks the standing deficit")
}

// The error-driven exchange rate is paced by the error magnitude:
// linear from zero at the deadband edge to the full design rate at the
// span, clamped beyond it. Position and axial-offset selections run at
// full rate. Tier note: the band and span are invented conventions
// (spec says only "large enough"), so this pins the declared shape of
// the pacing law, not spec'd numbers.
func TestCVCSAutoBoronDrivePacesWithError(t *testing.T) {
	c := CVCS{AutoBoronControl: true}
	c.applyAutoBoronControl(0.04, 0, CVCSAutoBoronPowerErrBandFrac*1.01)
	assert.InDelta(t, 0, c.AutoBoronDrive, 1e-2,
		"errors just past the deadband barely drive")

	c = CVCS{AutoBoronControl: true}
	c.applyAutoBoronControl(0.04, 0,
		(CVCSAutoBoronPowerErrBandFrac+CVCSAutoBoronPowerErrSpanFrac)/2)
	assert.InDelta(t, 0.5, c.AutoBoronDrive, 1e-9,
		"mid-span error drives half rate")

	c = CVCS{AutoBoronControl: true}
	c.applyAutoBoronControl(0.04, 0, CVCSAutoBoronPowerErrSpanFrac*4)
	assert.InDelta(t, 1.0, c.AutoBoronDrive, 1e-9,
		"errors past the span clamp at full drive")

	positional := CVCS{AutoBoronControl: true}
	positional.applyAutoBoronControl(
		CVCSAutoBoronLowInsertionFrac/2, 0, 0)
	assert.InDelta(t, 1.0, positional.AutoBoronDrive, 1e-9,
		"position-tier selections keep the design rate")
}

// Zero-value coherence: with the trim disengaged, a diluted-mode lineup
// exchanges at the full design rate even though AutoBoronDrive sits at
// its zero value - operator chemistry never inherits the automatic
// pacing scalar.
func TestCVCSDilutionRateFullWithoutAutoTrim(t *testing.T) {
	overrideTimeStep(t, time.Second)
	c := MakePlant().AuxiliaryBuilding.ChemicalVolumeControlSystem
	require.False(t, c.AutoBoronControl)
	require.Zero(t, c.AutoBoronDrive)
	c.Mode = CVCSDilution
	c.BTRSRecirculationPumpRunning = true

	start := c.BoronConcentration
	c = c.Step(Volume{},
		Temperature{Val: 580, Unit: u.Fahrenheit}, 0.5, PressurizerHeaterTarget)

	dropped := start - c.BoronConcentration
	assert.InDelta(t, float64(CVCSDilutionRate), dropped, 1e-6,
		"disengaged trim dilutes at the design rate")
}
