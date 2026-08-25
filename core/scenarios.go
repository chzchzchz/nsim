package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/chzchzchz/nsim/bridge"
)

// Scenario describes one operator evolution from the simulator
// training catalog (plans/x/scenarios.txt). StartState and Objective
// are operator-facing text for the web selection screen; Apply layers
// the minimal state override onto a fresh MakePlant() so each
// evolution opens on the lineup its procedure assumes.
//
// The overrides reuse the start-state fixtures proven by the
// core/scenarioN_test.go replay tests rather than inventing new
// initial conditions.
type Scenario struct {
	ID       string
	Number   int
	Category string
	Title    string

	// StartState summarizes the lineup the plant opens in.
	StartState string
	// Objective tells the operator what the evolution asks of them.
	Objective string

	Apply func(*Plant)

	// Script schedules control commands relative to simulation start
	// so an evolution can open on its healthy lineup and deliver the
	// trigger transient seconds in, instead of pre-inserting the
	// fault at time zero. Keys must be registered control paths.
	Script []ScriptCommand

	// Milestones are observed-condition markers an evolution may
	// require before its bounds count as met: pure end-state checks
	// cannot see a maneuver that returned to its starting value, so
	// scenarios whose objective is the maneuver itself name the
	// lever here. Evaluated by RunProgress, not Check.
	Milestones []Milestone
}

// Milestone is one such marker: Observed reports whether the plant
// shows the lever was exercised at least once.
type Milestone struct {
	Desc     string
	Observed func(*Plant) bool
}

// ScriptCommand is one scheduled operator action: the named control
// takes Value once AtMs of simulated time have elapsed.
type ScriptCommand struct {
	AtMs       int64
	ControlKey string
	Value      string
}

// Cold-condition reference points shared by the shutdown-state
// lineups. The 140 F loop temperature and day-old decay pool follow
// the pre-critical fixture (~0.4 percent of rated about a day after
// shutdown, specs/wiki/Decay_heat.txt); several-hours-old pools pace
// the hot-lineup fixtures at 44 MW slow / 2 MW fast.
const (
	scenarioColdLoopF    = 140.0
	scenarioWarmLoopF    = 220.0
	scenarioColdPsia     = 415.0
	scenarioDayOldDecayW = 0.004 * float64(CoreRatedThermalPower)
	// Several-hours-old decay pools pace the hot-lineup fixtures
	// just under the 50 F/hr limit (specs/wiki/Decay_heat.txt).
	scenarioHoursOldDecaySlowW = 44e6
	scenarioHoursOldDecayFastW = 2e6
)

// coldShutdownLineup builds the water-solid cold-shutdown plant of
// the pre-critical lineup: subcritical core on inserted rods holding
// neutron-source power, loops at 140 F and 415 psia.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-51
func coldShutdownLineup() Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: scenarioColdLoopF, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: scenarioColdLoopF, Unit: u.Fahrenheit}
	rpv.Pressure = scenarioColdPsia
	rpv.Insertion = 1.0
	rpv.DemandInsertion = 1.0
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = Watts(scenarioDayOldDecayW)

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Water = Volume{Val: pzr.Volume.Float(), Unit: u.Gallon}
	pzr.SteamBubble = Volume{}
	pzr.Temperature = Temperature{Val: scenarioColdLoopF, Unit: u.Fahrenheit}

	// The cold depressurized lineup sits below the automatic SI
	// setpoints, so its initiation signals stay blocked until the
	// heatup arms them again.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	if c, ok := GetControlMap(&p)["ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock"]; ok {
		_ = c.Set("true")
	}
	return p
}

// coldWaterSolidLineup builds the depressurized 135 F solid plant
// held by CVCS charging, ready for pressurizer bubble formation:
// tripped core with no nuclear heat, the heaters are the only
// pressing influence until the operator arms them.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:11-33
func coldWaterSolidLineup() Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Trip = true
	rpv.Inlet = Temperature{Val: 135, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 135, Unit: u.Fahrenheit}
	rpv.Insertion = 1
	rpv.DemandInsertion = 1
	rpv.FissionPower = 0
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Water = Volume{Val: 1500, Unit: u.Gallon}
	pzr.SteamBubble = Volume{}
	pzr.Pressure = PressurizerBasePressure
	pzr.HeatersOn = false

	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.AutoLevelControl = false
	cvcs.ChargingPumpRunning = true
	cvcs.LetdownValveOpen = false
	return p
}

// rhrsCooldownLineup builds the cold plant riding RHRS cooling at
// 138 F and 1600 psia with several-hours-old decay pools, poised for
// the controlled heatup after RHRS handback.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37
func rhrsCooldownLineup() Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: 138, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 138, Unit: u.Fahrenheit}
	rpv.Pressure = 1600
	rpv.Trip = true
	rpv.ControlRods.Insertion = 1
	rpv.ControlRods.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatSlow = Watts(scenarioHoursOldDecaySlowW)
	rpv.DecayHeatFast = Watts(scenarioHoursOldDecayFastW)

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Water = Volume{Val: 1460, Unit: u.Gallon}
	pzr.Pressure = 1600
	pzr.HeatersOn = false

	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.AutoLevelControl = false
	cvcs.ChargingPumpRunning = false
	cvcs.LetdownValveOpen = false

	rhr := &p.AuxiliaryBuilding.ResidualHeatRemovalSystem
	rhr.Running = true
	rhr.HasTakenOver = true
	for _, tr := range []*RHRSTrain{&rhr.TrainA, &rhr.TrainB} {
		tr.WarmupComplete = true
		tr.SuctionValves[0].Open = 1
		tr.SuctionValves[1].Open = 1
	}
	rhr.TrainA.Pump.Running = true
	return p
}

// hotZeroPowerLineup builds the hot-standby plant: subcritical core
// on inserted rods at the no-load temperature with rated-equilibrium
// xenon, ready for dilution to criticality or a cooldown.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26
func hotZeroPowerLineup() Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: CoreReferenceInletF}
	rpv.Outlet = Temperature{Val: CoreReferenceInletF}
	rpv.Insertion = 1
	rpv.DemandInsertion = 1
	rpv.BoronConcentration = CVCSInitialBoron
	// Rated-equilibrium poison inventories: the folded reactivity
	// reference reads a fresh core as light without them.
	rpv.Iodine135 = 1
	rpv.Xenon135 = xenonEquilibriumPhysical(1)
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatSlow = Watts(scenarioHoursOldDecaySlowW)
	rpv.DecayHeatFast = Watts(scenarioHoursOldDecayFastW)
	// Hot standby keeps the main turbine secured: an open stop valve
	// on a pressurized header free-admits steam and drags the SG
	// header below the low-steamline setpoints within a second.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:20-23
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.PlantOperation.Mode = HotStandby
	return p
}

