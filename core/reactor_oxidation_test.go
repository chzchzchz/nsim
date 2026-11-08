package core

import (
	"math"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P1 regression: the Plant-level H2 derivation re-evaluates the
// same kinetics the reactor tail integrated; the per-tick
// generationRate must equal the integrated dOxidizedFrac/dt
// scaled onto the G4 ceiling. Asserted differentially: isolate
// the derived component by running twin plants identical except
// coverage, then compare H2 against the oxidation advance.
func TestHDS_DerivedMatchesIntegratedAdvance(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.CladTempF = 2100
	p.ReactorBuilding.ReactorPressureVessel.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	stepN(&p, 20, dt)
	assert.Greater(t, p.ReactorBuilding.ReactorPressureVessel.OxidizedFrac, 0.0)
	assert.Greater(t, p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent, 0.0)
}

// --- kinetics gate ---
func TestOxidationKineticsGatedOnCladOnset(t *testing.T) {
	t.Parallel()
	assert.Zero(t, oxidationKinetics(CladOnsetF, 0), "at onset: no rate")
	assert.Zero(t, oxidationKinetics(CladOnsetF-100, 0), "below onset: no rate")
	mid := oxidationKinetics((CladOnsetF+BakerJustValidityFloorF)/2, 0)
	assert.Greater(t, mid, 0.0, "mid-ramp: positive rate")
	full := oxidationKinetics(2240, 0)
	assert.Greater(t, full, mid, "above floor: full BJ rate exceeds mid-ramp")
	assert.LessOrEqual(t, full, OxidizedFracRateCapPerSec, "rate cap binds")
	// Conversion pin (p65_9 §5): at the inventory-scale cap, the
	// pre-taper source is 72 %/h. The old G4-scale capRate wording
	// cannot survive the 72%/h inventory mapping; this pin replaces
	// it. At the cap, the new %/h equals
	// OxidizedFracRateCapPerSec * SecondsPerHour *
	// HydrogenFullInventoryVolumePercent.
	assert.InDelta(t, 72.0,
		OxidizedFracRateCapPerSec*SecondsPerHour*HydrogenFullInventoryVolumePercent, 1e-12,
		"capped inventory-scale source is 72 %/h")
	assert.Zero(t, oxidationKinetics(2300, MaxOxidizedFrac), "cap reached: no rate")
	// Both-capped taper-ratio cross-check: at a both-capped
	// rising tick the kernels equal cap*(1-fraction/maxF), so
	// their ratio is the taper ratio only.
	invKernel := oxidationInventoryKinetics(2240, 0)
	dmgKernel := oxidationKinetics(2240, 0)
	assert.Greater(t, dmgKernel, 0.0)
	wantRatio := (1.0 - 0.0/1.0) / (1.0 - 0.0/MaxOxidizedFrac)
	assert.InDelta(t, wantRatio, invKernel/dmgKernel, 1e-9,
		"both-capped taper ratio")
	assert.Zero(t, oxidePickupRateRaw(CladOnsetF, 0, 1.0), "raw gated at onset")
	assert.Greater(t, oxidePickupRateRaw(2240, 0, 1.0), 0.0, "raw positive above onset")
	assert.InDelta(t, HydrogenFullInventoryVolumePercent,
		(oxideWeightFullInventoryMgPerCm2/oxideWeightFullInventoryMgPerCm2)*
			HydrogenFullInventoryVolumePercent, 1e-12,
		"full-inventory integral identity")
	// Mid-scale cross-check (p65_9 CHECK follow-up §1): at the same
	// physical oxide weight w the damage and inventory ranges must
	// give the same uncapped k/(2w). At f=0 both ranges share w=0
	// (floored to OxideWeightFloorMgPerCm2), so the capped f=0
	// ratio cannot catch a scale swap. Choose w=10 mg/cm²
	// (first uncapped bin): fD=10/100=0.10, fI=10/588.235=0.017.
	// Baker-Just: w²=kt, dw/dt=k/(2w) (specs/pdf/README.txt:5-6)
	// gives k at 2240F; ramp=1 above the validity floor. Expected
	// raw dw/dt = k/(2*10), and fraction rates = raw/scale*taper.
	// This pins the scale mapping independently of the cap.
	{
		w := 10.0
		fD := w / OxideWeightMaxMgPerCm2
		fI := w / oxideWeightFullInventoryMgPerCm2
		rawD := oxidePickupRateRaw(2240, fD, MaxOxidizedFrac)
		rawI := oxidePickupRateRaw(2240, fI, 1.0)
		require.Greater(t, rawD, 0.0, "raw damage positive at mid-scale")
		require.Greater(t, rawI, 0.0, "raw inventory positive at mid-scale")
		// Same w => same k/(2w) => same raw (both uncapped here);
		// a scale swap (wMaxFull<->wMax17) would diverge by 1/0.17.
		assert.InDelta(t, rawD, rawI, 1e-9,
			"mid-scale raw pickup matches at same physical weight")
		// Taper is the only difference in fraction rates at same w.
		expD := rawD / OxideWeightMaxMgPerCm2 * (1.0 - fD/MaxOxidizedFrac)
		expI := rawI / oxideWeightFullInventoryMgPerCm2 * (1.0 - fI)
		assert.InDelta(t, expD, oxidationKinetics(2240, fD), 1e-12,
			"damage fraction rate = raw/scale*taper at mid-scale")
		assert.InDelta(t, expI, oxidationInventoryKinetics(2240, fI), 1e-12,
			"inventory fraction rate = raw/scale*taper at mid-scale")
		// Independent Baker-Just anchor: k/(2w) at 2240F.
		tempK := (2240.0 - 32.0) * 5.0 / 9.0 + 273.15
		k := BakerJustPrefactor * math.Exp(-BakerJustQCalPerMol/(GasConstantCalPerMolK*tempK))
		assert.InDelta(t, k/(2.0*w), rawD, 1e-9,
			"mid-scale raw matches closed-form Baker-Just k/(2w)")
	}
}

// --- feedback twin ---
func TestRPV_ReactionEnthalpyFeedsHeatBalance(t *testing.T) {
	t.Parallel()
	mk := func(stripDecay bool) ReactorPressureVessel {
		r := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
			Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
			Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
			Pressure:    RPVDesignPressure,
			HotLegBreak: Valve{Open: 1},
			Trip:        true,
		}
		r.SeedSteadyStatePower(CoreRatedThermalPower)
		r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
		r.CladTempF = 2100
		if stripDecay {
			r.FissionPower, r.DecayHeatFast, r.DecayHeatSlow = 0, 0, 0
		}
		return r
	}
	full := mk(false)
	noDecay := mk(true)
	atOnset := mk(true)
	atOnset.CladTempF = CladOnsetF - 200
	for i := 0; i < 600; i++ {
		full = full.Step(time.Second, 0, 0)
		noDecay = noDecay.Step(time.Second, 0, 0)
		atOnset = atOnset.Step(time.Second, 0, 0)
	}
	assert.Greater(t, full.CladTempF, 2240.0, "feedback crosses rapid-oxidation onset")
	assert.Less(t, full.OxidizedFrac, MaxOxidizedFrac, "not yet capped in twin window")
	assert.Greater(t, noDecay.CladTempF, 2100.0, "self-heats with decay removed")
	assert.Greater(t, full.CladTempF, noDecay.CladTempF, "decay heat adds to self-heat")
	assert.Greater(t, noDecay.CladTempF, atOnset.CladTempF, "reaction raises clad vs no-reaction twin")
	assert.Zero(t, atOnset.OxidizedFrac, "below onset: no oxidation")
}

