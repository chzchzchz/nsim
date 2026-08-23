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
func (r *ReactorPressureVessel) Step(rcpSpeed Velocity) ReactorPressureVessel {	ret := *r
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

	// 2) Primary-side thermal model.
	powerFrac := float64(ret.ThermalPower) / float64(CoreRatedThermalPower)
	if powerFrac < 0 {
		powerFrac = 0
	}
	refDeltaF := CoreReferenceOutletF - CoreReferenceInletF
	targetOutlet := CoreReferenceInletF + powerFrac*refDeltaF

	outletF := ret.Outlet.Float()
	outletF += (targetOutlet - outletF) * (dt.Seconds() / PrimaryThermalTimeConstant)
	if outletF < 0 {
		outletF = 0
	}
	ret.Outlet = u.NewValue(outletF, u.Fahrenheit)

	// The cold-leg / downcomer inlet temperature only blends
	// with the outlet when the RCP is actually spinning. The
	// gate is the actual rotor speed, not the operator latch:
	// if the ESF breaker opens the pump coasts down to zero
	// speed even though Running=true is latched, and there is
	// no flow to drive the inlet-outlet blend.
	rcpFlowFrac := float64(rcpSpeed) / float64(RCPRatedSpeed)
	if rcpFlowFrac < 0 {
		rcpFlowFrac = 0
	}
	if rcpFlowFrac > 0 {
		blend := CoreInletBlendFraction * powerFrac * rcpFlowFrac
		targetInlet := CoreReferenceInletF*(1-blend) + outletF*blend
		inletF := ret.Inlet.Float()
		inletF += (targetInlet - inletF) * (dt.Seconds() / PrimaryInletTimeConstant)
		if inletF < 0 {
			inletF = 0
		}
		ret.Inlet = u.NewValue(inletF, u.Fahrenheit)
	}

	// 3) Pressure relaxes toward the RPV design pressure. The
	// pressurizer (p5) sets the actual RCS pressure; this is the
	// design baseline the RPV itself sits at in steady state.
	pressure := float64(ret.Pressure)
	pressure += (float64(RPVDesignPressure) - pressure) *
		(dt.Seconds() / PrimaryPressureTimeConstant)
	if pressure < 0 {
		pressure = 0
	}
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
	rodReactivity := float32(ret.ControlRods.Insertion) * CoreRodWorthABCD
	dopplerDeltaF := outletF - float64(CoreDopplerReferenceTempF)
	// Net temperature coefficient (Doppler + moderator) varies
	// with RCS boron concentration: small positive at beginning
	// of life with full soluble-boron shim, substantially
	// negative at end of life with boron depleted. The
	// interpolation crosses zero at CoreNetTempCoefZeroCrossBoron
	// so the simplified model reproduces the BOL/EOL spec
	// behaviour:
	//   "At the beginning of life, the effect on reactivity is
	//    small since the moderator temperature coefficient is
	//    small. At the end of life, the coefficient is
	//    substantially more negative due to depletion and
	//    removal of shim boron [...]"
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
	tempCoef := netTempCoef(ret.ControlRods.BoronConcentration)
	dopplerPcm := float32(dopplerDeltaF * tempCoef)
	boronDelta := (ret.ControlRods.BoronConcentration - CVCSInitialBoron) / 100.0
	boronPcm := float32(boronDelta) * CoreBoronWorthPer100ppm
	ret.Reactivity = rodReactivity + dopplerPcm + boronPcm

	// 5) Thermal power output. At-power: rated scaled by the
	// reactivity power-defect correction. After trip: decay-heat
	// curve from p7 (unchanged).
	powerFrac32 := float32(powerFrac)
	if ret.Trip {
		t := ret.TimeSinceTrip.Seconds()
		if t < CoreDecayHeatFastTimeConstant.Seconds() {
			fast := float32(CoreDecayHeatShutdownFraction +
				(1.0-CoreDecayHeatShutdownFraction)*
					math.Exp(-float64(t)/CoreDecayHeatFastTimeConstant.Seconds()))
			powerFrac32 = fast
		} else {
			tSlow := t - CoreDecayHeatFastTimeConstant.Seconds()
			slow := float32(CoreDecayHeatMinFraction +
				(CoreDecayHeatShutdownFraction-CoreDecayHeatMinFraction)*
					math.Exp(-tSlow/CoreDecayHeatTimeConstant.Seconds()))
			powerFrac32 = slow
		}
	}
	powerCorrection := 1.0 + float64(ret.Reactivity)/100.0
	if powerCorrection < 0 {
		powerCorrection = 0
	}
	ret.ThermalPower = Watts(float64(CoreRatedThermalPower) * float64(powerFrac32) * powerCorrection)
	if ret.ThermalPower < 0 {
		ret.ThermalPower = 0
	}
	// Rated cap. At BOL the net temperature coefficient can be
	// small positive (specs/.../chapter_2/page_037.txt:117-122);
	// without rod-by-rod control, even a +1 pcm/°F coefficient
	// diverges to infinity in this simplified one-state-point
	// model. Real plants stay subcritical via control rods; the
	// simulation uses rated as the cap and treats any further
	// power demand as a rod-withdrawal request.
	if ret.ThermalPower > CoreRatedThermalPower {
		ret.ThermalPower = CoreRatedThermalPower
	}
	// RCP shaft-loss heat enters the primary coolant (chapter
	// 16 temperature balance). Added after the reactivity-based
	// power so the SG HeatInput sees the total primary-side
	// flux when the ReactorBuilding wiring copies
	// ret.ThermalPower into SteamGenerator.HeatInput.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:108-115
	ret.ThermalPower += ret.PumpHeatInput

	return ret
}

