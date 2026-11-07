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
	t.Parallel()
	low := CVCS{AutoBoronControl: true}
	low.applyAutoBoronControl(CVCSAutoBoronLowInsertionFrac/2, 0, 0, 1.0)
	assert.Equal(t, CVCSDilution, low.Mode,
		"bank out of travel dilutes")
	assert.True(t, low.BTRSRecirculationPumpRunning,
		"dilution needs the BTRS pump")

	high := CVCS{AutoBoronControl: true}
	high.applyAutoBoronControl(CVCSAutoBoronHighInsertionFrac*2, 0, 0, 1.0)
	assert.Equal(t, CVCSBoration, high.Mode,
		"buried bank borates")
}

// Mid-band travel restores the neutral volume-control lineup and
// stops the BTRS pump.
func TestCVCSAutoBoronNeutralMidBand(t *testing.T) {
	t.Parallel()
	mid := CVCS{
		Mode:                         CVCSBoration,
		BTRSRecirculationPumpRunning: true,
		AutoBoronControl:             true,
	}
	mid.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2, 0, 0, 1.0)
	assert.Equal(t, CVCSVolumeControl, mid.Mode)
	assert.False(t, mid.BTRSRecirculationPumpRunning)
}

// A bottom-skewed axial offset past the target band borates: negative
// chemistry lets the power loop withdraw the bank, and the withdrawn
// bank lifts the offset back toward the band. A top-skewed offset
// mirrors as dilution. Band excursions outrank travel position.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:120-123
func TestCVCSAutoBoronAnswersAxialOffsetBand(t *testing.T) {
	t.Parallel()
	bottomSkew := CVCS{AutoBoronControl: true}
	bottomSkew.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2,
		-CoreAFDTargetBandPct-1, 0, 1.0)
	assert.Equal(t, CVCSBoration, bottomSkew.Mode,
		"bottom-skewed offset borates")

	topSkew := CVCS{AutoBoronControl: true}
	topSkew.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2,
		CoreAFDTargetBandPct+1, 0, 1.0)
	assert.Equal(t, CVCSDilution, topSkew.Mode,
		"top-skewed offset dilutes")

	insideBand := CVCS{AutoBoronControl: true}
	insideBand.applyAutoBoronControl(
		(CVCSAutoBoronLowInsertionFrac+CVCSAutoBoronHighInsertionFrac)/2,
		-CoreAFDTargetBandPct/2, 0, 1.0)
	assert.Equal(t, CVCSVolumeControl, insideBand.Mode,
		"in-band offset stays neutral")

	// Combined extremes where the inputs DISAGREE: the band wins,
	// because a running spatial-xenon oscillation is the active
	// instability while travel-limit relief can wait for it to settle.
	// Bottom skew with a withdrawn bank: the travel rule would dilute,
	// the band rule borates - boration is pinned.
	skewVsBuried := CVCS{AutoBoronControl: true}
	skewVsBuried.applyAutoBoronControl(
		CVCSAutoBoronLowInsertionFrac/2, -CoreAFDTargetBandPct-1, 0, 1.0)
	assert.Equal(t, CVCSBoration, skewVsBuried.Mode,
		"bottom skew outranks withdrawal-limit dilution")

	// Top skew with a buried bank: the travel rule would borate, the
	// band rule dilutes - dilution is pinned.
	skewVsBuried2 := CVCS{AutoBoronControl: true}
	skewVsBuried2.applyAutoBoronControl(
		CVCSAutoBoronHighInsertionFrac*2, CoreAFDTargetBandPct+1, 0, 1.0)
	assert.Equal(t, CVCSDilution, skewVsBuried2.Mode,
		"top skew outranks burial-limit boration")
}

