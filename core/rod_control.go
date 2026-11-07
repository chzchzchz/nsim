package core

import "time"

// Rod Control System — the automatic rod control layer of the NSSS
// Power Control System. It holds Tavg at a programmed function of
// turbine load so reactor power follows the demand:
//
//	"The rod control system automatically adjusts the power level of
//	the reactor to match the power demanded by the turbine. The
//	needed control rod movement is based on the reactor coolant
//	temperature needed to meet the turbine power demand [...] and the
//	measured reactor coolant temperature."
//	specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-76
//	"The rod control system automatically controls RCS average
//	coolant temperature (Tavg) as a programmed function of turbine
//	load."
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27

const (
	// RodCtrlDeadbandFrac is the reactor-power fraction error
	// inside which the controller holds rod motion once
	// reactivity sits near balance. Derived magnitude; the spec
	// states a deadband exists without a number ("an expandable
	// temperature error control deadband",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:77-81).
	RodCtrlDeadbandFrac = 0.01

	// RodCtrlRhoSetpointPerFrac converts reactor-power fraction
	// error into the reactivity setpoint the chain should carry
	// (percent delta-k per unit power fraction). Recalibrated
	// against measured trajectories under the split temperature
	// feedback (plans/x/gaps.md item 15): the compounding fission
	// update folds power on pcm-scale reactivity residuals within a
	// few percent per minute, so a part-load error of several
	// percent must command tens of pcm for the integral inner loop
	// to outpace the fold; the earlier 0.0067 let half-load plateaus
	// sink to the protection floor before the error grew authority.
	RodCtrlRhoSetpointPerFrac = 0.03

	// RodCtrlRhoMaxSetpointPct clamps the reactivity setpoint
	// (percent delta-k). Derived alongside the setpoint gain.
	RodCtrlRhoMaxSetpointPct = 0.01

	// RodCtrlRhoRegainPerSecPerPct converts reactivity error
	// (percent delta-k) into rod-demand slew rate. Derived: the
	// multiplicative fission update turns any sustained pcm-level
	// defect into percent-per-second power drift, so this term
	// must outrun the drift it is braking; 10 pcm of error slews
	// demand at 5e-4 per second.
	RodCtrlRhoRegainPerSecPerPct = 5e-2

	// RodCtrlMaxDemandSlewPerSec caps the demand slew regardless
	// of error magnitude. Derived; sized so a post-dilution
	// chemistry rebalance (~5 percent of rod bank) completes
	// inside a minute.
	RodCtrlMaxDemandSlewPerSec = 3e-3

	// RodCtrlProgramSlewPerSec rate-limits the internal power
	// target so maneuvers stay quasi-static and the primary
	// temperatures track without inertia-driven overshoot.
	// "plant loading is continued to full power at a rate not to
	// exceed 5 percent of full power per minute."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
	RodCtrlProgramSlewPerSec = 0.05 / 60.0

	// RodCtrlTavgTrimSpanF is the measured-Tavg residual (°F) that
	// commands the trim's full authority. Derived magnitude: sized
	// above the program deadband so ordinary tracking noise never
	// saturates it, and small enough that a real secondary-line
	// offset reaches full trim.
	RodCtrlTavgTrimSpanF = 5.0

	// RodCtrlTavgTrimAuthorityFrac is the measured-Tavg trim's
	// maximum pull on the power target (fraction of rated). Derived:
	// a trim, not an override - the load-demand loop owns the
	// operating point and the measured temperature corrects its
	// residual against the actual heat balance.
	// "The needed control rod movement is based on the reactor
	//  coolant temperature needed to meet the turbine power demand
	//  [...] and the measured reactor coolant temperature."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-76
	RodCtrlTavgTrimAuthorityFrac = 0.05

	// RodCtrlTavgTrimParkBand is the power-error width over which
	// the measured-Tavg trim fades in. Derived alongside the
	// authority: while the program walks, the thermal lag guarantees
	// a large same-sign temperature residual and the power error
	// already owns the correction - giving the trim authority there
	// double-counts the lag and sinks the descent through the
	// low-steamline setpoint. Near the park the walk is done and a
	// residual is genuine line offset.
	RodCtrlTavgTrimParkBand = 0.05

	// CoreShutdownBankWorthShare is the shutdown banks' share of
	// the full rod-bank worth; the control banks carry the rest.
	// Derived convention: the sources name the bank sequence but no
	// worth split ("The shutdown banks of the reactor control rods
	// are withdrawn. A corresponding boron concentration is
	// calculated so that reactor criticality will occur at a
	// selected control bank position above the rod insertion
	// limit.").
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:25-28
	CoreShutdownBankWorthShare = 0.3

	// RodShutdownBankCount and RodControlBankCount are the bank
	// counts of each group. The plant operates "All shutdown banks
	// and control banks A and B must be fully withdrawn, while
	// control banks C and D must be withdrawn to positions that are
	// greater than the limits".
	// specs/westinghouse_plant_operations/chapter_19/page_012.txt:37-40
	RodShutdownBankCount = 2
	RodControlBankCount  = 4

	// RodBankOverlapFrac is the overlap between successive control
	// banks as a fraction of one bank's travel: an incoming bank
	// starts moving when the outgoing bank has covered half its own,
	// which flattens the differential worth into the linear curve
	// the reactivity model already assumes. Operators withdraw the
	// banks "while observing proper bank overlap".
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:55-58
	RodBankOverlapFrac = 0.5

	// RodDroppedBankInsertion is the aggregate-insertion floor a
	// dropped bank holds (convention; the sources give no worth
	// figure for one dropped group). Sized deep enough that the
	// movable compensation authority cannot recover it inside the
	// protective-response window.
	// specs/pwr_sim/chapter_4/page_049.txt:61-67 (secondary)
	RodDroppedBankInsertion = 0.85
)

