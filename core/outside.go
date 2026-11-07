package core

import "time"

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
	// as an alternate CW source with its own basin inventory
	// and cold-basin temperature. The CWS draws suction from
	// the basin when SourceIsCoolingTower is set.
	// "Depending on the plant, the source of circulating
	// water may be a river, lake, ocean, or cooling tower."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
	CoolingTower `json:"CoolingTower"`

	// AmbientWetBulbTemp drives cooling-tower evaporative
	// performance. The wet-bulb is the evaporative lower
	// bound: "effectively the lowest temperature that may be
	// achieved by evaporative cooling".
	// specs/wiki/Wet-bulb_temperature.txt:775
	AmbientWetBulbTemp Temperature

	// ServiceWaterSystem models the SWS — the ultimate heat
	// sink for the plant. The pumps live in a separate
	// intake house per
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:48-50.
	//
	// Spec justification:
	//   - "The Service Water System supplies all the cooling
	//     water for the plant, including that required for
	//     emergency shutdown. Consequently, high reliability
	//     and redundancy is required."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
	//   - "The pumps are frequently located in a separate
	//     intake house."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:48-50
	//   - "The three essential service water pumps for each
	//     unit are typically rated at approximately 20,000
	//     gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	ServiceWaterSystem ServiceWaterSystem

	RiverTemp Temperature
}

func (o *Outside) Step(dt time.Duration, electricalDriveReady bool,
	condenserOutlet Temperature) Outside {
	ret := *o
	// Circulating Water System step. The CW pump count is
	// collapsed to the spec's "one per half-condenser"
	// architecture when the pump is running; both pumps are
	// assumed online together so the per-step flow matches
	// CirculatingWaterDesignFlowGPM.
	cwRunning := o.CirculatingWaterPump.Running
	cwPumps := 0
	if cwRunning && electricalDriveReady {
		cwPumps = CirculatingWaterPumpCount
	}
	// The tower steps first so the CWS reads this tick's
	// basin level and cold-basin temperature; it uses last
	// tick's circulating flow (a 1 ms lag).
	ret.CoolingTower = o.CoolingTower.Step(dt,
		cwRunning,
		electricalDriveReady,
		o.CirculatingWaterSystem.FlowGPM,
		condenserOutlet,
		o.AmbientWetBulbTemp,
	)
	ret.CirculatingWaterSystem = o.CirculatingWaterSystem.Step(dt,
		cwRunning,
		electricalDriveReady,
		cwPumps,
		o.RiverTemp,
		&o.CoolingTower,
	)
	return ret
}
