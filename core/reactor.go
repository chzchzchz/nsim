package core

import (
	"math"
	"time"

	u "github.com/bcicen/go-units"
)

// Step advances the reactor pressure vessel by TimeStep. It
// implements the primary-side thermal model and the minimal
// reactivity model described in plans/p7_1.txt and plans/p8_1.txt.
//
// Spec justification for the underlying physics:
//   - Rod cluster control: "Rod cluster control (RCC) assemblies are
//     used for reactor startup or shutdown, to follow load changes,
//     and to control small transient changes in reactivity."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:90-93
//     "Full length RCCs provide operational reactivity control and
//     can shut the reactor down at all times, even with the most
//     reactive RCC stuck out of the core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:108-114
//   - Doppler defect: "This defect is due to an increase in width of
//     uranium-238 resonance peaks in the epithermal energy range.
//     […] the effective neutron temperature increases by about
//     1000°F (540°C) as the average physical pellet temperature is
//     increased with heat generation."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:89-96
//   - Power defect: "The negative reactivity associated with an
//     increase in reactor core power due to temperature feedback
//     effects from an increase in fuel and moderator temperature."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
//   - Moderator temperature coefficient: varies with boron
//     concentration; positive at beginning-of-life, negative at
//     end-of-life.
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:54-55
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
//   - Decay heat removal: "The system is designed to reduce the RCS
//     temperature to 140°F (60°C) within 20 hours following reactor
//     shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:104-105
//   - EFWS purpose: "The EFWS thereby maintains the capability of
//     the steam generators to remove plant stored heat and core
//     decay heat by converting the emergency feedwater to steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:28-31
//   - RCS design temperatures: "Reactor coolant inlet temperature:
//     621°F (327°C); outlet: 558°F (292°C)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
//   - Design full-load average temperature: "about 571°F (300°C)"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:115-116
//   - RPV design pressure: "designed for full system pressure of
//     2500 psia (172.3 bar a)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_054.txt:20
//   - RCP design capacity / head: "Design capacity 97,600 gpm
//     (6.15 m3/sec) Design head 280 ft (85.3 m)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:124-130
//     (cited for context; not directly used in the model).
func (r *ReactorPressureVessel) Step(rcpSpeed Velocity, secondaryExcess Watts) ReactorPressureVessel {	ret := *r
	dt := TimeStep

	// 0) Trip forces full rod insertion per the scram definition:
	// "reactor trip — When control rods are rapidly inserted into
	// the core to quickly take the reactor to a subcritical state."
	// specs/the_westinghouse_pressurized_water_react/glossary.md:142
	// While Trip is asserted, Insertion coerces to 1.0 (a gravity
	// scram, bypassing the operator's slow rate-limited drive)
	// regardless of DemandInsertion.
	if ret.Trip {
		ret.ControlRods.Insertion = 1.0
	}

	// 1) Time-since-trip accumulation.
	if ret.Trip {
		ret.TimeSinceTrip += dt
	} else {
		ret.TimeSinceTrip = 0
	}

	// 2) Primary-side thermal model. Heat flow drives the coolant
	// temperature: the relaxation strength scales with produced
	// power, so a zero-fission core holds its temperature instead of
	// being dragged toward the full-load reference (cold and hot-soak
	// states stay put until an active cooling system takes over).
	// Its remaining authority shrinks further by the share of
	// production pulled out through active sinks (secondaryExcess),
	// so sink-dominated operation integrates the energy balance
	// instead of fighting it — this is what lets steam-dump flow or
	// the RHRS carry a planned cooldown below the no-load program
	// value.
	//
	// "Steam dumps are adjusted to start a 50°F/hr (28°C/hr)
	// cooldown rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:73-74
	powerFrac := float64(ret.TotalThermalPower()) / float64(CoreRatedThermalPower)
	powerFrac = max(powerFrac, 0)

	// Thermosiphon channel: the SGs sit above the core, so a hot
	// loop carries its own buoyancy head against loop resistance
	// once forced flow is gone. The sqrt law is the head-vs-
	// resistance balance; the first-order lag is the loop
	// transport delay.
	//   "The higher elevation of the steam generators relative to
	//    the reactor vessel produces a thermal driving head to
	//    establish and maintain flow in the RCS"
	//   specs/westinghouse_tech_manual/chapter_3/page_0325.txt:11-13
	pumpedFrac := min(max(float64(rcpSpeed)/float64(RCPRatedSpeed), 0), 1)
	headDeltaT := max(r.Outlet.Float()-r.Inlet.Float(), 0)
	ncTarget := min(NatCircHeadGainPerDegF*math.Sqrt(headDeltaT),
		NatCircMaxFlowFrac)
	ret.NaturalCircFlowFrac += (ncTarget - ret.NaturalCircFlowFrac) *
		dt.Seconds() / NatCircTimeConstant
	totalFlow := min(pumpedFrac+ret.NaturalCircFlowFrac, 1.0)
	transportCapW := totalFlow * float64(CoreRatedThermalPower)

	refDeltaF := CoreReferenceOutletF - CoreReferenceInletF
	// Flow-degraded per-pass temperature rise: at fixed power the
	// coolant temperature rise across the core scales inversely with
	// total loop flow (delta-T proportional to power over flow), so
	// a lost or coasting machine raises the hot-leg temperature the
	// loop must reject, and a pumpless plant rides its thermosiphon.
	// Applied only above the at-power permissive, where reduced
	// forced flow is an off-normal condition rather than the normal
	// sequential-start / hot-standby lineup; at full flow the divisor
	// is one and nothing changes. The loss-of-flow margin challenge
	// this represents: "the cold leg temperature in loop #1 will
	// increase rapidly" (specs/pwr_sim/chapter_4/page_052.txt:26-27).
	flowDivisor := 1.0
	if totalFlow > 0 && powerFrac >= RPSAtPowerPermissiveFraction {
		flowDivisor = min(max(totalFlow, CoreFlowDegradedFloorFrac), 1.0)
	}
	targetOutlet := CoreReferenceInletF + powerFrac*refDeltaF/flowDivisor

	coreCapabilityW := powerFrac * float64(CoreRatedThermalPower)
	retained := 1.0
	if secondaryExcess > 0 {
		accepted := min(float64(secondaryExcess), transportCapW)
		if accepted >= coreCapabilityW {
			retained = 0
		} else {
			retained = 1 - accepted/coreCapabilityW
		}
	}

	outletF := ret.Outlet.Float()
	// Heating needs produced power; passive cooling toward a lower
	// target (steam dump, SG heat sink) runs unscaled, and both
	// directions are bounded by the operational program rate: Tavg
	// follows the program, it does not jump to it.
	//
	// Spec: "Primary plant heatup rate is limited to 50°F per hour."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
	// "Steam dumps are adjusted to start a 50°F/hr (28°C/hr)
	// cooldown rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:153-154
	relax := dt.Seconds() / PrimaryThermalTimeConstant * retained
	if targetOutlet > outletF {
		relax *= powerFrac
	}
	outletStep := (targetOutlet - outletF) * relax
	maxStep := OpHeatupRateLimit * dt.Seconds()
	outletStep = min(max(outletStep, -maxStep), maxStep)
	outletF += outletStep

	// Active-sink energy balance (secondary excess draw): primary
	// stored heat leaves at the m*cp rate scaled by the deficit's
	// share of production. The flowing loop caps what the sink can
	// accept — with pumps running the cap sits far above any dump
	// demand, while a pumpless plant accepts only decay-heat-sized
	// draws through its thermosiphon ("Natural circulation flow is
	// sufficient for removal of reactor decay heat",
	// specs/westinghouse_tech_manual/chapter_3/page_0328.txt:33).
	// A subcritical core under dump/RHRS draw gets the full rate;
	// at power the same draw is ordinary load-following absorbed by
	// the secondary, not primary cooling. Bounded by the operational
	// program envelope either way.
	if secondaryExcess > 0 {
		accepted := min(float64(secondaryExcess), transportCapW)
		share := 1.0
		if coreCapabilityW > accepted {
			share = accepted / coreCapabilityW
		}
		deltaTF := -share * accepted /
			(RCSMassKg * SpecificHeatWater) * dt.Seconds() * 9.0 / 5.0
		deltaTF = min(max(deltaTF, -maxStep), maxStep)
		outletF += deltaTF
	}
	outletF = max(outletF, 0)
	ret.Outlet = Temperature{Val: outletF, Unit: u.Fahrenheit}

	// The cold-leg / downcomer inlet temperature follows its
	// program line whenever anything turns the loop over — the
	// pumps or the thermosiphon. The gate is actual circulation,
	// not the operator latch or rotor speed alone: if the ESF
	// breaker opens the pump coasts down even though Running=true
	// is latched, and only buoyancy keeps the loop moving.
	if totalFlow > 0 {
		// Cold-leg program: the loop ΔT lives on the hot-leg rise,
		// so the inlet holds its own anchor line from the shared
		// no-load temperature to the rated cold-leg value. Mixing
		// the inlet toward the outlet instead collapses the loop
		// ΔT quadratically with load and drags every Tavg-anchored
		// consumer (rod-control program, CVCS level program, steam
		// dump reference) off its heat-balance line.
		targetInlet := RCSNoLoadTavgF + powerFrac*
			(CoreReferenceInletF-RCSNoLoadTavgF)
		inletF := ret.Inlet.Float()
		// Power-scaled like the outlet: no produced heat means no
		// forcing toward the full-load cold-leg reference.
		inletF += (targetInlet - inletF) *
			(dt.Seconds() / PrimaryInletTimeConstant) * powerFrac
		inletF = max(inletF, 0)
		ret.Inlet = Temperature{Val: inletF, Unit: u.Fahrenheit}
	}

	// 3) Pressure relaxes toward the RPV design pressure. The
	// pressurizer (p5) sets the actual RCS pressure; this is the
	// design baseline the RPV itself sits at in steady state. The
	// upward pull scales with produced power like the thermal
	// relaxation above: nothing passive raises a depressurized
	// primary toward the design point, so a cold plant holds the
	// operator-maintained pressure instead of drifting up it.
	// "The RCS and pressurizer are completely filled with borated
	// water and the system pressure is maintained by charging and
	// letdown flow control."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:94-97
	pressure := float64(ret.Pressure)
	pressureStep := (float64(RPVDesignPressure) - pressure) *
		(dt.Seconds() / PrimaryPressureTimeConstant)
	if pressureStep > 0 {
		pressureStep *= powerFrac
	}
	pressure = max(pressure+pressureStep, 0)
	ret.Pressure = Pressure(pressure)

	// 4) Reactivity contributions. The rod-bank term follows the
	// ABCD rod-worth curve (plans/p7_1.txt). The Doppler term
	// captures fuel-temperature feedback. The boron term
	// converts the CVCS boron concentration to reactivity
	// using a linear worth model centred on CVCSInitialBoron;
	// positive ΔBoron decreases reactivity (boron is a
	// neutron absorber).
	//
	// Spec: "the moderator temperature coefficient is positive
	// at low power (BOL), and [...] the boron concentration
	// changes significantly with fuel depletion."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
	// Rod insertion adds negative reactivity proportional to the
	// inserted fraction (Insertion=1 on a trip yields the full
	// shutdown worth). Conversely withdrawing (Insertion→0) removes
	// that negative margin so the core approaches criticality and
	// output can rise; see plans/x/trip.md.
	//
	// Spec: "reactor trip — When control rods are rapidly inserted
	// into the core to quickly take the reactor to a subcritical
	// state."
	// specs/the_westinghouse_pressurized_water_react/glossary.md:142
	//   - "Withdraw rods to criticality and raise output to power
	//     range level."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_223.txt:172
	// All reactivity contributions are combined on one scale:
	// percent Δk/k (the power-defect correction divides by 100).
	// 1 pcm = 0.001% Δk/k, so pcm-valued coefficients convert
	// with a factor of 1/1000.
	rodPct := float32(ret.ControlRods.Insertion) * CoreRodWorthABCD * 100
	outletFClamped := min(max(outletF, DopplerClampLowF), DopplerClampHighF)
	dopplerDeltaF := outletFClamped - float64(CoreDopplerReferenceTempF)
	// The temperature feedback splits into two regimes that the
	// combined-coefficient model conflated. The fuel-side Doppler /
	// power-defect base is a constant negative slope: "power defect -
	// The negative reactivity associated with an increase in reactor
	// core power due to temperature feedback effects from an increase
	// in fuel and moderator temperature."
	//   specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
	// On top of it the moderator coefficient varies with RCS boron
	// concentration: slightly positive at beginning of life with full
	// soluble-boron shim, substantially negative at end of life with
	// boron depleted, so the combined slope stays negative across the
	// boron range and load-follow self-regulation has a stable fixed
	// point:
	//   "reactor power is maintained equal to load by virtue of
	//    feedback effects inherent in the physics of a PWR"
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:69-71
	//   "At the beginning of life, the effect on reactivity is
	//    small since the moderator temperature coefficient is
	//    small. At the end of life, the coefficient is
	//    substantially more negative due to depletion and
	//    removal of shim boron [...]"
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
	// The Doppler base scales with the power fraction: it is a
	// fuel-heating effect, so a cold zero-power core carries no
	// Doppler deficit and cold-shutdown margin stays a boron/rod
	// question rather than a temperature artifact.
	dopplerPct := float32(powerFrac*dopplerDeltaF*CoreDopplerBasePcmPerDegF) / 1000
	moderatorPct := float32(dopplerDeltaF*
		netTempCoef(ret.ControlRods.BoronConcentration)) / 1000
	boronDelta := (ret.ControlRods.BoronConcentration - CVCSInitialBoron) / 100.0
	boronPct := float32(boronDelta) * CoreBoronWorthPer100ppm
	// Xenon contributes reactivity relative to its rated-power
	// equilibrium: the rod/boron calibration above already carries the
	// nominal equilibrium load (operators have diluted against it), so
	// deviations produce the correct signs, xenon above equilibrium
	// negative, below positive, without disturbing existing equilibria.
	// The inventory enters flux-weighted across the two core halves so
	// an axial skew weights the poison where the flux actually sits;
	// with zero deviation this reduces to the lumped form exactly.
	axialFrac := ret.AxialOffset / 100
	xenonPct := float32(CoreXenonEquilibriumWorthPct *
		(xenonNormalized(ret.Xenon135+axialFrac*ret.Xenon135AxialDeviation/2) - 1.0))
	ret.Reactivity = rodPct + dopplerPct + moderatorPct + boronPct + xenonPct

	// 5) Thermal power = fission chain power + fission-product
	// decay heat (+ RCP shaft loss, added below). The two nuclear
	// components obey different physics and are evolved separately:
	//
	//   - Fission power responds to reactivity: the quasi-static
	//     power-defect relation scales the previous fission power
	//     by 1 + rho/100 per reference tick. One equation set governs
	//     operation and shutdown alike; a scram simply drives rho
	//     negative via full rod insertion
	//     (specs/the_westinghouse_pressurized_water_react/glossary.md:142)
	//     and the chain collapses on its own.
	//   - Decay heat is fission-product beta decay: production is
	//     proportional to fission power while the inventory drains
	//     at fixed radioactive decay constants that reactivity
	//     cannot influence (specs/wiki/Decay_heat.txt).
	//
	// The compounding is anchored to DefaultTimeStep and raised to
	// the elapsed tick count, so the trajectory is a function of
	// simulated time alone: one 10 ms Step lands where ten 1 ms
	// Steps would. Without this anchoring a widened tick slows every
	// closed loop that steers against fission power.
	base := 1.0 + float64(ret.Reactivity)/100.0
	if base < 0 {
		base = 0
	}
	powerCorrection := math.Exp(math.Log(base) *
		(dt.Seconds() / DefaultTimeStep.Seconds()))
	ret.FissionPower = Watts(float64(ret.FissionPower) * powerCorrection)
	// The startup neutron sources keep a minimal fission population
	// alive even deep in subcritical conditions; without this floor
	// the purely multiplicative update could never raise power from
	// an exactly zero inventory and no scrammed core could ever
	// return to power.
	//   "Reactor startup neutron sources must be used because of fuel
	//    configuration and the initially low core activity."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_041.txt:18-20
	ret.FissionPower = max(ret.FissionPower, CoreNeutronSourcePower)
	ret.FissionPower = min(ret.FissionPower, CoreRatedThermalPower)

	// Nuclear-instrumentation observables. The source-range count
	// rate tracks the flux that drives the proportional counters;
	// the startup-rate reading low-passes the instantaneous log
	// derivative the way a control-room period amplifier displays
	// it. "Evidence of criticality is indicated by a positive
	// sustained startup rate with no rod motion and increasing
	// source range counts."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:161-163
	ret.SourceRangeCounts = CoreSourceRangeCountsPerWatt *
		float64(ret.FissionPower)
	prevW := float64(r.FissionPower)
	surInst := 0.0
	if prevW > 0 && dt.Seconds() > 0 {
		surInst = math.Log(float64(ret.FissionPower)/prevW) /
			dt.Seconds() / math.Ln10 * 60
	}
	filter := CoreStartupRateFilterTimeConstant.Seconds()
	ret.StartupRateDPM += (surInst - ret.StartupRateDPM) *
		min(dt.Seconds()/filter, 1)
	dtS := dt.Seconds()
	fastTarget := float64(ret.FissionPower) * CoreDecayHeatFastFraction
	slowTarget := float64(ret.FissionPower) * CoreDecayHeatSlowFraction
	ret.DecayHeatFast += Watts((fastTarget - float64(ret.DecayHeatFast)) * dtS / CoreDecayHeatFastTimeConstant.Seconds())
	ret.DecayHeatSlow += Watts((slowTarget - float64(ret.DecayHeatSlow)) * dtS / CoreDecayHeatTimeConstant.Seconds())
	ret.advanceXenon(dt)
	ret.advanceAxialOffset(dt)
	// RCP shaft-loss heat enters the primary coolant (chapter
	// 16 temperature balance). The rated cap and the pump
	// contribution are applied in TotalThermalPower, the number
	// the ReactorBuilding wiring routes into SteamGenerator.HeatInput.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
	return ret
}

