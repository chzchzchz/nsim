package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// At construction the refueling cycle is complete (Phase 5), no
// cavity flooding, no head/internals removed, the pit is full of
// borated water at the design concentration, and the core holds
// FuelAssembliesPerReactor (193) assemblies.
func TestRefueling_InitialStateHasCompletedCycleAndFullPit(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	r := p.ReactorBuilding.RefuelingOperations
	assert.Equal(t, RefuelingPhaseComplete, r.Phase,
		"want Phase=RefuelingPhaseComplete at startup, got %v", r.Phase)
	assert.False(t, r.CavityFlooding,
		"want CavityFlooding=false at startup, got %v", r.CavityFlooding)
	assert.False(t, r.CavityFlooded,
		"want CavityFlooded=false at startup, got %v", r.CavityFlooded)
	assert.Zero(t, r.CavityFloodDepth,
		"want CavityFloodDepth=0 at startup, got %v", r.CavityFloodDepth)
	assert.False(t, r.HeadRemoved,
		"want HeadRemoved=false at startup, got %v", r.HeadRemoved)
	assert.False(t, r.InternalsRemoved,
		"want InternalsRemoved=false at startup, got %v",
		r.InternalsRemoved)
	assert.False(t, r.WeightSafetyTripped,
		"want WeightSafetyTripped=false at startup, got %v",
		r.WeightSafetyTripped)
	assert.InDelta(t, RefuelingCavityMaxDepth, r.PitWaterLevel, 1e-9,
		"want PitWaterLevel=RefuelingCavityMaxDepth, got %v",
		r.PitWaterLevel)
	assert.InDelta(t, RefuelingPitBoronConcentration,
		r.PitBoronConcentration, 1e-9,
		"want PitBoronConcentration=RefuelingPitBoronConcentration, "+
			"got %v", r.PitBoronConcentration)
	assert.Equal(t, FuelAssembliesPerReactor, r.FuelAssembliesInCore,
		"want FuelAssembliesInCore=FuelAssembliesPerReactor, got %v",
		r.FuelAssembliesInCore)
	assert.Zero(t, r.FuelAssembliesInPit,
		"want FuelAssembliesInPit=0 at startup, got %v",
		r.FuelAssembliesInPit)
	assert.Zero(t, r.FuelAssembliesShipped,
		"want FuelAssembliesShipped=0 at startup, got %v",
		r.FuelAssembliesShipped)
}

// With the operator commanding cavity flooding during the
// preparation phase, the cavity depth ramps up at
// RefuelingCavityFloodRate and latches to CavityFlooded=true
// once it reaches RefuelingCavityMaxDepth (24 ft).
//
// Spec justification:
//   - "filled to a depth of approximately 24 feet (731 cm) of
//     water"
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:43-44
func TestRefueling_CavityFloodsToMaxDepth(t *testing.T) {
	t.Parallel()
	r := RefuelingOperations{
		Phase:          RefuelingPhasePreparation,
		CavityFlooding: true,
	}
	// 24 ft at 1 ft/min = 24 minutes = 1440 s.
	r = r.StepBy(DefaultTimeStep, 1500.0)
	assert.InDelta(t, RefuelingCavityMaxDepth, r.CavityFloodDepth, 1e-6,
		"want CavityFloodDepth=RefuelingCavityMaxDepth after flooding, "+
			"got %v", r.CavityFloodDepth)
	assert.True(t, r.CavityFlooded,
		"want CavityFlooded=true after flooding, got %v", r.CavityFlooded)
}