// netTempCoef returns the combined Doppler + moderator
// temperature coefficient (pcm/°F) at the given RCS boron
// concentration. The interpolation is linear between
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
	DecayHeat  float32

	// ThermalPower is the per-step heat flux produced by the core
	// and consumed downstream by the SG's HeatInput. Rated when
	// the operator has the reactor running at power; decays via
	// the standard shutdown curve when Trip is set.
	ThermalPower Watts

	// TimeSinceTrip is 0 while running and accumulates after Trip.
	TimeSinceTrip time.Duration

	// Trip is the operator-controlled trip flag. Set true to drop
	// the rods; the per-step output then follows the decay-heat
	// curve.
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
	// with Velocity² (plans/p33_1.txt). Zero when the pump is
	// stopped; full RCPRatedHeatInput at RCPRatedSpeed. The
	// RPV Step adds this into ThermalPower so the SG HeatInput
	// sees the total primary-side flux.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:108-115
	PumpHeatInput Watts
}
// Reactor core sizing. The spec gives steam-flow and pressure data
// (specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56,
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:80-82)
// but does not state a plant-wide rated thermal power. The number
// below is sized so that four SGs at SteamGeneratorFullLoadHeatInput
// match the standard 4-loop plant scale (~3.4 GWt).
//
// SteamGeneratorLoopCount is the number of primary loops the
// lumped single-SG model represents. SteamGeneratorFullLoadHeatInput
// is the per-SG vaporising duty (specs/.../chapter_3/page_065.txt:56,
// 480 kg/s), so CoreRatedThermalPower = SteamGeneratorLoopCount ×
// SteamGeneratorFullLoadHeatInput. The ReactorBuilding wiring
// (core/plant.go) routes rpv.ThermalPower through the SG, so the
// per-loop heat flux the SG actually sees is
// rpv.ThermalPower / SteamGeneratorLoopCount.
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
	// specs/.../chapter_3/page_065.txt:56). The 4-loop
	// configuration matches CoreRatedThermalPower ≈ 4 ×
	// SteamGeneratorFullLoadHeatInput.
	SteamGeneratorLoopCount = 4
)
// Decay-heat model constants.
//
// Spec justification:
//   - The spec does not state a decay-heat curve; the model follows
//     the textbook two-branch approximation of ANSI/ANS-5.1 (1971).
//     The fast branch (half-life ~ 40 s) drops the reactor from
//     rated to the ~6.5% shutdown level; the slow branch (e-folding
//     ~ 20 h) tracks the residual heat the Residual Heat Removal
//     System is designed to remove. The 20-hour cooldown target is
//     cited in
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:104-105
//     ("reduce the RCS temperature to 140°F (60°C) within 20 hours
//     following reactor shutdown").
const (
	CoreDecayHeatShutdownFraction float64 = 0.065 // 6.5% of rated
	CoreDecayHeatMinFraction      float64 = 0.005 // long tail ~0.5%
	CoreDecayHeatTimeConstant             = 20 * time.Hour
	CoreDecayHeatFastTimeConstant         = 50 * time.Second

	// Reactivity contributions. ABCD is the absolute delta-k at
	// full rod insertion, scaled so withdrawing the rods from 0 to
	// 1 shifts reactivity by 45 pcm in the positive direction.
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

	// CoreInletBlendFraction is the fraction of the power level
	// that contributes to the inlet mixing model; at zero power the
	// inlet is effectively decoupled from the outlet.
	CoreInletBlendFraction float64 = 0.8

	// CoreDopplerPcmPerDegF is the PWR net temperature coefficient
	// of reactivity (combined Doppler + moderator temperature
	// effect) expressed in pcm per degree Fahrenheit, applied to
	// the outlet temperature. The simplified linear model
	// interpolates between end-of-life (low boron: substantially
	// more negative per the spec) and beginning-of-life (high
	// boron: small positive per the spec).
	//
	// Spec justification:
	//   - "At the beginning of life, the effect on reactivity is
	//     small since the moderator temperature coefficient is
	//     small [positive]. At the end of life, the coefficient is
	//     substantially more negative due to depletion and removal
	//     of shim boron, and the effect on reactivity addition is
	//     greater."
	//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
	//   - "If soluble boron were the sole means of control, the
	//     concentration would be on the order of 1700 ppm and the
	//     moderator temperature coefficient would be on the order
	//     of +7 pcm/°F (a 15°F [...] increase in moderator
	//     temperature creates 0.001 reactivity increase)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:51-56
	//
	// The first-core case at 1000 ppm boron (with burnable
	// absorbers reducing the soluble-boron need from the 1700 ppm
	// bare-core value, per page_039.txt:51-56) yields a small
	// positive coefficient via the linear interpolation. End of
	// life (boron approaching 0 ppm) yields a substantially more
	// negative coefficient. The interpolation is anchored on
	// CoreNetTempCoefAtZeroBoronPcmPerDegF and
	// CoreNetTempCoefAtRefBoronPcmPerDegF.
	CoreDopplerPcmPerDegF float64 = -1.0

	// CoreNetTempCoefAtRefBoronPcmPerDegF is the combined
	// Doppler+moderator temperature coefficient at the
	// beginning-of-life RCS boron concentration
	// (CVCSInitialBoron = 1000 ppm). Sized slightly positive so
	// the model reproduces the "moderator temperature coefficient
	// is small [positive]" BOL behaviour described in
	// specs/.../chapter_2/page_037.txt:117-122 and the BOL
	// "+7 pcm/°F" moderator effect at 1700 ppm with burnable
	// absorbers reducing it in
	// specs/.../chapter_2/page_039.txt:51-56.
	CoreNetTempCoefAtRefBoronPcmPerDegF float64 = 1.0

	// CoreNetTempCoefAtZeroBoronPcmPerDegF is the combined
	// Doppler+moderator temperature coefficient at zero RCS
	// boron, representing end-of-life where the soluble boron
	// shim has been depleted and the moderator temperature
	// coefficient is substantially more negative per the spec.
	// Spec: "the coefficient is substantially more negative
	// due to depletion and removal of shim boron"
	// specs/.../chapter_2/page_037.txt:119-121
	CoreNetTempCoefAtZeroBoronPcmPerDegF float64 = -2.0
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
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:108-115
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
