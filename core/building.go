package core

// ReactorBuilding holds the primary loop, including the steam generator.
//
// Aux is a pointer to the auxiliary building subsystem bundle
// (CVCS, RHRS, CCWS, CSS, RCFC, EFWS, diesel, transformer, SW pump,
// SFP cooling). Plant.Step sets it before each Step call so the
// cross-subsystem access in ReactorBuilding.Step can read fresh
// values. Holding a pointer (rather than embedding) keeps the
// canonical subsystem instance on Plant.AuxiliaryBuilding so updates
// from the auxiliary Step and the cross-building Plant.Step logic
// land in one place.
type ReactorBuilding struct {
	ReactorPressureVessel `json:"ReactorPressureVessel"`
	SteamGenerator        `json:"SteamGenerator"`

	// ReactorCoolantPumps are the per-loop pumps, one per primary
	// loop, started sequentially during heatup
	// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37).
	ReactorCoolantPumps ReactorCoolantPumps

	// ReactorCoolantPump is the lumped hydraulic view of the pump
	// group: Running mirrors AnyRunning and Velocity the loop-
	// average shaft speed. Plant.Step is its single writer each
	// step; the flow graph, SVG overlay, and legacy aggregate bridge
	// paths read it. Operator commands must target the group (the
	// aggregate control fans out), never this view.
	ReactorCoolantPump Pump

	Pressurizer           `json:"Pressurizer"`
	SafetyValve           Valve
	BlockValve            Valve
	PORV                  Valve
	PressurizedReliefTank Tank

	// PRTRuptureDiscBurst latches once the PRT fills to capacity:
	// "A rupture disc vents the tank to the containment if design
	// pressure is exceeded." After it bursts, further relief
	// discharge raises ContainmentPressure instead of tank level.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:37-38
	PRTRuptureDiscBurst bool

	// PRTSprayValve meters cold water into the PRT: "Cold water
	// can be sprayed into the pressurizer relief tank to increase
	// the heat sink capacity."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:35-37
	PRTSprayValve Valve

	// Aux points at the canonical AuxiliaryBuilding that lives on
	// Plant. The Step function reads CVCS / RHRS / CCWS / CSS /
	// RCFC / EFWS through this pointer so its single-step writes
	// stay consistent with Plant.Step's cross-building wiring.
	Aux *AuxiliaryBuilding

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

	// SecondaryImbalanceFiltered is the first-order filtered signed
	// imbalance between steam-generator enthalpy draw and supplied
	// heat (Watts). Only its positive side drives primary cooling:
	// a sustained deficit means the SG is paying enthalpy out of
	// primary stored heat, while one-step telemetry lag and control
	// noise average out instead of rectifying into phantom cooldown.
	// Time constant SecondaryImbalanceTau (assumption; the source is
	// silent about bookkeeping dynamics).
	SecondaryImbalanceFiltered Watts

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

	// EFWSFeedwaterFlow is the total EFWS feedwater flow rate
	// delivered to the steam generators (gal/s). Populated by
	// the EFWS step in Plant.Step (EFWS lives on the
	// auxiliary building); consumed by the SG feedwater model
	// inside the containment.
	EFWSFeedwaterFlow Volume

	// ContainmentSpraySystem and the rest of the safeguard
	// subsystems (CCWS, RHRS, CSS, RCFC, EFWS, CVCS) live on
	// the auxiliary building; see core/auxiliary.go.

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

	// ServiceWaterSystem lives on Outside — see core/outside.go.
	// The pumps are in a separate intake house per
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:48-50.

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

	// PlantComputer models the data collection, processing,
	// display formatting, and malfunction detection system
	// (plans/p29_1.txt). The simplified model treats the
	// computer as a passive aggregator that derives
	// availability flags from electrical power and major
	// safety-system status.
	//
	// Spec justification:
	//   - Computer mission: "collects data from many
	//     locations within the plant and then processes it
	//     to provide a variety of information."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
	//   - Display formats: mimic, bar chart, tabular,
	//     graphical, data trends, polar graphical.
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:104-202
	//   - SPDS: succinct display of overall plant safety
	//     status.
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	PlantComputer PlantComputer

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
	Turbine   `json:"Turbine"`
	Generator PowerGenerator

	// SteamOfftakeFraction is the steam mass drawn from the
	// main-steam headers during the most recent StepWithSteam,
	// expressed as a fraction of rated full-load steam flow:
	// turbine admission plus condenser steam dump. Plant.Step passes
	// it into SteamGenerator.Step so the draw shapes header pressure;
	// condenser-directed offtake is internal recirculation and does
	// not debit shell inventory. One-step lag, same style as
	// CondensingRate.
	SteamOfftakeFraction float64

	Condenser        `json:"Condenser"`
	MakeupValve      Valve
	RejectValve      Valve
	RecirculateValve Valve

	// Hotwell	Tank (capacity owner: Condenser.creditHotwell,
	// plans/p64_1.txt)

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

	// CSTForRecirc points at Outside.CondensateStorageTank.Level
	// so the RecirculateValve ledger in StepWithSteam can credit
	// the CST (plans/p59_1.txt §3). Wired by Plant.Step before
	// each StepWithSteam call; nil on a standalone building step.
	// json:"-" so a round-trip never revives a detached pointer
	// (CHECK_SKILL.md §1: a pointer re-pointed per tick
	// deserializes as a detached duplicate; nil re-wires on the
	// next Plant.Step).
	CSTForRecirc *Volume `json:"-"`

	// FeedwaterRegulator is the three-element feedwater level
	// regulator. The simplified model collapses the spec's
	// per-SG regulators (A and B) into a single shared
	// regulator (plans/p22_1.txt).
	// "Feedwater flow to each steam generator is controlled by
	// a feedwater regulator in each feedwater line. The
	// regulator is controlled by steam generator level, steam
	// flow and feedwater flow."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
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

	// GeneratorExcitationAvailable is true when the turbine
	// shaft is turning and the brushless exciter is producing
	// field current (plans/p30_1.txt). Mirrors
	// Generator.ExciterActive for plant-level consumers.
	// "Excitation: Shaft-driven, air-cooled brushless exciter
	// of the rotating rectifier type."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
	GeneratorExcitationAvailable bool

	// GeneratorSCCWAlarm is true when stator coil cooling
	// water flow has fallen below GeneratorSCCWPumpMinFlow
	// while the pump is running (plans/p30_1.txt). Latches
	// the SCCW-loss alarm before the trip threshold.
	GeneratorSCCWAlarm bool

	// GeneratorSCCWTrip is true when SCCW flow has been
	// lost completely; the generator breaker is opened as
	// the protective action (plans/p30_1.txt).
	GeneratorSCCWTrip bool

	/*
	   MainFeedRegValve Valve
	   MainFeedIsolationValve Valve
	   MainFeedBypassValve Valve
	   MainFeedBypassIsolationValve Valve
	*/
}