// Bank indications decompose the lumped rod path into the physical
// bank groups. The aggregate Insertion stays the reactivity driver;
// these maps give the operator the per-bank positions behind it:
// shutdown banks travel first during withdrawal, then the four
// control banks in sequence with overlap.
//
// Spec justification:
//   - "The shutdown banks of the reactor control rods are
//     withdrawn. [...] The control rod banks are then manually
//     withdrawn to achieve criticality while observing proper bank
//     overlap, rod position indications, and nuclear instrumentation."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:25-26,55-58
//   - Shutdown banks fully withdrawn before control-bank motion:
//     "Verify all shutdown banks are fully withdrawn within 15
//     minutes of withdrawing control banks."
//     specs/westinghouse_plant_operations/chapter_19/page_017.txt:7-8

// ShutdownBankInsertion returns the common insertion fraction of the
// shutdown banks for the aggregate position. Withdrawal progress w =
// 1-Insertion spends its first CoreShutdownBankWorthShare on the
// shutdown group, so a trip (Insertion=1) reads every bank fully
// inserted.
func (c ControlRods) ShutdownBankInsertion() float64 {
	w := 1 - min(max(c.Insertion, 0), 1)
	sdOut := min(w/CoreShutdownBankWorthShare, 1)
	return 1 - sdOut
}

// ControlBankInsertions returns the per-bank insertion fractions of
// the four control banks (A first on withdrawal) under the standard
// half-travel overlap program. The overlapped segments tile the
// control share of travel so their worth-weighted sum matches the
// linear aggregate curve exactly.
func (c ControlRods) ControlBankInsertions() [RodControlBankCount]float64 {
	var out [RodControlBankCount]float64
	w := 1 - min(max(c.Insertion, 0), 1)
	wc := max(w-CoreShutdownBankWorthShare, 0) /
		(1 - CoreShutdownBankWorthShare)
	unit := 1 / (float64(RodControlBankCount) -
		float64(RodControlBankCount-1)*RodBankOverlapFrac)
	for k := range RodControlBankCount {
		start := float64(k) * unit * (1 - RodBankOverlapFrac)
		outFrac := min(max((wc-start)/unit, 0), 1)
		out[k] = 1 - outFrac
	}
	return out
}

// programmedRodTavg maps turbine load demand onto the Tavg program,
// anchored at hot-standby no-load (557°F) and the rated inlet/outlet
// midpoint, clamped outside [0,1].
func programmedRodTavg(loadFraction float64) float64 {
	load := min(max(loadFraction, 0), 1)
	return RCSNoLoadTavgF + load*(RCSFullLoadTavgF-RCSNoLoadTavgF)
}

