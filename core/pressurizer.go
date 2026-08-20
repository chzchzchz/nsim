package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the pressurizer by TimeStep. It implements the
// heater/spray energy balance and the bubble-driven pressure model
// described in plans/p5_1.txt:120-181.
//
// Spec justification for each block is inline below.
func (p *Pressurizer) Step(porv *Valve) Pressurizer {
	ret := *p
	dt := TimeStep

	// 1) Heater effect — boil water into the steam bubble. The
	// heaters vaporise liquid inventory, growing the bubble and
	// (via the bubble-stiffness model) raising pressure toward
	// PressurizerHeaterTarget. Sized so 200 psi/s × 1 s raises
	// pressure by ~200 psi (matching the spec's heater-rate
	// narrative).
	//
	// "All pressurizer heaters are energized to start the bubble
	// formation during plant startup."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
	// "operation of pressurizer heaters and spray maintains the
	// system pressure in accordance with the applicable temperature-
	// pressure curves."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
	if ret.HeatersOn {
		bubble := ret.Volume.Float() - ret.Water.Float()
		target := float64(PressurizerHeaterTarget)
		base := float64(PressurizerBasePressure)
		current := base + bubble*PressurizerBubbleStiffness
		if current < target {
			// Convert the desired psi rise into additional bubble
			// volume via the bubble stiffness, then drain that
			// volume from the liquid inventory.
			psiRise := float64(PressurizerHeaterRate) * dt.Seconds()
			if current+psiRise > target {
				psiRise = target - current
			}
			bubbleRise := psiRise / PressurizerBubbleStiffness
			newWater := ret.Water.Float() - bubbleRise
			if newWater < 0 {
				newWater = 0
			}
			ret.Water = vol(ret.Water, newWater)
		}
	}

	// 2) Spray effect — condense steam back into the pressurizer.
	// The spray valve draws cold RCS water through the spray line;
	// in this model it condenses bubble volume back into the liquid
	// inventory, pulling pressure toward PressurizerSprayMin.
	if ret.SprayOpen > 0 {
		bubble := ret.Volume.Float() - ret.Water.Float()
		current := float64(PressurizerBasePressure) + bubble*PressurizerBubbleStiffness
		if current > float64(PressurizerSprayMin) {
			psiDrop := ret.SprayOpen * float64(PressurizerSprayRate) * dt.Seconds()
			if current-psiDrop < float64(PressurizerSprayMin) {
				psiDrop = current - float64(PressurizerSprayMin)
			}
			bubbleDrop := psiDrop / PressurizerBubbleStiffness
			newWater := ret.Water.Float() + bubbleDrop
			if newWater > ret.Volume.Float() {
				newWater = ret.Volume.Float()
			}
			ret.Water = vol(ret.Water, newWater)
		}
	}

	// 3) Bubble and pressure derived from the (possibly modified)
	// water inventory. The fluid graph (see simulateFluidFlow) also
	// moves water in and out of ret.Water via the surge-line edges.
	// Pressure is clamped at the heater target so the heater
	// controls act as a hard ceiling: the bubble cannot grow past
	// the point that would push pressure above PressurizerHeaterTarget
	// while HeatersOn.
	//
	// "When the pressurizer fluid reaches saturation temperature for
	// the pressure being maintained, steam formation in the
	// pressurizer begins."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
	totalVol := ret.Volume.Float()
	waterVol := ret.Water.Float()
	bubble := totalVol - waterVol
	if bubble < 0 {
		bubble = 0
	}
	if bubble > totalVol {
		bubble = totalVol
	}
	pressure := float64(PressurizerBasePressure) + bubble*PressurizerBubbleStiffness
	if ret.HeatersOn && pressure > float64(PressurizerHeaterTarget) {
		maxBubble := (float64(PressurizerHeaterTarget) - float64(PressurizerBasePressure)) / PressurizerBubbleStiffness
		newWater := totalVol - maxBubble
		if newWater < 0 {
			newWater = 0
		}
		ret.Water = vol(ret.Water, newWater)
		waterVol = newWater
		bubble = maxBubble
	}
	ret.SteamBubble = u.NewValue(bubble, u.Gallon)
	ret.Pressure = Pressure(float64(PressurizerBasePressure) + bubble*PressurizerBubbleStiffness)

	// 4) Cooldown path. "All pressurizer heaters are turned off and
	// the spray valves are controlled manually to cool the
	// pressurizer." specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-82
	// Modeled as a slow bubble condensation (water rises) when the
	// pressurizer sits above its base pressure with no active
	// control.
	if !ret.HeatersOn && ret.SprayOpen == 0 {
		base := float64(PressurizerBasePressure)
		current := float64(ret.Pressure)
		if current > base {
			psiDrop := float64(PressurizerAmbientDecay) * dt.Seconds()
			if current-psiDrop < base {
				psiDrop = current - base
			}
			bubbleDrop := psiDrop / PressurizerBubbleStiffness
			newWater := waterVol + bubbleDrop
			if newWater > totalVol {
				newWater = totalVol
			}
			ret.Water = vol(ret.Water, newWater)
			ret.SteamBubble = u.NewValue(totalVol-newWater, u.Gallon)
			ret.Pressure = Pressure(float64(PressurizerBasePressure) +
				(totalVol-newWater)*PressurizerBubbleStiffness)
		}
	}

	// 5) PORV / safety valve. When the pressurizer over-pressurises
	// past the safety setpoint the valve opens; the step simply
	// toggles Open and lets the next step's pressure integration
	// react. "Pressurizer pressure control system [consists of]
	// pressurizer heaters, spray, and relief valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:5-9
	if porv != nil {
		v := *porv
		if ret.Pressure > PressurizerPORVTrip {
			if v.Open < 1 {
				v.Open = 1
			}
		} else if v.Open > 0 {
			v.Open = 0
		}
		*porv = v
	}

	return ret
}
