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

// Routing the atmosphere through containment ventilation removes
// hydrogen at exactly the ventilation stream: a two-phase closed-form
// wiring check against production constants - dormant baseline, then
// vent-on decline - with the direction physics-forced. The phase runs
// on a benign lineup: a LOCA-sensed plant isolates its vent (Table
// 7-1) and falls back to the recombiner-only arc covered above.
func TestScenario37_VentilationControlDeepensRemoval(t *testing.T) {
	t.Parallel()
	// Width note: the hydrogen state integrates linear streams
	// (hds.go: dH2 is exactly linear in dt), so a widened tick
	// reproduces the closed-form decline exactly. The baseline
	// freeze assertion below fails loudly if a spurious SI sense
	// ever fires at the widened tick, so the benign premise cannot
	// silently rot.
	testDt := 100 * time.Millisecond
	key := "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/"

	p := MakePlant()
	// Hold the secondary header at its shell pressure so the
	// low-steamline SI trip cannot latch and gate ventilation:
	// with the steam stop valve shut, production only builds
	// pressure, and the fresh-plant SG sits at design pressure.
	// Spec: "Full load pressure, shell side ... 920 psia".
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,80
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = 1.4
	cm := p.GetControlMap()

	// Dormant baseline: nothing crosses the wall, concentration
	// holds exactly.
	stepN(&p, int(300*time.Second/testDt), testDt)
	baseline := p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent
	assert.InDelta(t, 1.4, baseline, 1e-9,
		"with no suppression streams active the concentration freezes")

	// Vent on: the decline deepens by exactly the ventilation
	// stream over the window.
	require.NoError(t, cm[key+"Ventilation"].Set("1"))
	const ventWindow = 900 * time.Second
	stepN(&p, int(ventWindow/testDt), testDt)
	hds := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	want := baseline - ventWindow.Seconds()/3600.0*HydrogenVentilationRemovalRate
	assert.InDelta(t, want,
		hds.HydrogenVolumePercent, 0.01,
		"the decline must equal the ventilation stream alone")
	assert.Less(t, hds.HydrogenVolumePercent, baseline,
		"ventilation must carry hydrogen out of the atmosphere")
}

// The alarm latches at the setpoint and clears only through the
// operator's acknowledgement; the detector still re-latches on a later
// excursion so acknowledgement cannot silence the channel.
func TestScenario37_AlarmAcknowledgementClearsBelowSetpoint(t *testing.T) {
	t.Parallel()
	testDt := time.Second
	p := hdsScenarioPlant(3.0)
	cm := p.GetControlMap()
	resetKey := "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/AlarmReset"
	hdsOf := func() *HydrogenDetectionSuppression {
		return &p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	}

	stepN(&p, 1, testDt)
	require.True(t, hdsOf().HydrogenAlarm,
		"the seeded excursion must latch the high-hydrogen alarm")

	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = 1.9
	stepN(&p, 1, testDt)
	require.NoError(t, cm[resetKey].Set("Reset"))
	assert.False(t, hdsOf().HydrogenAlarm,
		"acknowledgement clears the latched alarm")
	stepN(&p, 5, testDt)
	assert.False(t, hdsOf().HydrogenAlarm,
		"a sub-setpoint concentration must not re-latch the alarm")

	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = 2.1
	stepN(&p, 1, testDt)
	assert.True(t, hdsOf().HydrogenAlarm,
		"the detector must re-latch on a fresh excursion")
}

// Recombiners drop out once suppression restores the margin into the
// lower hysteresis band, and the percentage never reads negative even
// with every removal stream active against zero generation.
func TestScenario37_RecombinersDeactivateInLowerBand(t *testing.T) {
	t.Parallel()
	testDt := time.Minute
	h := HydrogenDetectionSuppression{
		HydrogenVolumePercent: 3.0,
		GenerationActive:      false,
	}
	deactivated := false
	for range int(20 * time.Hour / time.Minute) { // 20 h of suppression
		h = h.Step(testDt)
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

// Containment vent isolation (Table 7-1) must shut the atmosphere-
// exchange path: with Phase A latched the operator's ventilation flag
// is overridden - nothing crosses an isolated wall, so the contained
// concentration freezes exactly even though the vent stream would
// otherwise carry hydrogen out. Clearing the phase returns command to
// the operator.
//
// Spec: "Containment Vent Isolation: Safety injection, [...]
// Manual actuation of Phase A isolation"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:51-54
func TestScenario37_VentIsolationStopsExchange(t *testing.T) {
	t.Parallel()
	// Production tick throughout: at widened ticks the hot-standby
	// header crashes through the low-steamline SI trigger within a
	// couple of steps and the fixture stops being benign.
	key := "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/"

	p := MakePlant()
	p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent = 1.4
	cm := p.GetControlMap()
	require.NoError(t, cm[key+"Ventilation"].Set("1"))
	stepN(&p, 3000, DefaultTimeStep)
	hds := p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	require.True(t, hds.VentilationActive,
		"precondition: the operator routed the ventilation stream")

	// Latch Phase A through the protection system - the CIS manual
	// flags mirror these ESFAS outputs each step.
	p.ReactorBuilding.ReactorProtectionSystem.
		ContainmentIsolationPhaseAActuated = true
	stepN(&p, 1000, DefaultTimeStep)
	cis := p.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	require.True(t, cis.PhaseAIsolated, "precondition: Phase A latched")
	hds = p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	require.False(t, hds.VentilationActive,
		"the isolated wall must force the ventilation stream off")

	// Seeded below every suppression band, the isolated containment
	// freezes exactly: no generation, no ventilation term crossing
	// the boundary, recombiners dormant.
	before := p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent
	stepN(&p, 600, 100*time.Millisecond)
	got := p.ReactorBuilding.ContainmentBuilding.
		HydrogenDetectionSuppression.HydrogenVolumePercent
	assert.InDelta(t, before, got, 1e-9,
		"an isolated containment exchanges no atmosphere")
	require.False(t,
		p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.VentilationActive,
		"ventilation stays forced off through isolation")

	// Clearing the protection output releases the CIS demand and
	// returns the valve to operator command.
	p.ReactorBuilding.ReactorProtectionSystem.
		ContainmentIsolationPhaseAActuated = false
	stepN(&p, 1000, DefaultTimeStep)
	require.False(t,
		p.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem.IsIsolated(),
		"precondition: phases cleared after reset")
	require.NoError(t, cm[key+"Ventilation"].Set("1"))
	stepN(&p, 100, DefaultTimeStep)
	assert.True(t,
		p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.VentilationActive,
		"the operator commands ventilation again once isolated paths clear")
}
