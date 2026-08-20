package core

// ReactorBuilding holds the primary loop, including the steam generator.
type ReactorBuilding struct {
	ReactorPressureVessel
	SteamGenerator
	ReactorCoolantPump Pump
	Pressurizer
	SafetyValve           Valve
	BlockValve            Valve
	PORV                  Valve
	PressurizedReliefTank Tank
}

// Turbine building holds the secondary loop, excluding the steam generator.
type TurbineBuilding struct {
	Turbine
	Generator PowerGenerator

	Condenser
	MakeupValve      Valve
	RejectValve      Valve
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
	HeaterDrainTank   Tank
	HeaterDrainPump   Pump
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

type CoolingTower struct{}
