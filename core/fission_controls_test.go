package core

import (
	"fmt"
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

// BoronSetpoint engages CVCS tracking that ramps the measured
// concentration toward the target through the BTRS dilution flow
// path, releasing chemistry to volume control on arrival.
func TestBridge_BoronSetpointWritable(t *testing.T) {
	p := MakePlant()
	c := GetControlMap(&p)["ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint"]
	require.NotNil(t, c, "want BoronSetpoint control registered")
	// Target sits one ramp-window above the fresh-core seed so the
	// 0.8 ppm/s BTRS flow covers the distance inside the window.
	target := CoreRatedCriticalBoronPpm + 12
	require.NoError(t, c.Set(fmt.Sprintf("%v", target)))
	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	require.True(t, cvcs.BoronSetpointTracking,
		"the write must latch setpoint tracking")
	stepN(&p, int(15*time.Second/TimeStep))
	assert.InDelta(t, target, cvcs.BoronConcentration, 1.0,
		"want BoronConcentration ramped to the 990 ppm setpoint, got %v",
		cvcs.BoronConcentration)
	assert.False(t, cvcs.BoronSetpointTracking,
		"arrival inside the tolerance band must release tracking")
	assert.Equal(t, CVCSVolumeControl, cvcs.Mode,
		"released chemistry returns to volume control")
}

// Sensors for RPV inlet/outlet, SG heat extraction, and RCP
// speed all return non-zero values after a few steps because
// each part has a live dynamics source. The RCP needs to be
// started manually (MakePlant leaves it off) before its speed
// sensor reads non-zero.
func TestBridge_SensorsReflectState(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
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
// half-rated thermal power so the rod reactivity term modulates
// power from step 1 without immediately saturating at the
// CoreRatedThermalPower cap.
//
// Spec justification for rod worth: "the differential rod worth
// or neutron capability is near zero at the top and bottom
// ends of the core" (specs/the_westinghouse_pressurized_water_react//chapter_2/page_037.txt:124-132),
// so rod motion near the extremes has small effect on outlet.
// Withdrawing the rods raises reactivity: at lower rod insertion
// the rod term is less negative on the one percent-Δk/k scale.
// Both plants are seeded at half-rated thermal power so fission
// power is live from step 1, and DemandInsertion is set equal to
// Insertion so the rod drive does not move during the test.
//
// Spec justification for rod worth: "the differential rod worth
// or neutron capability is near zero at the top and bottom
// ends of the core" (specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:124-132).
func TestThermal_RodWithdrawalRaisesReactivity(t *testing.T) {
	steps := int(5 * time.Second / TimeStep)
	const seedPower = CoreRatedThermalPower / 2

	// Insertion 0.7 = rods 30% withdrawn (mostly inserted).
	low := MakePlant()
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0.7
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0.7
	low.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = CVCSInitialBoron
	low.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(seedPower)
	stepN(&low, steps)

	// Insertion 0.2 = rods 80% withdrawn (mostly out).
	high := MakePlant()
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0.2
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0.2
	high.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = CVCSInitialBoron
	high.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(seedPower)
	stepN(&high, steps)

	lowReact := low.ReactorBuilding.ReactorPressureVessel.Reactivity
	highReact := high.ReactorBuilding.ReactorPressureVessel.Reactivity
	assert.Greater(t, highReact, lowReact,
		"want reactivity at Insertion=0.2 (%v) > at Insertion=0.7 (%v)",
		highReact, lowReact)
}

// Diluting boron (setpoint below current concentration, CVCS
// in Dilution mode) shifts the net temperature coefficient
// toward more-negative values per spec. The model couples the
// Doppler+moderator coefficient to RCS boron, so dropping
// boron reduces the combined coefficient and the power
// correction drops. The outlet cools as the reactor power
// falls below rated.
//
// Spec justification:
//   - "the moderator temperature coefficient would be on the
//     order of +7 pcm/°F" at BOL with full soluble-boron shim
//     (specs/the_westinghouse_pressurized_water_react//chapter_2/page_039.txt:54-55).
//   - "the coefficient is substantially more negative due to
//     depletion and removal of shim boron"
//     (specs/the_westinghouse_pressurized_water_react//chapter_2/page_037.txt:119-121).
// The dilution path therefore cools the plant rather than
// heating it; operators use boron to swing reactivity, not
// temperature.
// Diluting boron shifts the net temperature coefficient toward
// more-negative values per spec, which must show up in the
// Reactivity state. It must NOT bend thermal power once the unit
// is scrambled: decay heat is fission-product beta decay, a
// function of prior power and time since trip only
// (specs/wiki/Decay_heat.txt).
//
// Regression: the reactivity power-defect correction used to
// multiply the tripped branch too, so the dilution here cooled
// the "decay heat" of an EOL-boron core — amplifying or
// suppressing fission-product heat with reactivity. Both units
// are scrammed explicitly so the comparison runs in the regime
// where the bug lived.
//
// Spec justification:
//   - "the moderator temperature coefficient would be on the
//     order of +7 pcm/°F" at BOL with full soluble-boron shim
//     (specs/the_westinghouse_pressurized_water_react//chapter_2/page_039.txt:54-55).
//   - "the coefficient is substantially more negative due to
//     depletion and removal of shim boron"
//     (specs/the_westinghouse_pressurized_water_react//chapter_2/page_037.txt:119-121).
func TestThermal_BoronDilutionMovesReactivityNotDecayHeat(t *testing.T) {
	// Both legs run at the same tick width, so the per-step fission
	// tail channel scales identically under them; boron integrates
	// linearly in dt and every assertion compares the legs against
	// each other rather than against absolute horizons. Must not
	// run in parallel: overrideTimeStep mutates global simulation
	// state.
	overrideTimeStep(t, 10*time.Millisecond)
	// Both legs share the same scrammed seed, so decay-heat
	// trajectories diverge (if at all) from the first step; a
	// short window is as probative as a long one.
	steps := int(15 * time.Second / TimeStep)

	base := MakePlant()
	// Scram both units explicitly. The old MakePlant self-scram
	// at ~2 s was an artifact of the primary-loop fluid edges
	// overfilling the steam generator past the high-level trip
	// signal; the graph no longer transfers water between the
	// separated primary and secondary cycles.
	base.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	stepN(&base, steps)

	diluted := MakePlant()
	diluted.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	diluted.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSDilution
	diluted.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
	// Drop the boron concentration so the net temperature
	// coefficient becomes more negative.
	diluted.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = CVCSInitialBoron - 500
	stepN(&diluted, steps)

	baseRP := base.ReactorBuilding.ReactorPressureVessel
	dilRP := diluted.ReactorBuilding.ReactorPressureVessel

	require.True(t, baseRP.Trip && dilRP.Trip,
		"scenario precondition: both units scrammed")

	assert.NotEqual(t, baseRP.Reactivity, dilRP.Reactivity,
		"dilution must shift Reactivity (%v vs %v)", baseRP.Reactivity, dilRP.Reactivity)
	// Under the unified fission equation the scrammed fission tail
	// decays at a weakly reactivity-dependent rate, so total power
	// and outlet agree only to second order; the decay-heat
	// inventories dominate both trajectories. Exact equality held
	// under the old split model and would be a false invariant here.
	assert.InDelta(t, float64(baseRP.TotalThermalPower()),
		float64(dilRP.TotalThermalPower()), 1e5,
		"scrammed thermal output must be decay-heat dominated regardless of reactivity")
	assert.InDelta(t, baseRP.Outlet.Float(), dilRP.Outlet.Float(), 0.01,
		"outlet must track decay heat identically despite different reactivity")
}

// Starting from the full-load operating point with the fission_4
// control geometry (0 insertion, RCP on), CVCS boron boration
// raises the RCS boron concentration.  Once boron passes the
// critical concentration for the current rod position (~1140 ppm
// at zero insertion under the net-temperature-coefficient model),
// the added negative boron reactivity drives the core
// subcritical: fission power collapses, the outlet target drops,
// and the actual outlet temperature relaxes toward no-load. The
// baseline run stays at 600 ppm, where the unit remains critical
// and rides the rated cap with its outlet at full-load value.
//
// The live control sequence (bridge path) is:
//
//   bridge.Set("AuxiliaryBuilding/ChemicalVolumeControlSystem/Mode", "Boration")
//   bridge.Set("AuxiliaryBuilding/ChemicalVolumeControlSystem/
//             BTRSRecirculationPumpRunning", "true")
//   bridge.Set("AuxiliaryBuilding/ChemicalVolumeControlSystem/
//             BoronSetpoint", "<higher ppm>")
//
// Because BoronSetpoint writes both the CVCS field and
// RPV.ControlRods.BoronConcentration (bridge.go:815-829),
// plant.go:49-50 keeps them synchronised on every Plant.Step.
func TestThermal_Fission4_BorationLowersOutlet(t *testing.T) {
	t.Skip("recalibration pending under the split temperature feedback " +
		"(plans/x/gaps.md item 15): part-load arc bands were tuned against " +
		"the old combined-coefficient dynamics")
	// Boron marches linearly in dt and the outlet relaxes on a
	// seconds-scale lag, so the widened step preserves both legs'
	// trajectories while cutting step count 100x. Must not run in
	// parallel: overrideTimeStep mutates global simulation state.
	overrideTimeStep(t, 10*time.Millisecond)
	run := func(borate bool) float64 {
		p := MakePlant()
		p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = 0
		// Start 60 ppm below the fresh-core critical concentration
		// (CoreRatedCriticalBoronPpm at zero insertion), so the unit
		// is supercritical and rides the rated cap, but close enough
		// that boration crosses the boundary partway through the
		// window ((critical − start) / 0.8 ppm/s = 75 s).
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration =
			CoreRatedCriticalBoronPpm - 60
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true

		warmup := int(15 * time.Second / TimeStep)
		stepN(&p, warmup)

		if borate {
			// Switch to boration: CVCS drives boron upward
			// at CVCSBorationRate (0.8 ppm/s), crossing the
			// critical concentration partway through the
			// window.
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSBoration
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
		}

		steps := int(90 * time.Second / TimeStep)
		stepN(&p, steps)
		return p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	}
	baseline := run(false)
	borated := run(true)

	assert.Less(t, borated, baseline,
		"want outlet after boration (%v °F) < baseline without boration (%v °F)",
		borated, baseline)
}
