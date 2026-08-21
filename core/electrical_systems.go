package core

// Step advances the Plant Electrical Systems state machine by
// one timestep. The simplified model implements five per-step
// dynamics from chapter 11 of the spec:
//
//   - Offsite power availability. The plant normally receives
//     power from the off-site main distribution grid through
//     the system auxiliary transformers; OffsitePowerAvailable
//     is the model's boolean for that state.
//   - Diesel start timing. On loss of offsite power, both
//     diesel generators receive a start signal and each tracks
//     its own elapsed start time. After DieselRatedLoadTime
//     (30 s) the generator is "running" and ready to accept ESF
//     bus load.
//   - Engineered Safety Features bus availability. Each ESF bus
//     is powered from offsite when available; otherwise it
//     transfers to its corresponding diesel after the
//     AutoTransferDelay window.
//   - 118-volt inverter power. The instrument inverters are
//     fed from the ESF buses through 480/120-volt transformers
//     (spec text); the simplified model treats inverter power
//     as available whenever either ESF bus is powered.
//   - Station blackout declaration. StationBlackout is true
//     when offsite and both diesels are unavailable.
//
// Spec justification:
//   - Diversity of sources: "physically and electrically
//     isolated so that any single failure will affect only one
//     source of supply."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:9-14
//   - Auto transfer on loss of offsite: "automatic transfer of
//     supplies is provided on sensing loss of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
//   - Diesel start: "reach rated speed and be ready to begin
//     accepting load within 10 seconds, and be capable of
//     carrying rated load within 30 seconds after receipt of a
//     start signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-51
//   - Inverter power for RPS/ESF channels I-IV:
//     "The 118-volt ac instrument power supply is divided into
//     four buses."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-23
func (e *PlantElectricalSystems) Step() PlantElectricalSystems {
	ret := *e
	dt := TimeStep.Seconds()

	if ret.OffsitePowerAvailable {
		// Offsite healthy: diesels are not started, their
		// start timers reset.
		ret.Diesel1StartTime = 0
		ret.Diesel2StartTime = 0
		ret.Diesel1Running = false
		ret.Diesel2Running = false
	} else {
		// Offsite lost: both diesels receive a start
		// signal and accumulate elapsed start time.
		ret.Diesel1StartTime += dt
		ret.Diesel2StartTime += dt
		ret.Diesel1Running = ret.Diesel1StartTime >=
			DieselRatedLoadTime
		ret.Diesel2Running = ret.Diesel2StartTime >=
			DieselRatedLoadTime
	}

	// ESF bus power: from offsite when healthy, otherwise
	// transferred to the corresponding diesel. The spec
	// does not state a numerical delay for the transfer;
	// AutoTransferDelay (1 s) is the simplified model's
	// assumed window.
	ret.ESFBus1Available = ret.OffsitePowerAvailable ||
		(ret.Diesel1Running && ret.Diesel1StartTime >=
			DieselRatedLoadTime+AutoTransferDelay)
	ret.ESFBus2Available = ret.OffsitePowerAvailable ||
		(ret.Diesel2Running && ret.Diesel2StartTime >=
			DieselRatedLoadTime+AutoTransferDelay)

	// Inverter power is available whenever either ESF bus
	// is powered; the 480/120-volt step-down is treated as
	// ideal.
	ret.InverterPowerAvailable = ret.ESFBus1Available ||
		ret.ESFBus2Available

	// Station blackout: no offsite and no diesel ready.
	ret.StationBlackout = !ret.OffsitePowerAvailable &&
		!ret.Diesel1Running && !ret.Diesel2Running

	return ret
}

// StepBySeconds advances the Plant Electrical Systems state
// machine by the given elapsed wall-clock duration (seconds).
// It loops Step() once per TimeStep and is used by tests
// that need to walk through diesel-start or auto-transfer
// delays without spending the (TimeStep)^{-1} steps per
// second.
func (e *PlantElectricalSystems) StepBySeconds(seconds float64) PlantElectricalSystems {
	ret := *e
	steps := int(seconds / TimeStep.Seconds())
	if steps < 1 {
		steps = 1
	}
	for i := 0; i < steps; i++ {
		ret = ret.Step()
	}
	return ret
}