// TotalThermalPower is the per-step heat flux consumed by the
// downstream systems: fission power plus fission-product decay
// heat, capped at rated, plus RCP shaft loss.
func (r *ReactorPressureVessel) TotalThermalPower() Watts {
	nuclear := r.FissionPower + r.DecayHeatFast + r.DecayHeatSlow
	return max(min(nuclear, CoreRatedThermalPower), 0) + r.PumpHeatInput
}

// SeedSteadyStatePower initializes the fission and fission-product
// inventories to the equilibrium of steady operation at total
// thermal power p, with decay heat contributing its
// shutdown-fraction share (specs/wiki/Decay_heat.txt). A zero-valued
// vessel is a core with no operating history: no fission power, no
// decay heat.
func (r *ReactorPressureVessel) SeedSteadyStatePower(p Watts) {
	r.FissionPower = Watts(float64(p) * (1 - CoreDecayHeatShutdownFraction))
	r.DecayHeatFast = Watts(float64(p) * CoreDecayHeatFastFraction)
	r.DecayHeatSlow = Watts(float64(p) * CoreDecayHeatSlowFraction)
	pFraction := min(float64(p)/float64(CoreRatedThermalPower), 1.0)
	r.Iodine135 = pFraction
	r.Xenon135 = xenonEquilibriumPhysical(pFraction)
}

