package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At construction the containment atmosphere has 0% hydrogen, no
// alarm, and no active recombiners or ventilation.
func TestHDS_InitialStateHasZeroHydrogenAndNoAlarm(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	hds := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	assert.Zero(t, hds.HydrogenVolumePercent,
		"want HydrogenVolumePercent=0 at startup, got %v",
		hds.HydrogenVolumePercent)
	assert.False(t, hds.HydrogenAlarm,
		"want HydrogenAlarm=false at startup, got %v", hds.HydrogenAlarm)
	assert.False(t, hds.RecombinerActive,
		"want RecombinerActive=false at startup, got %v",
		hds.RecombinerActive)
	assert.False(t, hds.VentilationActive,
		"want VentilationActive=false at startup, got %v",
		hds.VentilationActive)
	assert.False(t, hds.GenerationActive,
		"want GenerationActive=false at startup, got %v",
		hds.GenerationActive)
}

// With hydrogen generation on, the volume percent rises by the
// rate × dt. After one step at 1 ms and a 1 %/hour generation
// rate, the increment is 1e-3/3600 ≈ 2.78e-7 %.
//
// Spec justification:
//   - "Under accident conditions [...] the Zirconium water
//     reaction can result in the release of hydrogen gas."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-33
func TestHDS_GenerationIncreasesHydrogen(t *testing.T) {
	t.Parallel()
	h := HydrogenDetectionSuppression{
		GenerationActive: true,
	}
	h2 := h.Step(DefaultTimeStep, 0)
	assert.Greater(t, h2.HydrogenVolumePercent, h.HydrogenVolumePercent,
		"want H2%% to rise with generation on, got %v vs %v",
		h2.HydrogenVolumePercent, h.HydrogenVolumePercent)
	// Expected delta: 1.0 %/h × 1 ms = 1e-3/3600 % = 2.78e-7 %.
	expected := HydrogenGenerationRate * DefaultTimeStep.Seconds() / 3600.0
	assert.InDelta(t, expected, h2.HydrogenVolumePercent, 1e-12,
		"want H2%% delta = rate × dt, got %v vs expected %v",
		h2.HydrogenVolumePercent, expected)
}

// The alarm latches true when the volume percent crosses the
// alarm setpoint.
//
// Spec justification:
//   - "Alarms at proper setpoints are provided."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:14-15
func TestHDS_AlarmLatchesAtSetpoint(t *testing.T) {
	t.Parallel()
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: HydrogenAlarmSetpoint - 0.1,
	}
	h2 := h.Step(DefaultTimeStep, 0)
	assert.False(t, h2.HydrogenAlarm,
		"want alarm false below setpoint, got %v", h2.HydrogenAlarm)
	h.HydrogenVolumePercent = HydrogenAlarmSetpoint + 0.1
	h3 := h.Step(DefaultTimeStep, 0)
	assert.True(t, h3.HydrogenAlarm,
		"want alarm true at/above setpoint, got %v", h3.HydrogenAlarm)
}

// Recombiners activate once the volume percent crosses the
// setpoint + hysteresis band; they stay active.
func TestHDS_RecombinersActivateAtSetpointPlusHysteresis(t *testing.T) {
	t.Parallel()
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: HydrogenAlarmSetpoint +
			HydrogenRecombinerHysteresis + 0.1,
	}
	h2 := h.Step(DefaultTimeStep, 0)
	assert.True(t, h2.RecombinerActive,
		"want RecombinerActive=true at setpoint+hysteresis, got %v",
		h2.RecombinerActive)
}

// With recombiners active, the hydrogen volume percent falls
// (negative net flow from removal > generation when generation
// is off).
func TestHDS_RecombinersRemoveHydrogen(t *testing.T) {
	t.Parallel()
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: 3.0,
		RecombinerActive:      true,
	}
	h2 := h.Step(DefaultTimeStep, 0)
	assert.Less(t, h2.HydrogenVolumePercent, h.HydrogenVolumePercent,
		"want H2%% to fall with recombiners active, got %v vs %v",
		h2.HydrogenVolumePercent, h.HydrogenVolumePercent)
}

// The volume percent is clamped to zero (no negative values).
func TestHDS_VolumePercentClampedAtZero(t *testing.T) {
	t.Parallel()
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: 0,
		RecombinerActive:      true,
		VentilationActive:     true,
	}
	h2 := h.Step(DefaultTimeStep, 0)
	assert.Zero(t, h2.HydrogenVolumePercent,
		"want H2%% clamped at 0, got %v", h2.HydrogenVolumePercent)
}

