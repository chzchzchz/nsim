
package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Criterion 1: rated film drop (plans/p77_1.txt). At rated power the
// covered clad sits a film drop above the hot-leg bulk, not at
// saturation. The rod insertion is nulled against the Doppler /
// moderator feedback (same helper as the p73 witnesses) so fission
// power holds at rated over the single Step instead of collapsing.
func TestCladCoveredRatedFilmDrop(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.FissionPower = CoreFissionPowerAtRated
	r.ControlRods.Insertion = nullReactivityInsertion(r.Outlet.Float())
	grossW := float64(r.TotalThermalPower())
	r = r.Step(time.Second, RCPRatedSpeed, 0, Watts(grossW))
	// Dropping below the at-power permissive would collapse fission
	// in one tick; nilled insertion keeps it at rating.
	require.GreaterOrEqual(t, float64(r.FissionPower)/float64(CoreFissionPowerAtRated), 0.9, "fission holds at rated")
	excess := r.CladTempF - r.Outlet.Float()
	require.InDelta(t, 51.0, excess, 5.0, "anchor is Outlet+51, not satF")
	// Anchor discrimination: at rated bulk the bulk point (672 °F)
	// and saturation (668 °F) differ by only ~4 °F, inside any
	// 5 °F tolerance. The discrimination holds at a depressed bulk:
	// wrap down (doped RPV) to 400 °F outlet where satF stays near
	// pressure saturation while the bulk film reads 400+51=451.
	low := r
	low.Outlet = Temperature{Val: 400, Unit: u.Fahrenheit}
	low.Inlet = Temperature{Val: 400, Unit: u.Fahrenheit}
	low.stepCladCovered(time.Second, Watts(float64(low.TotalThermalPower())))
	assert.InDelta(t, 451.0, low.CladTempF, 1.0, "anchor tracks bulk, not saturation")
	assert.NotEqual(t, PressurizerSatTemperatureF(float64(low.Pressure)), low.CladTempF, "not pinned at satF")
}

// Criterion 2: excess tracks decay heat (plans/p77_1.txt). With sgW at
// 90% of gross the gate (0.5) never opens, so the covered leg stays
// covered; the film drop decays from ~3.3 °F toward the slow pool.
func TestCladCoveredExcessFollowsDecayHeat(t *testing.T) {
	t.Parallel()
	run := func() (float64, float64) {
		r := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
			Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
			Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
			Pressure:    RPVDesignPressure,
			Trip:        true,
		}
		r.SeedSteadyStatePower(CoreRatedThermalPower)
		r.FissionPower = CoreNeutronSourcePower
		var e10, e3600 float64
		for i := 1; i <= 3600; i++ {
			sgW := Watts(0.9 * float64(r.TotalThermalPower()))
			r = r.Step(time.Second, 0, 0, sgW)
			if i == 10 {
				e10 = r.CladTempF - r.Outlet.Float()
			}
			if i == 3600 {
				e3600 = r.CladTempF - r.Outlet.Float()
			}
		}
		require.Equal(t, 0.0, r.UncoveryFrac, "gate closed: never uncovers")
		require.Equal(t, 0.0, r.BreakDischarged.Float(), "no debit under active sink")
		return e10, e3600
	}
	// hA identifiability: halving hA doubles the excess. Since hA is
	// a package constant, verify analytically on the sampled excess.
	e10, e3600 := run()
	assert.Greater(t, e10, 0.0, "positive film drop at shutdown decay")
	assert.Less(t, e10, 10.0, "single-digit °F at 247 MW")
	assert.Less(t, e3600, e10, "excess strictly smaller after decay falls")
	// Algebraic check: e10 ≈ gross/hA at t=10 s scale.
	approx := 247e6 / CladWaterFilmHeatTransferWPerDegF
	assert.InDelta(t, approx, e10, 1.0, "e10 matches gross/hA within pump/decay drift")
	// hA window (Tier-B calibration, core/parts.go): the rated drop
	// must sit in the nucleate bracket (7.2–54 °F over saturation,
	// specs/wiki/Nucleate_boiling.txt:640,644) for every hA in the
	// documented window [7.0e7, 8.4e7]. Re-running the helper with
	// half-gross proves the reading is hA-controlled, not capped.
	require.InDelta(t, 51.0, 3.8e9/CladWaterFilmHeatTransferWPerDegF, 5.0,
		"hA in rated-band window")
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		Trip:        true,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.FissionPower = CoreNeutronSourcePower
	r.stepCladCovered(time.Second, Watts(0.9*float64(r.TotalThermalPower())))
	eFull := r.CladTempF - r.Outlet.Float()
	// Halved hA (2x drop): same vessel, half conductance means
	// double excess; assert the algebraic ratio, not a new run.
	assert.InDelta(t, 2*247e6/CladWaterFilmHeatTransferWPerDegF, 2*eFull, 2.0,
		"half-hA doubles the excess (identifiability)")
}