// Xenon-135 fission-product model constants. Inventories are
// normalized to their rated-power equilibrium; rates are per second.
//
// Spec justification:
//   - "Produced via iodine-135, Xenon-135 is a fission product with
//     a 6.7 hour half-life." (iodine half-life; the Westinghouse doc
//     gives no other xenon data)
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:164-165
//   - Xe-135 half-life 9.14 h; "the yield of xenon-135 from fission is
//     about 6.6% (uranium) ... the great majority from iodine-135";
//     "95% of the 135Xe production is from decay of 135I"; capture at
//     1e14 flux: "2.65x10-4 s-1, which corresponds to a half-life of
//     about one hour"; equilibrium in "about 40 to 50 hours"; post-
//     shutdown peak "about 11 hours after reactor power is decreased".
//     specs/wiki/Xenon-135.txt
const (
	// Derived from the spec's 6.7 hour iodine half-life.
	CoreIodine135DecayRatePerSecond = math.Ln2 / (6.7 * 3600.0)

	// From the cached article's 9.14 hour xenon half-life.
	CoreXenon135DecayRatePerSecond = math.Ln2 / (9.14 * 3600.0)

	// Full-power xenon burnup rate: sigma*phi from the article's
	// 2.65e6 barn / 1e14 n/cm2/s worked example.
	CoreXenonBurnRateAtFullPowerPerSecond = 2.65e-4

	// Direct fission yield share of xenon production; the article
	// puts ~95% of xenon production on the iodine-decay path.
	CoreXenonDirectYieldFraction float64 = 0.05

	// CoreXenonIodineToXenonEquilibriumRatio is the rated-equilibrium
	// iodine-to-xenon inventory ratio. Because rated burnup suppresses
	// the absolute xenon inventory far below the iodine inventory, one
	// unit of normalized iodine corresponds to ~R units of rated-
	// normalized xenon worth: this is what produces the classic multi-
	// equilibrium post-shutdown iodine pit. Derived:
	// (lambdaX + burnup) / lambdaI. It converts the stored physical
	// xenon concentration into rated-normalized units at the worth
	// site (xenonNormalized).
	CoreXenonIodineToXenonEquilibriumRatio = (CoreXenon135DecayRatePerSecond +
		CoreXenonBurnRateAtFullPowerPerSecond) /
		CoreIodine135DecayRatePerSecond

	// CoreXenonEquilibriumWorthPct is the reactivity contribution of
	// one unit of normalized xenon inventory, percent delta-k/k.
	// Assumption: the sources give cross-section and yields but no
	// integrated worth; the magnitude is set to a typical PWR
	// equilibrium-xenon worth (~2.7% dk/k) and only its sign and
	// order are load-bearing in the scenario bands.
	CoreXenonEquilibriumWorthPct float64 = -2.7
)

