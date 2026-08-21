package core

// Step advances the Refueling Operations state machine by one
// timestep. The simplified model implements five state-machine
// transitions and a cavity flood/drain ramp:
//
//   - Phase advances when PhaseTimer ≥ RefuelingPhaseDurations[Phase].
//   - Cavity flooding starts in preparation phase (Phase 0) when
//     CavityFlooding is set; depth ramps up at
//     RefuelingCavityFloodRate until it reaches
//     RefuelingCavityMaxDepth (24 ft), then CavityFlooded latches
//     to true.
//   - Cavity draining starts in reassembly phase (Phase 3) when
//     CavityDraining is set; depth ramps down at
//     RefuelingCavityDrainRate until it reaches 0.
//   - Reactor head and internals removal: the HeadRemoved and
//     InternalsRemoved booleans are set true at the start of the
//     disassembly phase (Phase 1), reset at the start of
//     reassembly (Phase 3).
//   - Fuel assembly transfer: a single batch transfer of
//     FuelAssembliesPerRefuelingOutage assemblies happens at the
//     midpoint of the fuel-handling phase (Phase 2). Spent
//     assemblies move from core to pit; the core count is
//     unchanged (one spent out, one fresh in).
//   - Pit boron concentration is held constant at
//     RefuelingPitBoronConcentration throughout.
//
// The phase machine uses phase-relative time only; the caller is
// responsible for setting CavityFlooding / CavityDraining /
// WeightSafetyTripped from operator inputs.
//
// Spec justification:
//   - Five-phase cycle: "The refueling operation is divided into
//     five major phases: preparation, reactor disassembly, fuel
//     handling, reactor reassembly, and preoperational checks,
//     tests, and startup."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
//   - Cavity flood depth: "filled to a depth of approximately 24
//     feet (731 cm) of water."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:43-44
//   - Pit always full of water: "the fuel storage pit, which is
//     kept full of water and is always accessible to operating
//     personnel."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:25-29
//   - Borated water for subcriticality: "Boric acid is added to
//     the water to ensure subcritical conditions during
//     refueling."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:17-19
//   - First-core assembly count: "Of the 193 fuel assemblies
//     contained in a fourloop reactor first core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-39
func (r *RefuelingOperations) Step() RefuelingOperations {
	ret := *r

	dt := TimeStep.Seconds()

	// Phase-relative actions: cavity flooding in preparation,
	// head/internals removal in disassembly, fuel transfer in
	// fuel handling, cavity draining in reassembly.
	switch ret.Phase {
	case RefuelingPhasePreparation:
		// Cavity flooding. Operator signals flooding via
		// CavityFlooding; depth ramps at RefuelingCavityFloodRate
		// (feet per minute), converted to feet per second.
		if ret.CavityFlooding {
			ret.CavityFloodDepth += RefuelingCavityFloodRate *
				dt / 60.0
			if ret.CavityFloodDepth >= RefuelingCavityMaxDepth {
				ret.CavityFloodDepth = RefuelingCavityMaxDepth
				ret.CavityFlooded = true
			}
		}

	case RefuelingPhaseDisassembly:
		// Head and upper internals removed at start of
		// disassembly. Simplified model treats them as binary
		// latches set once when the phase is entered.
		ret.HeadRemoved = true
		ret.InternalsRemoved = true

	case RefuelingPhaseFuelHandling:
		// One-shot batch transfer at the midpoint of the
		// fuel-handling phase. Spent assemblies go to pit;
		// fresh assemblies come from pit; total in core is
		// unchanged.
		mid := RefuelingPhaseDurations[RefuelingPhaseFuelHandling] / 2.0
		if !ret.FuelTransferred && ret.PhaseTimer >= mid {
			ret.FuelAssembliesInPit += FuelAssembliesPerRefuelingOutage
			ret.FuelTransferred = true
		}

	case RefuelingPhaseReassembly:
		// Head and internals re-installed; cavity drained.
		ret.HeadRemoved = false
		ret.InternalsRemoved = false
		if ret.CavityDraining {
			ret.CavityFloodDepth -= RefuelingCavityDrainRate *
				dt / 60.0
			if ret.CavityFloodDepth <= 0 {
				ret.CavityFloodDepth = 0
				ret.CavityFlooded = false
			}
		}
	}

	// Pit water level is held at the design depth; the simplified
	// model does not compute leak dynamics. Boron concentration
	// is held constant at the refueling-cavity value.
	ret.PitWaterLevel = RefuelingCavityMaxDepth
	ret.PitBoronConcentration = RefuelingPitBoronConcentration

	// Advance phase timer. When the timer reaches the
	// configured duration for the current phase, advance to
	// the next phase and reset the timer.
	ret.PhaseTimer += dt
	if ret.Phase < RefuelingPhaseComplete {
		if ret.PhaseTimer >= RefuelingPhaseDurations[ret.Phase] {
			ret.Phase++
			ret.PhaseTimer = 0
		}
	}

	return ret
}
