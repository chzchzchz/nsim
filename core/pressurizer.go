package core

import (
	u "github.com/bcicen/go-units"
	"math"
)


// Step advances the pressurizer by TimeStep. Pressure follows a
// heater/spray/idle energy balance while liquid inventory moves through
// boiloff, spray condensation, and the CVCS, PORV, and relief-discharge
// paths, so the pressure loop (heaters and spray) stays independent of
// the level loop (charging versus letdown). Relief-valve actuation and
// discharge return the steam-equivalent volume relieved this step for
// the caller to route into the pressurizer relief tank.
//
// The rcsTemp argument is the hot-leg coolant temperature in contact
// with the surge line: it sets the saturation floor the spray, idle,
// and relief paths can depressurize toward, so manual pressure control
// tracks the applicable pressure-temperature limits during cooldown.
//
// sprayDrive is the reactor coolant pump flow fraction (0..1): normal
// spray needs an operating RCP in a loop with spray flow connections.
// auxSpray is the effective auxiliary spray drive (0..1) from throttled
// charging flow, available with the pumps secured.
//
// Spec justification for each block is inline below.
func (p *Pressurizer) Step(rcsTemp Temperature, porv, safety, block *Valve,
	sprayDrive, auxSpray float64,
) (Pressurizer, Volume) {
	ret := *p
	dt := TimeStep
	satFloor := float64(PressurizerSatPressure(rcsTemp.Float()))

	// Automatic pressure regulation. The thermostat latches the
	// heaters on once the pressure sags below the band around the
	// heater target and releases them at the target; the sag itself
	// comes from the idle-condensation and surge paths above. Inside
	// the band the previous latch state holds, so the pair regulates
	// without chattering.
	//   "Pressurizer pressure will recover as pressurizer heaters
	//    automatically energize."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:97-98
	heaters := ret.HeatersOn
	if ret.HeaterAutoControl {
		target := ret.heaterTarget()
		switch {
		case ret.Pressure <= target-PressurizerAutoHeaterBand:
			ret.AutoHeatersOn = true
		case ret.Pressure >= target:
			ret.AutoHeatersOn = false
		}
		heaters = heaters || ret.AutoHeatersOn
	}

	// Thermal-expansion surge: the loop inventory drains the
	// pressurizer on cooldown and refills it on heatup, moving the
	// bubble boundary (page_227.txt:68-72). The clamped bounds are
	// the ledger sink for extreme strokes.
	if p.PrevRCSTemp.Val != 0 {
		dTF := rcsTemp.Float() - p.PrevRCSTemp.Float()
		surgeGal := -PZRSurgeGalPerF * dTF
		ret.Water = vol(ret.Water,
			min(max(ret.Water.Float()-surgeGal, 0), ret.Volume.Float()))
	}
	ret.PrevRCSTemp = rcsTemp

	// 1) Heater effect. Energized heaters boil liquid into the steam
	// space, raising pressure toward the regulation target at
	// PressurizerHeaterRate and converting the corresponding liquid
	// volume to steam.
	//
	// "All pressurizer heaters are energized to start the heatup of
	// the pressurizer necessary to form the steam bubble."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-17
	if heaters && ret.Water.Float() > 0 {
		target := ret.heaterTarget()
		psiRise := min(float64(PressurizerHeaterRate)*dt.Seconds(),
			float64(target)-float64(ret.Pressure))
		if psiRise > 0 {
			ret.Pressure = Pressure(float64(ret.Pressure) + psiRise)
			bubbleRise := psiRise / PressurizerBubbleStiffness
			newWater := max(ret.Water.Float()-bubbleRise, 0)
			ret.Water = vol(ret.Water, newWater)
		}
	}

	// 2) Spray effect. Normal spray draws cold RCS water through the
	// spray line, condensing steam into the liquid inventory and
	// pulling pressure toward the coolant's saturation floor until
	// the steam space is exhausted; its authority scales with RCP
	// flow because "To provide spray flow a RCP must be operating in
	// one of the two loops with spray flow connections"
	// (page_227.txt:96-98). Auxiliary spray delivers the same
	// condensation from throttled charging flow when the pumps are
	// secured (page_227.txt:100-106).
	//
	// "RCS pressure is maintained by adjustment of the pressurizer
	// heaters and/or pressurizer spray."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:28-29
	if ret.SprayOpen > 0 || auxSpray > 0 {
		bubble := ret.Volume.Float() - ret.Water.Float()
		psiDrop := (ret.SprayOpen*sprayDrive*float64(PressurizerSprayRate) +
			auxSpray*float64(PressurizerAuxSprayRate)) * dt.Seconds()
		psiDrop = min(psiDrop, float64(ret.Pressure)-satFloor)
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
	// volume to the liquid inventory as pressure settles toward the
	// saturation pressure of the hot-leg coolant. Tracking saturation
	// is what lets a planned cooldown depressurize alongside the
	// cooling RCS instead of stalling at a fixed floor.
	// "All pressurizer heaters are turned off and the spray valves are
	// controlled manually to cool the pressurizer at a sufficiently
	// slow rate to maintain the system pressure within the applicable
	// pressure-temperature limits."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-82,
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:94-96
	if !heaters && ret.SprayOpen == 0 && auxSpray == 0 {
		base := satFloor
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
				satFloor))
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

	// HeaterAutoControl arms the automatic pressure regulation
	// described on page_233.txt:97-98: "Pressurizer pressure will
	// recover as pressurizer heaters automatically energize." While
	// armed, the operator's HeatersOn latch becomes a manual override
	// and the heaters cycle inside PressurizerAutoHeaterBand of the
	// heater target instead.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:95-98
	HeaterAutoControl bool

	// AutoHeatersOn is the derived thermostat latch of the armed
	// automatic pressure regulation; it is not an operator input.
	AutoHeatersOn bool

	// PressureSetpoint selects the pressure the heater regulation
	// holds. Staged setpoint writes walk heaters and spray along the
	// applicable temperature-pressure program instead of leaving only
	// the model floor and the no-load target reachable.
	// "operation of pressurizer heaters and spray maintains the
	// system pressure in accordance with the applicable
	// temperature-pressure curves."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:142-145
	PressureSetpoint Pressure

	// PrevRCSTemp is the hot-leg temperature seen by the previous
	// step; its finite difference drives thermal-expansion surge.
	PrevRCSTemp Temperature
}