// advanceXenon evolves the iodine-135 / xenon-135 fission-product
// pair over dt, sub-stepping so callers may pass long horizons (the
// plant step passes TimeStep). Iodine-135 is normalized to its rated-
// power equilibrium (1.0 at rated); Xenon135 is carried as the
// physical concentration relative to the normalized iodine scale,
// so its rated equilibrium sits at lambdaI/(lambdaX+burn) ~= 0.1 and
// readers convert through xenonNormalized.
//
// Spec justification:
//   - "Produced via iodine-135, Xenon-135 is a fission product with
//     a 6.7 hour half-life."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:164-165
//   - "When reactor power is decreased or shut down [...] the 135Xe
//     concentration peaks about 11 hours after reactor power is
//     decreased" and "95% of the 135Xe production is from decay of
//     135I"
//     specs/wiki/Xenon-135.txt
func (r *ReactorPressureVessel) advanceXenon(dt time.Duration) {
	const maxSubStep = 60 * time.Second
	n := int(math.Ceil(dt.Seconds() / maxSubStep.Seconds()))
	if n < 1 {
		n = 1
	}
	h := dt.Seconds() / float64(n)
	for range n {
		p := min(max(float64(r.FissionPower)/float64(CoreFissionPowerAtRated), 0), 1)
		lambdaI := CoreIodine135DecayRatePerSecond
		lambdaX := CoreXenon135DecayRatePerSecond
		r.Iodine135 += (p - r.Iodine135) * lambdaI * h
		production := ((1-CoreXenonDirectYieldFraction)*r.Iodine135 +
			CoreXenonDirectYieldFraction*p) * lambdaI
		destruction := (lambdaX + CoreXenonBurnRateAtFullPowerPerSecond*p) * r.Xenon135
		r.Xenon135 += (production - destruction) * h
		r.Iodine135 = max(r.Iodine135, 0)
		r.Xenon135 = max(r.Xenon135, 0)
	}
}

// xenonEquilibriumPhysical returns the physical steady-state xenon-135
// concentration at fractional rated power, on the same scale the ODE
// integrates: Xeq(p) = lambdaI*p / (lambdaX + burn*p). At rated this
// is ~0.1003 because burnup holds the inventory far below iodine.
func xenonEquilibriumPhysical(p float64) float64 {
	return CoreIodine135DecayRatePerSecond * p /
		(CoreXenon135DecayRatePerSecond +
			CoreXenonBurnRateAtFullPowerPerSecond*p)
}

// xenonNormalized converts a stored physical xenon-135 concentration
// into rated-normalized units (1.0 = rated equilibrium), the scale the
// reactivity worth is expressed in.
func xenonNormalized(x float64) float64 {
	return x * CoreXenonIodineToXenonEquilibriumRatio
}

// netTempCoef returns the moderator temperature coefficient
// (pcm/°F) at the given RCS boron concentration. The fuel-side
// Doppler base is carried separately by CoreDopplerBasePcmPerDegF.
// The interpolation is linear between
// CoreNetTempCoefAtZeroBoronPcmPerDegF (end-of-life, low
// boron: substantially more negative) and
// CoreNetTempCoefAtRefBoronPcmPerDegF (beginning-of-life,
// CVCSInitialBoron: small positive).
//
// Boron values inside (0, CVCSInitialBoron] interpolate.
// Boron < 0 is clamped to the EOL anchor. Boron == 0 returns
// the EOL anchor — a real plant with full dilution has the
// most-negative combined coefficient per the spec.
//
// Spec:
//   - "At the beginning of life, the effect on reactivity is
//     small since the moderator temperature coefficient is
//     small [positive]. At the end of life, the coefficient is
//     substantially more negative due to depletion and removal
//     of shim boron [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
//   - "+7 pcm/°F" moderator effect at 1700 ppm with burnable
//     absorbers reducing the boron need
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:51-56
func netTempCoef(boron float64) float64 {
	if boron <= 0 {
		return CoreNetTempCoefAtZeroBoronPcmPerDegF
	}
	if boron >= CVCSInitialBoron {
		return CoreNetTempCoefAtRefBoronPcmPerDegF
	}
	t := boron / CVCSInitialBoron
	return CoreNetTempCoefAtZeroBoronPcmPerDegF +
		t*(CoreNetTempCoefAtRefBoronPcmPerDegF-CoreNetTempCoefAtZeroBoronPcmPerDegF)
}


