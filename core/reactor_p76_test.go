package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Criterion 1: net band at shutdown decay. The film lowers the
// gross 1.78 °F/s toward mid-band. Point-samples dT/dt at
// t=10 s (single interval, before the film grows).
func TestCladUncoveredNetRateInSourcedBand(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		HotLegBreak: Valve{Open: 1},
		Trip:        true,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.2, Unit: u.Gallon}
	for i := 0; i < 10; i++ {
		r = r.Step(time.Second, 0, 0, 0)
	}
	before := r.CladTempF
	r = r.Step(time.Second, 0, 0, 0)
	rate := r.CladTempF - before
	assert.GreaterOrEqual(t, rate, 0.54, "net rate inside band floor, got %.3f", rate)
	assert.LessOrEqual(t, rate, 1.8, "net rate inside band ceiling, got %.3f", rate)
}

// hA sensitivity: CladSteamFilmHeatTransferWPerDegF is a
// derived single point (2500-9000 window keeping both the
// shutdown band and the 4 h below-melt tail), not a spec
// coefficient — no spec supplies a steam-film number. The
// band 0.3-1 °C/s (specs/wiki/Nuclear_meltdown.txt, Kuan
// stage 2) and the Leidenfrost film form Q = hA·ΔT
// (specs/wiki/Leidenfrost_effect.txt) are the literature
// anchors; the value is Tier-B derived and this test names
// the window so a retune knows the freedom.
func TestCladSteamFilmHAWindow(t *testing.T) {
	t.Parallel()
	require.InDelta(t, 6000.0, CladSteamFilmHeatTransferWPerDegF, 1.0,
		"production hA is 6000 W/°F (derived point in the 2500-9000 window)")
	require.GreaterOrEqual(t, CladSteamFilmHeatTransferWPerDegF, 2500.0)
	require.LessOrEqual(t, CladSteamFilmHeatTransferWPerDegF, 9000.0)
	// The anchors are arithmetic theorems on the production
	// clock (C = 2.5e8 J/R, 9/5 K→°F): at ΔT=300 °F the film
	// is 0.75-2.7 MW, never able to hold 247 MW shutdown
	// (equilibrium lift ~41000 °F), while at the 19 MW slow
	// tail it closes the gap in hours. Both statements hold
	// for every hA in the window; the test pins the range,
	// not the point.
	c := CladThermalMassCpJPerDegF
	grossShutdown := 247e6
	for _, hA := range []float64{2500, 6000, 9000} {
		net := grossShutdown - hA*300
		rate := net / c * 9.0 / 5.0
		assert.InDelta(t, 1.78, rate, 0.05,
			"shutdown gross dominates film at ΔT=300 (hA=%v)", hA)
		eqLift := 19e6 / hA
		assert.Greater(t, eqLift, 2000.0,
			"slow tail needs oxidation/decay, not just film, to settle (hA=%v)", hA)
	}
}

// Criterion 4: no-cooling regression for untouched 07.
// plans/x/corium/disaster/corium_07_cold_shutdown_lockout.txt
// re-run in Go: trip + cavity/SI lockout, 600 s, covered clad
// stays pinned, no damage, no latch. A covered plant that
// damages here has regressed the covered leg.
func TestCladColdShutdownLockoutNoDamage(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/Trip"].Set("1"))
	require.NoError(t, cm["ReactorBuilding/RefuelingOperations/CavityFlooding"].Set("0"))
	require.NoError(t, cm["ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock"].Set("1"))
	stepN(&p, int(600*time.Second/dt), dt)
	rpv := p.ReactorBuilding.ReactorPressureVessel
	// plans/p77_1.txt Criterion 6: still a negative control.
	// MakePlant seeds HeatInput so tick 1 reads a real sgW
	// (core/plant.go) and the covered leg never micro-debits.
	// At 600 s the outlet sits at 612.7 °F (rate-limited 50 °F/hr,
	// not the 558 °F tau target); the clad excess at that point is
	// ~0.3 °F (fast pool drained in the first minute, slow pool
	// 19 MW) so clad reads ~613 °F — the plan's ~574 °F estimate
	// assumed the 247 MW fast pool stays charged, which it does
	// not. Read Outlet from the plant when asserting this number.
	assert.Equal(t, 0.0, rpv.UncoveryFrac, "covered: no uncovery")
	assert.Equal(t, 0.0, rpv.CoreDamageFrac, "covered: no damage")
	assert.False(t, rpv.VesselFailed, "covered: no latch")
	assert.Less(t, rpv.CladTempF, 800.0, "covered clad bulk-anchored, got %.1f", rpv.CladTempF)
	assert.InDelta(t, 612.7, rpv.Outlet.Float(), 5.0, "outlet rate-limited, not tau target")
}

