package core

// Step advances the turbine by TimeStep on a 1 ms tick. The caller
// passes the upstream steam pressure, downstream condenser pressure,
// and the steam-inlet valve openness — keeping the step pure so
// tests can drive it without constructing a full Plant.
//
// State machine (Off / Running / Tripped) and trip causes:
//
//   - Off -> Running when SteamFlow > 0.
//   - Running -> Off when SteamFlow == 0 and SteamLossTimer is below
//     TurbineSteamLossDeadline (a short transient).
//   - Running -> Tripped on overspeed (Velocity > TurbineRatedSpeed *
//     TurbineOverspeedTripRatio) or when SteamFlow == 0 has persisted
//     past TurbineSteamLossDeadline.
//   - Tripped -> Off when reset is requested and Velocity has decayed
//     to 0.
//
// SteamFlow is the pressure differential across the steam-inlet
// valve, in arbitrary "psi-equivalent" units. The turbine converts it
// into angular Velocity up to TurbineRatedSpeed and into RealPower up
// to TurbineRatedkW. The SteamFlow definition is local to the step;
// the wiring in TurbineBuilding.Step feeds in the building's pressures
// and the MakeupValve.Open fraction.
func (t *Turbine) Step(sgPressure, condPressure Pressure, valveOpen float64, reset bool) Turbine {
	ret := *t
	dt := TimeStep

	steamFlow := float64(sgPressure-condPressure) * valveOpen
	if steamFlow < 0 {
		steamFlow = 0
	}

	switch ret.State {
	case TurbineOff:
		if steamFlow > 0 {
			ret.State = TurbineRunning
			ret.SteamLossTimer = 0
		}
	case TurbineRunning:
		if steamFlow == 0 {
			ret.SteamLossTimer += dt
			if ret.SteamLossTimer >= TurbineSteamLossDeadline {
				ret.State = TurbineTripped
			}
		} else {
			// Steam returned before the trip deadline — clear the
			// timer and keep running.
			ret.SteamLossTimer = 0
		}
	case TurbineTripped:
		if reset && ret.Velocity < turbineStopThreshold {
			ret.State = TurbineOff
			ret.SteamLossTimer = 0
		}
	}

	switch ret.State {
	case TurbineRunning:
		// Overspeed check before any ramp/coast logic, so a rotor
		// already above the trip threshold is latched immediately.
		if float64(ret.Velocity) > float64(TurbineRatedSpeed)*TurbineOverspeedTripRatio {
			ret.State = TurbineTripped
		} else if steamFlow > 0 {
			ret.Velocity += Velocity(steamFlow * dt.Seconds() * turbineRampGain)
			if ret.Velocity > TurbineRatedSpeed {
				ret.Velocity = TurbineRatedSpeed
			}
		} else {
			ret.Velocity -= Velocity(float64(ret.Velocity) * turbineCoastRate * dt.Seconds())
			if ret.Velocity < 0 {
				ret.Velocity = 0
			}
		}
	default:
		ret.Velocity -= Velocity(float64(ret.Velocity) * turbineCoastRate * dt.Seconds())
		if ret.Velocity < 0 {
			ret.Velocity = 0
		}
	}

	if ret.State == TurbineRunning &&
		float64(ret.Velocity) > float64(TurbineRatedSpeed)*TurbineOverspeedTripRatio {
		ret.State = TurbineTripped
	}

	if ret.State == TurbineRunning && TurbineRatedSpeed > 0 {
		ret.RealPower = Watts(float64(ret.Velocity) / float64(TurbineRatedSpeed) * float64(TurbineRatedkW))
	} else {
		ret.RealPower = 0
	}

	return ret
}

// turbineRampGain scales pressure-differential steam flow into angular
// velocity per second. Sized so a typical 1000 psid steam header
// drives the rotor from 0 to rated in a few seconds.
const turbineRampGain = 25.0

// turbineCoastRate is the per-second friction used when no steam is
// admitted. Sized so velocity decays to zero in a couple of seconds,
// matching the spec's expectation that closing the inlet valves stops
// the turbine promptly.
const turbineCoastRate = 3.0

// turbineStopThreshold is the velocity below which we treat the rotor
// as fully stopped for state-transition purposes. Float64 coast
// arithmetic never reaches exactly zero, so a tiny epsilon keeps the
// Tripped -> Off transition firing reliably.
const turbineStopThreshold = 1e-3
