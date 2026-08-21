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

	// ContainmentPressure is the lumped containment atmosphere
	// pressure (psig). A full thermodynamic model would track
	// containment as a separate building; the simplified model
	// uses a single scalar that rises when the PORV discharges
	// to containment and falls when the CSS sprays. The CSS
	// Step consumes this field as its auto-start signal:
	// "Initial Injection Mode – Containment spray is initiated
	// automatically in response to a containment high pressure
	// signal."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-57
	ContainmentPressure Pressure

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

	// ReactorProtectionSystem models the automatic protection
	// logic and engineered safeguards actuation — the "brain"
	// that monitors process variables, decides when to trip
	// the reactor (2/4 voting), and actuates the engineered
	// safety features per Table 7-1.
	//
	// Spec justification:
	//   - Functions: "Monitors the plant for abnormal
	//     conditions and alerts the operator to take
	//     appropriate action; Automatically provides reactor
	//     trip (shutdown) signals and engineered safeguards
	//     actuation signals when plant conditions [...] reach
	//     the plant safety limits."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:55-72
	//   - 2/4 voting: "A 2/4 logic means that a trip occurs
	//     when at least 2 signals out of 4 indicate a trip
	//     condition."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
	ReactorProtectionSystem ReactorProtectionSystem

	// SafetyInjectionSystem models the high-head injection phase
	// of the Emergency Core Cooling System. It bundles the
	// passive accumulator bank (four tanks, one per RCS loop),
	// the two HHSI pumps, the RWST suction alignment, and the
	// automatic switchover to containment-sump recirculation.
	//
	// Spec justification:
	//   - "The SIS consists of two independent and redundant
	//     mechanical subsystems, each of which receives
	//     electrical power from one of two separate and
	//     redundant Class 1E electrical power trains, and each
	//     of which receives an actuation signal from one of two
	//     separate and redundant actuation trains. The major
	//     components are two high-head safety injection pumps,
	//     two RHR pumps, and four accumulators."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:36-42
	//   - "For large pipe ruptures, the RCS would be rapidly
	//     depressurized and voided of coolant. [...] This high
	//     flow is provided by the accumulators, followed by the
	//     HHSI and RHR pumps, all discharging into the cold legs
	//     of the RCS."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-23
	//   - Recirculation switchover: "initiated automatically
	//     when the RWST level channels indicate an RWST level
	//     less than a low-level setpoint in conjunction with
	//     the initiation of the engineered safeguards actuation
	//     signal."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:56-60
	SafetyInjectionSystem SIS

	// EmergencyFeedwaterSystem is the secondary-side counterpart
	// to the SIS. Two identical subsystems (A and B), each with
	// one motor-driven and one turbine-driven pump sharing a
	// primary emergency feedwater tank. The motor-driven pumps
	// start on low-low SG level or on a safety-injection
	// signal; the turbine-driven pumps start when two SGs
	// simultaneously report low-low level.
	//
	// Spec justification:
	//   - "The primary function of the Emergency Feedwater
	//     System (EFWS) shown in Figure 5.2-1 is to supply
	//     feedwater to the steam generators following accident
	//     or transient conditions when the main feedwater
	//     system is not available. The EFWS thereby maintains
	//     the capability of the steam generators to remove
	//     plant stored heat and core decay heat by converting
	//     the emergency feedwater to steam which is then
	//     discharged to the condenser or to the atmosphere."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:23-32
	//   - "The use of both motor driven and turbine driven pumps
	//     satisfies the requirement that the pumps be powered by
	//     diverse power sources."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:1-5
	//   - "The system is designed to supply at least the minimum
	//     required flow, within one minute of the actuation
	//     signal, to at least two effective steam generators
	//     [...], and to continue this delivery for an indefinite
	//     period without operator action."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:23-28
	EmergencyFeedwaterSystem EFWS

	// EFWSFeedwaterFlow is the total EFWS feedwater flow rate
	// delivered to the steam generators (gal/s). Populated by
	// the EFWS step; consumed by the SG feedwater model.
	EFWSFeedwaterFlow Volume

	// ComponentCoolingWaterSystem models the CCWS — the
	// intermediate cooling loop that removes heat from
	// safeguards equipment and rejects it to the service water
	// system. The simplified model tracks a single lumped
	// inlet/outlet temperature that the RHRS Step consumes as
	// its ccwInletTemp argument.
	//
	// Spec justification:
	//   - "The Safeguards subsystem is divided into two separate
	//     and redundant safeguards trains and provides cooling
	//     water to safeguards equipment such as the residual
	//     heat removal heat exchangers [...] Each safeguards
	//     train is sized to supply 100-percent post-accident
	//     component cooling flow requirements."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:33-42
	//   - "The CCWS pumps, heat exchangers of the Safeguards
	//     subsystem and its associated piping, valves and
	//     instrumentations are seismically qualified as required
	//     for an engineered safeguards system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:116-120
	ComponentCoolingWaterSystem CCWS

	// ContainmentSpraySystem models the CSS — the engineered
	// safeguard that limits peak containment pressure and
	// removes airborne fission products following a LOCA.
	//
	// Spec justification:
	//   - "The Containment Spray System (CSS) (Figure 6-4) is an
	//     engineered safeguard designed to limit the peak
	//     pressure in the reactor containment building to a
	//     pressure less than the containment design pressure,
	//     in the event of a loss-of-coolant accident or a steam
	//     break accident inside the containment."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:75-81
	//   - CCWS services CSS pump HXs: "containment spray pump
	//     heat exchangers" are among the safeguards components
	//     cooled by the CCWS.
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:39
	ContainmentSpraySystem ContainmentSpraySystem

	// RCFCUnits are the four parallel fan units in the Reactor
	// Containment Fan Cooler System. At least RCFCUnitsRequired
	// must be running for the safeguard function to be met.
	//
	// Spec justification:
	//   - "the system includes four fan units that operate in
	//     parallel. A minimum of two units must function to
	//     satisfy design requirements for both normal operation
	//     and post-accident operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:19-24
	RCFCUnits [RCFCUnitCount]RCFCUnit

	// ContainmentBuilding models the reactor containment — the
	// pressure and temperature boundary that contains the RCS
	// and safeguards equipment. Tracked as lumped pressure and
	// temperature with sinks from CSS spray condensation and
	// RCFC heat removal.
	//
	// Spec justification:
	//   - "Transients resulting from the DBA serve as the basis
	//     for a containment design pressure of 60 psig (4.46
	//     bar)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
	//   - "This chapter discusses the design of the Reactor
	//     Containment Building (hereafter referred as the
	//     containment) that will contain and control any
	//     release of radioactivity to the environment under
	//     normal, upset, or emergency conditions."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:43-51
	ContainmentBuilding ContainmentBuilding

	// ServiceWaterSystem models the SWS — the ultimate heat
	// sink for the plant. All safety-grade coolers (CCWS HX,
	// RCFC coils, diesel coolers) depend on it.
	//
	// Spec justification:
	//   - "The Service Water System supplies all the cooling
	//     water for the plant, including that required for
	//     emergency shutdown. Consequently, high reliability
	//     and redundancy is required."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
	//   - "The three essential service water pumps for each unit
	//     are typically rated at approximately 20,000 gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	ServiceWaterSystem ServiceWaterSystem

	// SpentFuelPoolCoolingSystem models the SFPCS — the
	// subsystem that removes decay heat from the spent fuel
	// pool and maintains water clarity.
	//
	// Spec justification:
	//   - "The Spent Fuel Pool Cooling System is designed to
	//     remove residual decay heat generated by irradiated
	//     fuel stored in the spent fuel pool."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:124-126
	//   - "Normal operation requires two pumps with the third
	//     pump serving as a standby."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	SpentFuelPoolCoolingSystem SpentFuelPoolCoolingSystem

	// RefuelingOperations models the refueling operation cycle
	// — the five-phase state machine that tracks cavity
	// flooding, fuel assembly replacement, and spent fuel pit
	// management.
	//
	// Spec justification:
	//   - Five phases: "The refueling operation is divided into
	//     five major phases: preparation, reactor disassembly,
	//     fuel handling, reactor reassembly, and preoperational
	//     checks, tests, and startup."
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
	//   - Two facility types: refueling cavity (flooded only
	//     during refueling) and fuel storage pit (kept full of
	//     water).
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:23-29
	RefuelingOperations RefuelingOperations

	// ReactorMakeupControlValve is the makeup valve between the
	// refueling-water storage tank and the volume control tank.
	// Its Flow attribute is the per-step volume of fresh borated
	// water supplied to the VCT. The spec describes the reactor
	// makeup control system in narrative form.
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

	// PlantControlRoom models the operator interface for
	// monitoring and controlling all plant systems
	// (plans/p28_1.txt). Includes the Primary Control
	// Center, auxiliary panels, graphic and dedicated
	// displays, and the annunciator alarm system.
	//
	// Spec justification:
	//   - Integrates NSSS and BOP into unified
	//     man-machine interface.
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:1-5
	//   - PCC: all controls, displays, instrumentation for
	//     normal operation.
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:23-40
	//   - Auxiliary panels: safety systems arranged in
	//     stations per train.
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:61-67
	PlantControlRoom PlantControlRoom

	// PlantElectricalSystems models the plant electrical
	// power distribution, including offsite power, diesel
	// generators, and 6600-volt engineered safety features
	// buses, plus the 118V instrument-bus inverter supply
	// that feeds the four RPS/ESF instrumentation channels
	// (plans/p27_1.txt).
	//
	// Spec justification:
	//   - Diversity of reliable power sources.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:9-14
	//   - ESF buses: offsite normal, diesel backup.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:28-33
	//   - Diesel start: 10s speed, 30s rated load.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-51
	//   - 118V inverter power for RPS/ESF channels I-IV.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-27
	PlantElectricalSystems PlantElectricalSystems

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

	// MainFeedwaterControlValve is the motor-operated feedwater
	// control valve positioned by the FeedwaterRegulator to
	// modulate feedwater flow into the SG. Open=1 is full
	// design flow; Open=0 is closed.
	// "A signal from the Feedwater Control System also sets
	// the speed of the turbine-driven main feedwater pumps and
	// the position of the motor-driven feedwater pump discharge
	// control valve so as to maintain the main feedwater
	// regulators within their control range."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:73-76
	MainFeedwaterControlValve Valve

	// FeedwaterRegulator is the three-element feedwater level
	// regulator. The simplified model collapses the spec's
	// per-SG regulators (A and B) into a single shared
	// regulator (plans/p22_1.txt).
	// "Feedwater flow to each steam generator is controlled by
	// a feedwater regulator in each feedwater line. The
	// regulator is controlled by steam generator level, steam
	// flow and feedwater flow."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
	FeedwaterRegulator FeedwaterRegulator

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
