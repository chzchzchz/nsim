package core

import "time"

// Axial power distribution instrumentation and dynamics.
//
// The observable is the axial offset (AO), reported on the bridge as
// axial flux difference (AFD): power in the top half of the core minus
// power in the bottom half, divided by total core power, in percent.
//   "axial offset — A measure of power distribution in the axial
//    direction, obtained by subtracting the power in bottom of core
//    from power at top of core and dividing the remainder by the
//    total core power to obtain a percentage."
//   specs/the_westinghouse_pressurized_water_react/glossary.md:22
//
// Operationally the quantity matters because rod motion is what skews
// it: rods enter the core from the top, so inserting the bank suppresses
// upper-core flux and drives the offset negative, while soluble-boron
// changes reposition the rods and therefore steer the distribution.
//   "Power distribution is controlled by adjusting the RCS boron
//    concentration which results in control rod movement, thus
//    affecting the power distribution."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:111-116
// The load-follow discipline is to keep the indicated AFD inside a
// target band by adjusting boron whenever the rods approach the band
// limits, which preserves full reserve capability:
//   "the plant maintains full reserve capability for returning to full
//    power at the design ramp rate of 5 percent per minute [...] This
//    capability is maintained by automatic adjustment of the boron
//    concentration [...], thereby keeping the control rods within its
//    axial flux distribution (AFD) target band [...] the control system
//    makes boron concentration adjustments whenever the control rods
//    approach the limits of their AFD target band."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:109-123
//   "the operator must insure that the indicated axial flux difference
//    (AFD) is maintained within the limits [...] plants may operate
//    with AFD maintained within a required target band"
//   specs/westinghouse_plant_operations/chapter_19/page_012.txt:4-10
//
// The model carries the flux split plus per-half deviations of the
// iodine/xenon inventories around their lumped means. The deviations
// reproduce the poison side of the axial feedback loop described in the
// spec: a skewed distribution builds xenon in the high-flux end over
// hours, and the accumulated poison forces the distribution back
// toward (and past) symmetry.
//   "xenon oscillations can occur when the power distribution is
//    skewed to one end of the core. After several hours the xenon-135
//    concentration builds up in the end of the core [...] the power
//    distribution is forced to the opposite end of the core."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:85-93

const (
	// CoreAxialOffsetUnroddedPct anchors the equilibrium offset of an
	// unrodded core. Derived assumption: the spec pins the control
	// objective ("controlled so that the power distribution closely
	// approximates that of an unrodded core at full power",
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:74-77)
	// without giving a number, so the unrodded shape is taken as the
	// zero of the offset scale.
	CoreAxialOffsetUnroddedPct float64 = 0

	// CoreAxialOffsetRodShiftPctPerUnitInsertion converts rod insertion
	// into the equilibrium offset shift (percent per unit fraction of
	// travel). Derived calibration binding the operational insertion
	// window to the target band: with banks nearly fully withdrawn in
	// normal operation (specs/westinghouse_plant_operations/
	// chapter_19/page_012.txt:12-14) the small insertions used for
	// maneuvering sweep AO across roughly one band width, so the rod
	// withdrawal that restores reserve simultaneously re-centers the
	// distribution (page_231.txt:109-123 coupling).
	CoreAxialOffsetRodShiftPctPerUnitInsertion = -100.0

	// CoreAFDTargetBandPct is the half-width of the AFD target band the
	// operator holds the indicated value inside. Derived: the plant
	// operations text mandates a restrictive administrative target band
	// (chapter_19/page_012.txt:4-10) without printing its number, so a
	// +/-5 percent band is assumed and only its existence is load-bearing.
	CoreAFDTargetBandPct = 5.0

	// CoreAxialOffsetTimeConstant sets how fast the realized offset
	// relaxes onto its equilibrium after a drive change. Derived:
	// the flux/power shape follows rod position far faster than the
	// multi-hour poison inventories evolve.
	CoreAxialOffsetTimeConstant = 60 * time.Second

	// CoreXenonSkewAOGainPct converts a normalized top-minus-bottom
	// xenon deviation into the equilibrium offset contribution
	// (percent AO per unit normalized deviation). Derived calibration:
	// sized so the fast burn-side reinforcement of a sustained skew
	// stays inside the target band through the reference daily cycle,
	// while the slower iodine-driven production imbalance carries the
	// restoring direction past the static rod-shift line
	// (page_226.txt:85-93).
	CoreXenonSkewAOGainPct = 12.0
)

// advanceAxialOffset evolves the axial offset and the per-half
// fission-product deviations over dt, sub-stepping like advanceXenon
// so callers may pass long horizons.
//
// Notation: X is the lumped physical xenon concentration, dXe its
// top-minus-bottom half-difference, I the normalized iodine inventory,
// dI the iodine half-difference, p the fission power fraction, and
// a the offset as a fraction (top share of flux is (1+a)/2). The
// per-half ODEs are the lumped pair applied with half-share driving,
// subtracted; production splits with flux share while burnup tracks
// each half's own concentration times its flux share:
//
//	dI/dt      = lambdaI * (a*p - dI)
//	d(dXe)/dt  = lambdaI * ((1-y)*dI + y*p*a)
//	             - lambdaX*dXe - burn*p*(X*a + dXe/2)
//
// The realized offset relaxes onto an equilibrium set by rod position
// minus the xenon-skew worth of the current half-difference.
func (r *ReactorPressureVessel) advanceAxialOffset(dt time.Duration) {
	const maxSubStep = 60 * time.Second
	n := int(dt.Seconds() / maxSubStep.Seconds())
	if dt.Seconds() > float64(n)*maxSubStep.Seconds() {
		n++
	}
	if n < 1 {
		n = 1
	}
	h := dt.Seconds() / float64(n)
	for range n {
		p := min(max(float64(r.FissionPower)/float64(CoreFissionPowerAtRated), 0), 1)
		aofrac := min(max(r.AxialOffset/100, -1), 1)

		lambdaI := CoreIodine135DecayRatePerSecond
		lambdaX := CoreXenon135DecayRatePerSecond
		dIod := lambdaI * (aofrac*p - r.Iodine135AxialDeviation)
		skewProduction := lambdaI * ((1-CoreXenonDirectYieldFraction)*r.Iodine135AxialDeviation +
			CoreXenonDirectYieldFraction*p*aofrac)
		skewDestruction := lambdaX*r.Xenon135AxialDeviation +
			CoreXenonBurnRateAtFullPowerPerSecond*p*
				(r.Xenon135*aofrac + r.Xenon135AxialDeviation/2)
		r.Iodine135AxialDeviation += dIod * h
		r.Xenon135AxialDeviation += (skewProduction - skewDestruction) * h

		target := CoreAxialOffsetUnroddedPct +
			CoreAxialOffsetRodShiftPctPerUnitInsertion*r.ControlRods.Insertion -
			CoreXenonSkewAOGainPct*xenonNormalized(r.Xenon135AxialDeviation)
		r.AxialOffset += (target-r.AxialOffset) *
			h / CoreAxialOffsetTimeConstant.Seconds()
		r.AxialOffset = min(max(r.AxialOffset, -100), 100)
	}
}
