package core

import (
	u "github.com/bcicen/go-units"
)


// Step advances the pressurizer by TimeStep. Pressure follows a
// heater/spray/idle energy balance while liquid inventory moves through
// boiloff, spray condensation, and the CVCS, PORV, and relief-discharge
// paths, so the pressure loop (heaters and spray) stays independent of
// the level loop (charging versus letdown). Relief-valve actuation and
// discharge return the steam-equivalent volume relieved this step for
// the caller to route into the pressurizer relief tank.
//
// Spec justification for each block is inline below.
func (p *Pressurizer) Step(porv, safety, block *Valve) (Pressurizer, Volume) {
	ret := *p
	dt := TimeStep

	// 1) Heater effect. Energized heaters boil liquid into the steam
	// space, raising pressure toward PressurizerHeaterTarget at
	// PressurizerHeaterRate and converting the corresponding liquid
	// volume to steam.
	//
	// "All pressurizer heaters are energized to start the heatup of
	// the pressurizer necessary to form the steam bubble."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-17
	if ret.HeatersOn && ret.Water.Float() > 0 {
		psiRise := min(float64(PressurizerHeaterRate)*dt.Seconds(),
			float64(PressurizerHeaterTarget)-float64(ret.Pressure))
		if psiRise > 0 {
			ret.Pressure = Pressure(float64(ret.Pressure) + psiRise)
			bubbleRise := psiRise / PressurizerBubbleStiffness
			newWater := max(ret.Water.Float()-bubbleRise, 0)
			ret.Water = vol(ret.Water, newWater)
		}
	}

	// 2) Spray effect. The spray valve draws cold RCS water through
	// the spray line, condensing steam into the liquid inventory and
	// pulling pressure toward PressurizerSprayMin until the steam
	// space is exhausted.
	//
	// "RCS pressure is maintained by adjustment of the pressurizer
	// heaters and/or pressurizer spray."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:28-29
	if ret.SprayOpen > 0 {
		bubble := ret.Volume.Float() - ret.Water.Float()
		psiDrop := ret.SprayOpen * float64(PressurizerSprayRate) * dt.Seconds()
		psiDrop = min(psiDrop, float64(ret.Pressure)-float64(PressurizerSprayMin))
		psiDrop = min(psiDrop, bubble*PressurizerBubbleStiffness)
		if psiDrop > 0 {
			ret.Pressure = Pressure(float64(ret.Pressure) - psiDrop)
			bubbleDrop := psiDrop / PressurizerBubbleStiffness
			newWater := min(ret.Water.Float()+bubbleDrop, ret.Volume.Float())
			ret.Water = vol(ret.Water, newWater)
		}
	}

	// 3) The steam bubble is the vapor space above the liquid
	// inventory. Pressure is an independent saturation state driven
	// by the energy balance above rather than a function of bubble
	// volume, so draining liquid lowers the level while the steam
	// space grows at the maintained pressure.
	//
	// "When the pressurizer fluid reaches saturation temperature for
	// the pressure being maintained, steam formation in the
	// pressurizer begins."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:17-20
	totalVol := ret.Volume.Float()
	waterVol := ret.Water.Float()
	bubble := totalVol - waterVol
	bubble = min(max(bubble, 0), totalVol)
	ret.SteamBubble = Volume{Val: bubble, Unit: u.Gallon}

	// 4) Idle cooldown path. With no active control the steam space
	// slowly condenses at PressurizerAmbientDecay, returning its
	// volume to the liquid inventory as pressure settles toward
	// PressurizerBasePressure. (The source page's cooldown narrative
	// is OCR-truncated mid-sentence; see
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-82)
	if !ret.HeatersOn && ret.SprayOpen == 0 {
		base := float64(PressurizerBasePressure)
		psiDrop := float64(PressurizerAmbientDecay) * dt.Seconds()
		psiDrop = min(psiDrop, float64(ret.Pressure)-base)
		psiDrop = min(psiDrop, bubble*PressurizerBubbleStiffness)
		if psiDrop > 0 {
			ret.Pressure = Pressure(float64(ret.Pressure) - psiDrop)
			bubbleDrop := psiDrop / PressurizerBubbleStiffness
			newWater := min(waterVol+bubbleDrop, totalVol)
			ret.Water = vol(ret.Water, newWater)
			bubble = totalVol - newWater
			ret.SteamBubble = Volume{Val: bubble, Unit: u.Gallon}
		}
	}

	// 5) Relief valve actuation. The PORV opens automatically at a
	// setpoint and the code safety valves self-actuate above that;
	// below their setpoints they re-close. "Reductions in plant
	// electrical load [...] cause the air-operated relief valves to
	// open. These valves are automatically opened at a pressure
	// below system design pressure. [...] If system pressure
	// continues to rise, self-actuating ASME-code safety valves
	// will open."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:23-32
	if porv != nil {
		v := *porv
		if ret.Pressure > PressurizerPORVTrip {
			v.Open = max(v.Open, 1)
		} else if v.Open > 0 {
			v.Open = 0
		}
		*porv = v
	}
	if safety != nil {
		v := *safety
		if ret.Pressure > PressurizerSafetySetpoint {
			v.Open = max(v.Open, 1)
		} else if v.Open > 0 {
			v.Open = 0
		}
		*safety = v
	}

	// 6) Relief discharge. Steam leaving through an open relief
	// path shrinks the bubble the same way the spray does,
	// depressurizing toward base; the returned volume is what the
	// caller routes into the pressurizer relief tank. The model's
	// currency is liquid-equivalent gallons on both sides (see
	// PressurizerBubbleStiffness), so the transfer is 1:1.
	relief := Volume{}
	openPaths := 0
	blockOpen := block != nil && block.Open > 0
	if blockOpen {
		if porv != nil && porv.Open > 0 {
			openPaths++
		}
		if safety != nil && safety.Open > 0 {
			openPaths++
		}
	}
	if openPaths > 0 {
		totalVol := ret.Volume.Float()
		gal := min(PressurizerReliefDischargeGalPerSec*
			float64(openPaths)*dt.Seconds(), ret.SteamBubble.Float())
		if gal > 0 {
			ret.Water = vol(ret.Water, min(ret.Water.Float()+gal, totalVol))
			bubble := totalVol - ret.Water.Float()
			ret.SteamBubble = Volume{Val: bubble, Unit: u.Gallon}
			// Relief depressurizes the saturation state by the
			// discharged steam-equivalent rather than re-deriving
			// pressure from bubble volume, keeping the decoupling of
			// block 3 intact during transients.
			psiDrop := gal * PressurizerBubbleStiffness
			ret.Pressure = Pressure(max(float64(ret.Pressure)-psiDrop,
				float64(PressurizerBasePressure)))
			relief = Volume{Val: gal, Unit: u.Gallon}
		}
	}

	return ret, relief
}
// Pressurizer models the surge-line volume at the top of the primary
// loop that holds the steam bubble used for pressure control.
//
// Spec justification for the field set:
//   - Pressure, Water, Volume — "The pressurizer is a partial tank that
//     contains saturated water and steam in equilibrium at saturation
//     temperature corresponding to the pressure." (glossary.md
//     pressurizer, p. 169)
//   - HeatersOn — "All pressurizer heaters are energized to start the
//     bubble formation during plant startup."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
//   - SprayOpen — "Pressurizer heater and spray valve control is [...]
//     used to maintain the pressure in accordance with the applicable
//     temperature-pressure curves."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
//   - SteamBubble — "When the pressurizer fluid reaches saturation
//     temperature for the pressure being maintained, steam formation
//     in the pressurizer begins."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
type Pressurizer struct {
	Part
	Pressure
	Water       Volume
	Volume      Volume
	HeatersOn   bool
	SprayOpen   float64
	SteamBubble Volume
}