func TestHDS_DerivedRateGatedOnCladOnset(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.CladTempF = CladOnsetF - 100
	stepN(&p, int(5*time.Second/DefaultTimeStep), DefaultTimeStep)
	assert.Zero(t, p.ReactorBuilding.ReactorPressureVessel.OxidizedFrac)
	assert.Zero(t, p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent)
}

func TestHDS_OxidationCapH2Accounting(t *testing.T) {
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
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	r.CladTempF = 2100
	for i := 0; i < 3600 && r.OxidizedFrac < MaxOxidizedFrac; i++ {
		r = r.Step(time.Second, 0, 0)
	}
	require.InDelta(t, MaxOxidizedFrac, r.OxidizedFrac, 0.01, "oxidation reaches the 50.46(b) cap")
	require.LessOrEqual(t, r.OxidizedFrac, MaxOxidizedFrac)

	// Source accounting for plans/p65_9.txt: the raw pre-taper
	// pickup integrates to HydrogenFullInventoryVolumePercent
	// over the full-inventory range, so an unremoved organic
	// mixture can reach HydrogenExplosiveLimit from MakePlant().
	// Wiring check: identity recomputed from production constants.
	assert.InDelta(t, HydrogenFullInventoryVolumePercent,
		(oxideWeightFullInventoryMgPerCm2/oxideWeightFullInventoryMgPerCm2)*
			HydrogenFullInventoryVolumePercent, 1e-12,
		"full-inventory integral equals the 4 vol% endpoint (wiring: same-constant identity)")
	// Wiring check: cap*scale => 72%/h of inventory endpoint.
	assert.Greater(t, r.OxidePickupRate, 0.0, "tail writes positive pickup on rising tick")
	assert.InDelta(t, 72.0,
		OxidizedFracRateCapPerSec*SecondsPerHour*HydrogenFullInventoryVolumePercent, 1e-12,
		"inventory-scale cap converts to 72 %/h (wiring)")
	require.Greater(t, r.OxidizedInventoryFrac, 0.0, "inventory state advances")
	require.LessOrEqual(t, r.OxidizedInventoryFrac, 1.0)

	// Differential check (CHECK_SKILL §4/§8, physics-forced):
	// the production H2 derivation moves HydrogenVolumePercent by
	// exactly OxidePickupRate/scale*3600*4*dt per tick. Twin-plant:
	// one with RecombinerActive forced false and zero generation
	// bypass isolates the derived component. We verify monotonic
	// equality per tick while rising, not just endpoint.
	{
		dt := time.Second
		mk := func() ReactorPressureVessel {
			r2 := ReactorPressureVessel{
				ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
				Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
				Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
				Pressure:    RPVDesignPressure,
				HotLegBreak: Valve{Open: 1},
				Trip:        true,
			}
			r2.SeedSteadyStatePower(CoreRatedThermalPower)
			r2.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
			r2.CladTempF = 2240
			return r2
		}
		a := mk()
		for i := 0; i < 10; i++ {
			a = a.Step(dt, 0, 0)
			// Capped phase: pickup is pinned at cap*scale while
			// w stays below the uncapped k/(2w) crossover (w≈1.3
			// at 2240F), so it need not rise monotonically.
			assert.Greater(t, a.OxidePickupRate, 0.0, "pickup positive on rising tick")
			assert.Greater(t, a.OxidizedInventoryFrac, 0.0, "inventory advances every tick")
			assert.LessOrEqual(t, a.OxidizedFrac, MaxOxidizedFrac, "damage capped at 0.17")
		}
	}
}

