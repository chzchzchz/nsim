package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

// Step advances the turbine by ProductionStep on a 1 ms tick. The caller
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
//   - Running -> Tripped on any protective-device release: low vacuum,
//     low bearing-oil pressure, or a worn thrust bearing (the spec's
//     device list has no setpoint for device 4, so its cause is the
//     externally written ThrustBearingWorn failure input).
//   - Tripped -> Off when reset is requested and Velocity has decayed
//     to 0.
//
// SteamFlow is the throttled steam demand drawn from the main-steam
// headers: design full-load flow scaled by valve admission and header
// pressure. The turbine converts it into angular Velocity up to
// TurbineRatedSpeed and into RealPower up to TurbineRatedkW.
func (t *Turbine) Step(dt time.Duration, sgPressure, condPressure Pressure, tavg Temperature, reset bool, stopValveOpen *float64) Turbine {
	ret := *t

	// Consume the operator's latched reset request so the command
	// is edge-triggered: one Step evaluates it exactly once, then
	// clears it. Returning the tripped machine to service restores
	// operator valve control after the auto-stop-oil trip latch:
	//   "The operator may position the valves, when necessary,
	//    through the manual valve controllers."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:114-116
	reset = reset || ret.ResetRequest
	ret.ResetRequest = false

	admission := float64(0)
	if stopValveOpen != nil {
		admission = min(max(*stopValveOpen, 0), 1)
	} else {
		// Legacy call path (no stop valve gating): use 1.0 as a
		// neutral default. The MakeupValve wiring is no longer
		// authoritative for steam admission.
		admission = 1.0
	}
	// Governing valves throttle in series behind the stop valves;
	// the auto-stop-oil trip closes both.
	admission *= ret.GoverningValve.Open
	ret.FirstStagePressure = Pressure(float64(sgPressure) * admission)

	// Throttled design-flow steam demand (gal/step). At full
	// admission and rated header pressure this equals the condenser's
	// design full-load steam load, so the SG draw, condenser duty,
	// and dump capacity share one scale. Before synchronization the
	// draw scales with rotor speed — a slow-roll rotor cannot swallow
	// design flow; without this a rolling machine chills its steam
	// generator at standstill. Assumption floor (10%) keeps the
	// Off→Running self-start alive at zero speed.
	//   "Wide range speed control of the turbine from turning gear
	//    to rated speed."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
	steamFlow := CondenserDesignSteamLoadGalPer * admission *
		float64(sgPressure) / float64(SteamGeneratorShellPressure)
	if !ret.Synchronized {
		steamFlow *= max(float64(ret.Velocity)/float64(TurbineRatedSpeed),
			TurbineRollMinSteamFraction)
	}
	// After synchronization the load reference throttles the demand
	// directly: admitted steam may not exceed the Setpoint fraction
	// of design flow even while the governing-valve trim is still
	// converging (or parked wide open from the roll), so raising load
	// never over-draws the header ahead of the demand ladder.
	// Setpoint == 0 keeps the "no demand selected" uncapped draw.
	//
	// Spec: "After generator breaker closure, the ability to
	//    increase or decrease electrical load at a controlled
	//    selectable rate."
	//    specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
	if ret.Synchronized && ret.Setpoint > 0 {
		steamFlow = min(steamFlow, CondenserDesignSteamLoadGalPer*
			ret.Setpoint*float64(sgPressure)/float64(SteamGeneratorShellPressure))
	}
	steamFlow = max(steamFlow, 0)
	ret.steamDemandGalPerStep = steamFlow

	// Lubricating-oil header. The motor-driven auxiliary pump holds
	// design pressure when running; with the pump off the header
	// decays toward zero. A turbine rolled without its lube-oil pump
	// in service trips immediately on low bearing-oil pressure —
	// matching the pre-startup checklist item "checking the bearing
	// oil pump before startup".
	//
	// Spec: the DEHM supervises operation "from checking the bearing
	// oil pump before startup".
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_203.txt:87-89
	// The trip device itself: "Low bearing oil pressure trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
	if ret.LubeOilPumpRunning {
		ret.BearingOilPressure = BearingOilDesignPressure
	} else {
		decay := float64(ret.BearingOilPressure) * (dt.Seconds() / LubeOilDecayTimeConstant)
		ret.BearingOilPressure = Pressure(max(float64(ret.BearingOilPressure)-decay, 0))
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
		ret.Setpoint = max(ret.Setpoint, 0)
	}

	// Dispatcher load reference. When the grid dispatch supplies a
	// demand, the DEHM ramps the operator setpoint toward it at the
	// controlled selectable rate unless a protective runback holds
	// priority. A low-Tavg-deviation signal also stops the ascent:
	// while measured Tavg sits more than TurbineLowTavgDeviationF
	// below the load-referenced program, raising the load reference
	// is held - loading a rods-only core deeper than its poison
	// allows would otherwise drag Tavg down unbounded. Descent toward
	// a lowered demand stays permitted, and the hold releases itself
	// when the deviation recovers.
	//   "reference values established by the dispatcher"
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	//   "Tavg will continue to drop until turbine loading is stopped
	//    either by manual operation or automatically upon reaching
	//    the low Tavg deviation setpoint signal."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:62-66
	ret.LowTavgDeviationHold = ret.Synchronized &&
		ret.State == TurbineRunning &&
		tavg.Float()-programmedRodTavg(ret.Setpoint) < TurbineLowTavgDeviationF
	if ret.LoadDemandRef > 0 && ret.State == TurbineRunning && !ret.LoadRunback {
		if ret.Setpoint < ret.LoadDemandRef {
			if !ret.LowTavgDeviationHold {
				ret.Setpoint = min(ret.Setpoint+ret.RampRate*dt.Seconds(), ret.LoadDemandRef)
			}
		} else if ret.Setpoint > ret.LoadDemandRef {
			ret.Setpoint = max(ret.Setpoint-ret.RampRate*dt.Seconds(), ret.LoadDemandRef)
		}
	}

	// Frequency-droop governor. When the infinite-bus frequency
	// deviates from GridRatedFrequency the DEHM adjusts the
	// governing valves so the unit picks up load (under-frequency)
	// or sheds load (over-frequency). The bias is a fraction of
	// rated per the droop constant: at 4% droop a 1% frequency
	// drop raises output by 25% of rated (1/0.04 = 25). The
	// per-step bias is capped at TurbineDroopBiasLimit to avoid
	// saturating the operator's setpoint on a large grid
	// disturbance.
	//
	// GridFrequency == 0 means "not wired to a grid yet"
	// (uninitialised sentinel for tests that don't set up the
	// grid model); in that case the droop correction is skipped
	// so the turbine behaves as it did before p13.
	//
	// Spec: "computes signals comparing turbine speed and
	// first-stage pressure with reference values established by
	// the dispatcher or initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	// "Wide range speed control of the turbine from turning
	// gear to rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
	ret.DroopBias = 0
	if ret.GridFrequency != 0 && ret.State == TurbineRunning {
		freqErr := ret.GridFrequency - GridRatedFrequency
		droopBias := -freqErr / (GridRatedFrequency * TurbineDroop)
		droopBias = min(droopBias, TurbineDroopBiasLimit)
		droopBias = max(droopBias, -TurbineDroopBiasLimit)
		ret.DroopBias = droopBias
		ret.Setpoint += droopBias
		ret.Setpoint = min(ret.Setpoint, 1.0)
		ret.Setpoint = max(ret.Setpoint, 0)
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

	// Low-vacuum trip. The LP exhaust limit is eight inches HgA; the
	// emergency trip plate sits just above it.
	//
	// Spec: "Low vacuum trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:26
	// "...exhaust pressures as high as eight inches HgA during
	// emergency operations."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_199.txt:94
	if ret.State == TurbineRunning && condPressure > TurbineLowVacuumTripPsia {
		ret.TripByVacuum = true
	}

	// Low bearing-oil-pressure trip. The emergency trip system uses
	// lubricating-system oil; loss of header pressure releases auto
	// stop oil and closes all steam control valves.
	//
	// Spec: "Low bearing oil pressure trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
	if ret.State == TurbineRunning &&
		float64(ret.BearingOilPressure) < TurbineLowBearingOilTripPsi {
		ret.TripByBearingOil = true
	}

	// Thrust-bearing trip: device 4 trips the machine through the
	// same emergency trip valve as devices 1-3; the spec prints no
	// setpoint, so the cause arrives as the ThrustBearingWorn
	// failure latch.
	//   "Thrust bearing trip"
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:34
	if ret.State == TurbineRunning && ret.ThrustBearingWorn {
		ret.TripByThrustBearing = true
	}

	// Any emergency-trip release closes all steam control valves and
	// shuts the extraction nonreturn valves via the air pilot.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:24-34
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:100-104
	if ret.TripByVacuum || ret.TripByBearingOil || ret.TripByThrustBearing {
		ret.ExtractionNRVClosed = true
		ret.GoverningValve.Open = 0
		ret.StopValve.Open = 0
		ret.State = TurbineTripped
	}

	// Reset clears all trip latches once the rotor has stopped.
	// ThrustBearingWorn is hardware damage, not a latch: reset does
	// not clear it, and a restarted machine re-trips while it stands.
	if reset && ret.Velocity < turbineStopThreshold {
		ret.OPCTrip = false
		ret.TripByVacuum = false
		ret.TripByBearingOil = false
		ret.TripByThrustBearing = false
		ret.ExtractionNRVClosed = false
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
			// A synchronized machine is held at synchronous speed
			// by the infinite bus it is paralleled with; its rotor
			// follows system frequency, not its own admitted-steam
			// balance. Without this lock, throttling admission for
			// a low initial load sags shaft speed, drags terminal
			// frequency out of the synchronism tolerance, and pops
			// the generator breaker.
			//
			// "The turbine-generator is brought up to synchronous
			// speed and paralleled on the grid."
			// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:52-53
			if ret.Synchronized {
				ret.Velocity = TurbineRatedSpeed
			} else if ret.SpeedDemand > 0 {
				// Wide-range speed control engages when the
				// operator has raised a pre-breaker speed demand;
				// with no demand selected the rotor follows
				// admitted steam.
				// "Wide range speed control of the turbine from
				// turning gear to rated speed."
				// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
				target := Velocity(ret.SpeedDemand * float64(TurbineRatedSpeed))
				rampStep := Velocity(float64(TurbineRatedSpeed) * TurbineSpeedRampRate * dt.Seconds())
				if ret.Velocity < target {
					ret.Velocity = min(ret.Velocity+rampStep, target)
				} else {
					ret.Velocity = max(ret.Velocity-rampStep, target)
				}
			} else {
				ret.Velocity += Velocity(steamFlow * dt.Seconds() * turbineRampGain)
				ret.Velocity = min(ret.Velocity, TurbineRatedSpeed)
			}
		} else {
			ret.Velocity -= Velocity(float64(ret.Velocity) * turbineCoastRate * dt.Seconds())
			ret.Velocity = max(ret.Velocity, 0)
		}
	default:
		ret.Velocity -= Velocity(float64(ret.Velocity) * turbineCoastRate * dt.Seconds())
		ret.Velocity = max(ret.Velocity, 0)
		if ret.TurningGear {
			ret.Velocity = max(ret.Velocity, TurbineTurningGearRpm)
		}
	}

	if ret.State == TurbineRunning &&
		float64(ret.Velocity) > float64(TurbineRatedSpeed)*TurbineOverspeedTripRatio {
		ret.State = TurbineTripped
	}

	// DEHM governing-valve trim: bias the valve so the first-stage
	// pressure tracks the active reference (the load Setpoint after
	// synchronization, the speed demand before it). With no active
	// reference the valve is left wherever the operator or the last
	// trim put it.
	//
	// Spec: "computes signals comparing turbine speed and first-stage
	// pressure with reference values established by the dispatcher or
	// initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	ref := 0.0
	if ret.Synchronized {
		ref = ret.Setpoint
	} else {
		ref = ret.SpeedDemand
	}
	if ret.State == TurbineRunning && ref > 0 && ret.StopValve.Open > 0 {
		errFrac := ref - float64(ret.FirstStagePressure)/float64(SteamGeneratorShellPressure)
		ret.GoverningValve.Open += GoverningValveTrimGain * errFrac
		ret.GoverningValve.Open = min(max(ret.GoverningValve.Open, 0), 1)
	}

	// RealPower tracks admitted steam energy through a short lag,
	// not shaft speed. The synchronized Setpoint is the ceiling the
	// DEHM enforces via the governing valves; Setpoint == 0 means
	// "no demand selected" and leaves the steam-derived power
	// uncapped.
	//
	// Spec: "After generator breaker closure, the ability to
	// increase or decrease electrical load at a controlled
	// selectable rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
	steamFrac := 0.0
	if CondenserDesignSteamLoadGalPer > 0 {
		steamFrac = steamFlow / CondenserDesignSteamLoadGalPer
	}
	steamFrac = min(max(steamFrac, 0), 1)
	if ret.State == TurbineRunning {
		alpha := dt.Seconds() / TurbinePowerResponseSeconds
		alpha = min(alpha, 1)
		targetPower := Watts(steamFrac * float64(TurbineRatedkW))
		ret.RealPower += Watts((float64(targetPower) - float64(ret.RealPower)) * alpha)
		if ret.Synchronized && ret.Setpoint > 0 {
			maxPower := ret.Setpoint * float64(TurbineRatedkW)
			ret.RealPower = Watts(min(float64(ret.RealPower), maxPower))
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
	loadFraction = max(loadFraction, 0)
	extraction := loadFraction * FeedwaterDesignFlow.Float()
	if ret.ExtractionNRVClosed {
		extraction = 0
	}
	ret.ExtractionSteamFlow = Volume{Val: extraction, Unit: u.Gallon}

	// HP exhaust quality falls with load fraction because the
	// HP turbine does less expansion work per pound of steam.
	ret.HPExhaustQuality = HPExhaustQualityFullLoad -
		(HPExhaustQualityFullLoad-HPExhaustQualityLowLoad)*(1-loadFraction)
	ret.HPExhaustQuality = max(ret.HPExhaustQuality, HPExhaustQualityLowLoad)
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
	inletTemp := Temperature{Val: inletTempF, Unit: u.Fahrenheit}
	// MSR receives the highest-pressure extraction steam and main
	// steam before the throttle valves for reheating. The total
	// steam available to the MSR reheater tubes is a fraction of
	// the HP steam flow; with ExtractionSteamFlow = total HP steam
	// flow, applying MSRDesignExtractionFraction gives the design
	// reheat duty.
	reheatFlow := Volume{Val: ret.ExtractionSteamFlow.Float() * MSRDesignExtractionFraction, Unit: u.Gallon}
	ret.MoistureSeparatorReheater = ret.MoistureSeparatorReheater.Step(dt,
		ret.HPExhaustPressure,
		ret.HPExhaustQuality,
		inletTemp,
		reheatFlow,
		ret.HPExhaustPressure*0.98,
	)

	// Blade erosion: LP blade erosion from moisture carryover
	// past the MSR. The spec names the invisible driver ("Highly
	// effective moisture removal provisions result in improved
	// cycle efficiency and minimization of blade erosion.",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:57-59)
	// but gives no rate; the carryover law and allowance are
	// derived (plans/p72_5.txt). Carryover uses INLET quality
	// (1 - InletQuality), scaled by the MSR's removal efficiency.
	// At design (quality 0.90) the residual is 0.005, below the
	// 0.007 allowance, so full-load dry operation stays zero.
	// At low load (quality 0.70) the residual is 0.015, above
	// the allowance, so erosion accumulates at
	// (0.015-0.007)*0.00005 = 4e-7/s, reaching 1.0 over ~700 h.
	// Steady, wet-stagnant operation does NOT erode: carryover
	// is the driver.
	inletMoisture := max(0, 1.0-ret.MoistureSeparatorReheater.InletQuality)
	residual := inletMoisture * (1.0 - MSRMoistureRemovalEfficiency)
	carryover := max(0, residual-MSRRemovalAllowance)
	if ret.State == TurbineRunning {
		ret.BladeErosionFrac = min(ret.BladeErosionFrac+carryover*BladeErosionRatePerSec*dt.Seconds(), 1)
		ret.Damage = max(ret.Damage, float32(ret.BladeErosionFrac))
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

// dumpModulationDemand holds the header at the shell-pressure setpoint:
// proportional on the fractional pressure error plus an integral trim
// that absorbs the residual offset. Tuning assumptions live on the gain
// constants (full opening at a 5 percent error; integral catching up
// over a few seconds). The trip-bypass and pressure-control branches of
// the dump controller share this law; DumpPressureIntegral stays
// single-writer because those branches are disjoint within one Step.
func (t *Turbine) dumpModulationDemand(
	sgPressure Pressure, dt time.Duration,
) float64 {
	errFrac := (float64(sgPressure) - float64(SteamGeneratorShellPressure)) /
		float64(SteamGeneratorShellPressure)
	integral := t.DumpPressureIntegral +
		SteamDumpModulationIntegralGain*errFrac*dt.Seconds()
	t.DumpPressureIntegral = min(max(integral, 0), 1)
	return min(max(SteamDumpModulationProportionalGain*errFrac+
		t.DumpPressureIntegral, 0), 1)
}
