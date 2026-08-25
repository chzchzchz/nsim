package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 37 (containment hydrogen monitoring and suppression)
// durable assertions, per plans/x/scenario/37.txt. Spec anchors:
//
//   - "A hydrogen detection system that can detect and report in a
//     continuous manner the hydrogen content in the containment
//     atmosphere. Alarms at proper setpoints are provided."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:7-11
//   - "A containment ventilation system that circulates the containment
//     atmosphere to remove pockets of high hydrogen concentration after
//     an accident."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:15-17
//   - "Hydrogen recombiners that burn the free hydrogen. The
//     hydrogen/oxygen combination results in water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:21-22

// Must not run in parallel: overrideTimeStep mutates global simulation
// state shared across the package tests.

// hdsScenarioPlant seeds the evolution start state: SI-detected event
// driving generation, atmosphere pre-accumulated to pct volume percent.
func hdsScenarioPlant(pct float64) Plant {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal =
		DieselStartSafetyInjection
	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = pct
	return p
}

// Routing the atmosphere through containment ventilation deepens the
// decline from the recombiner-only rate by exactly the ventilation
// stream: closed-form wiring check against production constants, with
// the direction (two suppression streams beat one) physics-forced.
func TestScenario37_VentilationControlDeepensRemoval(t *testing.T) {
	overrideTimeStep(t, time.Second)
	key := "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/"

	p := hdsScenarioPlant(2.6)
	stepN(&p, 600)
	recombOnly := p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent
	wantRecombOnly := 2.6 - 600.0/3600.0*
		HydrogenGenerationRate // net: gen minus four recombiners
	assert.InDelta(t, wantRecombOnly, recombOnly, 0.02)

	cm := p.GetControlMap()
	require.NoError(t, cm[key+"Ventilation"].Set("1"))
	stepN(&p, 900)
	hds := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	netWithVent := HydrogenGenerationRate -
		float64(HydrogenNumRecombiners)*HydrogenRecombinerRemovalRate -
		HydrogenVentilationRemovalRate
	wantVent := recombOnly + 900.0/3600.0*netWithVent
	assert.InDelta(t, wantVent, hds.HydrogenVolumePercent, 0.03)
	assert.Less(t, hds.HydrogenVolumePercent, recombOnly,
		"ventilation must remove hydrogen beyond recombination alone")
	assert.True(t, hds.VentilationActive,
		"the routed ventilation must read back in service")
}

// The alarm latches at the setpoint and clears only through the
// operator's acknowledgement; the detector still re-latches on a later
// excursion so acknowledgement cannot silence the channel.
func TestScenario37_AlarmAcknowledgementClearsBelowSetpoint(t *testing.T) {
	overrideTimeStep(t, time.Second)
	p := hdsScenarioPlant(3.0)
	cm := p.GetControlMap()
	resetKey := "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/AlarmReset"
	hdsOf := func() *HydrogenDetectionSuppression {
		return &p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	}

	stepN(&p, 1)
	require.True(t, hdsOf().HydrogenAlarm,
		"the seeded excursion must latch the high-hydrogen alarm")

	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = 1.9
	stepN(&p, 1)
	require.NoError(t, cm[resetKey].Set("Reset"))
	assert.False(t, hdsOf().HydrogenAlarm,
		"acknowledgement clears the latched alarm")
	stepN(&p, 5)
	assert.False(t, hdsOf().HydrogenAlarm,
		"a sub-setpoint concentration must not re-latch the alarm")

	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = 2.1
	stepN(&p, 1)
	assert.True(t, hdsOf().HydrogenAlarm,
		"the detector must re-latch on a fresh excursion")
}

// Recombiners drop out once suppression restores the margin into the
// lower hysteresis band, and the percentage never reads negative even
// with every removal stream active against zero generation.
func TestScenario37_RecombinersDeactivateInLowerBand(t *testing.T) {
	overrideTimeStep(t, time.Minute)
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: 3.0,
		GenerationActive:      false,
	}
	deactivated := false
	for range int(20 * time.Hour / time.Minute) { // 20 h of suppression
		h = h.Step()
		assert.GreaterOrEqual(t, h.HydrogenVolumePercent, 0.0,
			"the percentage floor is exact")
		if !h.RecombinerActive {
			deactivated = true
			assert.Less(t, h.HydrogenVolumePercent,
				HydrogenAlarmSetpoint-HydrogenRecombinerHysteresis+0.05,
				"deactivation must wait for the lower band")
			break
		}
	}
	require.True(t, deactivated,
		"recombiners must eventually stand down below the lower band")
}