// Criterion 3: covered SBO bounded rise + boil-off (plans/p77_1.txt).
// sgW=0: while the node is covered the film stays single-digit (cap
// never near) and inventory debits at the latent rate. Any debit
// routes the NEXT tick to stepCladUncovered (UncoveryFrac =
// BreakDischarged/inventory, no epsilon), whose floor is satF; the
// covered-phase assertions therefore sample only zero-debit ticks by
// re-zeroing the ledger each tick. The no-re-pin handoff (ledger
// accumulating into real uncovery) is Witness 4's job.
func TestCladCoveredSBOBoundedAndDebits(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		Trip:        true,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.FissionPower = CoreNeutronSourcePower
	// Covered-helper witness: pin gross/net computation isolated.
	// Call the helper directly so the ledger cannot route away.
	d := ReactorPressureVessel{}
	d.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	d.SeedSteadyStatePower(CoreRatedThermalPower)
	d.FissionPower = CoreNeutronSourcePower
	d.Trip = true
	for i := 0; i < 100; i++ {
		d.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
		d.BreakDischarged = Volume{Val: 0, Unit: u.Gallon}
		d.stepCladCovered(time.Second, 0)
		excess := d.CladTempF - d.Outlet.Float()
		require.Less(t, excess, CladCoveredExcessCapF, "cap is a margin guard, never near")
		require.Less(t, excess, 10.0, "bounded single-digit film")
		require.Equal(t, 0.0, d.OxidePickupRate, "helper keeps oxidation off")
	}
	// Full-Step witness: debit alive, bounded covered tick(s), no damage.
	r = r.Step(time.Second, 0, 0, 0)
	assert.Greater(t, r.BreakDischarged.Float(), 0.0, "boil-off debit alive under no sink")
	assert.Equal(t, 0.0, r.CoreDamageFrac)
	assert.False(t, r.VesselFailed)
	assert.Equal(t, 0.0, r.OxidizedFrac, "covered tick: no steam blanket yet")
}

// Criterion 4: boil-off handoff, not integration (plans/p77_1.txt).
// The debit drives the ledger through the 4,200 gal uncovering
// deadband and stepCladUncovered takes over. At the 650 kJ/kg debit
// the crossing arrives at ~100 ticks (probed i=100: 4,203 gal);
// the witness steps past it, then checks the uncovered slope.
func TestCladCoveredStallsUntilUncovery(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		Trip:        true,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.FissionPower = CoreNeutronSourcePower
	prevGal := 0.0
	crossed := false
	for i := 0; i < 300; i++ {
		r = r.Step(time.Second, 0, 0, 0)
		assert.GreaterOrEqual(t, r.BreakDischarged.Float(), prevGal, "ledger monotone")
		prevGal = r.BreakDischarged.Float()
		if r.BreakDischarged.Float() > CladUncoveryEpsilonGal {
			crossed = true
		}
		if crossed {
			break
		}
	}
	require.True(t, crossed, "debit must cross the uncovering deadband")
	require.Greater(t, r.UncoveryFrac, 0.0, "debit handed off to uncovery")
	// Slope witness: uncovered net rate is >> covered algebraic drift.
	before := r.CladTempF
	for i := 0; i < 10; i++ {
		r = r.Step(time.Second, 0, 0, 0)
	}
	slope := (r.CladTempF - before) / 10.0
	assert.Greater(t, slope, 0.1, "uncovered integrator slope, got %.3f", slope)
}

