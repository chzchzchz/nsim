package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBladeErosion_WetCarryoverAccumulates drives the turbine Step
// through the production path at low load, where HP exhaust quality
// is HPExhaustQualityLowLoad (0.70) and inlet moisture is 0.30,
// residual 0.015 above the 0.007 allowance. Asserts erosion > 0 and
// Damage mirrors it.
func TestBladeErosion_WetCarryoverAccumulates(t *testing.T) {
	t.Parallel()
	turbine := Turbine{
		State:           TurbineRunning,
		Velocity:        TurbineRatedSpeed,
		Synchronized:    true,
		StopValve:       Valve{Open: 1},
		GoverningValve:  Valve{Open: 1},
		HPExhaustQuality: HPExhaustQualityLowLoad,
	}
	// Throttle the governing valve so load fraction drops and HP
	// exhaust quality falls to the low-load floor. Lube-oil pump
	// must be in service or the bearing-oil trip latches and the
	// machine trips (RealPower zeroed).
	turbine.LubeOilPumpRunning = true
	turbine.BearingOilPressure = BearingOilDesignPressure
	turbine.GoverningValve.Open = 0.2
	turbine.Setpoint = 0.05
	turbine.RealPower = Watts(0.05 * float64(TurbineRatedkW))
	for i := 0; i < 2000; i++ {
		turbine = turbine.Step(DefaultTimeStep, SteamGeneratorShellPressure, 0, Temperature{Val: 500, Unit: u.Fahrenheit}, false, nil)
	}
	// Carryover condition held if InletQuality is at low load.
	require.LessOrEqual(t, turbine.MoistureSeparatorReheater.InletQuality, HPExhaustQualityLowLoad+0.01,
		"carryover condition must hold: InletQuality=%v", turbine.MoistureSeparatorReheater.InletQuality)
	assert.Greater(t, turbine.BladeErosionFrac, 0.0,
		"low-load wet carryover must erode blades")
	assert.InDelta(t, turbine.BladeErosionFrac, float64(turbine.Damage), 1e-6,
		"Damage must mirror BladeErosionFrac")
}

// TestBladeErosion_DryFullLoadStaysZero drives full-load dry
// operation: HP quality 0.90, moisture 0.10, residual 0.005 below
// the 0.007 allowance. Erosion must stay zero.
func TestBladeErosion_DryFullLoadStaysZero(t *testing.T) {
	t.Parallel()
	turbine := Turbine{
		State:           TurbineRunning,
		Velocity:        TurbineRatedSpeed,
		Synchronized:    true,
		StopValve:       Valve{Open: 1},
		GoverningValve:  Valve{Open: 1},
		HPExhaustQuality: HPExhaustQualityFullLoad,
		RealPower:       Watts(float64(TurbineRatedkW)),
		LubeOilPumpRunning: true,
		BearingOilPressure: BearingOilDesignPressure,
	}
	for i := 0; i < 2000; i++ {
		turbine = turbine.Step(DefaultTimeStep, SteamGeneratorShellPressure, 0, Temperature{Val: 500, Unit: u.Fahrenheit}, false, nil)
	}
	require.GreaterOrEqual(t, turbine.MoistureSeparatorReheater.InletQuality, HPExhaustQualityFullLoad-0.01,
		"full-load quality must hold: InletQuality=%v", turbine.MoistureSeparatorReheater.InletQuality)
	assert.Zero(t, turbine.BladeErosionFrac,
		"full-load dry operation must not erode")
	assert.Zero(t, turbine.Damage,
		"no erosion means no Damage contribution")
}