// midHeatupLineup builds the plant partway up the Table 16-1 heatup
// ladder: steam bubble established holding roughly 400 psig with the
// loops warm, before the Technical Specification gates that arm above
// 200 F and 350 F. CVCS sits idle so the bubble holds still until the
// operator acts.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:101-153
func midHeatupLineup() Plant {
	p := coldWaterSolidLineup()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: scenarioWarmLoopF, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: scenarioWarmLoopF, Unit: u.Fahrenheit}

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Water = Volume{Val: 900, Unit: u.Gallon}
	pzr.SteamBubble = Volume{Val: 600, Unit: u.Gallon}
	pzr.Pressure = scenarioColdPsia
	pzr.HeatersOn = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	return p
}

// fullPowerLineup builds the coherent at-power lineup the maneuvering
// and malfunction evolutions initialize from: reactor at rated with
// seeded fission-product inventories, reactor coolant pumps running,
// turbine at rated speed, generator synchronized on the grid.
func fullPowerLineup() Plant {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		CoreRatedThermalPower)
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	turb := &p.TurbineBuilding.Turbine
	turb.State = TurbineRunning
	turb.Velocity = TurbineRatedSpeed
	turb.Setpoint = 1.0
	gen := &p.TurbineBuilding.Generator
	gen.BreakerClosed = true
	gen.Voltage = GridRatedVoltage
	gen.Frequency = GridRatedFrequency
	p.PlantOperation.Mode = PowerOperation
	return p
}

// postTripLineup builds the stabilized post-trip plant: rods fully
// inserted, turbine tripped and breaker open, core folded to the
// neutron source on hours-old decay pools while the steam dump holds
// no-load Tavg.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:62-101
func postTripLineup() Plant {
	p := fullPowerLineup()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Trip = true
	rpv.ControlRods.Insertion = 1
	rpv.ControlRods.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatSlow = Watts(scenarioHoursOldDecaySlowW)
	rpv.DecayHeatFast = Watts(scenarioHoursOldDecayFastW)
	rps := &p.ReactorBuilding.ReactorProtectionSystem
	rps.ManualTrip = true
	rps.ReactorTripped = true
	turb := &p.TurbineBuilding.Turbine
	turb.State = TurbineTripped
	turb.StopValve.Open = 0
	turb.Setpoint = 0
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.PlantOperation.Mode = HotStandby
	return p
}

// Scenarios returns the operator-evolution catalog in catalog order
// (plans/x/scenarios.txt items 1-29).
func Scenarios() []Scenario { return scenarioCatalog }

// ScenarioByID resolves a catalog entry by its stable identifier.
func ScenarioByID(id string) (Scenario, bool) {
	for _, s := range scenarioCatalog {
		if s.ID == id {
			return s, true
		}
	}
	return Scenario{}, false
}