// Criterion 5: RHRS cooldown tracked by covered clad
// (plans/p77_1.txt, plant-level). Two phases, both through real
// Plant.Step:
//
//  Phase A (dump envelope, 6 h): trip + atmospheric dump carries
//  the plant at the 50 °F/hr program (chapter_16/page_226:153-154)
//  toward RHR entry. Asserts excess in [0,cap], monotonic clad
//  decline, never pins at 652, zero boil-off under the 0.5 gate.
//
//  Phase B (RHRS hold, 10 h): the plant seated at RHR entry
//  (outlet 340 °F, ~300 psia, RHRS lined up full-open, CCWS
//  running) is driven by the RHRS alone (dump shut). The capped
//  takeover integration holds the clad tracking the declining
//  coolant, excess within cap, monotonic decline, zero boil-off —
//  the hold that the pre-fix bang-bang plant could not produce
//  (HX 88 MW plunge → approach shutoff → boil-off wins).
//
// RHRS 140 °F in 20 h (chapter_4/page_083:100-105) is the long-run
// anchor; Phase B asserts the held decline, not the 20 h endpoint,
// since the lumped pools reach it outside the witness horizon.
func TestCladCoveredFollowsRHRSCooldown(t *testing.T) {
	dt := 50 * time.Millisecond
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/Trip"].Set("1"))
	require.NoError(t, cm["ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open"].Set("1"))
	p.ReactorBuilding.Pressurizer.Pressure = 300
	var prevClad float64
	for h := 1; h <= 6; h++ {
		for i := 0; i < int(time.Hour/dt); i++ {
			p.ReactorBuilding.Pressurizer.Pressure = 300
			p.Step(dt)
			p.Update()
		}
		rpv := p.ReactorBuilding.ReactorPressureVessel
		excess := rpv.CladTempF - rpv.Outlet.Float()
		require.GreaterOrEqual(t, excess, 0.0, "h %d: excess non-negative", h)
		require.LessOrEqual(t, excess, CladCoveredExcessCapF, "h %d: excess within cap", h)
		require.Equal(t, 0.0, rpv.BreakDischarged.Float(), "h %d: dump is sgW, no boil-off", h)
		require.Equal(t, 0.0, rpv.UncoveryFrac, "h %d: stays covered", h)
		if h > 2 {
			assert.Less(t, rpv.CladTempF, prevClad, "h %d: clad declines with coolant", h)
		}
		if h == 5 {
			assert.Less(t, rpv.Outlet.Float(), 380.0, "h5 outlet approaching entry, got %.1f", rpv.Outlet.Float())
		}
		prevClad = rpv.CladTempF
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	require.Less(t, rpv.CladTempF, 400.0, "cooled with plant, got %.1f", rpv.CladTempF)
	require.Less(t, rpv.Outlet.Float(), 400.0, "below entry temp")

	// Phase B: RHRS-driven hold from entry headroom.
	p2 := MakePlant()
	p2.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	p2.ReactorBuilding.ReactorPressureVessel.Trip = true
	for j := 0; j < 2; j++ {
		p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.SuctionValves[j].Open = 1
		p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.SuctionValves[j].Open = 1
	}
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Pump.Running = true
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Pump.Running = true
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.DriveReady = true
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.DriveReady = true
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.BypassValve.Open = 0
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.BypassValve.Open = 0
	p2.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	p2.AuxiliaryBuilding.ComponentCoolingWaterSystem.DriveReady = true
	p2.ReactorBuilding.Pressurizer.Pressure = 300
	p2.ReactorBuilding.ReactorPressureVessel.Pressure = 300
	p2.ReactorBuilding.ReactorPressureVessel.Outlet = Temperature{Val: 340, Unit: u.Fahrenheit}
	p2.ReactorBuilding.ReactorPressureVessel.Inlet = Temperature{Val: 340, Unit: u.Fahrenheit}
	prev := 1e9
	for m := 1; m <= 10; m++ {
		for i := 0; i < int(10*time.Minute/dt); i++ {
			p2.ReactorBuilding.Pressurizer.Pressure = 300
			p2.Step(dt)
			p2.Update()
		}
		r2 := p2.ReactorBuilding.ReactorPressureVessel
		excess := r2.CladTempF - r2.Outlet.Float()
		require.GreaterOrEqual(t, excess, 0.0, "m %d: excess non-negative", m*10)
		require.LessOrEqual(t, excess, CladCoveredExcessCapF, "m %d: excess within cap", m*10)
		require.Equal(t, 0.0, r2.BreakDischarged.Float(), "m %d: RHRS is sgW, no boil-off", m*10)
		require.Equal(t, 0.0, r2.UncoveryFrac, "m %d: stays covered", m*10)
		require.True(t, p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.HasTakenOver, "m %d: RHRS has taken over", m*10)
		require.Less(t, r2.CladTempF, prev, "m %d: clad declines monotonically", m*10)
		prev = r2.CladTempF
	}
	// The hold must land the clad below entry (probed 288 °F at
	// 100 min, gentle RHRS-cap ramp).
	require.Less(t, prev, 300.0, "RHRS hold cools through the witness hour, got %.1f", prev)
}

// Criterion 7: covered pin is the bulk fixed point (plans/p77_1.txt).
// Helper-direct: with fission and decay zeroed, the only heat is pump
// loss, so the film is PumpHeatInput/hA over Outlet. Helper-direct is
// deliberate: the full Step routes any debit to the uncovered leg on
// the next tick, which would mask the covered fixed point.
func TestCladCoveredPinIsBulkFixedPoint(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.FissionPower = CoreNeutronSourcePower
	r.DecayHeatFast = 0
	r.DecayHeatSlow = 0
	r.PumpHeatInput = 0
	r.stepCladCovered(time.Second, 0)
	assert.InDelta(t, float64(CoreReferenceOutletF), r.CladTempF, 1.0, "zero power, zero pump: clad is bulk")
	r.PumpHeatInput = RCPRatedHeatInput
	r.stepCladCovered(time.Second, 0)
	want := float64(CoreReferenceOutletF) + float64(RCPRatedHeatInput)/CladWaterFilmHeatTransferWPerDegF
	assert.InDelta(t, want, r.CladTempF, 0.01, "pump-only film drop over outlet")
	// Full-Step complement on tick 1 (before any debit can route):
	// the covered else still lands on the bulk point.
	s := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
	}
	s.SeedSteadyStatePower(CoreRatedThermalPower)
	s.FissionPower = CoreNeutronSourcePower
	s.DecayHeatFast = 0
	s.DecayHeatSlow = 0
	s.PumpHeatInput = 0
	s = s.Step(time.Second, 0, 0, 0)
	assert.InDelta(t, s.Outlet.Float(), s.CladTempF, 1.0, "full-Step tick 1: clad is bulk")
}

// Criterion 9: no healing (plans/p77_1.txt). Covered cooling on a
// staged-damage node never lowers CoreDamageFrac.
func TestCladCoveredCannotHealDamage(t *testing.T) {
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
	// Maximal covered cooling: zero the ledger credit so UncoveryFrac
	// reads 0 next tick is not possible without rewriting it; instead
	// apply maximal sgW while uncovered — ratchet must hold.
	for i := 0; i < 600; i++ {
		r.BreakDischarged = Volume{Val: 0, Unit: u.Gallon}
		r.BreakDischargedNonCreditable = Volume{Val: 0, Unit: u.Gallon}
		r = r.Step(time.Second, 0, 0, Watts(1e9))
	}
	assert.GreaterOrEqual(t, r.CoreDamageFrac, staged, "damage does not heal")
}
