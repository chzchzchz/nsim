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
// (Position clamp, BoronSetpoint, RPV/SG/RCP sensors) are
// exposed through the bridge. These tests verify each entry
// exists, the rate limit fires, and the thermal model sees
// the resulting reactivity change.

// At a single 1 ms timestep the rod can move by no more than
// RodInsertionSpeed * TimeStep = 5e-5 fraction. A demand jump
// from 1.0 to 0.0 is therefore rate-limited to that exact
// amount per call, not the full 1.0. Step moves Position
// toward DemandPosition.
func TestControlRods_StepRateLimits(t *testing.T) {
	c := ControlRods{Position: 1, DemandPosition: 0}
	out := c.Step()
	want := RodInsertionSpeed * TimeStep.Seconds()
	assert.InDelta(t, want, 1-out.Position, 1e-12,
		"want 1-Position = %v after one Step from full-withdrawal, got %v",
		want, 1-out.Position)

	// 20 s of stepping should drive 1.0 → 0.0; the rate limit
	// is 5% of travel per second, so 20 s covers the full
	// travel with margin.
	c2 := ControlRods{Position: 1, DemandPosition: 0}
	for i := 0; i < int(20*time.Second/TimeStep); i++ {
		c2 = c2.Step()
	}
	assert.InDelta(t, 0, c2.Position, 1e-9,
		"want Position=0 after 20 s of stepping, got %v", c2.Position)
}

// Step clamps the result to [0,1] so a malicious or
// out-of-range demand cannot drive Position outside the
// physical rod travel.
func TestControlRods_StepClamps(t *testing.T) {
	high := ControlRods{Position: 0.5, DemandPosition: 2.0}
	for i := 0; i < int(60*time.Second/TimeStep); i++ {
		high = high.Step()
	}
	assert.LessOrEqual(t, high.Position, 1.0,
		"want Position <= 1, got %v", high.Position)

	low := ControlRods{Position: 0.5, DemandPosition: -1.0}
	for i := 0; i < int(60*time.Second/TimeStep); i++ {
		low = low.Step()
	}
	assert.GreaterOrEqual(t, low.Position, 0.0,
		"want Position >= 0, got %v", low.Position)
}

// The bridge Position control rejects values outside [0,1] so
// the rate-limit machinery is never bypassed by an out-of-range
// demand. Get returns the actual Position (after rate-limit),
// so a successful Set only takes effect after enough Plant
// steps have advanced the rod drive.
func TestBridge_RodPositionControlClamps(t *testing.T) {
	p := MakePlant()
	c := GetControlMap(&p)["ReactorBuilding/ReactorPressureVessel/ControlRods/Position"]
	require.NotNil(t, c, "want rod position control registered")
	require.Error(t, c.Set("-0.1"),
		"want Set(-0.1) to error, got nil")
	require.Error(t, c.Set("1.2"),
		"want Set(1.2) to error, got nil")
	require.NoError(t, c.Set("1"))
	stepN(&p, int(30*time.Second/TimeStep))
	assert.Equal(t, "1", c.Get(),
		"want Position=1 after Set(1) and 30 s of stepping, got %q", c.Get())
	require.NoError(t, c.Set("0"))
	stepN(&p, int(30*time.Second/TimeStep))
	assert.Equal(t, "0", c.Get(),
		"want Position=0 after Set(0) and 30 s of stepping, got %q", c.Get())
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

// Withdrawing the rods raises outlet temperature: at higher
// rod-bank position the rodReactivity term becomes less
// negative, the power correction rises, and the outlet relaxes
// to a higher target. The test runs two plants at different
// starting rod positions and compares after 5 s of simulated
// time. DemandPosition is set equal to Position so the rod
// drive does not move during the test.
func TestThermal_RodWithdrawalRaisesOutlet(t *testing.T) {
	const steps = int(5 * time.Second / TimeStep)

	low := MakePlant()
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 0.3
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandPosition = 0.3
	stepN(&low, steps)

	high := MakePlant()
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 0.8
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandPosition = 0.8
	stepN(&high, steps)

	lowOut := low.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	highOut := high.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	assert.Less(t, lowOut, highOut,
		"want outlet at rods=0.3 (%v) < outlet at rods=0.8 (%v)",
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