var scenarioCatalog = []Scenario{
	{
		ID:       "precritical-lineup",
		Number:   1,
		Category: "Startup",
		Title:    "Pre-critical lineup and checks",
		StartState: "Water-solid cold shutdown: loops at 140 F and " +
			"415 psia, rods inserted, SI blocked, neutron source only.",
		Objective: "Verify electrical power and instrumentation, line " +
			"in fluid systems, run CVCS at RCS boron concentration with " +
			"the volume control tank in its normal band, and place " +
			"component cooling water in service.",
		Apply: func(p *Plant) { *p = coldShutdownLineup() },
	},
	{
		ID:       "rcs-pressurization",
		Number:   2,
		Category: "Startup",
		Title:    "RCS pressurization and steam bubble formation",
		StartState: "Solid plant at 135 F held by charging flow with " +
			"pressurizer heaters off.",
		Objective: "Hold about 400 psig with charging/letdown control " +
			"while the heaters bring pressurizer water to saturation, " +
			"then raise letdown to drain the pressurizer to the 20-80 " +
			"percent no-load level without losing pressure.",
		Apply: func(p *Plant) { *p = coldWaterSolidLineup() },
	},
	{
		ID:       "primary-heatup",
		Number:   3,
		Category: "Startup",
		Title:    "Controlled primary heatup",
		StartState: "138 F at 1600 psia on residual heat removal " +
			"cooling, decay heat several hours old.",
		Objective: "Secure RHRS, start reactor coolant pumps " +
			"sequentially, and ride pump heat up to no-load Tavg 557 F " +
			"at 2250 psia while keeping the heatup within 50 F/hr using " +
			"heaters and spray.",
		Apply: func(p *Plant) { *p = rhrsCooldownLineup() },
	},
	{
		ID:       "secondary-spinup",
		Number:   4,
		Category: "Startup",
		Title:    "Secondary side spin-up",
		StartState: "Hot primary at the no-load temperature on " +
			"inserted rods with day-old decay heat; condenser at " +
			"atmospheric with no vacuum, stop valve shut.",
		Objective: "Draw steam from the steam generators to warm the " +
			"secondary plant, pull condenser vacuum with the vacuum " +
			"pump, and roll the turbine on its speed demand.",
		Apply: func(p *Plant) {
			q := MakePlant()
			rpv := &q.ReactorBuilding.ReactorPressureVessel
			// The spin-up evolution runs above 400 F coolant during
			// the heatup, so park the loops on the no-load anchor
			// instead of inheriting full-load temperatures.
			// specs/the_westinghouse_pressurized_water_react/
			// chapter_16/page_224.txt:171-177
			rpv.Inlet = Temperature{Val: scenarioNoLoadTavgF, Unit: u.Fahrenheit}
			rpv.Outlet = Temperature{Val: scenarioNoLoadTavgF, Unit: u.Fahrenheit}
			rpv.Insertion = 1
			rpv.DemandInsertion = 1
			rpv.FissionPower = CoreNeutronSourcePower
			rpv.DecayHeatFast = 0
			rpv.DecayHeatSlow = Watts(scenarioDayOldDecayW)
			q.TurbineBuilding.Condenser.VacuumPumpRunning = false
			q.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
			q.TurbineBuilding.Turbine.StopValve.Open = 0
			q.TurbineBuilding.Turbine.SpeedDemand = 0
			*p = q
		},
	},
	{
		ID:       "dilution-to-criticality",
		Number:   5,
		Category: "Startup",
		Title:    "Dilution to criticality",
		StartState: "Hot zero power at no-load conditions: rods fully " +
			"inserted, reference boron, rated-equilibrium xenon.",
		Objective: "Withdraw the shutdown banks, compute the estimated " +
			"critical boron concentration, dilute to it, then manually " +
			"withdraw control banks with overlap while watching source " +
			"range, never exceeding 1.0 decades per minute without rod " +
			"motion.",
		Apply: func(p *Plant) { *p = hotZeroPowerLineup() },
	},
	{
		ID:       "low-power-ascension",
		Number:   6,
		Category: "Startup",
		Title:    "Low-power ascension",
		StartState: "Hot zero power on inserted rods at reference " +
			"boron with auxiliary feedwater supplying the steam " +
			"generators.",
		Objective: "Withdraw banks (or dilute) to criticality, raise " +
			"power through the point of adding heat watching Tavg and " +
			"the steam dumps, then switch the steam generators from " +
			"auxiliary to main feedwater below about 3.5 percent steam " +
			"flow and secure AFW.",
		Apply: func(p *Plant) {
			// The 2-percent seed used previously was not a held
			// equilibrium: stepping drove power to the rated cap. Hot
			// zero power is coherent and makes the ascension itself
			// the operator's work.
			q := hotZeroPowerLineup()
			q.PlantOperation.AuxFeedwaterInUse = true
			*p = q
		},
	},
	{
		ID:       "grid-synchronization",
		Number:   7,
		Category: "Startup",
		Title:    "Grid synchronization and loading",
		StartState: "Critical at about 8 percent power, condenser " +
			"vacuum established, rotor stopped behind a shut stop valve.",
		Objective: "Roll the turbine to synchronous speed, parallel " +
			"the generator on the grid, ramp the steam dump shut as " +
			"loading commences, and escalate load at no more than 5 " +
			"percent of full power per minute.",
		Apply: func(p *Plant) {
			q := MakePlant()
			q.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
				Watts(0.08 * float64(CoreRatedThermalPower)))
			q.TurbineBuilding.Condenser.VacuumPumpRunning = true
			q.TurbineBuilding.Condenser.Pressure = 2
			q.Outside.CirculatingWaterPump.Running = true
			q.TurbineBuilding.Turbine.StopValve.Open = 0
			q.TurbineBuilding.Turbine.SpeedDemand = 0
			q.PlantOperation.Mode = HotStandby
			*p = q
		},
	},
	{
		ID:       "load-follow",
		Number:   8,
		Category: "Power operation",
		Title:    "Daily load follow",
		StartState: "At-power lineup: reactor at rated, all reactor " +
			"coolant pumps running, generator synchronized loaded.",
		Objective: "Step turbine demand through a load-follow cycle " +
			"and let rod control hold the Tavg program; trim boron " +
			"whenever rods approach their insertion limits or the " +
			"axial flux difference band so full reserve is preserved " +
			"for the return to power.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Milestones: []Milestone{{
			Desc: "turbine demand moved off rated",
			Observed: func(p *Plant) bool {
				return p.TurbineBuilding.Turbine.Setpoint <
					1-LoadRejectDetectFraction
			},
		}},
	},
	{
		ID:       "axial-xenon-management",
		Number:   9,
		Category: "Power operation",
		Title:    "Axial power distribution and xenon management",
		StartState: "At-power lineup at rated with equilibrium " +
			"fission-product inventories.",
		Objective: "Maneuver the reactor with boron changes that " +
			"reposition rods, keeping axial flux difference inside its " +
			"target band and avoiding xenon-induced oscillations.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
	},
	{
		ID:       "turbine-unload-hot-standby",
		Number:   10,
		Category: "Shutdown",
		Title:    "Turbine unload and hot standby",
		StartState: "At-power lineup: generator synchronized at rated " +
			"output.",
		Objective: "Unload the turbine-generator below house loads, " +
			"transfer controls to manual, secure the machine, insert " +
			"rods for subcritical hot standby, and manage boron for " +
			"xenon transients if you hold.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
	},
	{
		ID:       "boration-shutdown-margin",
		Number:   11,
		Category: "Shutdown",
		Title:    "Boration to cold shutdown margin",
		StartState: "Hot standby: subcritical core on inserted rods " +
			"at no-load temperature and pressure.",
		Objective: "Initiate boration toward the cold shutdown boron " +
			"concentration and sample periodically to verify shutdown " +
			"margin before starting any cooldown.",
		Apply: func(p *Plant) { *p = hotZeroPowerLineup() },
	},
	{
		ID:       "coordinated-cooldown",
		Number:   12,
		Category: "Shutdown",
		Title:    "Coordinated cooldown and depressurization",
		StartState: "Hot standby: subcritical core at no-load " +
			"conditions with the steam generators available for " +
			"cooldown.",
		Objective: "Raise pressurizer level for the contraction, cool " +
			"down at 50 F/hr on the steam dump, control pressure " +
			"manually, block SI and isolate accumulators at the right " +
			"steps, and place RHRS in service below 350 F and 400 psig.",
		Apply: func(p *Plant) { *p = hotZeroPowerLineup() },
	},
	{
		ID:       "load-maneuver-reactor-lead",
		Number:   13,
		Category: "Transient recovery",
		Title:    "Load maneuvering, reactor lead",
		StartState: "At-power lineup at rated with the generator " +
			"synchronized.",
		Objective: "Take a step load reduction with the reactor power " +
			"setpoint leading turbine demand, observe Tavg/Tref, power " +
			"error, and rod response, then return to full power.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
	},
	{
		ID:       "power-reduction-zero",
		Number:   14,
		Category: "Transient recovery",
		Title:    "Power reduction to zero",
		StartState: "At-power lineup at rated with CVCS automatic " +
			"pressurizer level control in service.",
		Objective: "Ramp the demanded power setpoint orderly to zero " +
			"while holding steam generator levels and the Tavg program " +
			"without tripping.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = true
			*p = q
		},
	},
	{
		ID:       "turbine-trip-recovery",
		Number:   15,
		Category: "Transient recovery",
		Title:    "Turbine trip and recovery",
		StartState: "Post-trip park: turbine tripped and breaker " +
			"open, core folded to the stepback level with the steam " +
			"dump carrying the header.",
		Objective: "Recover from the turbine trip: reset the trip, " +
			"restore rods and reparallel at synchronous speed, then " +
			"reload toward full power.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
				Watts(StepbackPowerFraction * float64(CoreRatedThermalPower)))
			turb := &q.TurbineBuilding.Turbine
			turb.State = TurbineTripped
			turb.StopValve.Open = 0
			turb.SpeedDemand = 0
			turb.Setpoint = 0
			q.TurbineBuilding.Generator.BreakerClosed = false
			q.PlantOperation.StepbackActive = true
			*p = q
		},
	},
	{
		ID:       "reactor-trip-recovery",
		Number:   16,
		Category: "Transient recovery",
		Title:    "Reactor trip and recovery",
		StartState: "Stabilized post-trip: rods inserted, turbine " +
			"tripped, steam dump restoring no-load Tavg.",
		Objective: "Confirm the stabilization, then perform the hot " +
			"startup return: clear the trip, withdraw rods back to " +
			"criticality, and escalate to power per the startup " +
			"sequence.",
		Apply: func(p *Plant) { *p = postTripLineup() },
	},
	{
		ID:       "load-rejection",
		Number:   17,
		Category: "Emergency response",
		Title:    "Full load rejection without trip",
		StartState: "At-power lineup: generator synchronized at full " +
			"load; grid demand steps down to house loads 30 seconds " +
			"in.",
		Objective: "Hold the plant through the rejection without a " +
			"trip: the steam dump absorbs the surplus steam while the " +
			"machine carries only its own auxiliaries.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "TurbineBuilding/Turbine/Setpoint",
				Value:      "0.05"},
		},
	},
	{
		ID:       "trip-response-supervision",
		Number:   18,
		Category: "Emergency response",
		Title:    "Reactor trip response supervision",
		StartState: "Stabilized post-trip: house loads on the station " +
			"transformer, steam dump restoring no-load Tavg.",
		Objective: "Supervise the post-trip response: confirm trip " +
			"breakers open and all rods fully inserted, verify the " +
			"steam dump restoring Tavg, pressurizer heaters recovering " +
			"pressure, and charging pumps recovering level.",
		Apply: func(p *Plant) { *p = postTripLineup() },
	},
	{
		ID:       "loss-of-offsite-power",
		Number:   19,
		Category: "Emergency response",
		Title:    "Loss of offsite power",
		StartState: "At-power lineup drawing both offsite sources; " +
			"the unit comes off the line and the offsite breakers " +
			"open 30 seconds in.",
		Objective: "Verify the emergency diesels auto-start and the " +
			"safety buses sequence onto them so the plant can safely " +
			"cool down.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		// The diesel trigger is the sensed bus state: with the unit
		// still feeding its own buses the engines correctly stay idle
		// (core/plant.go), so the script must strip every supply.
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "Electrical/OffsiteSourceA/BreakerClosed",
				Value:      "0"},
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "Electrical/OffsiteSourceB/BreakerClosed",
				Value:      "0"},
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "TurbineBuilding/Turbine/StopValve",
				Value:      "0"},
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "TurbineBuilding/Generator/BreakerClosed",
				Value:      "0"},
		},
	},
	{
		ID:       "techspec-gating",
		Number:   20,
		Category: "Mode-change gates",
		Title:    "Technical Specification gating",
		StartState: "Mid-heatup: bubble formed holding about 400 " +
			"psig with loops near 220 F, RHRS secured.",
		Objective: "Before proceeding, verify each gate: containment " +
			"integrity and AFW operable above 200 F, ECCS available " +
			"above 350 F, SG pressure over 585 psig before raising RCS " +
			"above 1900 psig, and SI blocking or accumulator isolation " +
			"only during planned cooldowns.",
		Apply: func(p *Plant) { *p = midHeatupLineup() },
	},
	{
		ID:       "xenon-burnout-return",
		Number:   21,
		Category: "Late fuel cycle",
		Title:    "Return to full power after xenon burnout",
		StartState: "Late-cycle chemistry (diluted shim) parked at 90 " +
			"percent power with rods alone holding the program.",
		Objective: "Return from reduced power to full power on rods " +
			"alone with no boron adjustment; the ascent is paced by " +
			"xenon burnout, typically 2-4 hours, trading reserve " +
			"capability for zero coolant effluent.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			const lateCycleBoron = 500.0
			q.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = lateCycleBoron
			q.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = lateCycleBoron
			q.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
				Watts(0.90 * float64(CoreRatedThermalPower)))
			q.PlantOperation.DemandedPowerSetpoint = 0.90
			q.ReactorBuilding.ReactorPressureVessel.Outlet = Temperature{
				Val: q.PlantOperation.referenceTempF(), Unit: u.Fahrenheit}
			*p = q
		},
	},
	{
		ID:       "refueling-outage",
		Number:   22,
		Category: "Refueling outage",
		Title:    "Refueling outage",
		StartState: "Cold shutdown at 140 F with the refueling cycle " +
			"reset to preparation and cavity flooding started.",
		Objective: "Work the five-phase outage: flood the cavity and " +
			"survey containment, disassemble the reactor head, shuffle " +
			"fuel underwater, reassemble and re-tension, then run " +
			"preoperational physics tests before the next startup.",
		Apply: func(p *Plant) {
			q := coldShutdownLineup()
			refuel := &q.ReactorBuilding.RefuelingOperations
			refuel.Phase = RefuelingPhasePreparation
			refuel.CavityFlooding = true
			*p = q
		},
	},
	{
		ID:       "loss-of-feedwater",
		Number:   23,
		Category: "Malfunction transients",
		Title:    "Loss of normal feedwater",
		StartState: "At-power lineup; the main feedwater pump " +
			"trips 30 seconds in.",
		Objective: "Watch steam generator levels fall driving a " +
			"reactor setback then trip; verify emergency feedwater " +
			"delivers within one minute with no operator action, then " +
			"modulate the EFW valves to restore narrow-range levels " +
			"and either resume power or cool down.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "TurbineBuilding/MainFeedwaterPump/Running",
				Value:      "0"},
		},
	},
	{
		ID:       "rcp-trip",
		Number:   24,
		Category: "Malfunction transients",
		Title:    "Reactor coolant pump trip",
		StartState: "At-power lineup; loop A's pump trips 30 seconds " +
			"in.",
		Objective: "Observe the immediate power stepback and the " +
			"decaying loop flow against the low-flow trip; try the " +
			"both-pumps-in-one-loop case to challenge the cold-leg " +
			"temperature margin.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "ReactorBuilding/ReactorCoolantPumps/A/Running",
				Value:      "0"},
		},
	},
	{
		ID:       "steam-line-break",
		Number:   25,
		Category: "Malfunction transients",
		Title:    "Main steam line break",
		StartState: "At-power lineup; a full break opens upstream of " +
			"the main steam stop valve 30 seconds in.",
		Objective: "Manage the excessive-cooldown transient: steam " +
			"demand overcools the RCS while the turbine runs back, " +
			"verify low compensated steamline pressure actuates safety " +
			"injection, and stabilize the plant afterward.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "ReactorBuilding/SteamGenerator/MainSteamLineBreak/Open",
				Value:      "1"},
		},
	},
	{
		ID:       "porv-stuck-open",
		Number:   26,
		Category: "Malfunction transients",
		Title:    "PORV fails open",
		StartState: "At-power lineup; the pressurizer relief valve " +
			"latches open 30 seconds in.",
		Objective: "Diagnose the stuck-open PORV venting to the " +
			"relief tank: heaters cannot hold pressure while level " +
			"rises. Isolate with the block valve before low pressure " +
			"trips the reactor, then clear the failure latch.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "ReactorBuilding/Pressurizer/PORVStuckOpen",
				Value:      "1"},
		},
	},
	{
		ID:       "dropped-rod-bank",
		Number:   27,
		Category: "Malfunction transients",
		Title:    "Dropped rod bank",
		StartState: "At-power lineup; one dark bank drops into the " +
			"core 30 seconds in.",
		Objective: "Compensate the dropped bank: watch the control " +
			"system exhaust gray and dark rod worth against the tilt, " +
			"ride the turbine runback and trip, and manage the xenon " +
			"buildup that walks power to zero.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "ReactorBuilding/ReactorPressureVessel/ControlRods/DroppedBank",
				Value:      "1"},
		},
	},
	{
		ID:       "si-recirculation",
		Number:   28,
		Category: "Engineered safeguards",
		Title:    "Safety injection and recirculation",
		StartState: "At-power lineup; manual safety injection " +
			"latches 30 seconds in.",
		Objective: "Supervise the injection phase: diesels, HHSI, " +
			"accumulators discharging as pressure falls, then switch " +
			"over to cold-leg recirculation at RWST low level and later " +
			"balance hot-leg injection for long-term cooling.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
		Script: []ScriptCommand{
			{AtMs: scenarioTriggerDelayMs,
				ControlKey: "ReactorBuilding/ReactorProtectionSystem/ManualSI",
				Value:      "1"},
		},
	},
	{
		ID:       "containment-spray",
		Number:   29,
		Category: "Engineered safeguards",
		Title:    "Containment spray and recirculation switchover",
		StartState: "At-power lineup with the containment spray " +
			"actuation signal latched at time zero.",
		Objective: "Confirm spray starts and the additive tank valves " +
			"open, then at the spray storage tank Lo-Lo setpoint switch " +
			"the pumps to containment sump recirculation without " +
			"additive and shift fan coolers to the post-LOCA path.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.ReactorProtectionSystem.ContainmentSprayActuated = true
			*p = q
		},
	},
	{
		ID:       "btrs-boron-cycling",
		Number:   31,
		Category: "Auxiliary system operations",
		Title:    "BTRS dilution and boration cycling",
		StartState: "At-power lineup with automatic charging/letdown " +
			"level control, the volume control tank mid-band, and the " +
			"BTRS isolated.",
		Objective: "Cycle boron without liquid effluent: dilute on the " +
			"cold regeneration beds to the target concentration, recirculate " +
			"with the loop isolated to recondition the beds hot, then " +
			"borate back to the starting rated-critical value through the " +
			"setpoint drive while watching VCT level and bed saturation.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			cvcs := &q.AuxiliaryBuilding.ChemicalVolumeControlSystem
			cvcs.AutoLevelControl = true
			cvcs.ChargingPumpRunning = true
			cvcs.LetdownValveOpen = true
			cvcs.VCTLevel = Volume{Val: 520, Unit: u.Gallon}
			q.ReactorBuilding.ReactorMakeupControlValve.Auto = true
			*p = q
		},
		// Procedure-timed offsets from plans/x/scenario/31.txt: each
		// leg moves 25 ppm at CVCSBorationRate (25 ppm / 0.8 ppm/s =
		// 31.25 s), behind a core warmup window; between legs the pump
		// stays lined up so the isolation interval reconditions the
		// beds toward boration temperature before the setpoint write.
		Script: []ScriptCommand{
			{AtMs: 15000,
				ControlKey: "AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode",
				Value:      "Dilution"},
			{AtMs: 15000,
				ControlKey: "AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump",
				Value:      "1"},
			{AtMs: 46250,
				ControlKey: "AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode",
				Value:      "VolumeControl"},
			{AtMs: 81250,
				ControlKey: "ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint",
				Value:      strconv.FormatFloat(CoreRatedCriticalBoronPpm, 'f', -1, 64)},
		},
	},
	{
		ID:       "ccw-train-lineup",
		Number:   33,
		Category: "Auxiliary system operations",
		Title:    "Component cooling water train lineup",
		StartState: "Full-power lineup with the lumped CCW loop " +
			"holding its design 105 F inlet, service water " +
			"flowing at design, and the cooling pump lined up " +
			"to auto-start on the energised ESF bus.",
		Objective: "Keep the component cooling barrier alive " +
			"across regimes: confirm the design inlet at " +
			"power, lose the ultimate heat sink and watch " +
			"the inlet soak toward - never past - the 120 F " +
			"cooldown ceiling, then restore service water " +
			"and ride the recovery lag back toward 105 F.",
		Apply: func(p *Plant) {
			*p = fullPowerLineup()
		},
	},
}

