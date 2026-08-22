package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plans/x/fission_controls.md acceptance tests.
//
// The rod-drive rate limit and operator-facing bindings
// (Insertion clamp, BoronSetpoint, RPV/SG/RCP sensors) are
// exposed through the bridge. These tests verify each entry
// exists, the rate limit fires, and the thermal model sees
// the resulting reactivity change.

// At a single 1 ms timestep the rod can move by no more than
// RodInsertionSpeed * TimeStep = 5e-5 fraction. A demand jump
// from 0.0 to 1.0 (withdrawn to fully inserted) is therefore
// rate-limited to that exact amount per call, not the full
// 1.0. Step moves Insertion toward DemandInsertion.
func TestControlRods_StepRateLimits(t *testing.T) {
	c := ControlRods{Insertion: 0, DemandInsertion: 1}
	out := c.Step()
	want := RodInsertionSpeed * TimeStep.Seconds()
	assert.InDelta(t, want, out.Insertion, 1e-12,
		"want Insertion = %v after one Step from full-withdrawal, got %v",
		want, out.Insertion)

	// 20 s of stepping should drive 0.0 → 1.0; the rate limit
	// is 5% of travel per second, so 20 s covers the full
	// travel with margin.
	c2 := ControlRods{Insertion: 0, DemandInsertion: 1}
	for i := 0; i < int(20*time.Second/TimeStep); i++ {
		c2 = c2.Step()
	}
	assert.InDelta(t, 1, c2.Insertion, 1e-9,
		"want Insertion=1 after 20 s of stepping, got %v", c2.Insertion)
}

// Step clamps the result to [0,1] so a malicious or
// out-of-range demand cannot drive Insertion outside the
// physical rod travel.
func TestControlRods_StepClamps(t *testing.T) {
	high := ControlRods{Insertion: 0.5, DemandInsertion: 2.0}
	for i := 0; i < int(60*time.Second/TimeStep); i++ {
		high = high.Step()
	}
	assert.LessOrEqual(t, high.Insertion, 1.0,
		"want Insertion <= 1, got %v", high.Insertion)

	low := ControlRods{Insertion: 0.5, DemandInsertion: -1.0}
	for i := 0; i < int(60*time.Second/TimeStep); i++ {
		low = low.Step()
	}
	assert.GreaterOrEqual(t, low.Insertion, 0.0,
		"want Insertion >= 0, got %v", low.Insertion)
}

// The bridge Insertion control rejects values outside [0,1] so
// the rate-limit machinery is never bypassed by an out-of-range
// demand. Get returns the actual Insertion, so the rate-limit
// applied by ControlRods.Step drives the visible value over
// multiple Plant steps. The per-step rate-limit machinery is
// covered exhaustively by TestControlRods_StepRateLimits; here
// we verify the bridge path: Set writes DemandInsertion, and
// ControlRods.Step moves Insertion toward it.
//
// The full Plant.Step is avoided here because the cold-plant
// initial state would trip the RPS on rising SG level
// (cold-leg trickle flow from the RPV), and the trip clamp
// (plans/x/trip.md) would then coerce Insertion to 1.0 and
// mask the rate-limit. We drive ControlRods.Step directly to
// observe the bridge integration without that interaction.
func TestBridge_RodInsertionControlClamps(t *testing.T) {
	p := MakePlant()
	rods := &p.ReactorBuilding.ReactorPressureVessel.ControlRods
	rods.Insertion = 0.5
	c := GetControlMap(&p)["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"]
	require.NotNil(t, c, "want rod insertion control registered")
	require.Error(t, c.Set("-0.1"),
		"want Set(-0.1) to error, got nil")
	require.Error(t, c.Set("1.2"),
		"want Set(1.2) to error, got nil")

	// Test rate-limit toward intermediate withdrawal.
	require.NoError(t, c.Set("0.1"))
	for i := 0; i < int(20*time.Second/TimeStep); i++ {
		*rods = rods.Step()
	}
	assert.InDelta(t, 0.1, rods.Insertion, 0.01,
		"want Insertion≈0.1 after Set(0.1) and 20 s of ControlRods.Step, got %v",
		rods.Insertion)

	// Test rate-limit toward intermediate insertion.
	require.NoError(t, c.Set("0.9"))
	for i := 0; i < int(20*time.Second/TimeStep); i++ {
		*rods = rods.Step()
	}
	assert.InDelta(t, 0.9, rods.Insertion, 0.01,
		"want Insertion≈0.9 after Set(0.9) and 20 s of ControlRods.Step, got %v",
		rods.Insertion)
}

