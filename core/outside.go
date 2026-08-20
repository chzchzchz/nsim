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

	// The diesel-driven AFW pump must be gated by the diesel's readiness:
	// an AFW pump labelled "diesel-driven" cannot deliver flow when its
	// diesel is not running. DriveReady mirrors the upstream; effective
	// flow is `Running && DriveReady`, computed by the consumer (the
	// fluid model). We do not stomp Running here so an operator's
	// pre-start intent survives the Cranking transient.
	// specs/.../chapter_5/page_099.txt:89-93.
	ret.DieselAFWPump.DriveReady = ret.DieselGenerator.ReadyToLoad
	return ret
}