// CheckResult reports whether an evolution's end-state bounds hold on
// the live plant. Detail names the bounds still unmet so the web UI
// can show progress instead of a bare pass/fail.
type CheckResult struct {
	Done   bool   `json:"done"`
	Detail string `json:"detail"`
}

// checkBounds accumulates the bound descriptions that fail; the
// evolution is complete when nothing is left unmet.
type checkBounds struct {
	unmet []string
}

func (c *checkBounds) require(ok bool, what string) {
	if !ok {
		c.unmet = append(c.unmet, what)
	}
}

func (c *checkBounds) result() CheckResult {
	if len(c.unmet) == 0 {
		return CheckResult{Done: true, Detail: "objective met"}
	}
	return CheckResult{Detail: strings.Join(c.unmet, "; ")}
}

const (
	// No-load coolant temperature anchor quoted by the heatup and
	// hot-startup procedures ("approximately 547°F",
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:178-179).
	scenarioNoLoadTavgF = 547.0

	// Invented completion bands disclosed per CHECK_SKILL section 8:
	// the sources state directions ("reduce to 0 percent", "return to
	// full power") but no numeric completion tolerance, so these set
	// how close counts.
	scenarioZeroPowerBandFrac   = 0.05
	scenarioFullPowerBandFrac   = 0.95
	scenarioXenonMovedFrac      = 0.02
	scenarioApproachBandF       = 10.0
	scenarioVCTNormalFloorFrac  = 0.75
	scenarioLevelNormalFraction = 0.20 // spec band floor, page_224.txt:22
	scenarioLevelMaxFraction    = 0.80 // spec band ceiling, page_224.txt:22

	// Invented pacing: scripted triggers fire after this much
	// simulated time so the operator sees the healthy lineup first.
	scenarioTriggerDelayMs = 30000

	// House-load demand level the rejection ride-through parks on,
	// matching the scripted trigger write; invented band.
	scenarioHouseLoadSetpointFrac = 0.05
)