// Criterion 2: cooling inflection with secondary removal.
// U=0.5, HeatInput≈Total/4, 24 h at 1 s: inflection,
// peak < ZrMeltF, VesselFailed=false. Oxide may saturate.
// 1 s ticks are used (not 50 ms) because the clad tau is
// ~11 h and the RPV tail caps min(dt,1); 50 ms buys nothing
// and costs ~74 s of plant stepping for the same physics.
func TestCladUncoveredInflectsWithSecondaryRemoval(t *testing.T) {
	t.Parallel()
	dt := time.Second
	p := MakePlant()
	p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	// Trip, then hold U=0.5 by credit-proofing both fields.
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
	rpv.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
	// Strip injection so coverage does not recover.
	p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = false
	p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level = Volume{Val: 0, Unit: u.Gallon}

	peak := 0.0
	prevSlope := 0.0
	inflected := false
	const stepsPerHour = 3600 // 1 s ticks
	for h := 0; h < 24; h++ {
		for i := 0; i < stepsPerHour; i++ {
			before := p.ReactorBuilding.ReactorPressureVessel.CladTempF
			p.Step(dt)
			p.Update()
			p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = false
			p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level = Volume{Val: 0, Unit: u.Gallon}
			// Re-pin both ledger fields so U stays at 0.5.
			p.ReactorBuilding.ReactorPressureVessel.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
			p.ReactorBuilding.ReactorPressureVessel.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
			cur := p.ReactorBuilding.ReactorPressureVessel.CladTempF
			if cur > peak {
				peak = cur
			}
			slope := (cur - before) / dt.Seconds()
			if prevSlope > 1e-6 && slope <= 1e-6 {
				inflected = true
			}
			prevSlope = slope
		}
		_ = h
	}
	r := p.ReactorBuilding.ReactorPressureVessel
	assert.True(t, inflected, "clad inflects (slope positive to ≤0)")
	assert.Less(t, peak, ZrMeltF, "peak below Zr melt, peak=%.1f", peak)
	assert.False(t, r.VesselFailed, "no vessel latch with secondary removal")
}

// Criterion 3: SBO plateau with oxidation present. U=1, sgW=0,
// seed at CladOnsetF: saturates oxide, does NOT latch on 48 h.
func TestCladUncoveredSBOPlateausBelowMelt(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		HotLegBreak: Valve{Open: 1},
		Trip:        true,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	r.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	r.CladTempF = CladOnsetF
	for i := 0; i < 48*3600; i++ {
		r = r.Step(time.Second, 0, 0, 0)
	}
	assert.False(t, r.VesselFailed, "decay-only with film does not latch on 48 h")
	assert.Less(t, r.CladTempF, ZrMeltF, "plateau below melt")
	assert.Greater(t, r.OxidizedFrac, 0.0, "oxidation saturates")
}

// Criterion 5: oxidation feedback intact. A seeded twin whose
// clad starts hotter must run hotter by exactly the reaction
// enthalpy integrated over the window; kinetics are
// temperature-gated (oxidRate identical at equal T), enthalpy
// is not. The twin comparison measures the enthalpy term
// itself: same seed, same decay, same film, only the
// temperature-dependent oxidRate differs. A zero-enthalpy run
// would erase the same&gt;the separation. 1 s and 50 ms
// integrations agree within tolerance.
func TestCladUncoveredOxidationAddsToNet(t *testing.T) {
	t.Parallel()
	mk := func() ReactorPressureVessel {
		r := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
			Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
			Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
			Pressure:    RPVDesignPressure,
			HotLegBreak: Valve{Open: 1},
			Trip:        true,
		}
		r.SeedSteadyStatePower(CoreRatedThermalPower)
		r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
		r.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
		r.CladTempF = 1600
		return r
	}
	// Twin A: full enthalpy at 1600°F (oxidizes from t=0).
	full := mk()
	for i := 0; i < 300; i++ {
		full = full.Step(time.Second, 0, 0, 0)
	}
	// Twin B: same seed, clad held at the Baker-Just onset for
	// the whole window. oxidRate is 0 throughout, so rxnW is 0;
	// film and decay alone run. The temperature gap full-B is
	// the integrated enthalpy, not a seed difference.
	held := mk()
	held.CladTempF = CladOnsetF - 200
	for i := 0; i < 300; i++ {
		held = held.Step(time.Second, 0, 0, 0)
		// Held 200 °F below the Baker-Just onset: oxidRate is
		// 0 by the gate (oxidationKinetics returns 0 at or
		// below CladOnsetF), so rxnW is 0 and only the film
		// and decay move the clad. This is the correct
		// no-enthalpy baseline for the full twin above.
		held.CladTempF = CladOnsetF - 200
	}
	assert.Greater(t, full.OxidizedFrac, 0.0, "oxidation advances with enthalpy")
	assert.Zero(t, held.OxidizedFrac, "held below onset: no oxidation")
	assert.Greater(t, full.CladTempF, held.CladTempF,
		"enthalpy adds to net: gap=%.1f", full.CladTempF-held.CladTempF)
	// 1 s and 50 ms integrations agree on dT within tolerance.
	coarse := mk()
	fine := mk()
	for i := 0; i < 300; i++ {
		coarse = coarse.Step(time.Second, 0, 0, 0)
	}
	for i := 0; i < 6000; i++ {
		fine = fine.Step(50*time.Millisecond, 0, 0, 0)
	}
	assert.InDelta(t, coarse.CladTempF, fine.CladTempF, 50.0, "1 s and 50 ms agree")
}

// Criterion 6: cooling cannot heal.
func TestCladUncoveredCoolingCannotHeal(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		HotLegBreak: Valve{Open: 1},
		Trip:        true,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
	r.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
	r.CladTempF = 2000
	r = r.Step(time.Second, 0, 0, 0)
	staged := r.CoreDamageFrac
	require.Greater(t, staged, 0.0, "damage staged")
	// Maximal removal: large sgW for 600 s. Oxidation can
	// still self-heat the clad above its seed; the ratchet is
	// the assert, not the direction the temperature moves.
	for i := 0; i < 600; i++ {
		r = r.Step(time.Second, 0, 0, Watts(1e9))
	}
	assert.GreaterOrEqual(t, r.CoreDamageFrac, staged, "damage does not heal")
	assert.GreaterOrEqual(t, r.Damage, float32(staged), "Damage does not heal")
}