type ReactorPressureVessel struct {
	Part
	FuelRods
	ControlRods
	Reactivity float32

	// FissionPower is the heat from the fission chain reaction
	// alone. It rescales by the power-defect correction every
	// step and collapses when the core is subcritical (rods
	// inserted, boron present).
	FissionPower Watts

	// SourceRangeCounts is the source-range count-rate reading
	// (counts per second) proportional to fission power. The
	// four source-range channels use proportional counters:
	// "Neutron flux, as measured in the primary shield area,
	// produces current pulses in the detectors."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_121.txt:40-43
	SourceRangeCounts float64

	// StartupRateDPM is the filtered startup-rate indication in
	// decades per minute. "Startup rate indication is also provided
	// in the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_121.txt:19-20
	StartupRateDPM float64

	// DecayHeatFast and DecayHeatSlow are fission-product decay
	// inventories: produced in proportion to FissionPower,
	// drained at fixed radioactive decay constants that
	// reactivity cannot touch.
	//   "at the moment of reactor shutdown, decay heat from these
	//    radioactive sources is still 6.5% of the previous core
	//    power ... About 1 hour after shutdown, the decay heat
	//    will be about 1.5% ... After a day, the decay heat falls
	//    to 0.4%"
	//   specs/wiki/Decay_heat.txt
	DecayHeatFast Watts
	DecayHeatSlow Watts

	// TimeSinceTrip is 0 while running and accumulates after Trip.
	TimeSinceTrip time.Duration

	// Iodine135 and Xenon135 are fission-product reactivity
	// inventories. Iodine135 is normalized to its rated-power
	// equilibrium (1.0 = rated equilibrium, 0 = none); Xenon135 is
	// stored as the physical concentration on the iodine scale, whose
	// rated equilibrium is ~lambdaI/(lambdaX+burn) ~= 0.1 — route
	// display/worth reads through xenonNormalized. Xenon-135 is
	// the strongest known neutron absorber and its swings dominate
	// slow reactivity behavior during maneuvering and after power
	// changes.
	//   "An additional concern during power operation is the axial
	//    power distribution [...] xenon oscillations [...]"
	//    specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:73-93
	//   "Produced via iodine-135, Xenon-135 is a fission product
	//    with a 6.7 hour half-life."
	//    specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:164-165
	Iodine135 float64
	Xenon135  float64

	// Trip is the operator-controlled trip flag. Set true to drop
	// the rods into the fully inserted scram position; fission
	// power then collapses through the same reactivity equations
	// that govern operation, leaving fission-product decay heat.
	Trip bool

	// Inlet is the temperature of coolant entering the core via the
	// cold legs (after the SG). Outlet is the temperature leaving
	// the core via the hot legs (before the SG). These two
	// temperatures drive the Doppler / moderator feedback in Step
	// and are the basis for the average-coolant-temperature
	// reactivity term.
	//
	// Spec justification:
	//   - "Reactor coolant inlet temperature: 621°F (327°C);
	//     outlet: 558°F (292°C)"
	//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
	//   - "The design full load average temperature is about 571°F
	//     (300°C)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:115-116
	Inlet  Temperature
	Outlet Temperature

	InletNozzle  Nozzle
	OutletNozzle Nozzle
	Pressure

	// PumpHeatInput is the per-step shaft-loss heat dumped
	// into the coolant by the reactor coolant pump, varying
	// with Velocity². Zero when the pump is stopped; full
	// RCPRatedHeatInput at RCPRatedSpeed. TotalThermalPower adds
	// it so the SG HeatInput sees the total primary-side flux.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
	PumpHeatInput Watts

	// NaturalCircFlowFrac is the loop flow fraction carried by
	// the thermosiphon: buoyancy head from the hot-leg /
	// cold-leg delta-T balanced against loop resistance. Zero
	// when the loop is isothermal; saturates at
	// NatCircMaxFlowFrac.
	//
	// Spec justification:
	//   - "a thermal driving head will exist to produce natural
	//     circulation flow if the RCPs are not operating"
	//     specs/westinghouse_tech_manual/chapter_3/page_0328.txt:31-32
	NaturalCircFlowFrac float64

	// AxialOffset is the axial power distribution observable, percent:
	// top-half power minus bottom-half power over total core power.
	// Rod insertion drives it negative (rods enter from the top);
	// xenon skew pulls it back over hours. See core/axial_offset.go.
	AxialOffset float64

	// Iodine135AxialDeviation and Xenon135AxialDeviation are the
	// top-minus-bottom half-differences of the fission-product
	// inventories around their lumped means, on the same scales as
	// Iodine135 / Xenon135. They reproduce the poison side of the
	// axial feedback loop; see core/axial_offset.go for the ODEs
	// and citations.
	Iodine135AxialDeviation float64
	Xenon135AxialDeviation  float64

	// HotLegBreak is the malfunction path modeling a LOCA rupture
	// at the RPV outlet nozzle connecting the upper plenum to a
	// hot leg. Open is the normalized break size (0 intact, 1
	// full crack); the discharge debits primary liquid inventory
	// through BreakFlow and depressurizes the RCS toward
	// containment pressure as the primary voids.
	// "This malfunction event causes a 'crack' opening at the
	//  reactor pressure vessel (RPV) outlet nozzle that connects
	//  the RPV Upper Plenum and the Hot Leg #1 piping."
	// specs/pwr_sim/chapter_4/page_053.txt:43-45
	HotLegBreak Valve

	// BreakFlow is the per-step volume discharged through
	// HotLegBreak during the most recent Step (gallons per step,
	// same convention as SteamGenerator.BreakFlow). Plant.Step
	// converts it into containment enthalpy input; it is lost
	// from the primary.
	BreakFlow Volume

	// BreakDischarged is the cumulative liquid-equivalent volume
	// lost through HotLegBreak since time zero (gal). The blowdown
	// ends when it reaches RCSDrainableInventoryGal, so a voided
	// primary cannot keep feeding the break.
	BreakDischarged Volume
}

// RCSHotLegBreakGalPerSec is the full-open discharge rate of the
// hot-leg break (gal/s, liquid-equivalent). The spec quotes no
// break size; the magnitude is sized so blowdown depressurization
// outruns pressurizer heater authority (200 psi/s) and drives the
// primary below the accumulator precharge within seconds.
// "For large pipe ruptures, the RCS would be rapidly
// depressurized and voided of coolant."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
const RCSHotLegBreakGalPerSec = 120.0

// RCSDrainableInventoryGal is the total liquid-equivalent primary
// inventory the break can drain, derived once from the same lumped
// coolant mass the RHRS-takeover heat balance uses. The pressurizer's
// 1500 gal ledger only carries pressure state; without this bound a
// voided primary would feed the break forever.
var RCSDrainableInventoryGal = RCSMassKg / KilogramsPerGallon

// ECCSInjectionTempF is the storage temperature (°F) of borated
// ECCS water, shared by the accumulators and the RWST. Assumption;
// the source quotes no storage temperature. Sized to near-ambient
// borated-water storage, far below any post-blowdown coolant
// temperature, so the mixing direction is always cooling during a
// LOCA transient.
const ECCSInjectionTempF = 100.0

// applyHotLegBreak discharges the hot-leg break against the
// pressurizer inventory ledger: each liquid-equivalent gallon lost
// drops RCS pressure by PressurizerBubbleStiffness toward floor
// (containment pressure) as the primary voids, matching the
// relief-discharge currency convention. The per-step discharged
// volume lands in BreakFlow for the containment enthalpy routing,
// and the cumulative draw stops at the drainable primary inventory
// so an empty primary cannot keep feeding the break.
//
// Spec justification:
//   - "For large pipe ruptures, the RCS would be rapidly
//     depressurized and voided of coolant."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
//   - Break location: specs/pwr_sim/chapter_4/page_053.txt:43-45.
func (r *ReactorPressureVessel) applyHotLegBreak(pzr *Pressurizer, floor Pressure) {
	r.BreakFlow = Volume{Val: 0, Unit: u.Gallon}
	if r.HotLegBreak.Open <= 0 {
		return
	}
	remaining := RCSDrainableInventoryGal - r.BreakDischarged.Float()
	if remaining <= 0 {
		return
	}
	rate := r.HotLegBreak.Open * RCSHotLegBreakGalPerSec
	gal := min(rate*TimeStep.Seconds(), remaining)
	// The liquid-equivalent draw comes out of the loop inventory the
	// surge line exposes as pressurizer water; level falls first and
	// the free space grows behind it.
	water := max(pzr.Water.Float()-gal, 0)
	pzr.Water = vol(pzr.Water, water)
	pzr.SteamBubble = vol(pzr.SteamBubble,
		pzr.Volume.Float()-pzr.Water.Float())
	psiDrop := gal * PressurizerBubbleStiffness
	pzr.Pressure = Pressure(max(float64(pzr.Pressure)-psiDrop, float64(floor)))
	r.BreakDischarged = vol(r.BreakDischarged,
		r.BreakDischarged.Float()+gal)
	r.BreakFlow = vol(r.BreakFlow, gal)
}

