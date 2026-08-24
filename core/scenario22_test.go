package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 22 (refueling outage): the operator commands the cavity
// fill through the control map and the cavity ramps toward the
// transfer depth while the plant steps.
//
// "Reactor cavity is filled with borated water to the vessel flange."
// specs/the_westinghouse_pressurized_water_react/
// chapter_13/page_185.txt:53-54
func TestScenario22_CavityFloodControlRaisesDepth(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhasePreparation

	setControl(t, &p,
		"ReactorBuilding/RefuelingOperations/CavityFlooding", "1")
	stepFor(&p, 5*time.Second)

	got := p.ReactorBuilding.RefuelingOperations.CavityFloodDepth
	want := RefuelingCavityFloodRate * 5.0 / 60.0
	assert.InDelta(t, want, got, 0.001,
		"cavity depth must rise at RefuelingCavityFloodRate while the "+
			"flood command is held")
}

// Setting the drain command clears the flood command: the two
// operator lineups are mutually exclusive.
//
// "When the head is about 1 foot above the vessel flange, the
// refueling cavity is completely drained and the flange surface is
// cleaned."
// specs/the_westinghouse_pressurized_water_react/
// chapter_13/page_186.txt:113-116
func TestScenario22_DrainCommandClearsFloodCommand(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhaseReassembly
	setControl(t, &p,
		"ReactorBuilding/RefuelingOperations/CavityFlooding", "1")

	setControl(t, &p,
		"ReactorBuilding/RefuelingOperations/CavityDraining", "1")
	r := p.ReactorBuilding.RefuelingOperations
	require.True(t, r.CavityDraining, "drain command must latch")
	assert.False(t, r.CavityFlooding,
		"drain command must clear the flood command")
}

// Seeding the fuel-handling phase timer just below its midpoint and
// stepping across it moves exactly one batch of FuelAssembliesPerRefuelingOutage
// (64) spent assemblies to the pit; the core keeps 193 assemblies
// because each spent assembly is exchanged against a fresh one.
//
// "New fuel assemblies are loaded into the proper region of the
// core." / partially spent assemblies moved between regions.
// specs/the_westinghouse_pressurized_water_react/
// chapter_13/page_185.txt:130-135
func TestScenario22_BatchTransferAtFuelHandlingMidpoint(t *testing.T) {
	mid := RefuelingPhaseDurations[RefuelingPhaseFuelHandling] / 2.0
	r := RefuelingOperations{
		Phase:                RefuelingPhaseFuelHandling,
		PhaseTimer:           mid - 1.0,
		FuelAssembliesInCore: FuelAssembliesPerReactor,
	}
	for range 1200 {
		r = r.Step()
	}
	require.True(t, r.FuelTransferred,
		"batch transfer must latch at the phase midpoint")
	assert.Equal(t, FuelAssembliesPerRefuelingOutage, r.FuelAssembliesInPit,
		"one outage batch must sit in the pit after the midpoint")
	assert.Equal(t, FuelAssembliesPerReactor, r.FuelAssembliesInCore,
		"the core must hold a full complement after the exchange")
}

// Walking each phase boundary exercises every transition of the
// five-phase cycle without replaying the full multi-hour durations:
// disassembly latches head/internals out, reassembly clears them and
// drains a flooded cavity, and the startup-preparation leg tips the
// machine into Complete.
//
// Five-phase cycle:
// specs/the_westinghouse_pressurized_water_react/
// chapter_13/page_183.txt:32-34; head lift/drain sequencing at
// chapter_13/page_185.txt:51-58 and chapter_13/page_186.txt:101-116.
func TestScenario22_PhaseWalkLatchesAndCompletion(t *testing.T) {
	// Disassembly entry latches head and internals out underwater.
	r := RefuelingOperations{Phase: RefuelingPhaseDisassembly}
	r = r.Step()
	assert.True(t, r.HeadRemoved, "head must be out during disassembly")
	assert.True(t, r.InternalsRemoved,
		"upper internals must be out during disassembly")

	// Reassembly entry clears the latches; a commanded drain empties
	// the flooded cavity at the drain rate.
	r = RefuelingOperations{
		Phase:            RefuelingPhaseReassembly,
		HeadRemoved:      true,
		InternalsRemoved: true,
		CavityFloodDepth: RefuelingCavityMaxDepth,
		CavityFlooded:    true,
		CavityDraining:   true,
	}
	drainSeconds := RefuelingCavityMaxDepth / RefuelingCavityDrainRate *
		60.0
	r = r.StepBy(drainSeconds + TimeStep.Seconds())
	assert.False(t, r.HeadRemoved,
		"head must be reseated on reassembly entry")
	assert.False(t, r.InternalsRemoved,
		"internals must be restored on reassembly entry")
	assert.InDelta(t, 0, r.CavityFloodDepth, 1e-6,
		"commanded drain must empty the cavity")
	assert.False(t, r.CavityFlooded,
		"cavity must lose its flooded latch when empty")

	// The startup-preparation leg is terminal: crossing its duration
	// marks the cycle complete.
	r = RefuelingOperations{
		Phase:      RefuelingPhaseStartup,
		PhaseTimer: RefuelingPhaseDurations[RefuelingPhaseStartup] - 0.5,
	}
	for range 600 {
		r = r.Step()
	}
	assert.Equal(t, RefuelingPhaseComplete, r.Phase,
		"cycle must reach Complete after the startup leg")
}
