package core

import (
	u "github.com/bcicen/go-units"
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
		StartState: "Hot primary with inserted rods; condenser at " +
			"atmospheric with no vacuum, stop valve shut.",
		Objective: "Draw steam from the steam generators to warm the " +
			"secondary plant, pull condenser vacuum with the vacuum " +
			"pump, and roll the turbine on its speed demand.",
		Apply: func(p *Plant) {
			q := MakePlant()
			rpv := &q.ReactorBuilding.ReactorPressureVessel
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
		StartState: "Just critical at a few percent power on " +
			"partially withdrawn rods with auxiliary feedwater " +
			"supplying the steam generators.",
		Objective: "Take critical data, raise power through the point " +
			"of adding heat watching Tavg and the steam dumps, then " +
			"switch the steam generators from auxiliary to main " +
			"feedwater below about 3.5 percent steam flow and secure " +
			"AFW.",
		Apply: func(p *Plant) {
			q := MakePlant()
			q.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
				Watts(0.02 * float64(CoreRatedThermalPower)))
			q.ReactorBuilding.ReactorPressureVessel.Insertion = 0.35
			q.ReactorBuilding.ReactorPressureVessel.DemandInsertion = 0.35
			q.PlantOperation.Mode = HotStandby
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
		StartState: "At-power lineup: turbine rolling at rated, " +
			"generator synchronized at full load.",
		Objective: "Trip the turbine, watch the automatic stepback " +
			"park the core near 60 percent while bypass valves hold " +
			"boiler pressure, then recover: reset, restore rods, " +
			"reparallel, and reload.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
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
			"load.",
		Objective: "Drop external load suddenly down to house loads " +
			"and hold the plant there without a reactor trip: steam " +
			"dump absorbs 70-85 percent as an artificial load while " +
			"rod control cuts back the core and feedwater holds levels.",
		Apply: func(p *Plant) { *p = fullPowerLineup() },
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
			"opening their breakers starts the event.",
		Objective: "Open both offsite source breakers to strip the " +
			"grid supply, then verify the emergency diesels auto-start " +
			"and the safety buses sequence so the plant can safely " +
			"cool down.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.Electrical.OffsiteSourceA.BreakerClosed = false
			q.Electrical.OffsiteSourceB.BreakerClosed = false
			*p = q
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
		StartState: "At-power lineup with the main feedwater pump " +
			"already tripped.",
		Objective: "Watch steam generator levels fall driving a " +
			"reactor setback then trip; verify emergency feedwater " +
			"delivers within one minute with no operator action, then " +
			"modulate the EFW valves to restore narrow-range levels " +
			"and either resume power or cool down.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.TurbineBuilding.MainFeedwaterPump.Running = false
			*p = q
		},
	},
	{
		ID:       "rcp-trip",
		Number:   24,
		Category: "Malfunction transients",
		Title:    "Reactor coolant pump trip",
		StartState: "At-power lineup with loop A pump tripped at " +
			"time zero.",
		Objective: "Observe the immediate power stepback and the " +
			"decaying loop flow against the low-flow trip; try the " +
			"both-pumps-in-one-loop case to challenge the cold-leg " +
			"temperature margin.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.ReactorCoolantPumps.LoopA.Running = false
			*p = q
		},
	},
	{
		ID:       "steam-line-break",
		Number:   25,
		Category: "Malfunction transients",
		Title:    "Main steam line break",
		StartState: "At-power lineup with a full break upstream of " +
			"the main steam stop valve opened at time zero.",
		Objective: "Manage the excessive-cooldown transient: steam " +
			"demand overcools the RCS while the turbine runs back, " +
			"verify low compensated steamline pressure actuates safety " +
			"injection, and stabilize the plant afterward.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.SteamGenerator.MainSteamLineBreak.Open = 1
			*p = q
		},
	},
	{
		ID:       "porv-stuck-open",
		Number:   26,
		Category: "Malfunction transients",
		Title:    "PORV fails open",
		StartState: "At-power lineup with the pressurizer relief " +
			"valve latched open at time zero.",
		Objective: "Diagnose the stuck-open PORV venting to the " +
			"relief tank: heaters cannot hold pressure while level " +
			"rises. Isolate with the block valve before low pressure " +
			"trips the reactor, then clear the failure latch.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.Pressurizer.PORVStuckOpen = true
			*p = q
		},
	},
	{
		ID:       "dropped-rod-bank",
		Number:   27,
		Category: "Malfunction transients",
		Title:    "Dropped rod bank",
		StartState: "At-power lineup with one dark bank dropped into " +
			"the core at time zero.",
		Objective: "Compensate the dropped bank: watch the control " +
			"system exhaust gray and dark rod worth against the tilt, " +
			"ride the turbine runback and trip, and manage the xenon " +
			"buildup that walks power to zero.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.ReactorPressureVessel.ControlRods.DroppedBank = true
			*p = q
		},
	},
	{
		ID:       "si-recirculation",
		Number:   28,
		Category: "Engineered safeguards",
		Title:    "Safety injection and recirculation",
		StartState: "At-power lineup with manual safety injection " +
			"latched at time zero.",
		Objective: "Supervise the injection phase: diesels, HHSI, " +
			"accumulators discharging as pressure falls, then switch " +
			"over to cold-leg recirculation at RWST low level and later " +
			"balance hot-leg injection for long-term cooling.",
		Apply: func(p *Plant) {
			q := fullPowerLineup()
			q.ReactorBuilding.ReactorProtectionSystem.ManualSI = true
			*p = q
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
}