// BoronSetpoint writes through to CVCS so the next step's
// CVCS→RPV propagation does not overwrite the operator demand.
func TestBridge_BoronSetpointWritable(t *testing.T) {
	p := MakePlant()
	c := GetControlMap(&p)["ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint"]
	require.NotNil(t, c, "want BoronSetpoint control registered")
	require.NoError(t, c.Set("800"))
	stepN(&p, 1)
	got, err := GetSensorMap(&p)["ReactorBuilding/ReactorPressureVessel/BoronConcentration"].Get()
	require.NoError(t, err)
	assert.Equal(t, "800", got,
		"want BoronConcentration=800 after BoronSetpoint, got %v", got)
}

// Sensors for RPV inlet/outlet, SG heat extraction, and RCP
// speed all return non-zero values after a few steps because
// each part has a live dynamics source. The RCP needs to be
// started manually (MakePlant leaves it off) before its speed
// sensor reads non-zero.
func TestBridge_SensorsReflectState(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPump.Running = true
	stepN(&p, int(15*time.Second/TimeStep))
	sensors := GetSensorMap(&p)
	for _, id := range []string{
		"ReactorBuilding/ReactorPressureVessel/Outlet",
		"ReactorBuilding/ReactorPressureVessel/Inlet",
		"ReactorBuilding/SteamGenerator/HeatInput",
		"ReactorBuilding/ReactorCoolantPump/Speed",
	} {
		s, ok := sensors[id]
		require.Truef(t, ok, "want sensor %q registered", id)
		out, err := s.Get()
		require.NoErrorf(t, err, "Get %q", id)
		assert.NotEqualf(t, "0", out,
			"want non-zero reading from sensor %q after stepping, got %v", id, out)
	}
}

// The SVG outlet-temperature overlay is exercised in
// core/svg/svg_test.go (TestSVG_RPVTempOverlay) so it can
// access the package-private constants.

// Withdrawing the rods raises outlet temperature: at lower
// rod insertion the rodReactivity term becomes less negative,
// the power correction rises, and the outlet relaxes to a
// higher target. The test runs two plants at different starting
// rod insertions and compares after 5 s of simulated time.
// DemandInsertion is set equal to Insertion so the rod drive
// does not move during the test. Both plants are seeded at
// CoreRatedThermalPower so the rod reactivity term modulates
// power from step 1; otherwise a cold plant (power=0) would
// have no power for the rod term to affect.
func TestThermal_RodWithdrawalRaisesOutlet(t *testing.T) {
	const steps = int(5 * time.Second / TimeStep)

	// Insertion 0.7 = rods 30% withdrawn (mostly inserted).
	low := MakePlant()
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0.7
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0.7
	low.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	stepN(&low, steps)

	// Insertion 0.2 = rods 80% withdrawn (mostly out).
	high := MakePlant()
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0.2
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0.2
	high.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	stepN(&high, steps)

	lowOut := low.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	highOut := high.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	assert.Less(t, lowOut, highOut,
		"want outlet at Insertion=0.7 (%v) < outlet at Insertion=0.2 (%v)",
		lowOut, highOut)
}

// Diluting boron (setpoint below current concentration, CVCS
// in Dilution mode) raises outlet temperature. The CVCS
// dilution path reduces RCS boron, the boron reactivity
// term becomes less negative, and the thermal model relaxes
// to a higher outlet target.
func TestThermal_BoronDilutionRaisesOutlet(t *testing.T) {
	const steps = int(30 * time.Second / TimeStep)

	base := MakePlant()
	stepN(&base, steps)

	diluted := MakePlant()
	diluted.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSDilution
	diluted.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
	// Drop the boron concentration so the reactivity term
	// shifts positive (less negative).
	diluted.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = CVCSInitialBoron - 500
	stepN(&diluted, steps)

	baseOut := base.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	dilOut := diluted.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	assert.Greater(t, dilOut, baseOut,
		"want diluted outlet (%v) > baseline outlet (%v)",
		dilOut, baseOut)
}