// creditSafetyInjection returns this tick's ECCS delivery to the
// primary inventory ledger: HHSI and accumulator flow reported by the
// SIS step refills pressurizer water, and the same volume comes back
// off the cumulative break draw so RCSDrainableInventoryGal bounds the
// primary's NET loss rather than its gross spill. RWST suction was
// already debited in SIS.Step; sump suction draws the implicit sump.
// The delivered gallonage also joins the lumped loop inventory at
// storage temperature (plans/p36_1.txt), so continued safety-injection
// flow drives coolant temperature toward storage instead of leaving it
// parked at its blowdown value.
//
// Spec justification:
//   - "All pumps will continue to operate and will deliver water to
//     the RCS cold legs without interruption."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:175-177
//   - Long-term cooling phase: page_097.txt:122-138.
func (r *ReactorPressureVessel) creditSafetyInjection(pzr *Pressurizer, gal float64) {
	if gal <= 0 {
		return
	}
	water := min(pzr.Water.Float()+gal, pzr.Volume.Float())
	refilled := water - pzr.Water.Float()
	pzr.Water = vol(pzr.Water, water)
	pzr.SteamBubble = vol(pzr.SteamBubble,
		pzr.Volume.Float()-pzr.Water.Float())
	r.BreakDischarged = vol(r.BreakDischarged,
		max(r.BreakDischarged.Float()-refilled, 0))
	outletF := r.Outlet.Float()
	mixed := outletF + gal/RCSDrainableInventoryGal*
		(ECCSInjectionTempF-outletF)
	r.Outlet = Temperature{Val: mixed, Unit: u.Fahrenheit}
}

// applyFaultOvercool integrates a fault-regime secondary draw into the
// primary energy balance without the operational program envelope: the
// ±50 F/hr clamp is operator pacing discipline for planned evolutions,
// while a break removes heat at the m·cp rate the discharge actually
// demands. The coolant cannot fall below the steam-generator shell's
// saturation temperature through the tubes, so the header pressure sets
// the floor; the draw itself vanishes once the header is empty.
// ReactorBuilding.Step owns the regime split and reports zero here
// whenever no fault path is open.
//
// Spec justification:
//   - "The pipe break also results in increase in steam flow from the
//     steam generators, leading to increase in heat removal from the
//     reactor coolant system. Therefore, coolant temperature and
//     pressure will drop."
//     specs/pwr_sim/chapter_4/page_053.txt:9-11
func (r *ReactorPressureVessel) applyFaultOvercool(faultExcess Watts,
	sgHeader Pressure) {
	if faultExcess <= 0 {
		return
	}
	deltaTF := -float64(faultExcess) / (RCSMassKg * SpecificHeatWater) *
		TimeStep.Seconds() * 9.0 / 5.0
	floorF := PressurizerSatTemperatureF(float64(sgHeader))
	r.Outlet = Temperature{
		Val: max(r.Outlet.Float()+deltaTF, floorF), Unit: u.Fahrenheit}
}

// BlowdownFlashTau is the first-order relaxation time constant
// (seconds) for coolant temperature toward the saturation temperature
// of the falling RCS pressure while the hot-leg break discharges.
// Assumption; the sources are silent about the flashing rate. Sized so
// the bulk of the thermal drop lands inside the multi-second
// depressurization the pressure model already produces.
const BlowdownFlashTau = 10.0