// With the latch off, operator-selected mode and pump state are never
// rewritten, matching the manual-lineup convention. Differential form:
// an identical twin with the latch engaged does respond, so the
// assertion proves gating rather than a vacuous no-op.
func TestCVCSAutoBoronManualOverride(t *testing.T) {
	t.Parallel()
	manual := MakePlant()
	manual.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSVolumeControl
	// Rated lineup insertion (~1 percent, trimming the no-load
	// Doppler offset against the 571°F anchor) sits inside the low
	// trigger band, so the engaged twin selects dilution while the
	// latch-off twin must not move.

	auto := MakePlant()
	auto.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true

	stepN(&manual, 1, DefaultTimeStep)
	stepN(&auto, 1, DefaultTimeStep)

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
	t.Parallel()
	// Width note: the descent is a program walk over the anchored
	// fission update (verified by
	// TestReactorFissionPower_WidthInvariance) plus fixed-rate boron
	// dilution selected by applyAutoBoronControl on rod position and
	// integrated linearly in dt; the assertions are wide bands around
	// the walked plateau, so the widened tick reproduces the
	// trajectory at one tenth the step count..
	testDt := 100 * time.Millisecond
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true
	p.TurbineBuilding.Turbine.Setpoint = 0.35

	powerFrac := func() float64 {
		return float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
			float64(CoreRatedThermalPower)
	}

	stepN(&p, int(240*time.Second/testDt), testDt)
	descending := powerFrac()
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
	assert.Greater(t, descending, 0.6,
		"descent stays quasi-static this early")
	assert.Less(t, descending, 0.95, "power is leaving the rated cap")
	assert.Less(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.
		BoronConcentration, CVCSInitialBoron,
		"boron trim has diluted below the fresh-fuel value")

	stepN(&p, int(240*time.Second/testDt), testDt)
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
	t.Parallel()
	midInsertion := (CVCSAutoBoronLowInsertionFrac +
		CVCSAutoBoronHighInsertionFrac) / 2

	underDemand := CVCS{AutoBoronControl: true}
	underDemand.applyAutoBoronControl(midInsertion, 0, CVCSAutoBoronPowerErrBandFrac*3, 1.0)
	assert.Equal(t, CVCSDilution, underDemand.Mode,
		"a standing deficit dilutes even with the bank mid-window")
	assert.True(t, underDemand.BTRSRecirculationPumpRunning)

	overSwing := CVCS{AutoBoronControl: true}
	overSwing.applyAutoBoronControl(midInsertion, 0, -CVCSAutoBoronPowerErrBandFrac*3, 1.0)
	assert.Equal(t, CVCSBoration, overSwing.Mode,
		"an achieved-power overswing borates")

	onTarget := CVCS{AutoBoronControl: true}
	onTarget.applyAutoBoronControl(midInsertion, 0, CVCSAutoBoronPowerErrBandFrac/2, 1.0)
	assert.Equal(t, CVCSVolumeControl, onTarget.Mode,
		"inside the deadband the mid-band position rule governs")

	// The tier sits below the axial-offset band: a running spatial
	// instability outranks load-error relief.
	skewWins := CVCS{AutoBoronControl: true}
	skewWins.applyAutoBoronControl(midInsertion, CoreAFDTargetBandPct+1, CVCSAutoBoronPowerErrBandFrac*3, 1.0)
	assert.Equal(t, CVCSDilution, skewWins.Mode,
		"band excursion and deficit agree on dilution")
	skewOpposes := CVCS{AutoBoronControl: true}
	skewOpposes.applyAutoBoronControl(midInsertion, -CoreAFDTargetBandPct-1, CVCSAutoBoronPowerErrBandFrac*3, 1.0)
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
	t.Parallel()
	c := CVCS{AutoBoronControl: true}
	c.applyAutoBoronControl(0.04, 0, CVCSAutoBoronPowerErrBandFrac*1.01, 1.0)
	assert.InDelta(t, 0, c.AutoBoronDrive, 1e-2,
		"errors just past the deadband barely drive")

	c = CVCS{AutoBoronControl: true}
	c.applyAutoBoronControl(0.04, 0, (CVCSAutoBoronPowerErrBandFrac+CVCSAutoBoronPowerErrSpanFrac)/2, 1.0)
	assert.InDelta(t, 0.5, c.AutoBoronDrive, 1e-9,
		"mid-span error drives half rate")

	c = CVCS{AutoBoronControl: true}
	c.applyAutoBoronControl(0.04, 0, CVCSAutoBoronPowerErrSpanFrac*4, 1.0)
	assert.InDelta(t, 1.0, c.AutoBoronDrive, 1e-9,
		"errors past the span clamp at full drive")

	positional := CVCS{AutoBoronControl: true}
	positional.applyAutoBoronControl(
		CVCSAutoBoronLowInsertionFrac/2, 0, 0, 1.0)
	assert.InDelta(t, 0.875*CVCSAutoBoronPositionDriveCapFrac, positional.AutoBoronDrive, 1e-9,
		"position tiers pace to the cap across the position span")
}

// The position tiers scale with depth past the threshold: zero at
// the threshold (mode still selected, beds pre-position), rising to
// the cap across CVCSAutoBoronPositionSpanFrac. Seeding axial offset
// to zero keeps the AFD tiers out of this witness - they own full
// drive.
func TestAutoBoron_PositionTiersPaceWithDepthPastThreshold(t *testing.T) {
	t.Parallel()
	cap := CVCSAutoBoronPositionDriveCapFrac
	span := CVCSAutoBoronPositionSpanFrac

	atLow := CVCS{AutoBoronControl: true}
	atLow.applyAutoBoronControl(CVCSAutoBoronLowInsertionFrac-1e-12, 0, 0, 1.0)
	assert.Equal(t, CVCSDilution, atLow.Mode)
	assert.InDelta(t, 0.0, atLow.AutoBoronDrive, 1e-9,
		"at the low threshold the drive is zero but the tier acts")

	atHigh := CVCS{AutoBoronControl: true}
	atHigh.applyAutoBoronControl(CVCSAutoBoronHighInsertionFrac+1e-12, 0, 0, 1.0)
	assert.Equal(t, CVCSBoration, atHigh.Mode)
	assert.InDelta(t, 0.0, atHigh.AutoBoronDrive, 1e-9,
		"at the high threshold the drive is zero but the tier acts")

	half := CVCS{AutoBoronControl: true}
	half.applyAutoBoronControl(CVCSAutoBoronLowInsertionFrac-span/2, 0, 0, 1.0)
	assert.InDelta(t, cap/2, half.AutoBoronDrive, 1e-9,
		"halfway to the position span drives half the cap")

	saturated := CVCS{AutoBoronControl: true}
	saturated.applyAutoBoronControl(
		CVCSAutoBoronHighInsertionFrac+span*4, 0, 0, 1.0)
	assert.InDelta(t, cap, saturated.AutoBoronDrive, 1e-9,
		"past the position span the drive clamps at the cap")
}