// scenarioPowerFraction expresses fission power against rated fission
// power for bound checks.
func scenarioPowerFraction(p *Plant) float64 {
	return float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) /
		float64(CoreFissionPowerAtRated)
}

// houseLoadSATSource marks house loads carried on the station
// auxiliary transformer after a trip or rejection transfers them off
// the unit transformer (core/station.go writes this string).
const houseLoadSATSource = "SAT"

// houseLoadUATSource marks auxiliaries still carried by the unit
// transformer while the generator stays breaker-closed at house load
// (core/station.go writes this string).
const houseLoadUATSource = "UAT"

// ApplyScript fires every scripted command whose offset has been
// reached by nowNs, consuming the pending prefix. Commands target
// registered controls; unmatched names and failed sets are dropped so
// a broken script degrades the exercise without stopping the plant.
func ApplyScript(
	script *[]ScriptCommand,
	nowNs int64,
	controls bridge.ControlMap,
) {
	i := 0
	for ; i < len(*script); i++ {
		cmd := (*script)[i]
		if cmd.AtMs*int64(time.Millisecond) > nowNs {
			break
		}
		if c, ok := controls[cmd.ControlKey]; ok && c.Set != nil {
			_ = c.Set(cmd.Value)
		}
	}
	*script = (*script)[i:]
}

