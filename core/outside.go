package core

type Outside struct {
	// 450K gallons,
	// 239K gallons to be operable
	// 196 of usable water
	// 27.7 is minimumal vortexing level
	// 14.4 unavialable gallons due to instrument level error
	// sums to 239K
	// Enough water to maintain plant in hot water standby for 2 hours
	// 350F is cutoff where residual heat removal system is capable of cooling.
	CondensateStorageTank Tank

	TurbineAFWPump AFWPump
	DieselAFWPump  AFWPump
	MotorAFWPump   AFWPump

	// The pump recirculation line is a 2 inch pipe
	CirculatingWaterPump Pump

	// CirculatingWaterSystem tracks the CW flow delivered to
	// the main condenser (plans/p31_1.txt). InletTemp and
	// FlowGPM flow downstream into Condenser.Step.
	// "The Circulating Water System serves as the major heat
	// sink for the power plant by providing large amounts of
	// cooling water for the main condensers."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
	CirculatingWaterSystem CirculatingWaterSystem

	// CoolingTower models the mechanical-draft cooling tower
	// as an alternate CW source. The CWS Step consults
	// CoolingTower.FanRunning to decide whether to apply the
	// cooling-tower approach temperature.
	// "Depending on the plant, the source of circulating
	// water may be a river, lake, ocean, or cooling tower."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
	CoolingTower

	// Transformer, ServiceWaterPump, and DieselGenerator were
	// moved to the AuxiliaryBuilding per plans/x/aux.md.

	RiverTemp Temperature
}

func (o *Outside) Step() Outside {
	ret := *o
	// Circulating Water System step. The CW pump count is
	// collapsed to the spec's "one per half-condenser"
	// architecture when the pump is running; both pumps are
	// assumed online together so the per-step flow matches
	// CirculatingWaterDesignFlowGPM.
	cwRunning := o.CirculatingWaterPump.Running
	cwPumps := 0
	if cwRunning {
		cwPumps = CirculatingWaterPumpCount
	}
	ret.CirculatingWaterSystem = o.CirculatingWaterSystem.Step(
		cwRunning,
		cwPumps,
		o.RiverTemp,
		&o.CoolingTower,
	)
	// The cooling-tower fan mirrors the CW pump state in
	// the simplified model; a future plan that adds a
	// dedicated cooling-tower fan control can decouple
	// these flags.
	ret.CoolingTower = o.CoolingTower.Step(cwRunning)
	return ret
}