// An SI signal alone (no break, covered primary) no longer
// latches generation. The SI latch was the pre-plan driver
// (plans/x/gap_hydrogen_kinetics.md); production writes to
// GenerationActive are gone, so the flag stays false and the
// covered plant generates nothing. The derived rate is zero
// because BreakDischarged is 0 and superheat is -31.5°F.
func TestPlant_HDS_GenerationActivatesOnLOCA(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	stepN(&p, int(time.Second/DefaultTimeStep), DefaultTimeStep)
	hds := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	assert.False(t, hds.GenerationActive,
		"want GenerationActive=false (no production writer), got %v",
		hds.GenerationActive)
	assert.Zero(t, hds.HydrogenVolumePercent,
		"want H2%% = 0 for SI-only covered plant, got %v",
		hds.HydrogenVolumePercent)
}

// Derived parity: both components active sum, never double-count.
func TestHDS_DerivedPlusFallbackSums(t *testing.T) {
	t.Parallel()
	h := HydrogenDetectionSuppression{GenerationActive: true}
	h2 := h.Step(DefaultTimeStep, 0.5)
	expected := (0.5 + HydrogenGenerationRate) * DefaultTimeStep.Seconds() / 3600.0
	assert.InDelta(t, expected, h2.HydrogenVolumePercent, 1e-12)
	h0 := h.Step(DefaultTimeStep, 0)
	expected0 := HydrogenGenerationRate * DefaultTimeStep.Seconds() / 3600.0
	assert.InDelta(t, expected0, h0.HydrogenVolumePercent, 1e-12)
}

// Empty-construction: fresh MakePlant + one step generates nothing.
func TestPlant_HDS_EmptyConstructionNoSpuriousRate(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	stepN(&p, 1, DefaultTimeStep)
	assert.Zero(t,
		p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent)
}

// SBO: decay heat alone at full pressure produces zero H2.
func TestPlant_HDS_SBOFullPressureZero(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	for i := 0; i < 2000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		assert.Zero(t,
			p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent,
			"SBO at full pressure must produce zero")
	}
}

// Small break + SI: covered, subcooled, zero H2 over long horizon.
func TestPlant_HDS_SmallBreakSIZero(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0.1"))
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	stepN(&p, int(10*time.Minute/DefaultTimeStep), DefaultTimeStep)
	inv := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float() / RCSDrainableInventoryGal
	assert.Less(t, inv, HydrogenInventoryGateFraction,
		"precondition: small break stays below drainable gate")
	assert.Zero(t,
		p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent)
}

// Twin-plant: identical up to the break commit, then open vs closed.
// dt is 50 ms: the post-blowdown inventories and H2 gaps are
// bit-identical at 1/10/50 ms (probe 2026-11-07), so the coarser
// pre-break/commit phase keeps the witness at ~6 s. The commit
// phase is short (3 min) and the pre-break phase has no
// break-driven pressure dynamics to resolve.
func TestPlant_HDS_TwinPlantOpenBreakGenerates(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	a := MakePlant()
	b := MakePlant()
	stepN(&a, int(10*time.Second/dt), dt)
	stepN(&b, int(10*time.Second/dt), dt)
	// Identical pre-break: H2 zero in both.
	assert.Zero(t, a.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent)
	assert.Zero(t, b.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent)
	cmA := GetControlMap(&a)
	cmB := GetControlMap(&b)
	require.NoError(t, cmA["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	require.NoError(t, cmB["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	stepN(&a, int(3*time.Minute/dt), dt)
	stepN(&b, int(3*time.Minute/dt), dt)
	// Identical up to the break commit.
	h2A := a.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent
	h2B := b.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent
	assert.InDelta(t, h2A, h2B, 1e-9, "pre-divergence plants identical")
	require.NoError(t, cmB["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0"))
	// Stuck-open plant `a` crosses the gate ~35 min; step `a` to ~40 min.
	for i := 0; i < int(40*time.Minute/dt); i++ {
		a.Step(dt)
		a.Update()
	}
	// Closed plant `b` settles covered; step it the same span.
	stepN(&b, int(40*time.Minute/dt), dt)
	h2A = a.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent
	h2B = b.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent
	const gapFloor = 0.02
	assert.Greater(t, h2A, h2B+gapFloor,
		"stuck-open plant must generate strictly more (a=%v b=%v)", h2A, h2B)
}