// applyBlowdownCooling tracks the coolant toward the saturation
// temperature of the current RCS pressure while the hot-leg break is
// actually discharging this tick: flashing cools the residual liquid to
// the reduced pressure's saturation point, which is what makes the LOCA
// transient rapid instead of paced like a planned cooldown. A drained,
// isolated, or absent break leaves the outlet untouched, and a target
// above the outlet never heats the coolant.
//
// Spec justification:
//   - "For large pipe ruptures, the RCS would be rapidly
//     depressurized and voided of coolant."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
//   - "Both the vapor and the residual liquid are cooled to the
//     saturation temperature of the liquid at the reduced pressure."
//     specs/wiki/Flash_evaporation.txt:651
func (r *ReactorPressureVessel) applyBlowdownCooling(rcsP Pressure) {
	if r.BreakFlow.Float() <= 0 {
		return
	}
	targetF := PressurizerSatTemperatureF(float64(rcsP))
	outletF := r.Outlet.Float()
	if targetF >= outletF {
		return
	}
	r.Outlet = Temperature{Val: outletF +
		(targetF-outletF)*(TimeStep.Seconds()/BlowdownFlashTau),
		Unit: u.Fahrenheit}
}
// Reactor core sizing. The spec gives steam-flow and pressure data
// (specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56,
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:80-82)
// but does not state a plant-wide rated thermal power. The number
// below is sized to the standard 4-loop plant scale, with the per-SG
// duty SteamGeneratorFullLoadHeatInput anchored to one loop's share
// of it so the secondary ratings close against this constant.
//
// SteamGeneratorLoopCount is the number of primary loops the
// lumped single-SG model represents. SteamGeneratorFullLoadHeatInput
// is the per-SG vaporising duty (specs/the_westinghouse_pressurized_water_react//chapter_3/page_065.txt:56,
// 480 kg/s), so CoreRatedThermalPower = SteamGeneratorLoopCount ×
// SteamGeneratorFullLoadHeatInput. The ReactorBuilding wiring
// (core/plant.go) routes rpv.TotalThermalPower() through the
// SG, so the per-loop heat flux the SG actually sees is
// rpv.TotalThermalPower() / SteamGeneratorLoopCount.
//
//   "Number and type: 1 vertical, U-tube steam generator with
//    integral steam-drum per loop"
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:31-32
//   "2-Loop Plant / 3-Loop Plant / 4-Loop Plant"
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:41-43
const (
	CoreRatedThermalPower Watts = 3_800_000_000

	// SteamGeneratorLoopCount is the number of primary coolant
	// loops the plant represents. The lumped SG model uses
	// this to scale the per-loop heat input so the single SG
	// does not carry the full core thermal output (one SG is
	// rated for ~480 kg/s of steam, per
	// specs/the_westinghouse_pressurized_water_react//chapter_3/page_065.txt:56). The 4-loop
	// configuration matches CoreRatedThermalPower ≈ 4 ×
	// SteamGeneratorFullLoadHeatInput.
	SteamGeneratorLoopCount = 4
)
// Decay-heat model constants.
//
// Spec justification:
//   - Equilibrium share: "at the moment of reactor shutdown, decay
//     heat from these radioactive sources is still 6.5% of the
//     previous core power" (specs/wiki/Decay_heat.txt);
//     CoreDecayHeatShutdownFraction is the sum of the two pool
//     equilibria at steady full power.
//   - Fast pool: short-lived fission products dominate the first
//     minutes after shutdown; the 50 s CoreDecayHeatFastTimeConstant
//     tracks the delayed-neutron-dominated rundown.
//   - Slow pool: the 20 h e-folding matches the residual heat the
//     Residual Heat Removal System is designed to remove. The
//     20-hour cooldown target is cited in
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:104-105
//     ("reduce the RCS temperature to 140°F (60°C) within 20 hours
//     following reactor shutdown").
//
// Two pools approximate the continuous fission-product spectrum:
// they reproduce the 6.5% shutdown level and the ~0.4% level about
// a day after shutdown (specs/wiki/Decay_heat.txt), but collapse
// the intermediate hours and underestimate the ~1.5% the wiki
// reports at 1 hour (model ≈ 0.55%).
const (
	CoreDecayHeatShutdownFraction float64 = 0.065 // 6.5% of rated at equilibrium
	CoreDecayHeatFastFraction     float64 = 0.060
	CoreDecayHeatSlowFraction     float64 = 0.005

	// CoreNeutronSourcePower is the standing fission power the
	// startup neutron sources maintain at the bottom of the power
	// range. The spec mandates the sources but is silent on their
	// thermal magnitude; 1e-9 of rated (a few watts) keeps startup
	// from a fully collapsed chain to full power within minutes of
	// simulated time while staying thermally invisible.
	CoreNeutronSourcePower Watts = 1e-9 * CoreRatedThermalPower
	CoreDecayHeatTimeConstant             = 20 * time.Hour
	CoreDecayHeatFastTimeConstant         = 50 * time.Second

	// CoreSourceRangeCountsPerWatt converts fission power into a
	// source-range count rate. The source-range channels use
	// proportional counters whose pulse rate tracks neutron flux:
	// "Neutron flux, as measured in the primary shield area,
	// produces current pulses in the detectors."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_121.txt:40-43
	// The spec gives no counts-per-flux calibration; derived so the
	// neutron-source floor reads a nominal 1000 cps, a typical
	// shutdown source-range observation level.
	CoreSourceRangeCountsPerWatt = 1e3 / float64(CoreNeutronSourcePower)

	// CoreStartupRateFilterTimeConstant smooths the instantaneous
	// startup-rate reading into the control-room indication. Derived;
	// the spec mandates the indication but not its response time:
	// "Startup rate indication is also provided in the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_121.txt:19-20
	CoreStartupRateFilterTimeConstant = 5 * time.Second

	// CoreFissionPowerAtRated is the fission-chain output at 100%
	// rated operation under the modeled 6.5% decay-heat share; the
	// xenon inventories normalize their power fraction against it so
	// a seeded full-load plant sits at exactly 1.0.
	CoreFissionPowerAtRated Watts = Watts(float64(CoreRatedThermalPower) *
		(1 - CoreDecayHeatShutdownFraction))

	// CoreRodWorthABCD is the full-insertion ABCD rod-bank worth
	// as a fraction of Δk/k: −4.5% (−4500 pcm), applied on the
	// percent Δk/k scale in Step. The Westinghouse spec does not
	// state a numeric rod worth; the value is a model assumption
	// typical of a full rod-bank worth.
	CoreRodWorthABCD float32 = -0.045

	// Design full-load average coolant temperature (°F); the
	// reference point at which the simplified Doppler term
	// contributes zero reactivity.
	//   "The design full load average temperature is about
	//   571°F (300°C)."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:115-116
	CoreDopplerReferenceTempF float32 = 571

	// RodInsertionSpeed is the maximum fraction-of-withdrawal
	// change per second the control-rod drive mechanism can
	// achieve. Derived from the spec's description of RCC
	// assemblies as the mechanism for "startup or shutdown, to
	// follow load changes" — implying a finite mechanical speed
	// rather than instantaneous insertion. At TimeStep=1 ms the
	// per-tick change is 5e-5, effectively continuous at
	// control-room time scale but preventing instantaneous
	// scram.
	//   "Rod cluster control (RCC) assemblies are used for
	//   reactor startup or shutdown, to follow load changes,
	//   and to control small transient changes in reactivity."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:88-93
	RodInsertionSpeed float64 = 0.05 // 5% of full travel per second
)
// Primary-side thermal model constants (plans/p8_1.txt).
//
// Spec justification:
//   - CoreReferenceInletF / CoreReferenceOutletF — Table 3.4-1 lists
//     "Reactor coolant inlet temperature: 621°F; outlet: 558°F" from
//     the steam-generator tube-side perspective. Walking the loop back
//     to the RPV (coolant enters the core cold via the SG cold leg and
//     leaves hot via the hot leg) means RPV-side Inlet=558°F and
//     Outlet=621°F.
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
//   - RPVDesignPressure — "designed for full system pressure of 2500
//     psia (172.3 bar a)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_054.txt:20-21
//     The reactor coolant pump design data table repeats "Design
//     pressure 2500 psia (172 bar)".
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_069.txt:84-91
//   - RCP design parameters: "Design capacity 97,600 gpm (6.15
//     m3/sec)" / "Design head 280 ft (85.3 m)" — cited for context
//     but not directly used in the model.
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:124-130
//   - CoreDopplerPcmPerDegF — combined Doppler + moderator
//     temperature coefficient of reactivity for this model. The spec
//     describes the constituent effects separately:
//     Doppler — "This defect is due to an increase in width of
//     uranium-238 resonance peaks in the epithermal energy range."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:89-96
//     Power defect — "The negative reactivity associated with an
//     increase in reactor core power due to temperature feedback
//     effects from an increase in fuel and moderator temperature."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
//     Temperature coefficient of reactivity — "The ratio of the
//     change in reactivity to the change in temperature when the
//     reactivity change is only due to a temperature change."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_245.txt:18-20
//     Moderator temperature coefficient — varies from positive at
//     beginning-of-life ("on the order of +7 pcm/°F",
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:54-55)
//     to negative at end-of-life ("the coefficient is substantially
//     more negative due to depletion and removal of shim boron",
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:119-121);
//     the simplified model uses a representative negative value.
const (
	// Reference full-load RPV-side temperatures (°F).
	CoreReferenceInletF  float64 = 558
	CoreReferenceOutletF float64 = 621

	// Primary loop design pressure (psia).
	RPVDesignPressure Pressure = 2500

	// PrimaryThermalTimeConstant is the thermal time constant of
	// the core coolant + fuel + internals in seconds. Sized to give
	// plausible ~1 s outlet response to a step in ThermalPower at
	// the 1 ms plant step. Not derived from a spec number; the spec
	// describes the steady-state temperatures only.
	PrimaryThermalTimeConstant float64 = 90

	// PrimaryInletTimeConstant is slower than the outlet because
	// the cold leg traverses the entire primary loop before
	// returning to the RPV. Roughly 3× the RPV-side thermal time
	// constant.
	PrimaryInletTimeConstant float64 = 270

	// PrimaryPressureTimeConstant is the rate at which the RPV
	// pressure tracks its design pressure in this simplified model.
	PrimaryPressureTimeConstant float64 = 300

	// CoreFlowDegradedFloorFrac bounds the flow-degradation stretch
	// of the outlet target: below this forced-flow fraction the
	// per-pass delta-T stops growing, keeping the worst-case target
	// at four times the full-flow rise. Assumption; the lumped band
	// stops stretching past a four-fold rise.
	CoreFlowDegradedFloorFrac float64 = 0.25

	// NatCircMaxFlowFrac caps the thermosiphon at its decay-heat-
	// sized capability: "Natural circulation flow is sufficient
	// only for decay heat removal of a shutdown reactor, not for
	// power operation."
	// specs/westinghouse_tech_manual/chapter_3/page_0325.txt:14-15
	// Derived: at the cap the flowing loop carries roughly the
	// post-shutdown decay-heat fraction (~6 percent of rated)
	// across a loop rise comparable to the design span
	// CoreReferenceOutletF-CoreReferenceInletF, so flow_frac ~ Q_frac.
	NatCircMaxFlowFrac = 0.06

	// NatCircHeadGainPerDegF converts loop delta-T into flow
	// fraction through the sqrt head-vs-resistance law
	// nc = gain*sqrt(dT). Derived so a typical decay-heat loop
	// rise (~56 degF at the cap below) lands just under
	// NatCircMaxFlowFrac: gain ~= 0.055/sqrt(56) ~= 0.0073.
	NatCircHeadGainPerDegF = 0.0073

	// NatCircTimeConstant is the loop transport lag of the
	// thermosiphon response (seconds). Derived: tens of seconds —
	// slower than the pump-ramp electrical transient, faster than
	// the minutes-scale thermal anchors above.
	NatCircTimeConstant = 30.0

	// CoreDopplerBasePcmPerDegF is the fuel-side Doppler /
	// power-defect base of the temperature feedback, a constant
	// negative slope applied to the outlet-temperature deviation.
	// Splitting it out of the boron interpolation keeps the combined
	// feedback negative across the whole boron range so load-follow
	// self-regulation has a stable fixed point:
	//   "reactor power is maintained equal to load by virtue of
	//    feedback effects inherent in the physics of a PWR"
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:69-71
	// The magnitude is a sizing convention: the source defines the
	// power defect qualitatively, not numerically -
	//   "power defect - The negative reactivity associated with an
	//    increase in reactor core power due to temperature feedback
	//    effects from an increase in fuel and moderator temperature."
	//   specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
	CoreDopplerBasePcmPerDegF float64 = -4.0

	// CoreRatedCriticalBoronPpm is the RCS boron concentration at
	// which the fresh core sits exactly critical at rated power with
	// the rod bank fully withdrawn, derived by solving
	//   0 = deltaF*(CoreDopplerBasePcmPerDegF + netTempCoef(b))/1000
	//       + (b - CVCSInitialBoron)/100 * CoreBoronWorthPer100ppm
	// for b at the rated outlet temperature. It is the fresh-core
	// seed MakePlant dilutes to: with a realistic negative power
	// defect, full-power criticality at the reference shim would sit
	// far subcritical.
	CoreRatedCriticalBoronPpm float64 = 571.4

	// CoreNetTempCoefAtRefBoronPcmPerDegF is the moderator
	// temperature coefficient at the beginning-of-life RCS boron
	// concentration (CVCSInitialBoron = 1000 ppm), the fuel-side
	// Doppler base being carried separately by
	// CoreDopplerBasePcmPerDegF. Sized slightly positive so
	// the model reproduces the "moderator temperature coefficient
	// is small [positive]" BOL behaviour described in
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122 and the BOL
	// "+7 pcm/°F" moderator effect at 1700 ppm with burnable
	// absorbers reducing it in
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:51-56.
	CoreNetTempCoefAtRefBoronPcmPerDegF float64 = 1.0

	// CoreNetTempCoefAtZeroBoronPcmPerDegF is the moderator
	// temperature coefficient at zero RCS boron, representing
	// end-of-life where the soluble boron
	// shim has been depleted and the moderator temperature
	// coefficient is substantially more negative per the spec.
	// Spec: "the coefficient is substantially more negative
	// due to depletion and removal of shim boron"
	// specs/the_westinghouse_pressurized_water_react//chapter_2/page_037.txt:119-121
	CoreNetTempCoefAtZeroBoronPcmPerDegF float64 = -2.0

	// DopplerClampLowF is the lower bound on the moderator
	// temperature the linear temperature-coefficient model is
	// anchored on. Below this the moderator would freeze (water
	// freezes at 32°F; the linear dρ/dT description no longer
	// applies). The cooldown target in
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:8
	// is 140°F, well above the clamp.
	DopplerClampLowF float64 = 32.0

	// DopplerClampHighF is the upper bound on the moderator
	// temperature the linear temperature-coefficient model is
	// anchored on. At the RCS design pressure of 2500 psia
	// (specs/the_westinghouse_pressurized_water_react//chapter_3/page_054.txt:20-21) water saturates
	// at ~652°F; the simulator caps at 700°F to leave margin
	// for transient overshoot before void formation takes over
	// as the reactivity mechanism
	// (specs/the_westinghouse_pressurized_water_react//chapter_2/page_037.txt:58).
	DopplerClampHighF float64 = 700.0
)
// Reactor Coolant Pump dynamics constants (plans/p33_1.txt).
//
// Spec justification:
//   - Operating speed: "Operating speed 1189 rpm"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:175-177
//   - Flywheel coastdown: "A flywheel in the motor increases the
//     total rotating inertia, allowing the pump to coastdown slowly
//     after deenergizing. This provision, together with the Reactor
//     Control and Protection System, maintains adequate flow through
//     the reactor core in the event of a loss of power to the
//     pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_061.txt:152-158
//   - Long coastdown during offsite-power loss: "If offsite power is
//     not available, the long coast down times of the reactor coolant
//     pumps assures adequate flow during the power decay transient.
//     The coast down time is prolonged by the heavy flywheel mounted
//     above the motor."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:80-94
//   - Heat balance: "Primary temperature is maintained through the
//     balanced use of reactor coolant pump heat input and a steam
//     dump system..."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
const (
	// RCPRatedSpeed is the design operating speed of the
	// reactor coolant pump from Table 3.3-1.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:175-177
	RCPRatedSpeed Velocity = 1189

	// RCPStartupTimeConstant is the exponential rise time for
	// the RCP motor to reach rated speed from zero when running
	// and powered. The spec does not state a numeric start
	// time; 2 s is a reasonable large-induction-motor value and
	// gives a visible but controlled acceleration at the
	// 1 ms plant step.
	RCPStartupTimeConstant = 2 * time.Second

	// RCPCoastdownTimeConstant is the exponential decay time
	// after the pump loses power or is stopped. The spec
	// characterises the flywheel coastdown as "long"
	// (chapter 1, chapter 3, chapter 16); 10 s honours that
	// description while still reaching near-zero speed within
	// the time window of a typical transient test.
	RCPCoastdownTimeConstant = 10 * time.Second

	// RCPRatedHeatInput is the motor + bearing loss power at
	// full speed, added to the primary coolant as a heat
	// source. The spec does not give a numeric per-pump loss
	// figure; this constant is derived from typical large-motor
	// losses (~ 200 kW per pump at rated speed). It is sized
	// so that four running pumps contribute ~ 800 kW total,
	// consistent with the spec's framing of pump heat as a
	// small fraction of the 3.8 GW thermal balance.
	RCPRatedHeatInput Watts = 200_000.0
)
