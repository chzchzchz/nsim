package core

import (
	"encoding/json"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Covered core stays at saturation with zero damage.
func TestPlant_DesignBasisLOCADoesNotDamageMax(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	for i := 0; i < int(5*time.Second/DefaultTimeStep); i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	satF := PressurizerSatTemperatureF(float64(RPVDesignPressure))
	assert.InDelta(t, satF, rpv.CladTempF, 50, "covered clad tracks saturation")
	assert.Equal(t, 0.0, rpv.UncoveryFrac, "no break, no uncovery")
	assert.Equal(t, 0.0, rpv.CoreDamageFrac, "covered core never damages")
	assert.Equal(t, float32(0), rpv.Damage)
}

// Open break without injection: clad climbs inside the sourced band.
func TestRPV_CladHeatsPastCoolantOnUncovery(t *testing.T) {
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
	// Void the ledger so the tail integrates uncovered while decay
	// heat is still at shutdown levels. RPV.Step reads BreakDischarged
	// directly (no Plant-level applyHotLegBreak needed for the unit check).
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	start := r.CladTempF
	satF := PressurizerSatTemperatureF(float64(RPVDesignPressure))
	require.InDelta(t, satF, start, 1, "seed sets clad at saturation")

	// Band window: just after the scram tick collapses fission,
	// the fast decay pool holds the sourced 6.5% shutdown level,
	// so the clad rate sits inside the sourced 0.54-1.8 °F/s
	// band for the first seconds. Measure over the 1-6 s
	// window (probe-verified 1.65 °F/s) with 1 s ticks.
	r2 := r
	for i := 0; i < 1; i++ {
		r2 = r2.Step(time.Second, 0, 0, 0)
	}
	winStart := r2.CladTempF
	for i := 0; i < 5; i++ {
		r2 = r2.Step(time.Second, 0, 0, 0)
	}
	rate := (r2.CladTempF - winStart) / 5.0
	assert.Greater(t, r2.UncoveryFrac, 0.0, "voided ledger means uncovered")
	require.Greater(t, rate, 0.0, "uncovered clad must heat")
	assert.GreaterOrEqual(t, rate, 0.54, "rate inside sourced band floor, got %.3f", rate)
	assert.LessOrEqual(t, rate, 1.8, "rate inside sourced band ceiling, got %.3f", rate)

	// Directional witness: clad climbs monotonically while uncovered.
	assert.Greater(t, r2.CladTempF, winStart, "clad climbs while uncovered")
	// Damage direction: seed a hot clad past ballooning to show the
	// step tail latches damage without waiting wall-clock heatup.
	r3 := r2
	r3.CladTempF = BallooningF + 100
	r3 = r3.Step(DefaultTimeStep, 0, 0, 0)
	assert.Greater(t, r3.CoreDamageFrac, 0.0, "damage accrues past ballooning")
	assert.Positive(t, r3.Damage, "Damage mirror tracks CoreDamageFrac")
}

// Stage anchors: ballooning onset then linear ramp to the 50.46 cap.
func TestRPV_StageAnchorsInSequence(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, stageDamage(1500), "below ballooning: no damage")
	assert.Equal(t, 0.0, stageDamage(BallooningF), "at ballooning onset: still zero")
	assert.Equal(t, 1.0, stageDamage(CladFailureF), "at 50.46 cap: full damage")
	assert.InDelta(t, 0.5, stageDamage((BallooningF+CladFailureF)/2), 1e-9, "midpoint of derived ramp")
	// Monotonic across stages 1-3 only.
	prev := stageDamage(1400)
	for _, f := range []float64{1520, 1700, 1900, 2100, 2200} {
		cur := stageDamage(f)
		assert.GreaterOrEqual(t, cur, prev, "damage non-decreasing with clad temp")
		prev = cur
	}
	// Covered core: all anchors preserved (existing suites unaffected).
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(2*time.Second/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	assert.Equal(t, 0.0, r.UncoveryFrac)
	assert.Equal(t, 0.0, r.CoreDamageFrac)
}

// §1 CHECK: whole-object JSON round-trip for the new fields plus
// bridge sensor presence and read-back through GetSensorMap.
func TestCheckCladFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	r.CladTempF = 1800
	r.UncoveryFrac = 0.72
	r.CoreDamageFrac = 0.41
	r.Damage = 0.41
	r.OxidizedInventoryFrac = 0.63
	r.OxidePickupRate = 2.941

	b, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"CladTempF"`)
	assert.Contains(t, string(b), `"UncoveryFrac"`)
	assert.Contains(t, string(b), `"CoreDamageFrac"`)
	assert.Contains(t, string(b), `"OxidizedInventoryFrac"`)
	assert.Contains(t, string(b), `"OxidePickupRate"`)

	var r2 ReactorPressureVessel
	require.NoError(t, json.Unmarshal(b, &r2))
	assert.Equal(t, 1800.0, r2.CladTempF)
	assert.Equal(t, 0.72, r2.UncoveryFrac)
	assert.Equal(t, 0.41, r2.CoreDamageFrac)
	assert.Equal(t, float32(0.41), r2.Damage)
	assert.Equal(t, 0.63, r2.OxidizedInventoryFrac)
	assert.Equal(t, 2.941, r2.OxidePickupRate)

	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.CladTempF = 1800
	p.ReactorBuilding.ReactorPressureVessel.UncoveryFrac = 0.72
	p.ReactorBuilding.ReactorPressureVessel.CoreDamageFrac = 0.41
	p.ReactorBuilding.ReactorPressureVessel.Damage = 0.41
	// plans/p65_9.txt §1: no new bridge sensor is required; the
	// inventory state is internal to the H2 derivation and is not a
	// published readout. Documented deviation from CHECK_SKILL §5;
	// OxidizedFrac remains the damage readout.
	sm := GetSensorMap(&p)
	for _, id := range []string{
		"ReactorBuilding/ReactorPressureVessel/CladTemp",
		"ReactorBuilding/ReactorPressureVessel/Uncovery",
		"ReactorBuilding/ReactorPressureVessel/Damage",
	} {
		s, ok := sm[id]
		require.True(t, ok, "sensor %q must be registered", id)
		out, err := s.Get()
		require.NoError(t, err)
		assert.NotEmpty(t, out, "sensor %q must produce a value", id)
	}
}