// Zero-value coherence: with the trim disengaged, a diluted-mode lineup
// exchanges at the full design rate even though AutoBoronDrive sits at
// its zero value - operator chemistry never inherits the automatic
// pacing scalar.
func TestCVCSDilutionRateFullWithoutAutoTrim(t *testing.T) {
	t.Parallel()
	testDt := time.Second
	c := MakePlant().AuxiliaryBuilding.ChemicalVolumeControlSystem
	require.False(t, c.AutoBoronControl)
	require.Zero(t, c.AutoBoronDrive)
	c.Mode = CVCSDilution
	c.BTRSRecirculationPumpRunning = true

	start := c.BoronConcentration
	c = c.Step(testDt, Volume{},
		Temperature{Val: 580, Unit: u.Fahrenheit}, 0.5, PressurizerHeaterTarget)

	dropped := start - c.BoronConcentration
	assert.InDelta(t, float64(CVCSDilutionRate), dropped, 1e-6,
		"disengaged trim dilutes at the design rate")
}

// Deep-band rods-first scaling of the power-error tier (p61, gaps.md item
// 18): inside the demand band the error-tier drive scales by remaining rod
// corrective travel. Borating at full withdrawal scales to zero drive but
// keeps mode and pump (pre-positioning contract); diluting at full
// withdrawal runs at full pace (dilution needs withdrawal, which is exactly
// the parked state). Authority reaches 1 at the position-tier thresholds.
// Spec: boron compensates "whenever control rod operation is restricted"
// (specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:79-82)
// and mode+magnitude key on the total mismatch (page_135.txt:58-69), so the
// tier keeps its mode and only its magnitude is spent with rod travel.
func TestCVCSAutoBoronDeepBandScalesWithRodAuthority(t *testing.T) {
	t.Parallel()

	// Borating an overpowered park at full withdrawal: drive 0, but the
	// tier selects the mode and latches the BTRS pump so beds
	// pre-position for the eventual flip.
	parkBorate := CVCS{AutoBoronControl: true}
	parkBorate.applyAutoBoronControl(0, 0, -CVCSAutoBoronPowerErrSpanFrac*4, 0.05)
	assert.Equal(t, CVCSBoration, parkBorate.Mode,
		"deep-band borating error still selects the boration mode")
	assert.True(t, parkBorate.BTRSRecirculationPumpRunning,
		"zero-drive pre-positioning keeps the BTRS pump latched")
	assert.InDelta(t, 0.0, parkBorate.AutoBoronDrive, 1e-9,
		"at full withdrawal the borating drive is zero (zero ppm/s exchange)")

	// Diluting a deficit park at full withdrawal: full paced drive,
	// because dilution *is* withdrawal and the pit after a down-maneuver
	// needs it at full pace.
	parkDilute := CVCS{AutoBoronControl: true}
	parkDilute.applyAutoBoronControl(0, 0, CVCSAutoBoronPowerErrSpanFrac*4, 0.05)
	assert.Equal(t, CVCSDilution, parkDilute.Mode,
		"deep-band diluting error still selects the dilution mode")
	assert.InDelta(t, 1.0, parkDilute.AutoBoronDrive, 1e-9,
		"at full withdrawal the diluting drive is full pace")

	// Authority reaches 1 at the thresholds: borating at the high
	// threshold, diluting at the low threshold, even deep in the band.
	highEdge := CVCS{AutoBoronControl: true}
	highEdge.applyAutoBoronControl(
		CVCSAutoBoronHighInsertionFrac, 0, -CVCSAutoBoronPowerErrSpanFrac*4, 0.05)
	assert.InDelta(t, 1.0, highEdge.AutoBoronDrive, 1e-9,
		"borating at the high threshold runs at full pace")
	lowEdge := CVCS{AutoBoronControl: true}
	lowEdge.applyAutoBoronControl(
		CVCSAutoBoronLowInsertionFrac, 0, CVCSAutoBoronPowerErrSpanFrac*4, 0.05)
	assert.InDelta(t, 1.0, lowEdge.AutoBoronDrive, 1e-9,
		"diluting at the low threshold runs at full pace")

	// Above the deep band the drive is unchanged (walk above 0.20 must not
	// regress): same zero-insertion borating error at demand 1.0 drives at
	// the ungated pace.
	aboveBand := CVCS{AutoBoronControl: true}
	aboveBand.applyAutoBoronControl(0, 0, -CVCSAutoBoronPowerErrSpanFrac*4, 1.0)
	assert.InDelta(t, 1.0, aboveBand.AutoBoronDrive, 1e-9,
		"outside the deep band the drive is ungated")
}