// heaterTarget is the pressure the heater paths regulate toward: the
// operator-selected setpoint when one is written, else the no-load
// target, so a zero-valued field preserves the historical behaviour.
func (p *Pressurizer) heaterTarget() Pressure {
	if p.PressureSetpoint > 0 {
		return p.PressureSetpoint
	}
	return PressurizerHeaterTarget
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
	// Active cooldown-control paths instead track the coolant
	// saturation (PressurizerSatPressure); this constant remains the
	// standstill calibration used by fixtures.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-58
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	PressurizerBasePressure Pressure = 375
	PressurizerHeaterTarget    Pressure = 2250
	PressurizerHeaterRate      Pressure = 200 // psi/s while HeatersOn
	// PressurizerAutoHeaterBand is the sag below the heater target at
	// which the armed automatic pressure regulation latches the
	// heaters on; it releases them back at the target. Derived; the
	// spec describes the automatic energization without a band.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:97-98
	PressurizerAutoHeaterBand Pressure = 50
	PressurizerSprayRate       Pressure = 300 // psi/s at SprayOpen == 1
	// PressurizerAuxSprayRate is the depressurization rate at full
	// auxiliary spray drive. Sized an order below normal spray:
	// charging flow is a small fraction of an RCP's spray injection,
	// so auxiliary control is deliberately slow per the manual P-T
	// limit discipline it serves; the spec states no rate figure.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-106
	PressurizerAuxSprayRate    Pressure = 30 // psi/s at aux drive == 1
	PressurizerAmbientDecay    Pressure = 5   // psi/s cooldown when idle
	// PressurizerBubbleStiffness converts heater and spray pressure
	// changes into displaced liquid volume during boiloff and
	// condensation.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:66-67
	PressurizerBubbleStiffness float64 = 2.5 // psi per gallon of bubble
	PressurizerPORVTrip        Pressure = 2500

	// PressurizerSafetySetpoint is where the self-actuating ASME
	// code safety valves open, above the design pressure and the
	// PORV setpoint so the escalation order is PORV first.
	// "If system pressure continues to rise, self-actuating
	// ASME-code safety valves will open."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-32
	// Derived magnitude: design pressure + ~4% margin.
	PressurizerSafetySetpoint Pressure = 2600

	// Antoine vapor-pressure coefficients for water, P in mmHg and
	// T in °C: log10(P) = A - B/(C+T). Two temperature branches.
	// specs/wiki/Vapor_pressure_of_water.txt:793-795
	antoineA1, antoineB1, antoineC1 = 8.07131, 1730.63, 233.426 // 1-99 °C
	antoineA2, antoineB2, antoineC2 = 8.14019, 1810.94, 244.485 // 100-374 °C
	mmHgPerPsi                      = 51.7149

	// PressurizerMinSatFloor keeps the correlation finite below its
	// validated range (assumption; source is silent about sub-atmospheric
	// layup pressures).
	PressurizerMinSatFloor = 14.7

	// PZRSurgeGalPerF scales coolant thermal contraction into
	// pressurizer level movement: cooling shrinks the loop inventory
	// and drains the pressurizer, growing the bubble. Sized so the
	// no-load-to-350°F contraction stroke spans the lumped
	// pressurizer's operating band (invented modeling convention: the
	// source describes the mechanism, not gallon figures).
	// "Pressurizer level is raised in anticipation of coolant
	// contraction during the cooldown."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:68-69
	PZRSurgeGalPerF = 5.8
)

// PressurizerSatPressure is the saturation pressure (psia) of water at
// tempF, floored at atmospheric. It is the lower bound the spray, idle,
// and relief paths can depressurize toward: manual pressure control
// during a planned cooldown follows the coolant temperature down the
// applicable pressure-temperature limits instead of stalling at a fixed
// floor, which is what makes the RHRS entry pressure reachable.
//
// "All pressurizer heaters are turned off and the spray valves are
// controlled manually to cool the pressurizer at a sufficiently slow
// rate to maintain the system pressure within the applicable
// pressure-temperature limits."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-82,
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:94-96
//
// Known approximation: the Antoine fit reads ~3 percent high against
// tabulated saturation pressures near 290 °C; acceptable for control
// purposes, not for thermodynamic bookkeeping.
func PressurizerSatPressure(tempF float64) Pressure {
	c := (tempF - 32.0) / 1.8
	a, b, c0 := antoineA2, antoineB2, antoineC2
	if c < 99 {
		a, b, c0 = antoineA1, antoineB1, antoineC1
	}
	c = min(max(c, 0), 373)
	pPsi := math.Pow(10, a-b/(c0+c)) / mmHgPerPsi
	return Pressure(max(pPsi, PressurizerMinSatFloor))
}

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