// Pressurizer control tunables. The numbers below are sized to give
// spec-anchored behaviour on the 1 ms plant step:
//
//   - PressurizerBubbleStiffness converts heater and spray pressure
//     changes into displaced liquid volume during boiloff and
//     condensation.
//   - PressurizerHeaterTarget is the no-load operating pressure the
//     heaters hold (2250 psia).
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:66-67
//   - PressurizerPORVTrip is the safety-valve setpoint the step uses to
//     crack the PORV.
//
// Spec justification:
//   - "operation of pressurizer heaters and spray maintains the system
//     pressure in accordance with the applicable temperature-pressure
//     curves." specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
const (
	// PressurizerBasePressure is the water-solid standstill pressure.
	// The spec's cold condition holds "<400 psig" and RHRS entry needs
	// "less than approximately 400 psig", so the solid-plant floor
	// must sit inside that regime for cooldown chains to reach RHR
	// entry; 375 psia (~360 psig) places it just under the ceiling.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-58
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	PressurizerBasePressure Pressure = 375
	PressurizerHeaterTarget    Pressure = 2250
	PressurizerHeaterRate      Pressure = 200 // psi/s while HeatersOn
	PressurizerSprayMin        Pressure = 1400
	PressurizerSprayRate       Pressure = 300 // psi/s at SprayOpen == 1
	PressurizerAmbientDecay    Pressure = 5   // psi/s cooldown when idle
	// PressurizerBubbleStiffness converts heater and spray pressure
	// changes into displaced liquid volume during boiloff and
	// condensation. Rescaled against PressurizerBasePressure so the
	// no-load 300 gal bubble still holds 2250 psia.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:66-67
	PressurizerBubbleStiffness float64 = 6.25 // psi per gallon of bubble
	PressurizerPORVTrip        Pressure = 2500

	// PressurizerSafetySetpoint is where the self-actuating ASME
	// code safety valves open, above the design pressure and the
	// PORV setpoint so the escalation order is PORV first.
	// "If system pressure continues to rise, self-actuating
	// ASME-code safety valves will open."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-32
	// Derived magnitude: design pressure + ~4% margin.
	PressurizerSafetySetpoint Pressure = 2600
)

const (
	// PressurizerReliefDischargeGalPerSec is the lumped steam-
	// equivalent relief capacity per open relief path (gal/s).
	// Derived: relieves the steady-state bubble in a few minutes,
	// matching the spec narrative that relief limits pressure
	// during transients.
	PressurizerReliefDischargeGalPerSec = 2.0

	// PressurizedReliefTankCapacityGal sizes the PRT. Derived
	// from a typical Westinghouse PRT of ≈1700 ft³; the primary
	// spec states no capacity.
	PressurizedReliefTankCapacityGal = 12700.0

	// PressurizedReliefTankInitialFraction is the initial water
	// fraction — the tank "contains sufficient water to condense
	// the steam" with headspace left for discharge
	// (chapter_3/page_068.txt:34-35).
	PressurizedReliefTankInitialFraction = 0.5

	// PressurizedReliefTankSprayGPS is the cold-water spray rate
	// into the PRT at full valve opening. "Cold water can be
	// sprayed into the pressurizer relief tank to increase the
	// heat sink capacity." Derived magnitude.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:35-37
	PressurizedReliefTankSprayGPS = 1.0

	// ContainmentVentRatePSIGPerS is the post-burst containment
	// rise while relief discharge continues through the burst
	// disc. Derived: fast enough to hold the CSS auto-start
	// signal against spray condensation, slow enough to observe
	// the transient.
	ContainmentVentRatePSIGPerS = 10.0
)
