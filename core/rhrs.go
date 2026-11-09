package core

import (
	u "github.com/bcicen/go-units"
	"time"
)

// Step advances one RHRS train by one timestep. The Step function
// implements the three-mode behaviour described in the spec:
//
//  1. Pump off: no flow, no heat rejection.
//  2. Pump on, suction closed, warmup incomplete: pump runs on
//     its minimum-flow recirculation line. HeatTransferRate is
//     zero; the HX is not in service yet.
//  3. Pump on, suction open, warmup complete: pump returns flow
//     to the RCS cold legs. The fraction routed through the HX
//     is set by BypassValve; the rest bypasses the HX. The spec
//     describes the bypass-controller architecture as keeping
//     total return flow constant while the operator adjusts the
//     throttle valve to control cooldown rate.
//
// Spec justification:
//   - Minimum-flow recirc: "A minimum flow for the RHR pump is
//     provided by a recirculation line from the heat exchanger
//     outlet to the pump suction which remains open in all system
//     operating modes."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:53-57
//   - Warmup procedure: "RHRS operation generally begins with a
//     short period wherein the pump is run on its miniflow circuit
//     in order to warm the pump and heat exchanger [...] Following
//     system warmup, the block valves in the RHRS suction lines
//     from the RCS are opened [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:26-32
//   - Bypass controller: "A second flow control valve located in
//     the bypass line around the RHR heat exchangers is
//     automatically positioned by a flow controller that measures
//     flow in the main return line to the RCS. Operation of the
//     controller and bypass valve maintains constant return flow
//     to the RCS."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:61-66
//   - Heat transfer: "transfer heat from the hot reactor coolant
//     fluid to the component cooling water circulating through
//     the RHR heat exchanger shell side."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:19-22
func (t *RHRSTrain) Step(dt time.Duration,
	rcsHotTemp Temperature,
	ccwInletTemp Temperature,
	suctionOpen bool,
) RHRSTrain {
	ret := *t

	// DriveReady is wired from the per-train ESF bus by
	// Plant.Step. RHR pumps need bus power to deliver flow;
	// the gate follows the AFWPump / CCWS / SWS pattern so
	// an unenergised bus drops the train back to "intent
	// without power" without operator action.
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:33-36
	if !ret.Pump.Running || !ret.DriveReady {
		ret.ReturnFlow = Volume{Val: 0, Unit: u.Gallon}
		ret.HeatExchanger.HeatTransferRate = 0
		return ret
	}

	ret.WarmupTimer = ret.WarmupTimer + dt
	if !ret.WarmupComplete && ret.WarmupTimer >= RHRSTrainWarmupTime {
		ret.WarmupComplete = true
	}

	minFlow := RHRSPumpMinFlowGalPerStep * dt.Seconds()
	if !suctionOpen || !ret.WarmupComplete {
		ret.ReturnFlow = Volume{Val: minFlow, Unit: u.Gallon}
		ret.HeatExchanger.HeatTransferRate = 0
		return ret
	}

	hxFlow := RHRSTrainDesignFlow * (1.0 - ret.BypassValve.Open) * dt.Seconds()
	bypassFlow := RHRSTrainDesignFlow * ret.BypassValve.Open * dt.Seconds()
	ret.ReturnFlow = Volume{Val: hxFlow + bypassFlow + minFlow, Unit: u.Gallon}

	rcsOut := rcsHotTemp.Float()
	ccwIn := ccwInletTemp.Float()
	if hxFlow <= 0 || rcsOut <= ccwIn+RHRSHXApproachTemp {
		ret.HeatExchanger.HeatTransferRate = 0
		return ret
	}
	// Duty scale (CHECK §2: one scale per sum). The HX duty is
	// heat removed per second: mass flow (kg/s) times specific
	// heat (J/kg·K) times the driving ΔT. RHRSTrainDesignFlow is
	// read as gal/step elsewhere (minFlow uses the same
	// convention, core/parts.go:3245, and hxFlow already includes
	// dt), so it converts via KilogramsPerGallon with a
	// /dtSeconds rate factor to mean kg/s on any tick, and the
	// driving ΔT is the approach residual
	// (deltaT − 20 °F) in °F converted to K (5/9). Eff 0.7
	// absorbs the secondary-side approach and fouling the tree
	// does not model. Tier-B (design-flow number is
	// tree-derived; only the form is textbook-Newton).
	//
	// Approach limiting: an HX cannot move heat across a zero
	// approach. The taper divides by the total ΔT instead of the
	// approach residual so duty falls linearly from full at
	// (ΔT = 2×approach) to zero AT the approach point — smooth
	// over a band, not a step. Outside the guard the duty is 0.
	// At delta <= 20 °F the residual is within approach: duty
	// zero; at delta=27 duty is small (1.5 MW) but cold-hold
	// heatup within the 50 °F/hr program limit (scenario 38's
	// ±2 °F is a hold-band, not a physical zero).
	fwMassKgPerS := float64(RHRSTrainDesignFlow) / dt.Seconds() * KilogramsPerGallon
	deltaT := rcsOut - ccwIn
	approach := deltaT - RHRSHXApproachTemp
	maxDuty := fwMassKgPerS * SpecificHeatWater * (approach * 5.0 / 9.0)
	eff := 0.7
	taper := approach / max(deltaT, 1.0)
	duty := Watts(eff * maxDuty * taper * (1.0 - ret.BypassValve.Open))
	ret.HeatExchanger.HeatTransferRate = duty

	return ret
}

