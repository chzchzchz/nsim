package core

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 5 (dilution to criticality) durable assertions.
//
// "A boron dilution is started to bring the boron concentration to the
// estimated critical concentration." and "When control rod motion is
// not in progress, a steady-state startup rate of 1.0 decades per
// minute should not be exceeded. Evidence of criticality is indicated
// by a positive sustained startup rate with no rod motion and
// increasing source range counts."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:136-138,155-163

// scenario5Vessel returns a hot zero-power vessel at the no-load
// equilibrium with rods fully inserted and reference boron.
func scenario5Vessel() ReactorPressureVessel {
	r := ReactorPressureVessel{}
	r.Inlet = Temperature{Val: CoreReferenceInletF}
	r.Outlet = Temperature{Val: CoreReferenceInletF}
	r.Insertion = 1
	r.DemandInsertion = 1
	r.BoronConcentration = CVCSInitialBoron
	// No-load equilibrium carries its rated xenon load; without it the
	// folded reactivity reference reads a fresh core as ~2.7% light.
	r.Iodine135 = 1
	r.Xenon135 = 1
	return r
}

// runReactivityApproach steps the vessel through the scenario 5 rod
// positions at the given boron, sampling fission power at each rest
// point after the hold duration.
func runReactivityApproach(t *testing.T, boron float64, restInsertion float64, hold time.Duration) []Watts {
	t.Helper()
	r := scenario5Vessel()
	r.BoronConcentration = boron
	samples := make([]Watts, 0, 4)
	for _, demand := range []float64{0.5, 0.1, restInsertion} {
		r.DemandInsertion = demand
		steps := int(hold / TimeStep)
		for range steps {
			r.ControlRods = r.ControlRods.Step()
			r = r.Step(0)
		}
		require.InDelta(t, r.DemandInsertion, r.Insertion, 1e-9,
			"rods must settle before sampling")
		samples = append(samples, r.FissionPower)
	}
	return samples
}

func TestScenario5_DilutionWithRodsInsertedStaysSubcritical(t *testing.T) {
	r := scenario5Vessel()
	r.BoronConcentration = CVCSInitialBoron - 400
	for range int(10 * time.Second / TimeStep) {
		r = r.Step(0)
	}
	assert.Equal(t, CoreNeutronSourcePower, r.FissionPower,
		"dilution alone must not lift power off the source floor")
	assert.Negative(t, r.Reactivity)
}

func TestScenario5_EstimatedCriticalPositionStaysSubcritical(t *testing.T) {
	samples := runReactivityApproach(t, 600, 0.05, 20*time.Second)
	last := samples[len(samples)-1]
	assert.Equal(t, CoreNeutronSourcePower, last,
		"estimated critical position must remain subcritical before the crossing step")
	assert.Negative(t, rAtRest(t, 600, 0.05))
}

// rAtRest computes reactivity for a settled rod position via the same
// arithmetic the plant uses (rod + doppler + boron terms).
func rAtRest(t *testing.T, boron float64, insertion float64) float32 {
	t.Helper()
	r := scenario5Vessel()
	r.Insertion = insertion
	r.BoronConcentration = boron
	out := r.Step(0)
	return out.Reactivity
}

func TestScenario5_CrossingStepYieldsSustainedRiseWithoutRodMotion(t *testing.T) {
	const (
		measure      = 2 * time.Second
		boron        = 600.0
		withdrawStep = 0.000625 // rod worth 4.5%/unit -> ~+0.0028% dk/k
	)

	// Critical insertion in closed form: with the coolant frozen at
	// the reference temperature and xenon at rated equilibrium, every
	// reactivity term except the rod worth is constant, so the
	// crossing sits exactly at constOffset / 4.5 percent per unit.
	r0 := scenario5Vessel()
	r0.BoronConcentration = boron
	r0.Insertion = 0
	constOffset := float64(r0.Step(0).Reactivity)
	atCritical := constOffset / 4.5

	r := scenario5Vessel()
	r.BoronConcentration = boron
	r.Insertion = atCritical
	r = r.Step(0)
	require.InDelta(t, 0.0, float64(r.Reactivity), 1e-6,
		"closed-form critical position must null reactivity")

	// At criticality the chain holds steady at the source floor;
	// growth begins once the withdrawal makes rho positive. The short
	// window keeps the average startup rate inside the spec band
	// against ongoing xenon buildup.
	first := r.FissionPower
	r.Insertion = atCritical - withdrawStep
	for range int(measure / TimeStep) {
		r = r.Step(0)
	}
	final := r.FissionPower

	require.Greater(t, float64(final), float64(CoreNeutronSourcePower),
		"sustained rise expected past the crossing step")
	require.InDelta(t, atCritical-withdrawStep, r.Insertion, 1e-9,
		"insertion must stay fixed through the rise")

	decades := math.Log10(float64(final) / float64(first))
	surDPM := decades / (measure.Seconds() / 60)
	assert.Greater(t, surDPM, 0.2, "startup rate should be positive")
	assert.LessOrEqual(t, surDPM, 1.0,
		"steady-state startup rate must stay within the spec band")
}

func TestScenario5_SmallReinsertionRestoresNegativeReactivity(t *testing.T) {
	before := rAtRest(t, 600, 0.0445)
	after := rAtRest(t, 600, 0.046)
	require.Positive(t, before)
	assert.Negative(t, after,
		"a small re-insertion must stop the power rise")
}
