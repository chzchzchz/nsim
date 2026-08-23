package core



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
	// (percent delta-k per unit power fraction). Derived: a
	// full-scale error commands ~7 pcm, i.e. about 0.7
	// percent-per-second of controlled drift toward the program,
	// fast against the 90 s thermal lag and slow enough that
	// pcm-level residuals never dominate.
	RodCtrlRhoSetpointPerFrac = 0.0067

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
)
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
// The inner loop slews rod demand until net reactivity tracks that
// setpoint; because the setpoint goes to zero exactly when power
// reaches target, the plant parks at balance with no residual
// reactivity to decay it further. Callers gate this on AutoControl
// and an unlatched trip; while tripped, Insertion is forced to 1
// regardless of demand and integrating would pre-spin withdrawal for
// a post-trip reset.
func (c *ControlRods) applyAutoControl(
	tavg Temperature,
	loadFraction float64,
	powerFraction float64,
	reactivityPct float64,
) {
	c.ProgrammedTavg = programmedRodTavg(loadFraction)
	program := programPowerFraction(c.ProgrammedTavg)
	if !c.programPrimed {
		c.programTargetFraction = powerFraction
		c.programPrimed = true
	}
	maxStep := RodCtrlProgramSlewPerSec * TimeStep.Seconds()
	c.programTargetFraction += min(max(program-
		c.programTargetFraction, -maxStep), maxStep)
	target := c.programTargetFraction
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
	c.DemandInsertion += slew * TimeStep.Seconds()
	c.DemandInsertion = min(max(c.DemandInsertion, 0), 1)
}