// Step advances the RHRS plant-level state. The function checks
// the entrance criteria (RCS temp ≤ 350°F, RCS pressure < 400 psig)
// and latches Running once. While running, both trains are stepped
// in parallel and HasTakenOver is set once any heat is being
// rejected to CCW — this gates steam-dump shutdown in Plant.Step.
//
// Spec justification:
//   - Entrance criteria: "When the reactor coolant temperature
//     has dropped to 350°F (177°C) and the pressure has been
//     reduced to less than approximately 400 psig (28 bar), the
//     RHR equipment is placed in operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
//   - Hand-off from steam dump: "At this point the reactor cooling
//     function has been taken over by the RHRS, and the steam dump
//     system may be shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:35-37
func (r *RHRS) Step(dt time.Duration,
	rcsHotTemp Temperature,
	rcsPressure float64,
	ccwInletTemp Temperature,
) RHRS {
	ret := *r

	canStart := rcsHotTemp.Float() <= RHRSTempThreshold &&
		rcsPressure < RHRSPressureThreshold
	if canStart && !ret.Running {
		ret.Running = true
	}

	if !ret.Running {
		return ret
	}

	aSuction := ret.TrainA.SuctionValves[0].Open == 1 &&
		ret.TrainA.SuctionValves[1].Open == 1
	bSuction := ret.TrainB.SuctionValves[0].Open == 1 &&
		ret.TrainB.SuctionValves[1].Open == 1

	ret.TrainA = ret.TrainA.Step(dt, rcsHotTemp, ccwInletTemp, aSuction)
	ret.TrainB = ret.TrainB.Step(dt, rcsHotTemp, ccwInletTemp, bSuction)

	totalHeatRejection := float64(ret.TrainA.HeatExchanger.HeatTransferRate) +
		float64(ret.TrainB.HeatExchanger.HeatTransferRate)
	if totalHeatRejection > 0 {
		ret.HasTakenOver = true
	}

	return ret
}

// SolidPlantRelieve trims a water-solid primary back toward the selected
// pressure through the pressure control valve between the RHR and CVC
// systems, mutating pzr in place and returning the liquid volume bled
// this step for the caller to route into the CVC letdown path. It has
// authority only while an RHR train provides forced flow between the two
// systems and pressure stands above the setpoint; at or below the
// setpoint it stands down, leaving further control to the heaters and
// spray paths.
//
// Each gallon bled drops pressure by PressurizerBubbleStiffness, the
// same liquid-equivalent currency the relief-discharge path uses; the
// drop is capped at the overshoot so the bleed settles on the selected
// pressure instead of undershooting it.
//
// "If the pressurizer is filled solid, rapid rises in primary system
// pressure can occur due to small inputs of water mass or heat. Solid
// plant primary pressure control utilizes a pressure control valve
// between the RHR and CVC systems which is set to maintain a selected
// pressure while charging makes up for any coolant contraction and
// losses."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:115-122,
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:25-32
func (r *RHRS) SolidPlantRelieve(dt time.Duration, pzr *Pressurizer) Volume {
	r.SolidPlantReliefFlow = Volume{Val: 0, Unit: u.Gallon}
	if r.SolidPlantPressureValve.Open <= 0 || !r.Running {
		return r.SolidPlantReliefFlow
	}
	overshoot := float64(pzr.Pressure - r.SolidPlantPressureSetpoint)
	if overshoot <= 0 {
		return Volume{}
	}

	gal := min(RHRSSolidPlantBleedGalPerSec*r.SolidPlantPressureValve.Open*
		dt.Seconds(), pzr.Water.Float())
	psiDrop := min(gal*PressurizerBubbleStiffness, overshoot)
	removed := psiDrop / PressurizerBubbleStiffness
	if removed <= 0 {
		return Volume{}
	}
	newWater := max(pzr.Water.Float()-removed, 0)
	pzr.Water = vol(pzr.Water, newWater)
	pzr.SteamBubble = Volume{
		Val: pzr.Volume.Float() - pzr.Water.Float(), Unit: u.Gallon}
	pzr.Pressure -= Pressure(psiDrop)

	r.SolidPlantReliefFlow = Volume{
		Val: removed / dt.Seconds(), Unit: u.Gallon}
	return Volume{Val: removed, Unit: u.Gallon}
}
