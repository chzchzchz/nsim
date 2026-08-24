package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 16 (reactor trip and recovery): post-trip stabilization and
// the start of the orderly return to power.
//
//   - "Post-trip stabilization (rods inserted, steam dump to no-load
//     Tavg) and orderly return to power per hot startup sequence."
//     plans/x/scenarios.txt item 16, from specs/pwr_sim/toc.md:51 and
//     specs/pwr_sim/chapter_3/page_042.txt
//   - Return to service proceeds from hot standby with the rods
//     withdrawn back toward criticality.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26
const scenario16Tick = 10 * time.Millisecond

func TestScenario16_ReactorTripStabilizationAndRecovery(t *testing.T) {
	// The fold trajectory is width-sensitive, so trip-side bands here
	// are coarse and directional; the dump controller's headroom limiter
	// and the rod clamp are latch/algebraic logic that tolerate the
	// coarse tick. Must not run in parallel.
	overrideTimeStep(t, scenario16Tick)
	p := scenario10FullPowerPlant()
	stepFor(&p, 5*time.Second)
	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State)

	startOutlet := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()

	// Trip the reactor: the RPV.Trip control enters through the RPS
	// manual-trip input, since Plant.Step overwrites the RPV latch from
	// the RPS output every step ("The RPS provides for manual initiation
	// of reactor trip by operator action in the control room.",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9).
	setControl(t, &p, "ReactorBuilding/ReactorPressureVessel/Trip", "true")
	stepFor(&p, 3*time.Second)

	rpv := &p.ReactorBuilding.ReactorPressureVessel
	require.True(t, rpv.Trip, "the manual trip must latch the reactor")
	assert.Equal(t, 1.0, rpv.ControlRods.Insertion,
		"trip clamps the rods fully inserted")

	// Stabilization window: the core folds, the steam-dump Tavg
	// restoration loop carries the header while outlet relaxes toward
	// the no-load program, and nothing spuriously actuates.
	maxSgP := 0.0
	for i := 0; i < int(120*time.Second/scenario16Tick); i++ {
		stepN(&p, 1)
		maxSgP = max(maxSgP, float64(p.ReactorBuilding.SteamGenerator.Pressure))
	}
	frac := fissionFraction(rpv.FissionPower)
	assert.LessOrEqual(t, frac, 0.2,
		"the core must fold away from rated after the trip, got %v", frac)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated,
		"the stabilized trip must not spurious-actuate SI")
	assert.GreaterOrEqual(t, maxSgP, 700.0,
		"the dump restoration must keep the header off the low-steamline trip")
	assert.LessOrEqual(t, maxSgP, float64(SteamGeneratorShellDesignPressure),
		"the header must stay under the design cap while dumped")
	assert.Less(t, rpv.Outlet.Float(), startOutlet,
		"outlet temperature must relax toward the no-load program")

	// Recovery initiation: clear the trip and withdraw the rods back
	// toward criticality. At reference-boron chemistry the folded core
	// sits subcritical even with rods out, so the hot-startup return
	// chases the critical boron concentration through the production
	// reactivity terms: dilute shim until the non-boron terms sum to
	// zero ("Return to service proceeds from hot standby with the rods
	// withdrawn back to criticality",
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26).
	setControl(t, &p, "ReactorBuilding/ReactorProtectionSystem/Reset", "Reset")
	require.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "0")
	foldedFrac := fissionFraction(rpv.FissionPower)
	recovered := false
	// The recovery chase drives the chemistry state directly: the
	// bridge BoronSetpoint path now ramps on the BTRS flow clock
	// (0.8 ppm/s), slower than the post-trip iodine-pit growth this
	// short-horizon window has to beat, so the ramped path cannot
	// demonstrate the return to power here (the ramp itself is
	// covered by TestScenario5_BoronSetpointRampsThroughFlowPath).
	for i := 0; i < int(600*time.Second/scenario16Tick); i++ {
		stepN(&p, 1)
		if i%int(10*time.Second/scenario16Tick) == 0 &&
			rpv.ControlRods.Insertion < 0.01 {
			// Chase a small supercritical bias on the measured folded
			// reactivity once the rods are clear of the core: the rod
			// swing carries the bulk of the margin and dilution trims
			// the rest. d(rho)/dB = CoreBoronWorthPer100ppm/100 per
			// ppm.
			const bias = 0.02
			t.Logf("t=%v frac=%.3e ins=%.3f B=%.1f rho=%v trip=%v",
				i/100, fissionFraction(rpv.FissionPower),
				rpv.ControlRods.Insertion,
				rpv.ControlRods.BoronConcentration,
				rpv.Reactivity, rpv.Trip)
			b := rpv.ControlRods.BoronConcentration +
				2000*(float64(rpv.Reactivity)-bias)
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.
				BoronSetpointTracking = false
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.
				BoronConcentration = max(b, 0)
		}
		if fissionFraction(rpv.FissionPower) > foldedFrac*1.5 {
			recovered = true
			break
		}
	}
	assert.True(t, recovered,
		"the diluted core must begin returning to power")
	assert.GreaterOrEqual(t, rpv.ControlRods.BoronConcentration, 0.0)
	assert.LessOrEqual(t, rpv.ControlRods.BoronConcentration, CVCSInitialBoron,
		"recovery dilutes shim; it never borates above reference")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"the recovery must hold without a re-trip")
	assert.LessOrEqual(t, float64(p.ReactorBuilding.SteamGenerator.Pressure),
		float64(SteamGeneratorShellDesignPressure))
}
