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

	// 1) Steam-bubble inventory. The fluid graph (see
	// simulateFluidFlow) moves water in and out of ret.Water via the
	// surge-line edges; this step just tracks the resulting bubble by
	// mass balance. We approximate by treating Water as the liquid
	// inventory and Pressure as proportional to (SteamBubble / Volume)
	// within bounds; this gives the bubble behaviour without a real
	// steam-table.
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
	ret.SteamBubble = u.NewValue(bubble, u.Gallon)

	// 2) Pressure from the bubble. Sized so a typical ~30% bubble
	// yields the 2250 psia operating pressure cited in
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:69.
	base := float64(PressurizerBasePressure)
	pressure := base + bubble*PressurizerBubbleStiffness

	// 3) Heater effect. "All pressurizer heaters are energized to start
	// the bubble formation during plant startup."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
	// "operation of pressurizer heaters and spray maintains the system
	// pressure in accordance with the applicable temperature-pressure
	// curves." specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
	if ret.HeatersOn && pressure < float64(PressurizerHeaterTarget) {
		pressure += float64(PressurizerHeaterRate) * dt.Seconds()
		if pressure > float64(PressurizerHeaterTarget) {
			pressure = float64(PressurizerHeaterTarget)
		}
	}

	// 4) Spray effect. "Pressurizer heater and spray valve control is
	// [used] to maintain the pressure in accordance with the
	// applicable temperature-pressure curves."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
	if ret.SprayOpen > 0 && pressure > float64(PressurizerSprayMin) {
		drop := ret.SprayOpen * float64(PressurizerSprayRate) * dt.Seconds()
		pressure -= drop
		if pressure < float64(PressurizerSprayMin) {
			pressure = float64(PressurizerSprayMin)
		}
	}

	// 5) Cooldown path. "All pressurizer heaters are turned off and
	// the spray valves are controlled manually to cool the
	// pressurizer." specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-82
	if !ret.HeatersOn && ret.SprayOpen == 0 && pressure > base {
		pressure -= float64(PressurizerAmbientDecay) * dt.Seconds()
		if pressure < base {
			pressure = base
		}
	}

	ret.Pressure = Pressure(pressure)

	// 6) PORV / safety valve. When the pressurizer over-pressurises
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
