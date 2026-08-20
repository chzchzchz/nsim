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

	// ChemicalVolumeControlSystem models the CVCS — the primary
	// chemistry and volume-regulation loop. Lives on
	// ReactorBuilding because it is in the nuclear steam supply
	// system and supplies the RCS cold legs via the charging
	// pumps.
	// "The volume control tank (part of the Chemical and
	// Volume Control System)"
	// specs/the_westinghouse_pressurized_water_react/chapter_14/page_189.txt:77
	ChemicalVolumeControlSystem CVCS

	// ResidualHeatRemovalSystem models the RHRS — the post-
	// shutdown decay-heat removal path. The RHRS takes RCS hot-
	// leg coolant through two redundant trains, rejects heat to
	// component cooling water via shell-and-tube heat exchangers,
	// and returns cooled flow to the RCS cold legs. The system is
	// gated on RCS temperature ≤ 350°F and pressure < 400 psig.
	// "The primary function of the Residual Heat Removal System
	// (RHRS) is to transfer heat energy from the core and
	// Reactor Coolant System (RCS) during plant cooldown and
	// refueling operations."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:26-29
	// "When the reactor coolant temperature has dropped to 350°F
	// (177°C) and the pressure has been reduced to less than
	// approximately 400 psig (28 bar), the RHR equipment is
	// placed in operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	ResidualHeatRemovalSystem RHRS

	// ReactorMakeupControlValve is the operator-controlled
	// makeup valve between the refueling-water storage tank
	// and the volume control tank. Its Flow attribute is the
	// per-step volume of fresh borated water supplied to the
	// VCT. The spec describes the reactor makeup control
	// system in narrative form.
	// "Because of the influx of makeup water from the reactor
	// makeup control system, the volume control tank level
	// controller automatically diverts the outflow from the
	// BTRS into the BRS holdup tanks."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-7
	// "The reactor makeup control system dilution is [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:17
	ReactorMakeupControlValve MakeupValve

	// Grid is the infinite-bus model of the off-site distribution
	// grid. Lives on ReactorBuilding because it is a plant-wide
	// service consulted by both the primary (electrical RCP
	// supply) and secondary (turbine governor) sides.
	// "transmit power to the off-site distribution grid."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:18-19
	Grid Grid

	// StationTransformer models the UAT/SAT split that supplies
	// the plant distribution bus. House load is automatically
	// transferred to SAT on turbine trip / UAT de-energisation.
	// "Because plant buses are capable of receiving power from
	// either of two supplies and because automatic transfer of
	// supplies is provided on sensing loss of power, reliability
	// of power is assured."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
	StationTransformer StationTransformer

	// HouseLoad is the station's own electrical demand
	// (RCPs, feedwater pumps, condensate pumps, heaters, ESF
	// bus trickle charge, HVAC, lighting, control systems).
	// "supply power for its own auxiliaries."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
	HouseLoad HouseLoad
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

	// Downstream of polisher: four-stage LP feedwater heater
	// train, then deaerator (HeaterDrainTank plays the role of
	// the deaerator water storage tank).
	// "The condensate pumps take suction from the condenser
	// hotwell and pump condensate [...] through four stages of
	// low-pressure feedwater heaters to the deaerator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
	LowPressureHeater LowPressureHeater
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
	// Two-stage HP feedwater heater train on the main-feed-pump
	// discharge side, draining to the deaerator tank.
	// "The water discharge from the feedwater pumps flows
	// through two stages of high-pressure heaters then into the
	// steam generators."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
	HighPressureHeater HighPressureHeater

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
