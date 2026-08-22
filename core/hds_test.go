package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// At construction the containment atmosphere has 0% hydrogen, no
// alarm, and no active recombiners or ventilation.
func TestHDS_InitialStateHasZeroHydrogenAndNoAlarm(t *testing.T) {
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
	h := HydrogenDetectionSuppression{
		GenerationActive: true,
	}
	h2 := h.Step()
	assert.Greater(t, h2.HydrogenVolumePercent, h.HydrogenVolumePercent,
		"want H2%% to rise with generation on, got %v vs %v",
		h2.HydrogenVolumePercent, h.HydrogenVolumePercent)
	// Expected delta: 1.0 %/h × 1 ms = 1e-3/3600 % = 2.78e-7 %.
	expected := HydrogenGenerationRate * TimeStep.Seconds() / 3600.0
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
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: HydrogenAlarmSetpoint - 0.1,
	}
	h2 := h.Step()
	assert.False(t, h2.HydrogenAlarm,
		"want alarm false below setpoint, got %v", h2.HydrogenAlarm)
	h.HydrogenVolumePercent = HydrogenAlarmSetpoint + 0.1
	h3 := h.Step()
	assert.True(t, h3.HydrogenAlarm,
		"want alarm true at/above setpoint, got %v", h3.HydrogenAlarm)
}

// Recombiners activate once the volume percent crosses the
// setpoint + hysteresis band; they stay active.
func TestHDS_RecombinersActivateAtSetpointPlusHysteresis(t *testing.T) {
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: HydrogenAlarmSetpoint +
			HydrogenRecombinerHysteresis + 0.1,
	}
	h2 := h.Step()
	assert.True(t, h2.RecombinerActive,
		"want RecombinerActive=true at setpoint+hysteresis, got %v",
		h2.RecombinerActive)
}

// With recombiners active, the hydrogen volume percent falls
// (negative net flow from removal > generation when generation
// is off).
func TestHDS_RecombinersRemoveHydrogen(t *testing.T) {
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: 3.0,
		RecombinerActive:      true,
	}
	h2 := h.Step()
	assert.Less(t, h2.HydrogenVolumePercent, h.HydrogenVolumePercent,
		"want H2%% to fall with recombiners active, got %v vs %v",
		h2.HydrogenVolumePercent, h.HydrogenVolumePercent)
}

// The volume percent is clamped to zero (no negative values).
func TestHDS_VolumePercentClampedAtZero(t *testing.T) {
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: 0,
		RecombinerActive:      true,
		VentilationActive:     true,
	}
	h2 := h.Step()
	assert.Zero(t, h2.HydrogenVolumePercent,
		"want H2%% clamped at 0, got %v", h2.HydrogenVolumePercent)
}

// When the plant wires HDS into Plant.Step, a LOCA signal (safety
// injection) flips GenerationActive on and the volume percent
// begins to climb.
func TestPlant_HDS_GenerationActivatesOnLOCA(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	stepN(&p, int(time.Second/TimeStep))
	hds := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	assert.True(t, hds.GenerationActive,
		"want GenerationActive=true after LOCA signal, got %v",
		hds.GenerationActive)
	assert.Greater(t, hds.HydrogenVolumePercent, 0.0,
		"want H2%% > 0 after LOCA, got %v", hds.HydrogenVolumePercent)
}