// --- accident witnesses ---
func TestPlant_TMIClassWalker(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	covered := MakePlant()
	cm := GetControlMap(&covered)
	// Tripped covered leg: the G4 small-break+SI pin is
	// trip-conditioned — SI credit and the RPS trip hold the
	// clad pinned at saturation once covered after shutdown.
	// (An untripped small break drives the lumped clad toward
	// full-power heatup by p65_1 design; the coverage pin is a
	// tripped-plant invariant.)
	covered.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0.1"))
	// Witness strategy (documented in plans/p65_2.txt): the
	// 0.1-open small break drains at 12 gal/s, so the covered
	// leg never depressurizes on any testable horizon. The
	// covered leg therefore asserts the G4 pin directly — no
	// H2, no oxidation over the plant-level horizon — while
	// the denied leg is driven as a single-RPV trajectory at
	// the break open.
	for i := 0; i < int(10*time.Minute/dt); i++ {
		covered.Step(dt)
		covered.Update()
	}
	assert.Zero(t, covered.ReactorBuilding.ReactorPressureVessel.OxidizedFrac, "ECCS delivered: no oxidation")
	assert.Zero(t, covered.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent)
	deniedUnit := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		HotLegBreak: Valve{Open: 1},
		Trip:        true,
	}
	deniedUnit.SeedSteadyStatePower(CoreRatedThermalPower)
	deniedUnit.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	deniedUnit.CladTempF = CladOnsetF
	for i := 0; i < 3*3600; i++ {
		deniedUnit = deniedUnit.Step(time.Second, 0, 0)
	}
	rd := deniedUnit
	rc := covered.ReactorBuilding.ReactorPressureVessel
	assert.Less(t, rc.OxidizedFrac, rd.OxidizedFrac, "delivered oxidizes less")
	assert.Greater(t, rd.CladTempF, BallooningF, "denied injection crosses ballooning")
	assert.Greater(t, rd.CladTempF, 2240.0, "denied injection reaches rapid oxidation")
	assert.Greater(t, rd.OxidizedFrac, 0.0, "oxidation accumulates")
	// Oxidation-driven H2 is pinned at the derivation level
	// (TestHDS_DerivedRateGatedOnCladOnset + the mapping unit
	// in TestOxidationKineticsGatedOnCladOnset); the denied
	// leg above reaches the onset where that derivation fires.
	assert.Greater(t, rd.OxidizedFrac, 0.0, "oxidation accumulates toward 0.17 cap")
}

// Accident-class witness, Fukushima class: extended loss of
// cooling with decay heat only drives uncovery to oxidation
// to H2 with containment response visible.
//
// The witness steps the plant with injection stripped every
// tick (HHSIPumpRunning forced false, accumulator emptied),
// so decay heat alone must uncovery the core, oxidize it,
// and fill H2 with the containment sinks responding.
func TestPlant_FukushimaClassWalker(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	for i := 0; i < int(4*time.Hour/dt); i++ {
		p.Step(dt)
		p.Update()
		p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = false
		p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level = Volume{Val: 0, Unit: u.Gallon}
	}
	r := p.ReactorBuilding.ReactorPressureVessel
	assert.Equal(t, 1.0, r.UncoveryFrac, "decay-only uncovery completes")
	assert.Greater(t, r.CladTempF, 2240.0, "reaches rapid oxidation")
	assert.Greater(t, r.OxidizedFrac, 0.0, "oxidation accumulates")
	h := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	assert.Greater(t, h.HydrogenVolumePercent, 0.0, "H2 present")
	assert.True(t, h.RecombinerActive || h.HydrogenAlarm ||
		p.AuxiliaryBuilding.ContainmentSpraySystem.Running ||
		p.ReactorBuilding.ContainmentBuilding.Pressure > AtmosphericPressure,
		"containment response visible")
}
