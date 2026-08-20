package core

import (
	u "github.com/bcicen/go-units"
)

// ReactorBuilding holds the primary loop, including
// the steam generator.
type ReactorBuilding struct {
	ReactorPressureVessel
	SteamGenerator
	ReactorCoolantPump Pump
	Pressurizer
	SafetyValve Valve
	BlockValve Valve
	PORV Valve
	PressurizedReliefTank Tank
}

// Turbine building holds the secondary loop, excluding
// the steam generator.
type TurbineBuilding struct {
	Turbine
	Generator	PowerGenerator

	Condenser
	MakeupValve Valve
	RejectValve Valve
	RecirculateValve Valve


	// Hotwell	Tank

	CondensatePump Pump

	// Downstream of the condensate pumps are the
	// connections to the demineralizers.
	// TODO: 7.2-2
	// CondensatePolisher Polisher

	// Downstream of polisher
	LowPressureHeater
	/*
	The heater drain tanks are horizontal 13,120-gal tanks. Each tank normally receives
drains from its associated fifth-stage heater and moisture separator drain tank. Each
tank also receives alternate drains from the associated sixth- and seventh-stage
heaters and first- and second-stage reheater drain tanks. Each tank is designed for
220 psig and 400°F. The tanks are cross-connected by a 20-in. line. A dump valve
which taps off the cross-connection drains the tanks to condenser A on a high level
in either tank. */
	HeaterDrainTank Tank
	HeaterDrainPump Pump
	MainFeedwaterPump Pump
	HighPressureHeater

	SecondStageHeaterDrainTank Tank
	SecondStageHeaterDrainPump Pump

/*
	MainFeedRegValve Valve
	MainFeedIsolationValve Valve
	MainFeedBypassValve Valve
	MainFeedBypassIsolationValve Valve
*/
}

type CoolingTower struct {}

type Outside struct {
	// 450K gallons,
	// 239K gallons to be operable
	// 196 of usable water
	// 27.7 is minimumal vortexing level
	// 14.4 unavialable gallons due to instrument level error
	// sums to 239K
	// Enough water to maintain plant in hot water standby for 2 hours
	// 350F is cutoff where residual heat removal system is capable of cooling.
	CondensateStorageTank	Tank

	TurbineAFWPump AFWPump
	DieselAFWPump AFWPump
	MotorAFWPump AFWPump

	// The pump recirculation line is a 2 inch pipe
	CirculatingWaterPump Pump
	CoolingTower
	Transformer

	ServiceWaterPump Pump
	DieselEngine

	RiverTemp Temperature
}

func (o *Outside) Step() Outside {
	ret := *o
	if o.CirculatingWaterPump.Running {
		// increase pipe pressure
	} else {
		// decrease pipe pressure
	}
	return ret
}

func (r *ReactorBuilding) Step() ReactorBuilding {
	return *r
}

func (t *TurbineBuilding) Step() TurbineBuilding {
	return *t
}


type Plant struct {
	ReactorBuilding
	TurbineBuilding
	Outside
	nextPlant *Plant
}

func (p *Plant) Step() {
	p.nextPlant.ReactorBuilding = p.ReactorBuilding.Step()
	p.nextPlant.TurbineBuilding = p.TurbineBuilding.Step()
	p.nextPlant.Outside = p.Outside.Step()
}

func (p *Plant) Update() {
	p.ReactorBuilding = p.nextPlant.ReactorBuilding
	p.TurbineBuilding = p.nextPlant.TurbineBuilding
	p.Outside = p.nextPlant.Outside
}

func MakePlant() Plant {
	p := Plant{nextPlant: &Plant{}}
	p.TurbineBuilding.HeaterDrainTank = Tank{
		Level: u.NewValue(0, u.Gallon),
		Capacity: u.NewValue(13120, u.Gallon),
		Temperature: u.NewValue(80, u.Fahrenheit),
	}
	p.Outside = Outside{
		CondensateStorageTank : Tank{
			Level: u.NewValue(300000, u.Gallon),
			Capacity: u.NewValue(450000, u.Gallon),
			Temperature: u.NewValue(80, u.Fahrenheit),
		},
		DieselEngine : DieselEngine{
			Speed : 0,
			Fuel : Tank{
				Level: u.NewValue(0, u.Gallon),
				// Can run feed pump for 24 hours
				// Can run generators for 4 days
				Capacity: u.NewValue(500, u.Gallon),
				Temperature: u.NewValue(80, u.Fahrenheit),
			},
		},
		RiverTemp: u.NewValue(75, u.Fahrenheit),
	}
	return p
}
