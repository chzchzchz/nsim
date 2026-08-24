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
	// Intentional update (physical-xenon storage): rated equilibrium
	// is the physical concentration on the iodine scale.
	r.Iodine135 = 1
	r.Xenon135 = xenonEquilibriumPhysical(1)
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
			r = r.Step(0, 0)
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
		r = r.Step(0, 0)
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
	out := r.Step(0, 0)
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
	constOffset := float64(r0.Step(0, 0).Reactivity)
	atCritical := constOffset / 4.5

	r := scenario5Vessel()
	r.BoronConcentration = boron
	r.Insertion = atCritical
	r = r.Step(0, 0)
	require.InDelta(t, 0.0, float64(r.Reactivity), 1e-6,
		"closed-form critical position must null reactivity")

	// At criticality the chain holds steady at the source floor;
	// growth begins once the withdrawal makes rho positive. The short
	// window keeps the average startup rate inside the spec band
	// against ongoing xenon buildup.
	first := r.FissionPower
	r.Insertion = atCritical - withdrawStep
	for range int(measure / TimeStep) {
		r = r.Step(0, 0)
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

// G2 closure: the source-range channel reads the flux-driven count
// rate and the startup-rate indication reports the rise without
// post-hoc sampling between segment endpoints.
//
// "Evidence of criticality is indicated by a positive sustained
// startup rate with no rod motion and increasing source range
// counts."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:161-163
func TestScenario5_InstrumentsReportCriticalityEvidence(t *testing.T) {
	r := scenario5Vessel()
	r.BoronConcentration = 600
	for range int(5 * time.Second / TimeStep) {
		r.ControlRods = r.ControlRods.Step()
		r = r.Step(0, 0)
	}
	// Wiring check: 1000 cps is the derived CoreSourceRangeCountsPerWatt
	// anchor, recomputed from the same production constant.
	assert.InDelta(t, 1e3, r.SourceRangeCounts, 1e-6,
		"the neutron-source floor reads the nominal source-range level")
	assert.InDelta(t, 0, r.StartupRateDPM, 1e-9,
		"a floored core has no startup rate")

	// Cross criticality in closed form against the CURRENT poison
	// state: iodine decay feeds xenon even at zero fission power, so
	// a fresh-vessel reference sits off by more than the thin
	// crossing margin. With the coolant frozen at the reference
	// temperature every non-rod term is constant, so critical
	// insertion is constOffset / 4.5 percent-per-unit; the short
	// window keeps the reading inside the band before ongoing
	// buildup erodes it.
	r0 := r
	r0.Insertion = 0
	constOffset := float64(r0.Step(0, 0).Reactivity)
	atCritical := constOffset / 4.5

	firstCounts := r.SourceRangeCounts
	r.Insertion = atCritical - 0.000625
	r.DemandInsertion = r.Insertion
	for range int(2 * time.Second / TimeStep) {
		r.ControlRods = r.ControlRods.Step()
		r = r.Step(0, 0)
	}
	assert.Greater(t, r.SourceRangeCounts, firstCounts*1.01,
		"counts climb while the chain builds without rod motion")
	assert.Positive(t, r.StartupRateDPM,
		"the instrument shows the sustained positive startup rate")
	assert.LessOrEqual(t, r.StartupRateDPM, 1.0,
		"steady-state startup rate stays inside the 1.0 DPM band")
}

// G3 closure: the dilution setpoint ramps the measured concentration
// down on the BTRS flow-rate clock and releases chemistry on arrival,
// instead of stepping the concentration instantly.
//
// "A boron dilution is started to bring the boron concentration to
// the estimated critical concentration."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:136-138
func TestScenario5_BoronSetpointRampsThroughFlowPath(t *testing.T) {
	c := CVCS{BoronConcentration: 1000}
	c.BoronTargetConcentration = 900
	c.BoronSetpointTracking = true

	var midRate float64
	steps := int(150 * time.Second / TimeStep)
	for i := range steps {
		c = c.Step(Volume{}, Temperature{Val: CoreReferenceInletF}, 0.25, 2250)
		if i == steps/2 {
			midRate = c.DilutionRate
			midMode := c.Mode
			require.Equal(t, CVCSDilution, midMode,
				"tracking selects the dilution mode mid-ramp")
		}
	}
	assert.InDelta(t, 900, c.BoronConcentration,
		CVCSBoronSetpointTolerancePpm,
		"concentration arrives at the setpoint through the flow ramp")
	assert.False(t, c.BoronSetpointTracking,
		"arrival releases the tracking latch")
	assert.Equal(t, CVCSVolumeControl, c.Mode,
		"released chemistry returns to volume control")
	assert.Greater(t, midRate, 0.0,
		"the ramp runs on the BTRS dilution flow, not an instant write")
}

// G4 closure: the enforced insertion limit caps the rod demand so
// resting positions stay above the shutdown-margin envelope.
//
// "Control rods must be pulled above a certain height, the rod
// insertion limit [...]"
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:30-31
func TestScenario5_InsertionLimitBlocksDeepRestingPositions(t *testing.T) {
	c := ControlRods{
		Insertion:             1,
		DemandInsertion:       0.5,
		EnforceInsertionLimit: true,
		InsertionLimit:        0.05,
	}
	for range int(30 * time.Second / TimeStep) {
		c = c.Step()
	}
	assert.InDelta(t, 0.05, c.Insertion, 1e-9,
		"withdrawal demand clamps at the insertion limit")
	assert.InDelta(t, 0.05, c.DemandInsertion, 1e-9,
		"the clamped demand is what the operator sees")

	c.DemandInsertion = 0.0445
	for range int(5 * time.Second / TimeStep) {
		c = c.Step()
	}
	assert.InDelta(t, 0.0445, c.Insertion, 1e-9,
		"positions shallower than the limit stay reachable")
}

// G1 closure: bank position indications decompose the lumped path
// into the shutdown group followed by the overlapped control banks.
//
// "The shutdown banks of the reactor control rods are withdrawn.
// [...] The control rod banks are then manually withdrawn to achieve
// criticality while observing proper bank overlap."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:125,155-157
func TestScenario5_BankIndicationsShowSequenceAndOverlap(t *testing.T) {
	tripped := ControlRods{Insertion: 1}
	assert.InDelta(t, 1, tripped.ShutdownBankInsertion(), 1e-9,
		"full aggregate insertion reads every bank inserted")
	for k, ins := range tripped.ControlBankInsertions() {
		assert.InDelta(t, 1, ins, 1e-9,
			"control bank %d fully inserted on the trip state", k)
	}

	shutdownsOut := ControlRods{Insertion: 1 - CoreShutdownBankWorthShare}
	assert.InDelta(t, 0, shutdownsOut.ShutdownBankInsertion(), 1e-9,
		"shutdown banks complete travel before control-bank motion")
	for k, ins := range shutdownsOut.ControlBankInsertions() {
		assert.InDelta(t, 1, ins, 1e-9,
			"control bank %d still seated while shutdowns travel", k)
	}

	// Mid control-bank travel: bank A leads B which leads C; D stays
	// seated, and B moving before A finishes is the overlap evidence.
	wc := 0.35 * (1 - CoreShutdownBankWorthShare)
	mid := ControlRods{Insertion: 1 - CoreShutdownBankWorthShare - wc}
	banks := mid.ControlBankInsertions()
	assert.Less(t, banks[0], banks[1], "bank A leads bank B")
	assert.Less(t, banks[1], banks[2], "bank B leads bank C")
	assert.InDelta(t, 1, banks[3], 1e-9, "bank D still seated")
	assert.Less(t, banks[1], 1.0,
		"bank B has left its seat while A still travels")
}
