package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances one RHRS train by one timestep. The Step function
// implements the three-mode behaviour described in the spec:
//
//   1. Pump off: no flow, no heat rejection.
//   2. Pump on, suction closed, warmup incomplete: pump runs on
//      its minimum-flow recirculation line. HeatTransferRate is
//      zero; the HX is not in service yet.
//   3. Pump on, suction open, warmup complete: pump returns flow
//      to the RCS cold legs. The fraction routed through the HX
//      is set by BypassValve; the rest bypasses the HX. The spec
//      describes the bypass-controller architecture as keeping
//      total return flow constant while the operator adjusts the
//      throttle valve to control cooldown rate.
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
func (t *RHRSTrain) Step(
	rcsHotTemp Temperature,
	ccwInletTemp Temperature,
	suctionOpen bool,
) RHRSTrain {
	ret := *t
	dt := TimeStep

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
	ret.ReturnFlow = Volume{Val: hxFlow+bypassFlow+minFlow, Unit: u.Gallon}

	rcsOut := rcsHotTemp.Float()
	ccwIn := ccwInletTemp.Float()
	if hxFlow <= 0 || rcsOut <= ccwIn+RHRSHXApproachTemp {
		ret.HeatExchanger.HeatTransferRate = 0
		return ret
	}
	fwMassKg := float64(RHRSTrainDesignFlow) * KilogramsPerGallon
	maxDuty := fwMassKg * SpecificHeatWater * (rcsOut - ccwIn)
	eff := 0.7
	// Duty scales with the fraction of train flow actually routed
	// through the HX: the bypass throttle is the operator's cooldown-
	// rate handle ("Operation of the controller and bypass valve
	// maintains constant return flow to the RCS" while the throttle
	// sets heat-transfer share).
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:61-66
	ret.HeatExchanger.HeatTransferRate = Watts(eff * maxDuty *
		(1.0 - ret.BypassValve.Open))

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
func (r *RHRS) Step(
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

	ret.TrainA = ret.TrainA.Step(rcsHotTemp, ccwInletTemp, aSuction)
	ret.TrainB = ret.TrainB.Step(rcsHotTemp, ccwInletTemp, bSuction)

	totalHeatRejection := float64(ret.TrainA.HeatExchanger.HeatTransferRate) +
		float64(ret.TrainB.HeatExchanger.HeatTransferRate)
	if totalHeatRejection > 0 {
		ret.HasTakenOver = true
	}

	return ret
}
