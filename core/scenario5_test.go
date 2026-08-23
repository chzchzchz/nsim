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
		crossingDemand = 0.0445
		hold           = 300 * time.Second
	)
	samples := runReactivityApproach(t, 600, crossingDemand, hold)

	first := samples[len(samples)-2]
	final := samples[len(samples)-1]
	require.Greater(t, float64(final), float64(first),
		"sustained rise expected past the crossing step")

	decades := math.Log10(float64(final) / float64(first))
	surDPM := decades / (hold.Seconds() / 60)
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
