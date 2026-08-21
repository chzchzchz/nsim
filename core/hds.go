package core

// Step advances the Hydrogen Detection and Suppression System by
// one timestep. The simplified model integrates the containment
// atmosphere hydrogen volume percent with three external flows:
//
//	d(H2%)/dt = GenerationRate
//	           - RecombinerRemovalRate × NumRecombiners × bool(RecombinerActive)
//	           - VentilationRemovalRate × bool(VentilationActive)
//
// The high-concentration alarm latches when the volume percent
// crosses HydrogenAlarmSetpoint; recombiners activate once the
// concentration reaches setpoint + hysteresis and stay active
// (the simplified model has no recombiner-deactivation path).
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
func (h *HydrogenDetectionSuppression) Step() HydrogenDetectionSuppression {
	ret := *h

	dt := TimeStep.Seconds()

	// Volume balance. Rates are in % per hour; convert to per
	// step by multiplying by dt/3600.
	var dH2 float64
	if ret.GenerationActive {
		dH2 += HydrogenGenerationRate * dt / 3600.0
	}
	if ret.RecombinerActive {
		dH2 -= HydrogenRecombinerRemovalRate *
			float64(HydrogenNumRecombiners) * dt / 3600.0
	}
	if ret.VentilationActive {
		dH2 -= HydrogenVentilationRemovalRate * dt / 3600.0
	}

	ret.HydrogenVolumePercent += dH2
	if ret.HydrogenVolumePercent < 0 {
		ret.HydrogenVolumePercent = 0
	}

	// Alarm logic: latch true when the volume percent crosses
	// the alarm setpoint. Once latched, only operator reset
	// (out of scope for this simplified model) clears it; the
	// alarm stays latched even if the concentration falls
	// back below the setpoint.
	if ret.HydrogenVolumePercent >= HydrogenAlarmSetpoint {
		ret.HydrogenAlarm = true
	}

	// Recombiner activation: with hysteresis to prevent
	// cycling at the setpoint. Once active, recombiners
	// stay active.
	if ret.HydrogenVolumePercent >=
		HydrogenAlarmSetpoint+HydrogenRecombinerHysteresis {
		ret.RecombinerActive = true
	}

	return ret
}
