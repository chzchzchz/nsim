package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the main condenser by TimeStep.
//
// Two coupled phenomena are modelled:
//
//  1. The condenser is normally held at sub-atmospheric vacuum by
//     the steam-jet air ejectors / vacuum pumps. When those gas-
//     removal devices are off, in-leakage and dissolved gases let
//     the back-pressure climb toward atmospheric. The spec describes
//     the vacuum subsystem:
//     "Under normal operation the air is removed by steam jet air
//     ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
//
//  2. The latent heat of condensation is rejected to the
//     circulating-water (CW) flow. The CW outlet temperature rises
//     by CondenserCWRise at design flow. When the CW pumps are off,
//     the heat sink is lost and Pressure rises aggressively.
//
// Spec justification for the couplings:
//   - "The main condenser condenses the exhaust from the main
//     turbines and the two feedwater pump turbines."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-15
//   - "There is also sufficient surface to condense steam from the
//     steam dump system with an acceptable increase in turbine back
//     pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:16-18
//
// The CW-pump / vacuum-pump operating state is supplied by the
// caller (Plant.Step), since those pumps live on the Outside and
// Electrical structs respectively. The condenser itself is a passive
// heat exchanger with no internal state machine.
func (c *Condenser) Step(cwIn Temperature, vacuumPumpRunning, cwPumpRunning bool) Condenser {
	ret := *c
	dt := TimeStep

	// 1) Steam mass balance. CondensingRate is set by
	// TurbineBuilding.StepWithSteam before this call and represents
	// the per-step steam mass condensed (in gallons). Convert to
	// condensate level on the shell side.
	steamMass := ret.CondensingRate.Float()
	ret.Level = u.NewValue(ret.Level.Float()+steamMass, u.Gallon)

	// 2) Heat sink: when the CW pump is off, the tube bundle has no
	// flow. Condenser pressure relaxes toward atmospheric at the
	// no-CW degrade rate.
	cwFlowGpm := float64(0)
	if cwPumpRunning {
		cwFlowGpm = CondenserDesignCWFlowGPM
	}
	if cwFlowGpm <= 0 {
		pressure := float64(ret.Pressure)
		pressure += (float64(AtmosphericPressure) - pressure) *
			(dt.Seconds() / CondenserNoCWDegradeTimeConstant)
		if pressure < 0 {
			pressure = 0
		}
		ret.Pressure = Pressure(pressure)
		ret.CWOut = cwIn
		ret.CondensingRate = u.NewValue(0, u.Gallon)
		return ret
	}

	// 3) Vacuum-pump effect. With vacuum pumps running and minimal
	// steam load, Pressure tracks toward CondenserDesignVacuum.
	// Incoming steam load pushes the back-pressure upward — the
	// pressure rise is proportional to the ratio of actual load to
	// design full-load steam load, capped at 30% of the atmospheric
	// gap so a saturated condenser can never reach atmospheric.
	target := float64(CondenserDesignVacuum)
	if !vacuumPumpRunning {
		// Vacuum pumps off: degrade toward atmospheric (but slower
		// than the no-CW case since there is still some flow).
		target = float64(AtmosphericPressure)
	}
	if steamMass > 0 {
		loadFactor := steamMass / CondenserDesignSteamLoadGalPer
		if loadFactor > 1.0 {
			loadFactor = 1.0
		}
		target += loadFactor * (float64(AtmosphericPressure) - float64(CondenserDesignVacuum)) * 0.3
	}
	pressure := float64(ret.Pressure)
	pressure += (target - pressure) * (dt.Seconds() / CondenserVacuumTimeConstant)
	if pressure < 0 {
		pressure = 0
	}
	ret.Pressure = Pressure(pressure)

	// 4) CW outlet temperature. The CW absorbs the latent heat of
	// condensation; at design flow the delta-T across the bundle
	// is CondenserCWRise. Below design flow the delta-T scales
	// inversely with the flow ratio.
	cwDelta := CondenserCWRise * (CondenserDesignCWFlowGPM / cwFlowGpm)
	ret.CWOut = u.NewValue(cwIn.Float()+cwDelta, u.Fahrenheit)

	// 5) Hotwell level. Accumulates the per-step condensed steam
	// minus the condensate pump's suction flow (proportional to
	// the CW flow proxy). The hotwell feeds the LP heater train.
	// "The condensate pumps take suction from the condenser
	// hotwell [...]."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-42
	pumpSuctionFlow := cwFlowGpm * 0.01 // gal/s at design flow
	netInflow := steamMass - pumpSuctionFlow*dt.Seconds()
	newLevel := ret.HotwellLevel.Float() + netInflow
	if newLevel < 0 {
		newLevel = 0
	}
	ret.HotwellLevel = u.NewValue(newLevel, u.Gallon)

	ret.CondensingRate = u.NewValue(0, u.Gallon)
	return ret
}
