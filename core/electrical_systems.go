package core

import "time"

// Step advances the Plant Electrical Systems state machine by
// one timestep. The model senses the real ESF-bus and diesel
// state rather than keying on the operator's offsite-power
// declaration, so buses fed from the unit's own generator while
// offsite is declared lost stay live and their diesels do not
// start against them. Per-step dynamics from chapter 11:
//
//   - Diesel start timing. Each train's diesel accumulates its
//     elapsed start time while that train's ESF bus is dark;
//     "running" means the physical diesel is carrying rated
//     load (DieselRatedLoadTime, 30 s after the start signal).
//   - Engineered Safety Features bus availability. Sensed bus
//     voltage decides: each bus is powered from offsite when
//     available, otherwise from the unit generator through the
//     UAT while the unit carries its house loads, otherwise it
//     transfers to its corresponding diesel after the
//     AutoTransferDelay window.
//   - 118-volt inverter power. The instrument inverters are
//     fed from the ESF buses through 480/120-volt transformers
//     (spec text); the simplified model treats inverter power
//     as available whenever either ESF bus is powered.
//   - Station blackout declaration. StationBlackout is true
//     when both sensed ESF buses are dark and neither diesel
//     can carry load.
//
// Spec justification:
//   - Diversity of sources: "physically and electrically
//     isolated so that any single failure will affect only one
//     source of supply."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:9-14
//   - Auto transfer on loss of power: "automatic transfer of
//     supplies is provided on sensing loss of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
//   - House-load capability: steam-dump power reduction
//     "enables the plant to continue operation and supply
//     power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:55-58
//   - Inverter power for RPS/ESF channels I-IV:
//     "The 118-volt ac instrument power supply is divided into
//     four buses."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-23
func (e *PlantElectricalSystems) Step(dt time.Duration,
	busA, busB ESFBus,
	dieselA, dieselB *DieselGenerator,
) PlantElectricalSystems {
	ret := *e

	busDark := func(bus ESFBus) bool { return !bus.Energized }
	dieselCarrying := func(d *DieselGenerator) bool {
		return d != nil && d.State == DieselRunning &&
			d.CrankStartTime+d.ReadyTime >= DieselRatedLoadTime*time.Second
	}

	// Start timers accumulate per train while that train's bus
	// is dark; a live bus (offsite or the unit through the UAT)
	// resets them.
	if busDark(busA) {
		ret.Diesel1StartTime += dt.Seconds()
	} else {
		ret.Diesel1StartTime = 0
	}
	if busDark(busB) {
		ret.Diesel2StartTime += dt.Seconds()
	} else {
		ret.Diesel2StartTime = 0
	}

	// "Running" mirrors the physical diesels: carrying rated
	// load within DieselRatedLoadTime of the start signal
	// ("be capable of carrying rated load within 30 seconds
	// after receipt of a start signal").
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-51
	ret.Diesel1Running = dieselCarrying(dieselA)
	ret.Diesel2Running = dieselCarrying(dieselB)

	// ESF bus availability is the sensed bus state; the spec
	// does not state a numerical delay for the transfer, and
	// AutoTransferDelay remains the modeled dark-time budget
	// before the diesel breaker closes.
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:52-55
	ret.ESFBus1Available = !busDark(busA)
	ret.ESFBus2Available = !busDark(busB)

	// Inverter power is available whenever either ESF bus
	// is powered; the 480/120-volt step-down is treated as
	// ideal.
	ret.InverterPowerAvailable = ret.ESFBus1Available ||
		ret.ESFBus2Available

	// Station blackout: both sensed buses dark and neither
	// diesel able to carry load.
	ret.StationBlackout = !ret.ESFBus1Available &&
		!ret.ESFBus2Available &&
		!ret.Diesel1Running && !ret.Diesel2Running

	return ret
}

// StepBySeconds advances the Plant Electrical Systems state
// machine by the given elapsed wall-clock duration (seconds).
// It loops Step() once per ProductionStep and is used by tests
// that need to walk through diesel-start or auto-transfer
// delays without spending the (ProductionStep)^{-1} steps per
// second.
func (e *PlantElectricalSystems) StepBySeconds(dt time.Duration,
	seconds float64,
	busA, busB ESFBus,
	dieselA, dieselB *DieselGenerator,
) PlantElectricalSystems {
	ret := *e
	steps := int(seconds / dt.Seconds())
	steps = max(steps, 1)
	for i := 0; i < steps; i++ {
		ret = ret.Step(dt, busA, busB, dieselA, dieselB)
	}
	return ret
}
