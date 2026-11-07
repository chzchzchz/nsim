package core

import "time"

// stepHydrogen advances the Hydrogen Detection and Suppression
// System by one timestep, mutating the embedded containment's
// node fields in place. The simplified model integrates the
// two-node containment hydrogen with source, recombiner, and
// inter-region exchange:
//
//	d(H2_upper)/dt = GenerationRate/UpperVolumeFraction
//	                 - RecombinerRemovalRate × NumRecombiners × bool(RecombinerActive) (proportional)
//	                 - lambda * (Upper - Lower)  (exchange)
//	d(H2_lower)/dt = + lambda * (Upper - Lower) * (V_upper/V_lower) (exchange)
//	d(H2_mixture)/dt = recomputed volume-weighted mean
//
// The high-concentration alarm latches when the volume percent
// crosses HydrogenAlarmSetpoint and clears only through the
// operator's acknowledgement control. Recombiners activate once
// the concentration reaches setpoint + hysteresis and drop out
// below setpoint - hysteresis (assumption A-2 in
// plans/x/scenario/37.txt; the spec is silent on restart logic).
//
// Spec justification:
//   - Hydrogen source: "Under accident conditions, coolant
//     radioactivity and the Zirconium water reaction can result
//     in the release of hydrogen gas to the containment
//     atmosphere."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-33
//   - Recombiner function: "Hydrogen recombiners that burn the
//     free hydrogen. The hydrogen/oxygen combination results in
//     water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:25-26
//   - Ventilation function: "A containment ventilation system
//     that circulates the containment atmosphere to remove
//     pockets of high hydrogen concentration after an accident."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:19-21
//   - Detection alarm: "A hydrogen detection system that can
//     detect and report in a continuous manner the hydrogen
//     content in the containment atmosphere. Alarms at proper
//     setpoints are provided."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:11-15
//   - Explosive mixture concern: "controlling hydrogen to avoid
//     an explosive mixture."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:54-55
//
// stepHydrogen advances the HDS one timestep. Pointer receiver:
// the node fields live on the embedded ContainmentBuilding and
// are mutated in place, so the step has one writer and no
// second return value to wire.
func (h *HydrogenDetectionSuppression) stepHydrogen(
	c *ContainmentBuilding, dt time.Duration,
	sourceRate float64) {
	dtSec := dt.Seconds()
	upper := c.UpperHydrogenPercent
	lower := c.LowerHydrogenPercent

	// Volume balance. Rates are in % per hour; convert to per
	// step by multiplying by dt/3600. Single invariant: source =
	// derived rate (Plant.Step, from inventory/thermal state,
	// plans/x/gap_hydrogen_kinetics.md §1) plus the training-mode
	// fallback pin; never a double count of either component.
	// Source enters at mixture-equivalent rate: upper node's
	// percent rises by sourceRate/UpperVolumeFraction per hour,
	// so the volume-weighted average rises by exactly sourceRate
	// per hour (plans/x/gap_two_node_containment.md §2).
	upper += sourceRate / UpperVolumeFraction * dtSec / 3600.0
	if h.GenerationActive {
		upper += HydrogenGenerationRate / UpperVolumeFraction * dtSec / 3600.0
	}

	// Exchange: ventilation drives Upper-Lower toward equality
	// at PocketExchangePerHour; ventilation off leaves natural
	// buoyancy (NaturalExchangePerHour), still nonzero so a
	// secured plant stratifies rather than freezes.
	lambda := NaturalExchangePerHour
	if h.VentilationActive {
		lambda = PocketExchangePerHour
	}
	gap := upper - lower
	dGap := lambda * gap * dtSec / 3600.0
	upper -= dGap
	lower += dGap * (UpperVolumeFraction / (1 - UpperVolumeFraction))

	// Recombiner removal applies to both nodes proportionally,
	// so when the nodes are equal the mixture falls at the
	// current closed-form rate.
	if h.RecombinerActive {
		removal := HydrogenRecombinerRemovalRate *
			float64(HydrogenNumRecombiners) * dtSec / 3600.0
		upper -= removal
		lower -= removal
	}

	upper = max(upper, 0)
	lower = max(lower, 0)
	c.UpperHydrogenPercent = upper
	c.LowerHydrogenPercent = lower
	h.HydrogenVolumePercent = c.UpperHydrogenPercent*UpperVolumeFraction + c.LowerHydrogenPercent*(1-UpperVolumeFraction)

	// Alarm logic: latch true when the volume percent crosses
	// the alarm setpoint. Once latched, only operator reset
	// (out of scope for this simplified model) clears it; the
	// alarm stays latched even if the concentration falls
	// back below the setpoint.
	if h.HydrogenVolumePercent >= HydrogenAlarmSetpoint {
		h.HydrogenAlarm = true
	}

	// Recombiner activation: with hysteresis to prevent
	// cycling at the setpoint.
	if h.HydrogenVolumePercent >=
		HydrogenAlarmSetpoint+HydrogenRecombinerHysteresis {
		h.RecombinerActive = true
	}

	// Recombiner deactivation mirrors the activation hysteresis:
	// burning stops once suppression has restored the margin.
	//
	// Spec justification:
	//   - "Hydrogen recombiners that burn the free hydrogen."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:21-22
	if h.RecombinerActive &&
		h.HydrogenVolumePercent <
			HydrogenAlarmSetpoint-HydrogenRecombinerHysteresis {
		h.RecombinerActive = false
	}
}