// programPowerFraction inverts the Tavg program for the fission-power
// fraction that settles there. The program itself is linear between
// the no-load and full-load anchors, and the plant's rated lineup
// anchors full-load Tavg at RCSFullLoadTavgF, so the inverse is the
// same line read backwards; programs below the no-load floor resolve
// to zero power (hot zero power).
func programPowerFraction(programTavgF float64) float64 {
	span := RCSFullLoadTavgF - RCSNoLoadTavgF
	if span <= 0 {
		return 0
	}
	return min(max((programTavgF-RCSNoLoadTavgF)/span, 0), 1)
}

// applyAutoControl is a cascade regulator. The outer loop turns
// reactor-power error into a reactivity setpoint: power above the
// program point (programPowerFraction inverts the primary temperature
// map) maps to a negative reactivity target, i.e. insertion, and the
// chain drifts down at a rate proportional to how far off it sits.
// The measured Tavg folds into that target as a bounded trim: a hot
// atmosphere means the heat balance sits above the program even with
// power on target, so the controller trims power down until the
// temperature residual closes. The inner loop slews rod demand until
// net reactivity tracks the setpoint; because the setpoint goes to
// zero exactly when power reaches target, the plant parks at balance
// with no residual reactivity to decay it further. Callers gate this
// on AutoControl and an unlatched trip; while tripped, Insertion is
// forced to 1 regardless of demand and integrating would pre-spin
// withdrawal for a post-trip reset.
func (c *ControlRods) applyAutoControl(dt time.Duration,
	tavg Temperature,
	loadFraction float64,
	powerFraction float64,
	reactivityPct float64, demandFraction float64,
) {
	c.ProgrammedTavg = programmedRodTavg(loadFraction)
	program := programPowerFraction(c.ProgrammedTavg)
	if !c.programPrimed {
		c.programTargetFraction = powerFraction
		c.programPrimed = true
	}
	maxStep := RodCtrlProgramSlewPerSec * dt.Seconds()
	c.programTargetFraction += min(max(program-
		c.programTargetFraction, -maxStep), maxStep)
	target := c.programTargetFraction
	// Measured-Tavg trim: a hot residual trims the power target
	// down, a cold one up, bounded so the demand loop keeps the
	// operating point. The trim fades in only near the park: mid-
	// maneuver the thermal lag produces a large same-sign residual
	// that the power error already corrects.
	parkErr := c.programTargetFraction - powerFraction
	if parkErr < 0 {
		parkErr = -parkErr
	}
	parkBand := 1 - min(parkErr/RodCtrlTavgTrimParkBand, 1)
	tavgTrimFrac := -(tavg.Float() - c.ProgrammedTavg) /
		RodCtrlTavgTrimSpanF * RodCtrlTavgTrimAuthorityFrac * parkBand
	target += min(max(tavgTrimFrac, -RodCtrlTavgTrimAuthorityFrac),
		RodCtrlTavgTrimAuthorityFrac)
	// Inside the deep part-load band a hot residual that would trim the
	// target below the deadband is clamped at the deadband, per the
	// spec's expandable reduced-power deadband
	// (specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:77-81)
	// and the house-load manual-transfer posture
	// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:156-159).
	// The park band fades at RodCtrlTavgTrimParkBand but the deadband
	// stays; the park-relative residual (22.6F line offset, probe)
	// keeps full-weight trim above the deadband, so a hot park trims
	// only as far as the deadband and can still climb back when the
	// park cools — hold power cannot be driven through that floor.
	if demandFraction <= CVCSAutoBoronParkMaxLoadFrac && target < RodCtrlDeadbandFrac {
		target = RodCtrlDeadbandFrac
	}
	errFrac := target - powerFraction

	rhoSetpoint := RodCtrlRhoSetpointPerFrac * errFrac
	rhoSetpoint = min(max(rhoSetpoint,
		-RodCtrlRhoMaxSetpointPct), RodCtrlRhoMaxSetpointPct)

	if errFrac > -RodCtrlDeadbandFrac && errFrac < RodCtrlDeadbandFrac &&
		reactivityPct > -RodCtrlRhoMaxSetpointPct/5 &&
		reactivityPct < RodCtrlRhoMaxSetpointPct/5 {
		return
	}
	maxSlew := RodCtrlMaxDemandSlewPerSec
	slew := min(max(RodCtrlRhoRegainPerSecPerPct*
		(reactivityPct-rhoSetpoint), -maxSlew), maxSlew)
	c.DemandInsertion += slew * dt.Seconds()
	c.DemandInsertion = min(max(c.DemandInsertion, 0), 1)
}