// scenarioChecks pairs every catalog entry with its end-state bounds;
// scenarios_test.go asserts the key sets match the catalog exactly.
// Each bound cites its spec line, or marks an invented tolerance as
// such.
var scenarioChecks = map[string]func(*Plant) CheckResult{
	"precritical-lineup": func(p *Plant) CheckResult {
		var c checkBounds
		cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
		c.require(p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning,
			"component cooling water in service")
		c.require(cvcs.ChargingPumpRunning, "charging pump lined up")
		vct := cvcs.VCTLevel.Float()
		c.require(vct >= VCTDesignLevel*scenarioVCTNormalFloorFrac &&
			vct <= VCTHighLevel, "VCT level inside its normal range")
		return c.result()
	},
	"rcs-pressurization": func(p *Plant) CheckResult {
		var c checkBounds
		pzr := &p.ReactorBuilding.Pressurizer
		c.require(pzr.HeatersOn, "pressurizer heaters energized")
		c.require(pzr.Pressure > PressurizerBasePressure,
			"pressure raised off the cold base")
		bubble := pzr.SteamBubble.Float()
		c.require(bubble > 0, "steam bubble formed")
		fraction := pzr.Water.Float() / pzr.Volume.Float()
		c.require(fraction >= scenarioLevelNormalFraction &&
			fraction <= scenarioLevelMaxFraction,
			"level in the 20-80 percent no-load range")
		return c.result()
	},
	"primary-heatup": func(p *Plant) CheckResult {
		var c checkBounds
		rhr := &p.AuxiliaryBuilding.ResidualHeatRemovalSystem
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		c.require(!rhr.Running && !rhr.HasTakenOver, "RHRS secured")
		c.require(rpv.Outlet.Float() >= scenarioNoLoadTavgF-scenarioApproachBandF,
			"heated to the no-load temperature")
		c.require(rpv.Pressure >= RPSLowPressurizerPressureTrip,
			"pressurized above the low-pressure trip band")
		return c.result()
	},
	"secondary-spinup": func(p *Plant) CheckResult {
		var c checkBounds
		turb := &p.TurbineBuilding.Turbine
		c.require(p.TurbineBuilding.Condenser.Pressure < AtmosphericPressure,
			"condenser vacuum established") // chapter_15/page_216.txt:21-22
		c.require(turb.Velocity > 0 && turb.State == TurbineRunning,
			"turbine rolling at speed")
		return c.result()
	},
	"dilution-to-criticality": func(p *Plant) CheckResult {
		var c checkBounds
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		// Criticality evidence: positive sustained startup rate with
		// rising source-range indication and banks off their stops.
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:155-163
		c.require(rpv.Insertion < 1, "control banks withdrawn")
		c.require(rpv.FissionPower > CoreNeutronSourcePower,
			"power lifted off the source floor")
		c.require(rpv.StartupRateDPM > 0, "sustained positive startup rate")
		return c.result()
	},
	"low-power-ascension": func(p *Plant) CheckResult {
		var c checkBounds
		// Point of adding heat is about 1 percent of power:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:164-167
		c.require(scenarioPowerFraction(p) >= 0.01,
			"power through the point of adding heat")
		// AFW hands over to main feedwater near 3.5 percent steam
		// flow: specs/the_westinghouse_pressurized_water_react/
		// chapter_16/page_226.txt:45-48
		c.require(!p.PlantOperation.AuxFeedwaterInUse,
			"main feedwater supplying the generators")
		c.require(p.TurbineBuilding.MainFeedwaterPump.Running,
			"main feedwater pump running")
		return c.result()
	},
	"grid-synchronization": func(p *Plant) CheckResult {
		var c checkBounds
		gen := &p.TurbineBuilding.Generator
		turb := &p.TurbineBuilding.Turbine
		// Paralleled and loading: specs/the_westinghouse_
		// pressurized_water_react/chapter_16/page_226.txt:52-55
		c.require(gen.BreakerClosed, "generator breaker closed")
		c.require(turb.State == TurbineRunning && turb.Velocity > 0,
			"turbine at synchronous speed")
		c.require(gen.RealPower > 0, "loading commenced")
		return c.result()
	},
	"load-follow": func(p *Plant) CheckResult {
		var c checkBounds
		// Boron-and-rod maneuvering holds full reserve capability for
		// the return to full power:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:24-27
		c.require(p.PlantOperation.Mode == PowerOperation &&
			!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"plant held in power operation without a trip")
		c.require(p.PlantOperation.RodControlAuto, "rod control in automatic")
		c.require(scenarioPowerFraction(p) >= scenarioFullPowerBandFrac,
			"returned to full power")
		return c.result()
	},
	"axial-xenon-management": func(p *Plant) CheckResult {
		var c checkBounds
		// Chemistry adjustments reposition rods while holding power:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:111-116
		c.require(p.PlantOperation.Mode == PowerOperation,
			"plant held in power operation")
		c.require(scenarioPowerFraction(p) >= OpPowerOperationMinFraction,
			"power kept above the operational band floor")
		// The maneuver must have moved xenon off its rated equilibrium
		// (deviation tolerance invented; the source quotes no number).
		xenon := math.Abs(xenonNormalized(
			p.ReactorBuilding.ReactorPressureVessel.Xenon135) - 1)
		c.require(xenon > scenarioXenonMovedFrac,
			"xenon transient exercised by the maneuver")
		return c.result()
	},
	"turbine-unload-hot-standby": func(p *Plant) CheckResult {
		var c checkBounds
		// Subcritical hot standby: rods inserted, machine off the line:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:20-23
		c.require(p.PlantOperation.Mode == HotStandby, "mode in hot standby")
		c.require(p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion >= 1,
			"rods fully inserted")
		c.require(!p.TurbineBuilding.Generator.BreakerClosed,
			"generator off the grid")
		return c.result()
	},
	"boration-shutdown-margin": func(p *Plant) CheckResult {
		var c checkBounds
		// Boration toward the cold shutdown concentration:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:50-52
		c.require(p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration >
			CVCSInitialBoron, "boron concentration raised")
		c.require(p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion >= 1,
			"rods fully inserted")
		c.require(p.ReactorBuilding.ReactorPressureVessel.FissionPower <=
			CoreNeutronSourcePower, "core held subcritical")
		return c.result()
	},
	"coordinated-cooldown": func(p *Plant) CheckResult {
		var c checkBounds
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		// RHRS entry conditions: 350 F and under about 400 psig:
		// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
		c.require(rpv.Outlet.Float() <= RHRSTempThreshold,
			"cooled to the RHRS entry temperature")
		c.require(rpv.Pressure <= scenarioColdPsia,
			"depressurized below 400 psig")
		c.require(p.AuxiliaryBuilding.ResidualHeatRemovalSystem.HasTakenOver,
			"RHRS in service")
		return c.result()
	},
	"load-maneuver-reactor-lead": func(p *Plant) CheckResult {
		var c checkBounds
		// Reactor power setpoint leads and the return leg completes at
		// full power: specs/pwr_sim/chapter_3/page_036.txt:57-70
		c.require(p.PlantOperation.LeadLagMode == ReactorLead,
			"reactor lead mode selected")
		c.require(p.PlantOperation.RodControlAuto, "rod control in automatic")
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"no trip through the maneuver")
		c.require(scenarioPowerFraction(p) >= scenarioFullPowerBandFrac,
			"returned to full power")
		return c.result()
	},
	"power-reduction-zero": func(p *Plant) CheckResult {
		var c checkBounds
		// Orderly ramp toward zero without a trip (completion band is
		// an invented tolerance; the source states the endpoint only):
		// specs/pwr_sim/chapter_3/page_039.txt:23-30
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"reduction completed without a trip")
		c.require(scenarioPowerFraction(p) <= scenarioZeroPowerBandFrac,
			"power reduced to the zero-power band")
		return c.result()
	},
	"turbine-trip-recovery": func(p *Plant) CheckResult {
		var c checkBounds
		// Recovery ends reparalleled and reloading:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:65-66
		turb := &p.TurbineBuilding.Turbine
		c.require(turb.State == TurbineRunning && turb.Velocity > 0,
			"turbine reset and rolling")
		c.require(p.TurbineBuilding.Generator.BreakerClosed,
			"generator reparalleled")
		c.require(!p.PlantOperation.StepbackActive, "stepback cleared")
		c.require(scenarioPowerFraction(p) >= scenarioFullPowerBandFrac,
			"power restored to full")
		return c.result()
	},
	"reactor-trip-recovery": func(p *Plant) CheckResult {
		var c checkBounds
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		// Hot startup from hot standby withdraws banks back toward
		// criticality:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"trip cleared")
		c.require(rpv.ControlRods.Insertion < 1, "banks withdrawn")
		c.require(rpv.FissionPower > CoreNeutronSourcePower,
			"core critical again")
		return c.result()
	},
	"load-rejection": func(p *Plant) CheckResult {
		var c checkBounds
		// Steam dump carries the loss as an artificial load:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:27-31.
		// The simplified core holds near-rated through the ride-through
		// (its fold-to-load feedback is not modeled), so bounds check
		// the house-load lineup instead of a power cutback magnitude;
		// the house-level setpoint cap is an invented band matching the
		// scripted trigger.
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"ride-through completed without a trip")
		c.require(p.TurbineBuilding.Turbine.DumpValve.Open > 0,
			"steam dump carrying the rejected load")
		c.require(p.TurbineBuilding.Turbine.Setpoint <= scenarioHouseLoadSetpointFrac,
			"demand parked at house loads")
		c.require(p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom ==
			houseLoadUATSource, "auxiliaries carried by the unit")
		return c.result()
	},
	"trip-response-supervision": func(p *Plant) CheckResult {
		var c checkBounds
		// Post-trip lineup: breakers open, rods fully in, Tavg being
		// restored below the full-load anchor:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:62-66
		c.require(p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"trip latched")
		c.require(p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion >= 1,
			"rods fully inserted")
		c.require(p.TurbineBuilding.Turbine.State == TurbineTripped,
			"turbine tripped")
		c.require(p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom ==
			houseLoadSATSource, "house loads transferred")
		c.require(p.ReactorBuilding.ReactorPressureVessel.Outlet.Float() <=
			CoreReferenceOutletF, "coolant relaxing toward no-load")
		return c.result()
	},
	"loss-of-offsite-power": func(p *Plant) CheckResult {
		var c checkBounds
		// Diesels auto-start and sequence the safety buses back
		// energized:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-109
		dg := &p.AuxiliaryBuilding.DieselGenerator
		dgb := &p.AuxiliaryBuilding.DieselGeneratorB
		c.require(dg.State == DieselRunning || dgb.State == DieselRunning,
			"an emergency diesel at rated")
		c.require(p.Electrical.ESFBusA.Energized && p.Electrical.ESFBusB.Energized,
			"safety buses energized")
		return c.result()
	},
	"techspec-gating": func(p *Plant) CheckResult {
		var c checkBounds
		// Heatup continued past the ECCS gate temperature with the
		// gated equipment verified available:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:101-105
		efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
		c.require(p.ReactorBuilding.ReactorPressureVessel.Outlet.Float() >
			RHRSTempThreshold, "heatup progressed above 350 F")
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionBlocked,
			"ECCS available (SI unblocked)")
		c.require(efws.SubsystemA.Tank.Level.Float() > 0 &&
			efws.SubsystemB.Tank.Level.Float() > 0, "AFW tanks carrying inventory")
		return c.result()
	},
	"xenon-burnout-return": func(p *Plant) CheckResult {
		var c checkBounds
		// Return paced by xenon burnout ends back at full power:
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:12-15
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"ascent completed without a trip")
		c.require(scenarioPowerFraction(p) >= scenarioFullPowerBandFrac,
			"full power recovered")
		return c.result()
	},
	"refueling-outage": func(p *Plant) CheckResult {
		var c checkBounds
		refuel := &p.ReactorBuilding.RefuelingOperations
		// Five-phase cycle through fuel handling:
		// specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
		c.require(refuel.CavityFlooded, "refueling cavity flooded")
		c.require(refuel.Phase >= RefuelingPhaseReassembly,
			"fuel handling and reassembly complete")
		c.require(refuel.FuelTransferred, "fuel batch exchanged")
		return c.result()
	},
	"loss-of-feedwater": func(p *Plant) CheckResult {
		var c checkBounds
		// EFW delivering within one minute of actuation, then levels
		// hand-restored to the level program:
		// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:4-9
		efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
		sg := &p.ReactorBuilding.SteamGenerator
		c.require(efws.Active, "EFWS actuated")
		c.require(efws.SubsystemA.MotorPump.Running ||
			efws.SubsystemB.MotorPump.Running, "an EFW pump delivering")
		c.require(sg.Water.Float() >= sg.Volume.Float()*
			FeedwaterLevelSetpoint/100, "SG level restored to program")
		return c.result()
	},
	"rcp-trip": func(p *Plant) CheckResult {
		var c checkBounds
		// Loss of one pump immediately initiates the power stepback
		// and may then reach the low-flow scram, so the evolution
		// resolves either folded into the stepback park band or
		// stabilized tripped (malfunction writeup:
		// specs/pwr_sim/chapter_4/page_051.txt:40-48).
		tripped := p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped
		c.require(
			(tripped && p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion >= 1) ||
				scenarioPowerFraction(p) <= StepbackPowerFraction,
			"reactor stepped back to the park band or stabilized tripped")
		return c.result()
	},
	"steam-line-break": func(p *Plant) CheckResult {
		var c checkBounds
		// Excessive cooldown protection must have actuated injection:
		// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:65-69.
		// HHSI pump-running is deliberately not required here: no
		// production path latches the diesels' SI start signal from
		// channel actuation yet, so pump flow stays operator-driven.
		rps := &p.ReactorBuilding.ReactorProtectionSystem
		c.require(rps.SafetyInjectionActuated, "safety injection actuated")
		c.require(rps.ReactorTripped &&
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion >= 1,
			"reactor tripped and stabilized")
		return c.result()
	},
	"porv-stuck-open": func(p *Plant) CheckResult {
		var c checkBounds
		// Operator isolates the relief path with the block valve and
		// pressure recovers above the low-pressure trip band
		// (malfunction source: specs/pwr_sim/chapter_4/page_049.txt:28-31).
		c.require(p.ReactorBuilding.BlockValve.Open == 0,
			"block valve isolating the PORV")
		c.require(p.ReactorBuilding.Pressurizer.Pressure >=
			RPSLowPressurizerPressureTrip, "pressure recovered")
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"recovered without a reactor trip")
		return c.result()
	},
	"dropped-rod-bank": func(p *Plant) CheckResult {
		var c checkBounds
		// Combined worth cannot recover: power folds into the runback
		// park band while the turbine sheds (initiator list:
		// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29).
		turb := &p.TurbineBuilding.Turbine
		c.require(turb.LoadRunback || turb.State == TurbineTripped ||
			!p.TurbineBuilding.Generator.BreakerClosed,
			"turbine shedding the dropped-bank runback")
		c.require(scenarioPowerFraction(p) <= StepbackPowerFraction,
			"power folded to the runback band")
		return c.result()
	},
	"si-recirculation": func(p *Plant) CheckResult {
		var c checkBounds
		// Suction swaps to the containment sump when RWST level falls
		// low: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:150-155
		sis := &p.ReactorBuilding.SafetyInjectionSystem
		c.require(sis.HHSISuction == SISSuctionSump,
			"swapped to sump recirculation")
		c.require(sis.RWSTLevel.Float() <= SISRWSTLowLevel,
			"RWST drawn to its low-level switchover point")
		c.require(sis.HHSIPumpRunning, "injection continuing")
		return c.result()
	},
	"containment-spray": func(p *Plant) CheckResult {
		var c checkBounds
		// At the CSST Lo-Lo setpoint the pumps are manually switched
		// to recirculation:
		// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:17-21
		css := &p.AuxiliaryBuilding.ContainmentSpraySystem
		c.require(css.CSSTLevel.Float() <= css.CSSTLowLevelSetpoint.Float(),
			"spray tank drained to Lo-Lo")
		c.require(css.SuctionSump, "pumps on sump recirculation")
		c.require(css.Running, "spray still operating")
		return c.result()
	},
	"btrs-boron-cycling": func(p *Plant) CheckResult {
		var c checkBounds
		// The round trip releases chemistry back to volume control
		// inside the setpoint tolerance (core/cvcs.go:250-269); mode
		// directions:
		// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:12-31
		cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
		c.require(!cvcs.BoronSetpointTracking && cvcs.Mode == CVCSVolumeControl &&
			!cvcs.BTRSRecirculationPumpRunning,
			"setpoint drive released chemistry to volume control")
		c.require(math.Abs(cvcs.BoronConcentration-CoreRatedCriticalBoronPpm) <=
			CVCSBoronSetpointTolerancePpm,
			"boron round-tripped to its rated-critical starting concentration")
		// Saturation advanced past zero is what separates the completed
		// cycle from the untouched start lineup; the capacity constant
		// itself is a derived model choice (GAP-2 in
		// plans/x/scenario/31.txt).
		c.require(cvcs.BTRSBedSaturation > 0,
			"coolant actually circulated through the BTRS beds")
		c.require(cvcs.BRSHoldupLevel.Float() == 0,
			"no liquid effluent sent to the Boron Recycle System")
		vct := cvcs.VCTLevel.Float()
		c.require(vct >= VCTLowLevel && vct <= VCTHighLevel,
			"volume control tank kept inside its normal band")
		c.require(!p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
			"no trip through the evolution")
		return c.result()
	},
	"ccw-train-lineup": func(p *Plant) CheckResult {
		var c checkBounds
		ccw := &p.AuxiliaryBuilding.ComponentCoolingWaterSystem
		c.require(ccw.PumpRunning, "component cooling pump in service")
		c.require(math.Abs(ccw.InletTemp.Float()-CCWSInletSteadyStateTemp) <= 1,
			"CCW inlet holding the design 105 F value")
		return c.result()
	},
}