// TestLinerFatigue_WindowedCycleCounts drives containment
// temperature through reversals ≥ 30°F in the 20,000-step windowed
// detector and asserts the cycle counter increments.
func TestLinerFatigue_WindowedCycleCounts(t *testing.T) {
	t.Parallel()
	cb := ContainmentBuilding{
		Part:          Part{Name: "containment"},
		Pressure:      AtmosphericPressure,
		Temperature:   Temperature{Val: 120, Unit: u.Fahrenheit},
		UpperTemperature: Temperature{Val: 120, Unit: u.Fahrenheit},
		LowerTemperature: Temperature{Val: 120, Unit: u.Fahrenheit},
		FreeVolume:    2_000_000,
	}
	// Prime the tracker with a few steady steps.
	for i := 0; i < 10; i++ {
		cb = cb.Step(DefaultTimeStep, 0, 0)
	}
	base := cb.ThermalCycleCount
	// Heat to 160°F (+40°F lift) then cool back to 120°F: a full
	// window reversal above the 30°F band.
	for i := 0; i < ContainmentCycleWindowSteps; i++ {
		cb.UpperTemperature = Temperature{Val: 120 + 40*float64(i)/float64(ContainmentCycleWindowSteps), Unit: u.Fahrenheit}
		cb.LowerTemperature = cb.UpperTemperature
		cb = cb.Step(DefaultTimeStep, 0, 0)
	}
	for i := 0; i < ContainmentCycleWindowSteps; i++ {
		cb.UpperTemperature = Temperature{Val: 160 - 40*float64(i)/float64(ContainmentCycleWindowSteps), Unit: u.Fahrenheit}
		cb.LowerTemperature = cb.UpperTemperature
		cb = cb.Step(DefaultTimeStep, 0, 0)
	}
	// One more window to land the reversal.
	for i := 0; i < ContainmentCycleWindowSteps; i++ {
		cb = cb.Step(DefaultTimeStep, 0, 0)
	}
	delta := cb.ThermalCycleCount - base
	assert.Greater(t, delta, 0.0,
		"a ≥30°F windowed reversal must count a cycle, got delta=%v", delta)
	// LinerFatigueFrac = count/500.
	assert.Equal(t, cb.ThermalCycleCount/ContainmentStartupCycleBudget, cb.LinerFatigueFrac,
		"LinerFatigueFrac must equal count/500")
	// Sub-band lift must not fire: heat only 10°F.
	cb2 := ContainmentBuilding{
		Part:          Part{Name: "containment"},
		Pressure:      AtmosphericPressure,
		Temperature:   Temperature{Val: 120, Unit: u.Fahrenheit},
		UpperTemperature: Temperature{Val: 120, Unit: u.Fahrenheit},
		LowerTemperature: Temperature{Val: 120, Unit: u.Fahrenheit},
		FreeVolume:    2_000_000,
	}
	for i := 0; i < 10; i++ {
		cb2 = cb2.Step(DefaultTimeStep, 0, 0)
	}
	base2 := cb2.ThermalCycleCount
	for i := 0; i < ContainmentCycleWindowSteps; i++ {
		cb2.UpperTemperature = Temperature{Val: 120 + 10*float64(i)/float64(ContainmentCycleWindowSteps), Unit: u.Fahrenheit}
		cb2.LowerTemperature = cb2.UpperTemperature
		cb2 = cb2.Step(DefaultTimeStep, 0, 0)
	}
	for i := 0; i < ContainmentCycleWindowSteps; i++ {
		cb2.UpperTemperature = Temperature{Val: 130 - 10*float64(i)/float64(ContainmentCycleWindowSteps), Unit: u.Fahrenheit}
		cb2.LowerTemperature = cb2.UpperTemperature
		cb2 = cb2.Step(DefaultTimeStep, 0, 0)
	}
	for i := 0; i < ContainmentCycleWindowSteps; i++ {
		cb2 = cb2.Step(DefaultTimeStep, 0, 0)
	}
	assert.Equal(t, base2, cb2.ThermalCycleCount,
		"sub-band lift must not count")
}

// TestLinerFatigue_FirstStepSuppressed asserts the zero-value
// first Step cannot count a phantom cycle.
func TestLinerFatigue_FirstStepSuppressed(t *testing.T) {
	t.Parallel()
	cb := ContainmentBuilding{
		Part:          Part{Name: "containment"},
		Pressure:      AtmosphericPressure,
		Temperature:   Temperature{Val: 120, Unit: u.Fahrenheit},
		UpperTemperature: Temperature{Val: 120, Unit: u.Fahrenheit},
		LowerTemperature: Temperature{Val: 120, Unit: u.Fahrenheit},
		FreeVolume:    2_000_000,
	}
	c1 := cb.Step(DefaultTimeStep, 0, 0)
	assert.Zero(t, c1.ThermalCycleCount, "first Step must be suppressed")
	assert.Zero(t, c1.LinerFatigueFrac)
}

// TestChlorideSCCRisk_LOCAAndAdditive exercises the CSS-step
// derived risk predicate.
func TestChlorideSCCRisk_LOCAAndAdditive(t *testing.T) {
	t.Parallel()
	locaPressure := AtmosphericPressure + CSSContainmentPressureSetpoint + 10

	// LOCA without additive delivered → risk true.
	css := ContainmentSpraySystem{
		Part:                    Part{Name: "css"},
		Running:                 true,
		DriveReady:              true,
		SprayHeaderOpen:         true,
		SATIsolationValveBOpen:  false,
		SATLevel:                Volume{Val: 100, Unit: u.Gallon},
		NextContainmentPressure: locaPressure,
	}
	out := css.Step(DefaultTimeStep)
	assert.True(t, out.ChlorideSCCRisk,
		"LOCA with B closed must raise SCC risk")

	// LOCA with additive delivered → risk false.
	css.SATIsolationValveBOpen = true
	out = css.Step(DefaultTimeStep)
	assert.False(t, out.ChlorideSCCRisk,
		"LOCA with additive delivered must clear SCC risk")

	// Steady state (no LOCA) → risk false even with B closed.
	css.NextContainmentPressure = AtmosphericPressure
	css.SATIsolationValveBOpen = false
	out = css.Step(DefaultTimeStep)
	assert.False(t, out.ChlorideSCCRisk,
		"no LOCA means no SCC risk")
}

