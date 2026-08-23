package core

// Step advances the Westinghouse Mark III brushless-exciter
// state machine by one timestep. The simplified model
// implements the spec's shaft-driven topology from chapter
// 15.2:
//
//   - When the rotor is not turning, the brushless exciter
//     cannot produce field current; ExciterActive is false
//     and ExciterOutput / PilotExciterFrequency are zero.
//   - When the rotor is turning, the exciter is active and
//     the WTA-300 regulator drives it to its full output
//     (ExciterOutput = 1.0); the pilot-exciter frequency is
//     proportional to shaft speed.
//
// Spec justification:
//   - "Excitation: Shaft-driven, air-cooled brushless exciter
//     of the rotating rectifier type."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
//   - "all high-power components are mounted on the shaft.
//     This eliminates the need for troublesome carbon
//     brushes."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:64-69
//   - "The Type WTA-300 voltage regulating system is of the
//     latest design incorporating all solid state circuitry."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:5-7
//   - "The high-frequency permanent magnet generator provides
//     power to the automatic voltage regulator which
//     regulates and controls the output of the exciter."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:27-30
func (e *GeneratorExcitationSystem) Step(
	shaftVelocity float64,
) GeneratorExcitationSystem {
	ret := *e

	if shaftVelocity <= 0 {
		// Rotor not turning: the shaft-driven exciter
		// cannot produce field current.
		ret.ExciterActive = false
		ret.ExciterOutput = 0
		ret.PilotExciterFrequency = 0
		return ret
	}

	ret.ExciterActive = true
	// The high-frequency permanent-magnet pilot exciter
	// output frequency scales with shaft speed; the
	// simplified model uses shaftVelocity * 60.0 as a
	// diagnostic proxy that increases with rotor velocity.
	ret.PilotExciterFrequency = shaftVelocity * 60.0
	// With the regulator online and the rotor turning, the
	// exciter drives the field winding to its rated output.
	ret.ExciterOutput = 1.0
	// Set the regulator setpoint the first time it comes
	// online; otherwise leave it at its construction value.
	if ret.RegulatorVoltageSetpoint <= 0 {
		ret.RegulatorVoltageSetpoint = GeneratorRatedVoltage
	}

	return ret
}

// Step advances the GeneratorCoolingSystem state machine by
// one timestep. The simplified model implements the
// spec's two independent cooling circuits from chapter 15.2:
//
//   - SCCW: when the pump is running, the closed-loop
//     demineralized-water flow is at GeneratorSCCWDesignFlow;
//     otherwise flow is zero.
//   - Hydrogen coolers: the shaft-driven blower fraction
//     scales each of the four per-cooler effectiveness
//     values (0..1). All four coolers are in service at
//     construction time.
//
// Spec justification:
//   - "The hydrogen gas is circulated by a blower mounted on
//     the generator rotor."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:27-28
//   - "Four hydrogen coolers are mounted horizontally at the
//     top of the generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:46-47
//   - "Demineralized cooling water is provided for the stator
//     by a closed recirculating water system."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
func (c *GeneratorCoolingSystem) Step(
	h2BlowerFraction float64,
) GeneratorCoolingSystem {
	ret := *c

	if ret.SCCWPumpRunning {
		ret.SCCWFlow = GeneratorSCCWDesignFlow
	} else {
		ret.SCCWFlow = 0
	}

	blowFraction := h2BlowerFraction
	blowFraction = min(blowFraction, 1)
	blowFraction = max(blowFraction, 0)
	for i := range ret.HydrogenCoolerEffectiveness {
		ret.HydrogenCoolerEffectiveness[i] = blowFraction
	}

	// Hold the design hydrogen pressure; the simplified
	// model does not track leak dynamics.
	if ret.HydrogenPressure <= 0 {
		ret.HydrogenPressure = GeneratorDesignHydrogenPressurePsig
	}

	return ret
}

// SCCWAlarmState describes the protective state of the
// stator coil cooling water circuit (plans/p30_1.txt). The
// alarm is raised when the SCCW pump is commanded running but
// flow has fallen below the minimum stable threshold; the
// trip is asserted when flow has been lost entirely.
type SCCWAlarmState struct {
	// Alarm is true when flow sits below
	// GeneratorSCCWPumpMinFlow while the pump is on.
	Alarm bool
	// Trip is true when flow has been lost while the pump
	// is on; the protective action is to drop the
	// generator breaker.
	Trip bool
}

// SCCWAlarm evaluates the alarm/trip gating for the SCCW
// circuit given a pump-running flag and a measured flow
// (gal/s). It is a free function so that tests can exercise
// the protective logic without driving the full Plant.Step
// path.
func SCCWAlarm(pumpRunning bool, flow float64) SCCWAlarmState {
	if !pumpRunning {
		return SCCWAlarmState{}
	}
	return SCCWAlarmState{
		Alarm: flow < GeneratorSCCWPumpMinFlow,
		Trip:  flow == 0,
	}
}
