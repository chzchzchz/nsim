package core

// Step advances the three-element feedwater regulator by one
// timestep. The regulator implements a proportional-integral (PI)
// control law on steam-generator level error, with steam flow as a
// feedforward input:
//
//	e_level = LevelSetpoint - Level
//	ValveDemand = Kp * e_level + Ki * integral(e_level) + K_ff * SteamFlow
//
// Where:
//
//	Kp = FeedwaterRegulatorGain           (proportional gain, 1/s)
//	Ki = 1 / FeedwaterRegulatorIntegralTime (integral gain, 1/s^2)
//	K_ff = FeedwaterSteamFlowGain         (feedforward, dimensionless)
//
// The integral term accumulates only when Running is true, which
// prevents integrator windup during shutdown. ValveDemand is
// clamped to [0.0, 1.0] before return.
//
// Spec justification:
//   - Three-element control: "Feedwater flow to each steam
//     generator is controlled by a feedwater regulator in each
//     feedwater line. The regulator is controlled by steam
//     generator level, steam flow and feedwater flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
//   - Pump speed / control valve link: "A signal from the
//     Feedwater Control System also sets the speed of the
//     turbine-driven main feedwater pumps and the position of the
//     motor-driven feedwater pump discharge control valve so as
//     to maintain the main feedwater regulators within their
//     control range."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:73-76
//   - Control-range accommodation: "This setting allows the
//     system to accommodate all operating conditions
//     automatically and provides control margin to accommodate
//     load transients."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:82-85
//   - Valve clamp to [0, 1]: position limits are inherent to
//     motor-operated control valves and are required for stable
//     regulator action; the simplified model omits the
//     spec's function-generator curves and uses a linear
//     flow-vs-position characteristic instead.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:83-85
func (r *FeedwaterRegulator) Step() FeedwaterRegulator {
	ret := *r

	kp := FeedwaterRegulatorGain
	ki := 1.0 / FeedwaterRegulatorIntegralTime
	kff := FeedwaterSteamFlowGain

	// Normalise steam flow against FeedwaterDesignFlow so the
	// feedforward contribution lands on the same dimensionless
	// scale as the proportional and integral terms.
	ffInput := 0.0
	if FeedwaterDesignFlow.Float() > 0 {
		ffInput = kff * ret.SteamFlow / FeedwaterDesignFlow.Float()
	}

	if !ret.Running {
		// When the regulator is stopped the integral freezes
		// and ValveDemand decays toward zero (the control
		// valve closes when not driven).
		ret.ValveDemand = 0
		return ret
	}

	eLevel := ret.LevelSetpoint - ret.Level
	ret.IntegralError += eLevel * TimeStep.Seconds()

	demand := kp*eLevel + ki*ret.IntegralError + ffInput
	demand = min(max(demand, 0), 1)
	ret.ValveDemand = demand

	return ret
}
