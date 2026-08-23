package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the Safety Injection System by one timestep.
// The step function implements three independent state machines
// that run in parallel:
//
//   1. Accumulator discharge — passive: a tank begins
//      discharging into the RCS cold legs as soon as RCS pressure
//      drops below the nitrogen precharge. The tank empties at
//      SISAccumulatorDischargeRate × SISAccumulatorCount until
//      Level reaches zero, at which point Discharging latches
//      false.
//   2. RWST depletion — active while the HHSI pumps are running
//      and aligned to RWST suction. During the injection phase
//      the pumps draw from the RWST at SISHHSIDesignFlow per
//      pump. SISHHSIMiniFlowFraction recirculates back to the
//      RWST via the continuous minimum-flow path so an
//      inadvertent start cannot deadhead the pump.
//   3. Recirculation switchover — once RWSTLevel falls below
//      SISRWSTLowLevel the protection logic aligns suction to
//      containment sump. The switchover is latched so it does
//      not toggle on subsequent steps.
//
// Spec justification:
//   - Passive accumulator discharge: "The accumulator tanks are
//     passive components normally requiring no maintenance, and
//     are located inside the containment to provide rapid
//     injection of water following a LOCA."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:11-14
//   - Discharge trigger: "the accumulator discharge will begin
//     automatically as the RCS pressure decreases below the
//     accumulator tank pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
//   - HHSI miniflow: "A continuous minimum flow path which does
//     not require isolation during the injection phase is
//     provided so that an inadvertent pump start will not result
//     in the deadheading of the HHSI pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_096.txt:26-30
//   - HHSI shutoff head and miniflow operation: "the HHSI pumps
//     will be started upon receipt of the actuation signal and
//     will run on miniflow until the RCS pressure falls below
//     the respective shutoff heads of the pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:27-30
//   - RWST suction during injection: "The HHSI and RHR pumps
//     are aligned to take suction directly from the RWST."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
//   - Recirculation switchover: "The changeover from the
//     injection mode to the cold-leg recirculation mode is
//     initiated automatically when the RWST level channels
//     indicate an RWST level less than a low-level setpoint in
//     conjunction with the initiation of the engineered
//     safeguards actuation signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:56-60
//   - Sump recirculation: "both the HHSI and RHR pumps for
//     each subsystem take suction directly from one of the two
//     containment sump recirculation lines."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:173-175
//   - Miniflow isolation valves close at switchover: "To
//     prevent backflow to the RWST of contaminated water from
//     the containment sump, the HHSI pump miniflow isolation
//     valves are automatically closed at this time."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:61-63
func (s *SIS) Step(rcsPressure float64) SIS {
	ret := *s
	dt := TimeStep

	ret.AccumulatorInjectionFlow = Volume{Val: 0, Unit: u.Gallon}
	ret.HHSIInjectionFlow = Volume{Val: 0, Unit: u.Gallon}

	// 1) Accumulator discharge.
	//
	// Discharge begins automatically when RCS pressure drops
	// below the precharge threshold (and remains active while
	// the tank still has water). The Discharge bit latches
	// true once started — but if Level reaches zero the bit
	// clears so the empty tank does not produce phantom flow.
	ret.Accumulator.Pressure = Pressure(SISAccumulatorPrechargePressure)
	canDischarge := ret.Accumulator.Level.Float() > 0 &&
		!ret.Accumulator.IsolationShut
	pressureBelow := rcsPressure < float64(SISAccumulatorPrechargePressure)
	if ret.Accumulator.Discharging || (canDischarge && pressureBelow) {
		ret.Accumulator.Discharging = true
		dV := SISAccumulatorDischargeRate.Float() *
			float64(SISAccumulatorCount) * dt.Seconds()
		newLevel := ret.Accumulator.Level.Float() - dV
		if newLevel <= 0 {
			newLevel = 0
			ret.Accumulator.Discharging = false
		}
		ret.Accumulator.Level = Volume{Val: newLevel, Unit: u.Gallon}
		ret.AccumulatorInjectionFlow = Volume{Val: SISAccumulatorDischargeRate.Float()*
				float64(SISAccumulatorCount), Unit: u.Gallon}
	}

	// 2) RWST depletion under HHSI injection.
	//
	// HHSI pump flow splits: the SISHHSIMiniFlowFraction
	// recirculates back to the RWST via the continuous minimum-
	// flow path so the pump cannot deadhead, and the
	// remainder goes into the RCS cold legs. HHSI pumps
	// need ESF bus power; DriveReady is wired from
	// ESFBusA.Energized || ESFBusB.Energized by Plant.Step.
	if ret.HHSIPumpRunning && ret.HHSIDriveReady && ret.HHSISuction == SISSuctionRWST {
		dW := SISHHSIDesignFlow.Float() * dt.Seconds()
		newRWST := ret.RWSTLevel.Float() - dW
		newRWST = max(newRWST, 0)
		ret.RWSTLevel = Volume{Val: newRWST, Unit: u.Gallon}
		netFlow := SISHHSIDesignFlow.Float() *
			(1.0 - SISHHSIMiniFlowFraction)
		ret.HHSIInjectionFlow = Volume{Val: netFlow, Unit: u.Gallon}
	} else if ret.HHSIPumpRunning && ret.HHSIDriveReady && ret.HHSISuction == SISSuctionSump {
		netFlow := SISHHSIDesignFlow.Float() *
			(1.0 - SISHHSIMiniFlowFraction)
		ret.HHSIInjectionFlow = Volume{Val: netFlow, Unit: u.Gallon}
	}

	// 3) Recirculation switchover.
	//
	// The switchover is initiated automatically when RWST
	// level drops below the low-level setpoint and the
	// engineered-safeguards actuation signal is in effect.
	// In this model HHSIPumpRunning encodes the actuation
	// signal (the pumps start on the signal). The
	// switchover completes after SISRecirculationSwitchoverDelay
	// seconds to model the protection-logic + valve-stroke
	// latency. The result is latched so the alignment does
	// not toggle. HHSIDriveReady is required so the
	// switchover only progresses while the bus is energised.
	if !ret.SwitchoverComplete && ret.HHSIPumpRunning &&
		ret.HHSIDriveReady &&
		ret.RWSTLevel.Float() < SISRWSTLowLevel {
		ret.SwitchoverTimer += dt
		if ret.SwitchoverTimer.Seconds() >=
			SISRecirculationSwitchoverDelay {
			ret.HHSISuction = SISSuctionSump
			ret.SwitchoverComplete = true
		}
	}

	return ret
}