// Once the cavity is flooded and the phase advances to
// reassembly, CavityDraining=true ramps the depth down to zero.
// HeadRemoved and InternalsRemoved are cleared on entry to the
// reassembly phase.
func TestRefueling_CavityDrainsOnReassembly(t *testing.T) {
	t.Parallel()
	r := RefuelingOperations{
		Phase:            RefuelingPhaseReassembly,
		CavityFlooding:   false,
		CavityDraining:   true,
		CavityFloodDepth: RefuelingCavityMaxDepth,
		CavityFlooded:    true,
		HeadRemoved:      true,
		InternalsRemoved: true,
	}
	// 24 ft at 1 ft/min = 24 minutes = 1440 s.
	r = r.StepBy(DefaultTimeStep, 1500.0)
	assert.Zero(t, r.CavityFloodDepth,
		"want CavityFloodDepth=0 after draining, got %v",
		r.CavityFloodDepth)
	assert.False(t, r.CavityFlooded,
		"want CavityFlooded=false after draining, got %v",
		r.CavityFlooded)
	assert.False(t, r.HeadRemoved,
		"want HeadRemoved=false after reassembly, got %v", r.HeadRemoved)
	assert.False(t, r.InternalsRemoved,
		"want InternalsRemoved=false after reassembly, got %v",
		r.InternalsRemoved)
}

// Phase advances when the phase timer reaches the configured
// duration. With PhaseTimer zeroed on each transition, a
// sufficiently long step sequence walks the phase machine from
// preparation through complete.
//
// Spec justification:
//   - "The refueling operation is divided into five major
//     phases."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
func TestRefueling_PhaseAdvancesOnTimer(t *testing.T) {
	t.Parallel()
	r := RefuelingOperations{Phase: RefuelingPhasePreparation}
	totalSec := 0.0
	for i := 0; i < 6; i++ {
		totalSec += RefuelingPhaseDurations[i]
	}
	r = r.StepBy(DefaultTimeStep, totalSec+100*DefaultTimeStep.Seconds())
	assert.Equal(t, RefuelingPhaseComplete, r.Phase,
		"want Phase=RefuelingPhaseComplete after full cycle, got %v",
		r.Phase)
}

// At the midpoint of the fuel-handling phase a single batch
// transfer of FuelAssembliesPerRefuelingOutage (64) assemblies
// moves from core to pit; the core count is unchanged.
func TestRefueling_FuelTransfersAtMidPhaseHandling(t *testing.T) {
	t.Parallel()
	r := RefuelingOperations{
		Phase:                RefuelingPhaseFuelHandling,
		FuelAssembliesInCore: FuelAssembliesPerReactor,
		FuelAssembliesInPit:  0,
	}
	midSec := RefuelingPhaseDurations[RefuelingPhaseFuelHandling] / 2.0
	r = r.StepBy(DefaultTimeStep, midSec+100*DefaultTimeStep.Seconds())
	assert.True(t, r.FuelTransferred,
		"want FuelTransferred=true after mid-phase, got %v",
		r.FuelTransferred)
	assert.Equal(t, FuelAssembliesPerReactor, r.FuelAssembliesInCore,
		"want FuelAssembliesInCore unchanged, got %v",
		r.FuelAssembliesInCore)
	assert.Equal(t, FuelAssembliesPerRefuelingOutage,
		r.FuelAssembliesInPit,
		"want FuelAssembliesInPit=FuelAssembliesPerRefuelingOutage, "+
			"got %v", r.FuelAssembliesInPit)
}

// HeadRemoved and InternalsRemoved are set true on entry to the
// disassembly phase.
func TestRefueling_HeadAndInternalsRemovedAtDisassembly(t *testing.T) {
	t.Parallel()
	r := RefuelingOperations{Phase: RefuelingPhaseDisassembly}
	r = r.Step(DefaultTimeStep)
	assert.True(t, r.HeadRemoved,
		"want HeadRemoved=true in disassembly, got %v", r.HeadRemoved)
	assert.True(t, r.InternalsRemoved,
		"want InternalsRemoved=true in disassembly, got %v",
		r.InternalsRemoved)
}

// When the plant wires RefuelingOperations into Plant.Step, the
// initial phase stays at RefuelingPhaseComplete and no cavity
// flooding happens without operator action.
func TestPlant_Refueling_InitialStatePreserved(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	r := p.ReactorBuilding.RefuelingOperations
	assert.Equal(t, RefuelingPhaseComplete, r.Phase,
		"want Phase=RefuelingPhaseComplete after Plant step, got %v",
		r.Phase)
	assert.Zero(t, r.CavityFloodDepth,
		"want CavityFloodDepth=0 after Plant step, got %v",
		r.CavityFloodDepth)
}
