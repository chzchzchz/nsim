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
	CoolingTower
	Transformer

	ServiceWaterPump Pump
	DieselGenerator

	RiverTemp Temperature
}

func (o *Outside) Step() Outside {
	ret := *o
	if o.CirculatingWaterPump.Running {
		// increase pipe pressure
	} else {
		// decrease pipe pressure
	}
	ret.DieselGenerator = o.DieselGenerator.Step(o.ServiceWaterPump.Running)
	return ret
}
