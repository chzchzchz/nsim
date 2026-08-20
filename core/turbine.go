package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the turbine by TimeStep on a 1 ms tick. The caller
// passes the upstream steam pressure, downstream condenser pressure,
// and a stop-valve-gated admission factor. When stopValveOpen is
// nil, admission is the MakeupValve.Open fraction (legacy
// behaviour). When stopValveOpen is non-nil, admission is
// *stopValveOpen — allowing Plant.Step to gate the admission via
// the auto-stop-oil protective trip system.
//
// Spec justification for the trip cascade:
//   - "Release of this oil, called auto stop oil, by any one of the
//     turbine trip devices [...] causes all steam control valves to
//     close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
//   - "The mechanical hydraulic overspeed trip mechanism (Item 1)
//     causes the turbine valve hydraulic fluid pressure to dump to
//     drain, thereby closing the main steam stop and governing
//     valves and the reheat steam stop and intercept valves at a
//     pre-selected percent above rated speed."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:9-15
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
// to TurbineRatedkW.
func (t *Turbine) Step(sgPressure, condPressure Pressure, reset bool, stopValveOpen *float64) Turbine {
	ret := *t
	dt := TimeStep

	admission := float64(0)
	if stopValveOpen != nil {
		v := *stopValveOpen
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		admission = v
	} else {
		// Legacy call path (no stop valve gating): use 1.0 as a
		// neutral default. The MakeupValve wiring is no longer
		// authoritative for steam admission.
		admission = 1.0
	}
	steamFlow := float64(sgPressure-condPressure) * admission
	if steamFlow < 0 {
		steamFlow = 0
	}

	// Load runback (DEHM protective layer). When any reactor-
	// protection signal asserts (reactor trip, dropped rod, high
	// kw/ft, low DNBR, rapid nuclear flux decrease, rod bottom
	// circuitry), Plant.Step sets LoadRunback; the Setpoint
	// descends toward zero at RampRate so RealPower follows.
	//
	// Spec: "Automatic load runback initiated by a dropped rod
	// signal, high kw/ft signal or low departure from nucleate
	// boiling ratio signal."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
	// "Automatic turbine load runback via load limit for a
	// dropped rod control cluster assembly is initiated by
	// either a rapid decrease in nuclear flux or by the rod
	// bottom circuitry."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:93-96
	if ret.LoadRunback && ret.Setpoint > 0 {
		ret.Setpoint -= ret.RampRate * dt.Seconds()
		if ret.Setpoint < 0 {
			ret.Setpoint = 0
		}
	}

	// OPC alarm at TurbineOPCALarmFraction (103% of rated
	// speed). Informational only — no automatic action.
	//
	// Spec: "The two speed channels and OPC redundantly provide
	// OPC alarm (103%), OPC trip (110%), and fast valving
	// functions."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-73
	ret.OPCAlarm = ret.State == TurbineRunning &&
		float64(ret.Velocity) > float64(TurbineRatedSpeed)*TurbineOPCALarmFraction

	// OPC trip at TurbineOPCTripFraction (110% of rated speed).
	// Closes the stop valve; latches until reset.
	//
	// Spec: "The mechanical hydraulic overspeed trip mechanism
	// (Item 1) causes the turbine valve hydraulic fluid pressure
	// to dump to drain, thereby closing the main steam stop and
	// governing valves and the reheat steam stop and intercept
	// valves at a pre-selected percent above rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:9-15
	//   (chapter 15.1 numbering in the spec maps to page_207.txt
	//   in this repo)
	if ret.State == TurbineRunning &&
		float64(ret.Velocity) > float64(TurbineRatedSpeed)*TurbineOPCTripFraction {
		ret.OPCTrip = true
		ret.StopValve.Open = 0
		ret.State = TurbineTripped
	}

	// Reset clears the OPC trip latch once the rotor has
	// stopped.
	if reset && ret.OPCTrip && ret.Velocity < turbineStopThreshold {
		ret.OPCTrip = false
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

	// RealPower tracks shaft speed up to the operator-selected
	// Setpoint. The setpoint ceiling is what the DEHM would
	// enforce via the governing valves; we approximate that by
	// clamping RealPower after the speed-based calculation.
	// Setpoint == 0 means "no demand selected" (operator has
	// not yet called for a load); we leave the velocity-derived
	// power uncapped so existing tests that don't set a Setpoint
	// keep working, and Plant.Step's synchronisation path
	// enforces a real ceiling once the operator closes the
	// breaker at a non-zero setpoint.
	//
	// Spec: "After generator breaker closure, the ability to
	// increase or decrease electrical load at a controlled
	// selectable rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
	if ret.State == TurbineRunning && TurbineRatedSpeed > 0 {
		ret.RealPower = Watts(float64(ret.Velocity) / float64(TurbineRatedSpeed) * float64(TurbineRatedkW))
		if ret.Setpoint > 0 {
			maxPower := ret.Setpoint * float64(TurbineRatedkW)
			if float64(ret.RealPower) > maxPower {
				ret.RealPower = Watts(maxPower)
			}
		}
	} else {
		ret.RealPower = 0
	}

	// Extraction steam supply for feedwater heaters and MSR
	// reheater. The total HP steam flow is the upstream value
	// from which the per-extraction-tap fractions (MSR, HP
	// heaters, LP sixth-stage) are taken in Plant.Step.
	//
	// Spec: "A small portion of the steam in the high-pressure
	// turbine is extracted to the highest pressure feedwater
	// heaters; the remainder is exhausted to moisture
	// separator-re heaters with a small extraction to the sixth
	// stage feedwater heaters."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:30-34
	loadFraction := float64(ret.RealPower) / float64(TurbineRatedkW)
	if loadFraction < 0 {
		loadFraction = 0
	}
	ret.ExtractionSteamFlow = u.NewValue(
		loadFraction*FeedwaterDesignFlow.Float(),
		u.Gallon,
	)

	// HP exhaust quality falls with load fraction because the
	// HP turbine does less expansion work per pound of steam.
	ret.HPExhaustQuality = HPExhaustQualityFullLoad -
		(HPExhaustQualityFullLoad-HPExhaustQualityLowLoad)*(1-loadFraction)
	if ret.HPExhaustQuality < HPExhaustQualityLowLoad {
		ret.HPExhaustQuality = HPExhaustQualityLowLoad
	}
	// HP exhaust pressure is slightly below SG pressure due to
	// the pressure drop across the HP turbine stages.
	ret.HPExhaustPressure = sgPressure * 0.90

	// MSR: moisture removal + reheating before LP turbine.
	//
	// Spec: "In the moisture separator-reheaters, moisture is
	// mechanically separated from the turbine steam and the
	// steam is then super heated before entering the three
	// low-pressure turbines."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:31-35
	//
	// Inlet temperature is approximated as the saturated steam
	// temperature at the HP exhaust pressure. For "important
	// aspects" fidelity a simple linear surrogate is sufficient.
	inletTempF := float64(ret.HPExhaustPressure)*5.5 + 80.0
	inletTemp := u.NewValue(inletTempF, u.Fahrenheit)
	// MSR receives the highest-pressure extraction steam and main
	// steam before the throttle valves for reheating. The total
	// steam available to the MSR reheater tubes is a fraction of
	// the HP steam flow; with ExtractionSteamFlow = total HP steam
	// flow, applying MSRDesignExtractionFraction gives the design
	// reheat duty.
	reheatFlow := u.NewValue(
		ret.ExtractionSteamFlow.Float()*MSRDesignExtractionFraction,
		u.Gallon,
	)
	ret.MoistureSeparatorReheater = ret.MoistureSeparatorReheater.Step(
		ret.HPExhaustPressure,
		ret.HPExhaustQuality,
		inletTemp,
		reheatFlow,
		ret.HPExhaustPressure*0.98,
	)

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