// TestChlorideSCCRisk_EarlyReturnNotStale covers the predicate
// recomputation before the `!Running || !DriveReady` early return
// (plans/p72_5.txt Chemistry drivers). A LOCA raised while the spray
// pumps are secured must still raise the risk, and clearing the LOCA
// while secured must clear it.
func TestChlorideSCCRisk_EarlyReturnNotStale(t *testing.T) {
	t.Parallel()
	locaPressure := AtmosphericPressure + CSSContainmentPressureSetpoint + 10

	// LOCA active, pumps secured (Running=false) → risk true via the
	// early-return path, not the spray path.
	css := ContainmentSpraySystem{
		Part:                    Part{Name: "css"},
		Running:                 false,
		DriveReady:              false,
		SATIsolationValveBOpen:  false,
		SATLevel:                Volume{Val: 100, Unit: u.Gallon},
		NextContainmentPressure: locaPressure,
	}
	out := css.Step(DefaultTimeStep)
	assert.True(t, out.ChlorideSCCRisk,
		"LOCA with pumps secured must raise SCC risk")

	// LOCA clears while pumps still secured → risk false.
	css.NextContainmentPressure = AtmosphericPressure
	out = css.Step(DefaultTimeStep)
	assert.False(t, out.ChlorideSCCRisk,
		"clearing the LOCA while pumps are secured must clear SCC risk")

	// Hysteresis guard: raise LOCA again with B open → still false,
	// so the early-return path keys on delivery state, not on the
	// pre-return latch value.
	css.NextContainmentPressure = locaPressure
	css.SATIsolationValveBOpen = true
	out = css.Step(DefaultTimeStep)
	assert.False(t, out.ChlorideSCCRisk,
		"LOCA with additive delivered must clear SCC risk even when pumps secured")
}

// TestCorrosionProductControl_SteadyStateDegraded asserts the
// honesty note: at a fresh MakePlant volume-control lineup the BTRS
// demineralizer path is out of service, so the sensor reads true.
func TestCorrosionProductControl_SteadyStateDegraded(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	// Volume-control lineup: no BTRS run, letdown closed.
	assert.False(t, cvcs.BTRSRecirculationPumpRunning)
	assert.False(t, cvcs.LetdownValveOpen)
	// The step recomputes the predicate from the live state.
	out := cvcs.Step(DefaultTimeStep, Volume{Val: 0, Unit: u.Gallon},
		Temperature{Val: 500, Unit: u.Fahrenheit}, 0.5, 2250)
	assert.True(t, out.CorrosionProductControlDegraded,
		"volume-control lineup (no BTRS service) must read degraded")
}

// TestCorrosionProductControl_BTRSPredicate exercises the CVCS
// demineralizer-service predicate.
func TestCorrosionProductControl_BTRSPredicate(t *testing.T) {
	t.Parallel()
	// In service: BTRS running, letdown flowing, not diverting.
	cvcs := CVCS{
		Part:                        Part{Name: "cvcs"},
		LetdownValveOpen:            true,
		BTRSRecirculationPumpRunning: true,
		LetdownFlow:                 Volume{Val: 10, Unit: u.Gallon},
		Diverting:                   false,
	}
	out := cvcs.Step(DefaultTimeStep, Volume{Val: 0, Unit: u.Gallon}, Temperature{Val: 500, Unit: u.Fahrenheit}, 0.5, 2250)
	assert.False(t, out.CorrosionProductControlDegraded,
		"demineralizer in service must read false")

	// BTRS pump off → degraded.
	cvcs.BTRSRecirculationPumpRunning = false
	out = cvcs.Step(DefaultTimeStep, Volume{Val: 0, Unit: u.Gallon}, Temperature{Val: 500, Unit: u.Fahrenheit}, 0.5, 2250)
	assert.True(t, out.CorrosionProductControlDegraded,
		"BTRS pump off must read degraded")

	// Diverting → degraded.
	cvcs.BTRSRecirculationPumpRunning = true
	cvcs.Diverting = true
	cvcs.VCTLevel = Volume{Val: 100000, Unit: u.Gallon}
	out = cvcs.Step(DefaultTimeStep, Volume{Val: 0, Unit: u.Gallon}, Temperature{Val: 500, Unit: u.Fahrenheit}, 0.5, 2250)
	assert.True(t, out.CorrosionProductControlDegraded,
		"diverted letdown must read degraded")
}