// Check evaluates the evolution's end-state bounds against the live
// plant. Scenarios without a registered check report not-done rather
// than pretending completion.
func (s Scenario) Check(p *Plant) CheckResult {
	check, ok := scenarioChecks[s.ID]
	if !ok {
		return CheckResult{}
	}
	return check(p)
}

// RunProgress tracks one running evolution across ticks: which
// milestones have been observed and whether the end-state bounds have
// ever held. Completion latches, so overshoot after meeting the
// objective does not un-meet it.
type RunProgress struct {
	scenario Scenario
	seen     []bool
	doneNs   int64
}

// NewRunProgress starts progress tracking for s at simulation time
// nowNs.
func NewRunProgress(s Scenario, nowNs int64) *RunProgress {
	return &RunProgress{
		scenario: s,
		seen:     make([]bool, len(s.Milestones)),
		doneNs:   -1,
	}
}

// Observe samples the live plant, marking milestones and latching
// completion the first time every milestone and bound holds at once.
func (r *RunProgress) Observe(p *Plant, nowNs int64) {
	for i, m := range r.scenario.Milestones {
		if !r.seen[i] && m.Observed(p) {
			r.seen[i] = true
		}
	}
	if r.doneNs >= 0 {
		return
	}
	for _, ok := range r.seen {
		if !ok {
			return
		}
	}
	if r.scenario.Check(p).Done {
		r.doneNs = nowNs
	}
}

// MetMs returns the simulated millisecond completion first latched,
// or -1 while the objective is still pending.
func (r *RunProgress) MetMs() int64 {
	if r.doneNs < 0 {
		return -1
	}
	return r.doneNs / int64(time.Millisecond)
}

// Result reports completion against the live plant: a latched
// objective stays done with its met time; otherwise Detail names the
// unobserved milestones and unmet bounds.
func (r *RunProgress) Result(p *Plant) CheckResult {
	if r.doneNs >= 0 {
		return CheckResult{
			Done:   true,
			Detail: fmt.Sprintf("met at %d s simulated",
				r.doneNs/int64(time.Second)),
		}
	}
	var missing []string
	for i, m := range r.scenario.Milestones {
		if !r.seen[i] {
			missing = append(missing, m.Desc)
		}
	}
	bounds := r.scenario.Check(p)
	if !bounds.Done {
		missing = append(missing, bounds.Detail)
	}
	if len(missing) == 0 {
		// Everything holds; the next Observe call latches.
		return CheckResult{Done: true, Detail: "objective met"}
	}
	return CheckResult{Detail: strings.Join(missing, "; ")}
}