// CoolingTower models a mechanical-draft cooling tower as an
// alternative CW source. The simplified model tracks fan state, basin
// inventory (evaporation + blowdown out, makeup in), and a bulk
// cold-basin temperature that relaxes toward wet-bulb + approach with
// fans running and soaks toward the hot return water without them. It
// does not model fill hydraulics, fan curves, or drift as a separate
// stream (drift is folded into the evaporation fraction).
//
// Spec justification:
//   - CWS source may be "a river, lake, ocean, or cooling
//     tower."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
//   - "The cooled water falls into the cooling tower basin [...]
//     The cooling tower basin has a capacity of 5.2 x 106 gallons."
//     specs/westinghouse_tech_manual/chapter_14/page_1044.txt:37-39
type CoolingTower struct {
	Part

	// FanRunning is true when at least one cooling-tower fan
	// is operating.
	FanRunning bool

	// ApproachF is this tower's design approach: the steady-
	// state difference between the cold-basin temperature and
	// the entering-air wet-bulb while the fans run (°F). It is
	// a per-tower parameter, not global state: fill condition
	// sets how close evaporative cooling gets to the wet-bulb
	// limit, so a fouled or undersized tower carries a larger
	// approach.
	// "The approach is the difference in temperature between
	// the cooled-water temperature and the entering-air wet
	// bulb temperature."
	// specs/wiki/Cooling_tower.txt:1421
	// Zero falls back to the CoolingTowerApproachF design
	// default so raw structs stay well-defined.
	ApproachF float64

	// Basin holds the cold-water inventory the CW pumps draw
	// from.
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:37-39
	Basin Tank

	// ColdBasinTemp is the temperature of the water the CW
	// pumps return to the condenser. It relaxes toward
	// AmbientWetBulbTemp + ApproachF with fans running and
	// toward HotInletTemp without them.
	ColdBasinTemp Temperature

	// HotInletTemp is the condenser CW outlet temperature
	// returning to the tower (°F).
	HotInletTemp Temperature

	// EvaporationFlow is the current evaporation loss
	// (gal/s), proportional to circulating flow times the
	// range (hot minus cold). Drift is folded into this
	// fraction.
	EvaporationFlow Volume

	// MakeupValve meters makeup supply into the basin;
	// BlowdownValve bleeds a continuous fraction of
	// circulating flow.
	// "Due to system impurities, a continuous blowdown is
	// performed on the circulating water system. This
	// inventory loss, combined with losses due to evaporation
	// and drift, requires two cooling tower makeup pumps [...]"
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:12-17
	MakeupValve   Valve
	BlowdownValve Valve
}
