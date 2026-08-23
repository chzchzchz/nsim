package core

import (
	"encoding/json"
	"io"

	u "github.com/bcicen/go-units"
)

func (r *ReactorBuilding) Step() ReactorBuilding {
	ret := *r
	dt := TimeStep
	ret.Pressurizer = ret.Pressurizer.Step(&ret.PORV)

	// PORV discharge fills the pressurizer relief tank, which
	// condenses the steam; once full, a rupture disc vents the
	// tank (and any further discharge path) to containment.
	//
	// "Steam from the safety and/or relief valves is piped to the
	// pressurizer relief tank which contains sufficient water to
	// condense the steam. [...] A rupture disc vents the tank to
	// the containment if design pressure is exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-38
	if ret.PORV.Open == 1 && !ret.RuptureDiscRuptured {
		moved := min(PRTReliefFlow*dt.Seconds(), ret.Pressurizer.Water.Float())
		ret.Pressurizer.Water = vol(ret.Pressurizer.Water,
			ret.Pressurizer.Water.Float()-moved)
		prt := min(ret.PressurizedReliefTank.Level.Float()+moved,
			PRTRuptureCapacity)
		ret.PressurizedReliefTank.Level = Volume{Val: prt, Unit: u.Gallon}
		if prt >= PRTRuptureCapacity {
			ret.RuptureDiscRuptured = true
		}
	}

	// Reactor makeup control valve: the operator sets Open
	// (0..1); Flow is the effective makeup rate actually delivered,
	// clamped against the RWST inventory that sources it. Derived
	// once per step so downstream consumers see a stable value.
	//
	// Spec: "Low-level signals from the volume control tank initiate
	// reactor makeup control or flow from the refueling water
	// storage tank as a backup."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
	makeupDemand := ret.ReactorMakeupControlValve.Open *
		ReactorMakeupDesignFlow.Float()
	rwstLevel := ret.SafetyInjectionSystem.RWSTLevel.Float()
	makeupRate := min(makeupDemand, rwstLevel/dt.Seconds())
	ret.ReactorMakeupControlValve.Flow = Volume{Val: makeupRate, Unit: u.Gallon}
	newRWST := max(rwstLevel-makeupRate*dt.Seconds(), 0)
	ret.SafetyInjectionSystem.RWSTLevel = Volume{Val: newRWST, Unit: u.Gallon}
	// CVCS: charging, letdown, VCT level, and boron
	// concentration. The CVCS boron's reactivity contribution
	// is read by the RPV step below, so CVCS must run first.
	//
	// Spec: "The pressurizer level is controlled by balancing
	// the charging flow to the letdown flow."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
	// Spec: "The operating mode (boration or dilution) and the
	// magnitude of the boron concentration change required is
	// determined by comparison of the measured Tc (cold leg
	// temperature) with the programmed (function of power) Tc."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
	ret.Aux.ChemicalVolumeControlSystem = ret.Aux.ChemicalVolumeControlSystem.Step(
		ret.ReactorMakeupControlValve.Flow,
	)
	// Propagate the CVCS boron concentration into the
	// reactivity model. The RPV step reads
	// ControlRods.BoronConcentration to compute the boron
	// reactivity contribution.
	//
	// Spec: "the boron concentration changes significantly
	// with fuel depletion."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
	ret.ReactorPressureVessel.ControlRods.BoronConcentration =
		ret.Aux.ChemicalVolumeControlSystem.BoronConcentration

	// Charging/letdown couple to pressurizer volume so the CVCS is
	// a real volume circuit rather than a display rate.
	//
	// "pressurizer level represents a balance of injection water
	// flow into the RCS and water letdown flow into the Chemical
	// and Volume Control System."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
	cvcs := ret.Aux.ChemicalVolumeControlSystem
	netCVCS := (cvcs.ChargingFlow.Float() - cvcs.LetdownFlow.Float()) *
		dt.Seconds()
	pzWater := ret.Pressurizer.Water.Float() + netCVCS
	pzWater = min(max(pzWater, 0), ret.Pressurizer.Volume.Float())
	ret.Pressurizer.Water = vol(ret.Pressurizer.Water, pzWater)

	// Reactor Protection System step. Runs before the
	// ReactorPressureVessel step so the ReactorTripped latch
	// is available when RPV drops rods this step. The RPS
	// evaluates trip conditions from the current process
	// variables (steamline pressure, SG level, Tcold,
	// containment pressure) and computes ESFAS outputs.
	// Pressurizer-pressure and high-level trips are deferred
	// at this fidelity band (see rps.go preamble).
	//
	// Spec justification:
	//   - 2/4 voting: "A 2/4 logic means that a trip occurs
	//     when at least 2 signals out of 4 indicate a trip
	//     condition."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
	//   - Turbine trip interlock: "The RPS initiates a
	//     turbine trip signal whenever reactor trip occurs."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:10-11
	sgLevelPct := 0.0
	if ret.SteamGenerator.Volume.Float() > 0 {
		sgLevelPct = 100.0 * ret.SteamGenerator.Water.Float() /
			ret.SteamGenerator.Volume.Float()
	}
	containmentGauge := ret.ContainmentBuilding.Pressure -
		AtmosphericPressure
	ret.ReactorProtectionSystem = ret.ReactorProtectionSystem.Step(
		ret.SteamGenerator.Pressure,
		sgLevelPct,
		ret.ReactorPressureVessel.Inlet.Float(),
		containmentGauge,
	)
	// Propagate the reactor trip to the reactor pressure
	// vessel so the RPV step drops rods and follows the
	// decay-heat curve this same step.
	ret.ReactorPressureVessel.Trip = ret.ReactorProtectionSystem.ReactorTripped

	// Control-rod drive rate limit (plans/x/fission_controls.md).
	// The bridge writes the operator's demand directly into
	// ControlRods.DemandInsertion; this step rate-limits the
	// transition toward that demand so the thermal model sees
	// incremental reactivity changes rather than an
	// instantaneous scram. A latched trip supersedes both and
	// forces full insertion inside ReactorPressureVessel.Step
	// (plans/x/trip.md).
	//   "Rod cluster control (RCC) assemblies are used for
	//   reactor startup or shutdown, to follow load changes,
	//   and to control small transient changes in reactivity."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:88-93
	ret.ReactorPressureVessel.ControlRods =
		ret.ReactorPressureVessel.ControlRods.Step()

	rpv := ret.ReactorPressureVessel.Step(ret.ReactorCoolantPump.Velocity)
	ret.ReactorPressureVessel = rpv

	// RCP velocity dynamics (plans/p33_1.txt). Exponential
	// ramp toward RCPRatedSpeed when Running; exponential decay
	// (the flywheel coastdown) when not. The Euler update
	// mirrors the diesel (`core/diesel.go:17-18`) and turbine
	// (`core/turbine.go:315-319`) start/coast patterns.
	//
	// Spec justification:
	//   - "A flywheel in the motor increases the total
	//     rotating inertia, allowing the pump to coastdown
	//     slowly after deenergizing."
	//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_061.txt:152-158
	//   - "A heavy flywheel on the pump motor shaft provides
	//     long coastdown times to preclude rapid decreases in
	//     core cooling flow during pump trips."
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_020.txt:83-84
	//   - "the long coast down times of the reactor coolant
	//     pumps assures adequate flow during the power decay
	//     transient."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:80-94
	rcpV := float64(ret.ReactorCoolantPump.Velocity)
	if ret.ReactorCoolantPump.Running {
		rcpV += (float64(RCPRatedSpeed) - rcpV) *
			TimeStep.Seconds() / float64(RCPStartupTimeConstant.Seconds())
	} else {
		rcpV -= rcpV *
			TimeStep.Seconds() / float64(RCPCoastdownTimeConstant.Seconds())
	}
	rcpV = max(rcpV, 0)
	ret.ReactorCoolantPump.Velocity = Velocity(rcpV)

	// RCP shaft-loss heat entering the primary coolant. The
	// motor and bearing waste power scale with speed squared
	// (over the small 1 ms step this is linear in ω² at the
	// current velocity). Zero when the rotor is stopped.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:108-115
	rcpHeat := float64(RCPRatedHeatInput) * rcpV * rcpV /
		(float64(RCPRatedSpeed) * float64(RCPRatedSpeed))
	ret.ReactorPressureVessel.PumpHeatInput = Watts(rcpHeat)

	// RHRS: once RCS has cooled below the entrance threshold
	// (350°F, <400 psig) the RHRS steps and assumes decay-heat
	// removal. HeatTransferRate is integrated into the RCS
	// outlet temperature and SteamGenerator.HeatInput is zeroed
	// so the SG no longer carries the cooldown duty.
	//
	// Spec justification:
	//   - Entrance criteria: "When the reactor coolant
	//     temperature has dropped to 350°F (177°C) and the
	//     pressure has been reduced to less than approximately
	//     400 psig (28 bar), the RHR equipment is placed in
	//     operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	//   - Hand-off: "At this point the reactor cooling function
	//     has been taken over by the RHRS, and the steam dump
	//     system may be shutdown."
	//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:35-37
	rcsOutletF := ret.ReactorPressureVessel.Outlet.Float()
	rcsPressure := float64(ret.Pressurizer.Pressure)
	// The CCWS was already stepped this timestep by
	// AuxiliaryBuilding.Step (single-writer discipline); the RHRS
	// reads that fresh OutletTemp here.
	ret.Aux.ResidualHeatRemovalSystem = ret.Aux.ResidualHeatRemovalSystem.Step(
		ret.ReactorPressureVessel.Outlet,
		rcsPressure,
		ret.Aux.ComponentCoolingWaterSystem.OutletTemp,
	)
	if ret.Aux.ResidualHeatRemovalSystem.HasTakenOver {
		rejectionW := float64(
			ret.Aux.ResidualHeatRemovalSystem.TrainA.HeatExchanger.HeatTransferRate,
		) + float64(
			ret.Aux.ResidualHeatRemovalSystem.TrainB.HeatExchanger.HeatTransferRate,
		)
		decayW := float64(ret.ReactorPressureVessel.TotalThermalPower())
		netW := decayW - rejectionW
		rcsMassKg := 3_500_000.0 * KilogramsPerPound
		deltaTK := netW / (rcsMassKg * SpecificHeatWater) * dt.Seconds()
		ret.ReactorPressureVessel.Outlet = Temperature{Val: rcsOutletF+deltaTK*9.0/5.0, Unit: u.Fahrenheit}
		ret.SteamGenerator.HeatInput = 0
	} else {
		// Per-loop SG heat input: the lumped single-SG model
		// represents SteamGeneratorLoopCount primary loops, so
		// each SG carries one loop's fraction of the core
		// thermal output. Without this division the single
		// SG would see the full CoreRatedThermalPower (~3.8
		// GW), roughly 3.5× the per-SG design rating of
		// SteamGeneratorFullLoadHeatInput (~1.08 GW at 480 kg/s
		// steam flow).
		// Spec: "Number and type: 1 vertical, U-tube steam
		// generator with integral steam-drum per loop"
		// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:31-32
		// Spec: "Steam flow per steam generator: 3,813,000
		// lb/hr (480 kg/sec)"
		// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
		ret.SteamGenerator.HeatInput = Watts(
			float64(rpv.TotalThermalPower()) / float64(SteamGeneratorLoopCount),
		)
	}
	ret.SteamGenerator = ret.SteamGenerator.Step()

	// SIS: passive accumulator discharge and HHSI pump
	// injection. The accumulator discharges into the RCS cold
	// legs whenever RCS pressure drops below the nitrogen
	// precharge; the HHSI pumps deliver borated water from the
	// RWST (or, after the low-level switchover, the containment
	// sump).
	//
	// Spec justification:
	//   - Accumulator passive discharge: "the accumulator
	//     discharge will begin automatically as the RCS
	//     pressure decreases below the accumulator tank
	//     pressure."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
	//   - HHSI pump start: "The receipt of the safety injection
	//     signal initiates the following emergency core cooling
	//     actions: [...] The HHSI pumps are started."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	//   - "This high flow is provided by the accumulators,
	//     followed by the HHSI and RHR pumps, all discharging
	//     into the cold legs of the RCS."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:20-23
	//   - "The HHSI and RHR pumps are aligned to take suction
	//     directly from the RWST."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	rcsPressureSIS := float64(ret.ReactorPressureVessel.Pressure)
	ret.SafetyInjectionSystem = ret.SafetyInjectionSystem.Step(rcsPressureSIS)

	// EFWS: per-step tank depletion and secured-condition
	// check. The auto-start logic is in Plant.Step (it needs
	// the cross-building DieselStartSignal). The secured
	// condition fires when the hot-leg temperature has been
	// reduced to EFWSSecureTemp and the RHRS has taken over.
	//
	// Spec justification:
	//   - "the reactor coolant hot leg temperature is reduced
	//     to 350°F (176.7°C) and the RHRS has been activated,
	//     the EFWS is secured."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
	rhrsActive := ret.Aux.ResidualHeatRemovalSystem.Running &&
		ret.Aux.ResidualHeatRemovalSystem.HasTakenOver
	ret.Aux.EmergencyFeedwaterSystem = ret.Aux.EmergencyFeedwaterSystem.Step(
		ret.ReactorPressureVessel.Outlet, rhrsActive,
	)
	ret.EFWSFeedwaterFlow = ret.Aux.EmergencyFeedwaterSystem.EFWSFeedwaterFlow()

	// CSS: containment pressure model. The simplified
	// containment pressure rises while the PORV is open (steam
	// dump into containment) and falls while the CSS is spraying
	// (effectiveness × spray flow × excess pressure).
	//
	// Spec justification:
	//   - "Initial Injection Mode – Containment spray is
	//     initiated automatically in response to a containment
	//     high pressure signal."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-57
	//   - "comes in contact with the steam in the containment
	//     atmosphere, the steam is condensed and falls to the
	//     containment floor as drops of a borated sodium
	//     hydroxide solution."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:8-11
	csP := float64(ret.ContainmentPressure)
	if ret.PORV.Open == 1 {
		// PORV discharge dumps saturated steam into containment;
		// model as a constant rise rate. The chosen 100 psig/s
		// saturates above the design pressure in <1 s, giving a
		// fast rising signal that triggers the CSS auto-start.
		csP += 100.0 * dt.Seconds()
	}
	cssRunning := ret.Aux.ContainmentSpraySystem.Running
	if cssRunning && ret.Aux.ContainmentSpraySystem.SprayHeaderOpen {
		// Spray condenses steam: effectiveness × spray flow
		// × (P - P_atm) drag toward atmospheric.
		excess := csP - float64(AtmosphericPressure)
		if excess > 0 {
			csP -= CSSSprayEffectiveness *
				ret.Aux.ContainmentSpraySystem.SprayFlow * excess *
				dt.Seconds()
			csP = max(csP, float64(AtmosphericPressure))
		}
	}
	ret.ContainmentPressure = Pressure(csP)
	ret.Aux.ContainmentSpraySystem = ret.Aux.ContainmentSpraySystem.Step()

	return ret
}

func (t *TurbineBuilding) Step() TurbineBuilding {
	ret := *t
	// SteamGenerator lives on ReactorBuilding, not TurbineBuilding;
	// the caller (Plant.Step) passes the upstream pressure in via
	// StepWithSteam. This signature stays for the legacy call path
	// where no upstream is wired; production uses StepWithSteam.
	return ret
}

// StepWithSteam advances the TurbineBuilding with the upstream steam
// generator's pressure supplied by the caller. The condenser is
// stepped first so the turbine reads a fresh back-pressure; the
// condenser accumulates the turbine's exhaust mass flow via
// CondensingRate for the next call.
//
// Spec justification for the order (condenser → turbine):
//   - "The main condenser condenses the exhaust from the main
//     turbines [...]" — the condenser's pressure is the turbine's
//     back-pressure.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-15
//   - The dump valve is described as opening under steam-dump
//     conditions; the condenser must be stepped first so the
//     turbine can see its pressure rise:
//     "These valves open under a sudden reduction in turbine-
//     generator load (in excess of 10%) or a turbine trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:5-11
func (t *TurbineBuilding) StepWithSteam(sgPressure Pressure, cwPumpRunning bool) TurbineBuilding {
	ret := *t

	cwIn := ret.Condenser.CirculatingWaterIn
	c := ret.Condenser.Step(cwIn, ret.Condenser.VacuumPumpRunning, cwPumpRunning)
	ret.Condenser = c

	stopOpen := ret.Turbine.StopValve.Open
	ret.Turbine = ret.Turbine.Step(
		sgPressure,
		c.Pressure,
		false,
		&stopOpen,
	)

	// Generator excitation system. The brushless exciter is
	// shaft-driven: it can only produce field current when
	// the rotor is turning.
	//
	// Spec justification:
	//   - "Excitation: Shaft-driven, air-cooled brushless
	//     exciter of the rotating rectifier type."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
	//   - "The Type WTA-300 voltage regulating system is of
	//     the latest design incorporating all solid state
	//     circuitry."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:5-7
	ret.Generator.GeneratorExcitationSystem =
		ret.Generator.GeneratorExcitationSystem.Step(
			float64(ret.Turbine.Velocity),
		)
	ret.GeneratorExcitationAvailable =
		ret.Generator.ExciterActive

	// Generator cooling system. SCCW pump mirrors the
	// feedwater/condensate pump availability in the
	// simplified model (real plant has dedicated redundant
	// pumps; for p30 the gating flag is "at least one
	// pump-running boolean").
	//
	// Spec justification:
	//   - "Demineralized cooling water is provided for the
	//     stator by a closed recirculating water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	//   - "Redundancy is provided in the water pumps, water
	//     coolers, and the alarm system so that safe,
	//     reliable operation is assured under all conditions."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:55-61
	ret.Generator.SCCWPumpRunning = ret.MainFeedwaterPump.Running ||
		ret.CondensatePump.Running
	// H₂ blower fraction tracks shaft speed relative to
	// rated; the four cooler effectiveness slots scale
	// linearly with this fraction.
	h2Blower := float64(ret.Turbine.Velocity) /
		float64(TurbineRatedSpeed)
	h2Blower = min(h2Blower, 1)
	h2Blower = max(h2Blower, 0)
	ret.Generator.GeneratorCoolingSystem =
		ret.Generator.GeneratorCoolingSystem.Step(h2Blower)

	// SCCW alarm and trip gating (plans/p30_1.txt). When
	// the SCCW circuit is active but the flow has dropped
	// below the minimum stable threshold, the alarm is
	// raised. Total loss of flow while the pump is
	// commanded running drops the generator breaker.
	sccwState := SCCWAlarm(ret.Generator.SCCWPumpRunning,
		ret.Generator.SCCWFlow)
	ret.GeneratorSCCWAlarm = sccwState.Alarm
	ret.GeneratorSCCWTrip = sccwState.Trip
	if ret.GeneratorSCCWTrip && ret.Generator.BreakerClosed {
		ret.Generator.BreakerClosed = false
	}

	// Generator terminal state. Voltage and frequency track rated
	// values whenever the breaker is closed and the rotor is at
	// speed; otherwise they decay to zero (the generator has no
	// excitation or grid back-feed when off-line).
	//
	// Spec: "Output voltage: 24,000 to 26,000 volts, three-phase,
	// 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	// "Speed: 1800 rpm"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:22
	rated := ret.Turbine.State == TurbineRunning &&
		float64(ret.Turbine.Velocity) >= float64(TurbineRatedSpeed)*0.99
	if ret.Generator.BreakerClosed && rated && ret.Generator.ExciterActive {
		ret.Generator.Voltage = GeneratorRatedVoltage
		ret.Generator.Frequency = GeneratorRatedFrequency
	} else if ret.Generator.BreakerClosed && ret.Generator.ExciterActive {
		// Rotor spinning up: terminal voltage builds
		// toward rated at the regulator's forcing rate.
		// "The high-frequency permanent magnet generator
		// provides power to the automatic voltage
		// regulator which regulates and controls the
		// output of the exciter."
		// specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:27-30
		voltageTarget := GeneratorRatedVoltage
		ret.Generator.Voltage += (voltageTarget - ret.Generator.Voltage) *
			GeneratorExciterFieldForcingFraction
		if ret.Generator.Voltage > GeneratorRatedVoltage*0.99 {
			ret.Generator.Voltage = GeneratorRatedVoltage
		}
		ret.Generator.Frequency = 0
	} else {
		ret.Generator.Voltage = 0
		ret.Generator.Frequency = 0
	}

	// RealPower and ReactivePower are zero whenever the breaker
	// is open, regardless of the turbine's mechanical output. The
	// breaker gates grid connection.
	//
	// Spec: "output power, the power factor, and speed to ensure
	// that the limits of the generator are not exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
	// "Power factor: 0.9, short circuit ratio: 0.58"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
	if ret.Generator.BreakerClosed && ret.Turbine.RealPower > 0 {
		ret.Generator.RealPower = ret.Turbine.RealPower
		pf := ret.Generator.PowerFactor
		if pf <= 0 {
			pf = GeneratorPowerFactor
		}
		ret.Generator.ReactivePower =
			float64(ret.Turbine.RealPower) * (1.0/pf - 1.0)
	} else {
		ret.Generator.RealPower = 0
		ret.Generator.ReactivePower = 0
	}

	// OPC trip pops the breaker. The spec describes OPC as part
	// of the protective trip layer that closes steam valves AND
	// disconnects the generator from the grid.
	//
	// Spec: "OPC trip (110%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
	if ret.Turbine.OPCTrip {
		ret.Generator.BreakerClosed = false
	}

	// Steam dump: open when condenser pressure is above the
	// threshold. The dump valve provides a relief path for
	// inertial heat from the SG when the turbine cannot accept
	// steam (trip or low load).
	// "Condenser steam dump valves" (item 8 of the protective
	// features list) — these open automatically on load reduction.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:62
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:5-11
	if c.Pressure > SteamDumpOpenThreshold {
		ret.Turbine.DumpValve.Open = 1.0
	} else {
		ret.Turbine.DumpValve.Open = 0.0
	}

	// CondensingRate: pass the turbine's exhaust mass flow back
	// into the condenser so its level/pressure step can react.
	// The condensing rate tracks the turbine's mechanical output
	// regardless of breaker state — the condenser sees exhaust
	// steam whether or not the unit is exporting to the grid.
	if ret.Turbine.State == TurbineRunning {
		ret.Condenser.CondensingRate = Volume{Val: steamMassFromPower(ret.Turbine.RealPower), Unit: u.Gallon}
	} else {
		ret.Condenser.CondensingRate = Volume{Val: 0, Unit: u.Gallon}
	}

	// Feedwater heater train: four LP stages between the
	// condenser hotwell and the deaerator, then two HP stages on
	// the main-feed-pump discharge side. The hotwell feedwater
	// temperature is the condenser drain temperature (~120°F at
	// design vacuum).
	//
	// Spec justification for the topology:
	//   - "The condensate pumps take suction from the condenser
	//     hotwell and pump condensate [...] through four stages of
	//     low-pressure feedwater heaters to the deaerator."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
	//   - "The water discharge from the feedwater pumps flows
	//     through two stages of high-pressure heaters then into
	//     the steam generators."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
	//
	// The per-step extraction steam flow is sized at the design
	// extraction rate derived from the turbine's load fraction
	// (plans/p11_1.txt). The MSR reheater section uses the same
	// extraction supply for both the LP sixth-stage taps and the
	// HP heater train, so the per-stage temperature rise
	// converges to LPTemperatureRisePerStage /
	// HPTemperatureRisePerStage at design operating point.
	fwIn := Temperature{Val: 120, Unit: u.Fahrenheit}
	lpSteam := ret.Turbine.ExtractionSteamFlow.Float() * MSRSixthStageExtractionFraction
	hpSteam := ret.Turbine.ExtractionSteamFlow.Float() * MSRDesignExtractionFraction
	lp := ret.LowPressureHeater
	for i := 0; i < 4; i++ {
		lp = lp.Step(fwIn.Float(), lpSteam)
		fwIn = Temperature{Val: lp.Inlet.Float()+lp.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	// LP heater drains cascade back to the condenser instead of
	// accumulating inside the heater shell.
	// "Drains from the high-pressure feedwater heaters are routed
	// to the deaerator. Drains from the four low-pressure
	// feedwater heaters are cascaded back to the condenser."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:88-90,
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:1-2
	ret.Condenser.HotwellLevel = Volume{Val: ret.Condenser.HotwellLevel.Float() +
		lp.DrainLevel.Float(), Unit: u.Gallon}
	lp.DrainLevel = Volume{Val: 0, Unit: u.Gallon}
	ret.LowPressureHeater = lp

	// MSR moisture drain to hotwell (feedwater system).
	// Spec: "where the water is removed and drained to the
	// feedwater system."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:62-63
	msrDrain := ret.Turbine.MoistureSeparatorReheater.DrainFlow.Float()
	ret.Condenser.HotwellLevel = Volume{Val: ret.Condenser.HotwellLevel.Float()+msrDrain, Unit: u.Gallon}

	// HP train: two stages.
	hp := ret.HighPressureHeater
	for i := 0; i < 2; i++ {
		hp = hp.Step(fwIn.Float(), hpSteam)
		fwIn = Temperature{Val: hp.Inlet.Float()+hp.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	ret.HighPressureHeater = hp

	// HP drains route to the deaerator tank. The
	// HeaterDrainTank plays the role of the deaerator water
	// storage tank:
	// "The booster pumps draw suction from the deaerator water
	// storage tank, which is mounted directly below the
	// deaerator [...]."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:47-49
	// "Drains from the high-pressure feedwater heaters are
	// routed to the deaerator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-14
	hpDrain := hp.DrainLevel
	newHDTLevel := ret.HeaterDrainTank.Level.Float() + hpDrain.Float()
	ret.HeaterDrainTank.Level = Volume{Val: newHDTLevel, Unit: u.Gallon}

	// Minimum-flow recirculation. When the main feed pump is
	// running but the net flow to the HP heaters is below
	// FeedwaterMinFlow, the discharge recirculates to the
	// deaerator.
	// "The discharge from the pumps is automatically recirculated
	// back to the deaerator whenever the flow to the high-pressure
	// feedwater heaters falls below the minimum."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
	// We approximate "net flow" with the pump's Running flag: when
	// the pump is stopped (or no SG-side demand exists), the
	// minimum-flow line moves FeedwaterMinFlow per step back to
	// the deaerator tank.
	if !ret.MainFeedwaterPump.Running {
		recirc := FeedwaterMinFlow.Float() * float64(TimeStep.Seconds())
		newHDTLevel := ret.HeaterDrainTank.Level.Float() + recirc
		ret.HeaterDrainTank.Level = Volume{Val: newHDTLevel, Unit: u.Gallon}
	}

	// Drain-tank accounting: clamp at capacity, draw suction while
	// the main feed pump runs, and open the dump valve to the
	// condenser on a high level.
	//
	// "The feedwater booster pumps draw suction from the deaerator
	// water storage tank, which is mounted directly below the
	// deaerator, and supply the main feedwater pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:46-49
	// "A dump valve which taps off the cross-connection drains the
	// tanks to condenser A on a high level in either tank."
	// specs/westinghouse_tech_manual/chapter_7/page_0636.txt:36-38
	dtSec := TimeStep.Seconds()
	hdt := min(ret.HeaterDrainTank.Level.Float(), HeaterDrainTankCapacityGal)
	if ret.MainFeedwaterPump.Running {
		hdt = max(hdt-FeedPumpSuctionFlow*dtSec, 0)
	}
	if hdt > HeaterDrainTankCapacityGal*HDTHighLevelFraction {
		dumped := min(HDTDumpFlow*dtSec,
			hdt-HeaterDrainTankCapacityGal*HDTHighLevelFraction)
		hdt -= dumped
		ret.Condenser.HotwellLevel = Volume{Val: ret.Condenser.HotwellLevel.Float() +
			dumped, Unit: u.Gallon}
	}
	ret.HeaterDrainTank.Level = Volume{Val: hdt, Unit: u.Gallon}

	return ret
}

// steamMassFromPower converts electrical RealPower into per-step
// steam mass condensed at the condenser. Sized for a 1000 MWe
// turbine exhausting roughly 480 kg/s of steam per SG × 4 SGs at
// full load; we ignore feedwater-heater extractions and treat the
// mass flow as proportional to RealPower / TurbineRatedkW.
func steamMassFromPower(p Watts) float64 {
	if TurbineRatedkW <= 0 {
		return 0
	}
	frac := float64(p) / float64(TurbineRatedkW)
	frac = min(max(frac, 0), 1)
	return frac * CondenserDesignSteamLoadGalPer
}

// Plant represents all state relevant to the facility.
type Plant struct {
	ReactorBuilding     `json:"ReactorBuilding"`
	AuxiliaryBuilding   `json:"AuxiliaryBuilding"`
	TurbineBuilding     `json:"TurbineBuilding"`
	Outside             `json:"Outside"`
	Electrical          `json:"Electrical"`
	// PlantOperation is the Chapter 16 operational-mode
	// layer (plans/p32_1.txt) that tracks the mode state
	// machine (ColdShutdown → HotStandby → PowerOperation)
	// and enforces rate limits, the auxiliary-to-main
	// feedwater transition, and the hot-standby RCP cap.
	//
	// Spec justification:
	//   - Operational modes: chapter 16 covers startup,
	//     normal operation, load follow, shutdown.
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
	//   - Rate limits: 5%/min loading, 50°F/hr heatup/cooldown.
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
	PlantOperation PlantOperation
	nextPlant     *Plant
}

func (p *Plant) Step() {
	if p.nextPlant == nil {
		p.nextPlant = &Plant{}
	}
	// Step the electrical distribution first so the ESF bus
	// Energized state for this step is visible to every
	// downstream consumer (CW pump, SFPC pump, RHRS, CCWS,
	// CSS, HHSI, SWS). The Electrical.Step call is pure on
	// its input and only writes to nextPlant.Electrical.
	p.nextPlant.Electrical = p.Electrical.Step()
	plantDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized

	// Run Outside.Step first so the CW pump / cooling-tower
	// state is in nextPlant. Step the AuxiliaryBuilding next so
	// its diesel / RHRS / CCWS state is fresh for the
	// ReactorBuilding step (CVCS / RHRS / CCWS / CSS / RCFC /
	// EFWS live on the auxiliary building and read the
	// pointer populated below).
	//
	// The cross-building source pointers must point at
	// nextPlant (where the fresh state is being written), not
	// at p (the current state being read).
	// The tower's hot inlet is the condenser CW outlet stepped
	// in the previous tick's TurbineBuilding pass (a 1 ms lag,
	// same as the tower's flow input).
	p.nextPlant.Outside = p.Outside.Step(
		plantDriveReady, p.TurbineBuilding.Condenser.CWOut)
	// Latch the CW pump DriveReady (naming-convention
	// counterpart to ServiceWaterSystem.DriveReady) so the
	// bridge can publish it as a sensor. The pump motors ride
	// on the 6600 V plant distribution bus fed by UAT or SAT
	// (see plans/x/uat_sat_gating.md); a blackout drops it.
	p.nextPlant.Outside.CirculatingWaterPump.DriveReady = plantDriveReady
	// SWS flow at this point is the prior step's delivered flow
	// in gpm (the SWS Step runs later, after the cross-building
	// DriveReady wiring). PumpRunning alone is not enough — the
	// 6600V ESF breaker must be closed for the pump motor to
	// develop flow, so the diesel jacket cooler sees Flow > 0
	// only when power is actually delivered to the pump.
	// The SFPC pump reads the same plantDriveReady signal
	// (480 V MCC downstream of an ESF bus).
	p.nextPlant.AuxiliaryBuilding = p.AuxiliaryBuilding.Step(
		p.Outside.ServiceWaterSystem.InletTemp,
		p.Outside.ServiceWaterSystem.Flow,
		plantDriveReady,
	)
	// Latch the SFPC DriveReady so the bridge can publish it
	// as a sensor (plans/x/uat_sat_gating.md).
	p.nextPlant.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.DriveReady =
		plantDriveReady
	p.ReactorBuilding.Aux = &p.nextPlant.AuxiliaryBuilding
	p.Electrical.DieselGenA = &p.nextPlant.AuxiliaryBuilding.DieselGenerator
	p.Electrical.DieselGenB = &p.nextPlant.AuxiliaryBuilding.DieselGenerator

	// RHR TrainA on BusA, TrainB on BusB. The spec assigns
	// RHR pumps to their own ESF train, mirroring the EFWS
	// motor-pump assignment.
	// "Loads important to plant safety are split and
	// diversified between independent engineered safety
	// features system switchgear groups."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:33-36
	p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.DriveReady =
		p.nextPlant.Electrical.ESFBusA.Energized
	p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.DriveReady =
		p.nextPlant.Electrical.ESFBusB.Energized

	p.nextPlant.ReactorBuilding = p.ReactorBuilding.Step()
	// Latch the RCP DriveReady after the ReactorBuilding step so
	// the assignment above (which copies the prior state) cannot
	// clobber it. The bridge sensor then reflects the actual
	// upstream power state, matching the pattern used for the CW
	// and SWS pumps. The RCP motors ride on the 6600 V ESF buses:
	//   "All of the engineered safety system loads are fed from
	//    two 6600-volt engineered safety features buses."
	//   specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
	p.nextPlant.ReactorBuilding.ReactorCoolantPump.DriveReady = plantDriveReady

	// Refueling operations step. The phase machine advances
	// based on phase-relative time; the cavity flood/drain
	// booleans are operator-driven inputs (set by the test
	// harness or higher-fidelity HMI). The pit water level
	// and boron concentration are held constant at the
	// refueling-cavity values per the spec.
	//
	// Spec justification:
	//   - Five-phase cycle: "The refueling operation is divided
	//     into five major phases: preparation, reactor
	//     disassembly, fuel handling, reactor reassembly, and
	//     preoperational checks, tests, and startup."
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
	//   - Pit always full of water: "the fuel storage pit,
	//     which is kept full of water and is always accessible
	//     to operating personnel."
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:25-29
	p.nextPlant.ReactorBuilding.RefuelingOperations =
		p.nextPlant.ReactorBuilding.RefuelingOperations.Step()

	// SIS actuation: receipt of the safety-injection signal
	// starts the HHSI pumps and aligns them to the RWST. The
	// signal is the same DieselStartSafetyInjection that
	// starts the diesel (the spec ties both actions to the
	// same actuation event).
	//
	// Spec justification:
	//   - "The receipt of the safety injection signal initiates
	//     the following emergency core cooling actions: The
	//     onsite emergency diesel generators are started. The
	//     HHSI pumps are started."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	//   - "The HHSI and RHR pumps are aligned to take suction
	//     directly from the RWST."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	if p.AuxiliaryBuilding.DieselGenerator.StartSignal == DieselStartSafetyInjection {
		// HHSI pumps need ESF bus power; DriveReady
		// reflects whether at least one ESF bus is
		// energised. The actual injection flow is gated
		// on HHSIPumpRunning && HHSIDriveReady, so the
		// auto-start latches PumpRunning on the safety
		// signal (operator intent) and the bus-energised
		// state decides whether flow actually develops.
		p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSIDriveReady =
			p.nextPlant.Electrical.ESFBusA.Energized ||
				p.nextPlant.Electrical.ESFBusB.Energized
		p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = true
		if p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSISuction !=
			SISSuctionSump {
			p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSISuction =
				SISSuctionRWST
		}
	}

	// EFWS auto-start. The pump-start logic is the most
	// load-bearing safety function of the EFWS; the spec
	// enumerates three independent actuation signals.
	//
	// Spec justification:
	//   - Safety-injection signal: "Safety injection / Motor
	//     driven pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:94-98
	//   - Low-low level in any one SG: "Low-low level in 2/4
	//     level channels in any one steam generator / Motor
	//     driven pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:71-79
	//   - Low-low level in any two SGs: "Low-low level in 2/4
	//     level channels in any two steam generators / Turbine
	//     drive pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:82-92
	//   - Diverse power sources: "The use of both motor driven
	//     and turbine driven pumps satisfies the requirement
	//     that the pumps be powered by diverse power sources."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:3-5
	efws := &p.nextPlant.AuxiliaryBuilding.EmergencyFeedwaterSystem
	if !efws.Active {
		sgWaterLow := p.ReactorBuilding.SteamGenerator.Water.Float() <
			EFWSLevelLowLow
		// The simplified model uses a single combined
		// SteamGenerator field, so "low-low in any one SG"
		// reduces to a single boolean and "low-low in any
		// two SGs" becomes equivalent to "low-low in any
		// one SG". The per-loop SGs are not modelled
		// separately at this fidelity band.
		if p.AuxiliaryBuilding.DieselGenerator.StartSignal ==
			DieselStartSafetyInjection ||
			p.nextPlant.ReactorBuilding.ReactorProtectionSystem.AuxiliaryFeedPumpsStartActuated ||
			sgWaterLow {
			if !efws.SubsystemA.MotorPump.Running {
				efws.SubsystemA.MotorPump.Running = true
				efws.MotorPumpStartCount++
			}
			if !efws.SubsystemB.MotorPump.Running {
				efws.SubsystemB.MotorPump.Running = true
				efws.MotorPumpStartCount++
			}
			efws.Active = true
		}
		if sgWaterLow {
			// Single-SG low-low simultaneously satisfies
			// the "two SG" condition in the simplified
			// model, so the turbine pump starts too.
			if !efws.SubsystemA.TurbinePump.Running {
				efws.SubsystemA.TurbinePump.Running = true
				efws.TurbinePumpStartCount++
			}
			if !efws.SubsystemB.TurbinePump.Running {
				efws.SubsystemB.TurbinePump.Running = true
				efws.TurbinePumpStartCount++
			}
		}
	}

	// EFWS: turbine-driven pump steam admission. The
	// SteamAdmissionValve is fail-open on loss of air or
	// electrical power; whenever an SG header has pressure the
	// valve is open and the pump is ready to drive.
	//
	// Spec justification:
	//   - "The steam supply line for each turbine driven pump
	//     is connected to the main steam line from a steam
	//     generator. This line is fitted with a steam admission
	//     valve which is a pneumatically operated valve arranged
	//     to fail-open on loss of air or electrical power."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
	if float64(p.ReactorBuilding.SteamGenerator.Pressure) > 0 {
		efws.SubsystemA.TurbinePump.SteamAdmissionValve.Open = 1.0
		efws.SubsystemB.TurbinePump.SteamAdmissionValve.Open = 1.0
		efws.SubsystemA.TurbinePump.DriveReady = true
		efws.SubsystemB.TurbinePump.DriveReady = true
	}
	// Motor-driven pump DriveReady tracks the ESF bus for
	// each subsystem. The spec assigns each subsystem's
	// motor pump to its corresponding ESF train.
	// "Each subsystem [of the EFWS] receives electrical power
	// from one of two separate Safety Class 1E electrical power
	// trains."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:61-65
	efws.SubsystemA.MotorPump.DriveReady =
		p.nextPlant.Electrical.ESFBusA.Energized
	efws.SubsystemB.MotorPump.DriveReady =
		p.nextPlant.Electrical.ESFBusB.Energized

	// CCWS pumps derive power from the ESF bus; the diesel
	// energises them through the same path as the EFWS motor
	// pumps. DriveReady reflects whether at least one ESF bus
	// is energised; PumpRunning auto-latches when the bus is
	// up so the CCWS Step applies the recovery-lag to the
	// inlet.
	//
	// Spec justification:
	//   - "The CCWS Safeguards subsystem is considered an
	//     engineered safeguards system because it is required
	//     to remove decay heat during postaccident and to
	//     provide cooling water to safeguards equipment."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:135-138
	ccwsDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized
	p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.DriveReady =
		ccwsDriveReady
	// CCWS pump auto-start: latch PumpRunning on first entry
	// to "DriveReady=true" so the Step applies the recovery-
	// lag. DriveReady stays true only while an ESF bus is
	// energised, so power loss simply drops the pump back
	// to "intent without power" without operator action.
	if ccwsDriveReady &&
		!p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning {
		p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	}

	// CSS auto-start: containment spray is initiated in
	// response to a containment high-pressure signal. DriveReady
	// requires at least one ESF bus energised (CSS pumps are
	// 6600V ESF loads); the auto-start only fires when both the
	// actuation signal is present AND DriveReady is true.
	//
	// Spec justification:
	//   - "Initial Injection Mode – Containment spray is
	//     initiated automatically in response to a containment
	//     high pressure signal. This signal starts the spray
	//     pumps, opens the spray header isolation valves "A",
	//     and opens the SAT isolation valves "B"."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
	css := &p.nextPlant.AuxiliaryBuilding.ContainmentSpraySystem
	cssDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized
	css.DriveReady = cssDriveReady
	// CSS auto-start: latch on the actuation signal
	// regardless of bus state; DriveReady gates the actual
	// spray contribution in the Step function. This matches
	// the AFWPump pattern (latch Running on the start signal;
	// DriveReady gates effective flow).
	if !css.Running &&
		(p.ReactorBuilding.ContainmentPressure > CSSContainmentPressureSetpoint ||
			p.nextPlant.ReactorBuilding.ReactorProtectionSystem.ContainmentSprayActuated) {
		css.Running = true
		css.SprayHeaderOpen = true
	}

	// SWS pumps are fed from the 6600V engineered safety
	// features bus, so they receive power automatically when
	// the diesel (or off-site power) energises the bus on a
	// safety-injection signal. The simplified model latches
	// PumpRunning when the SI signal is present, so the
	// diesel jacket cooling and the RCFC / CCWS heat-rejection
	// paths become active without operator action.
	//
	// The operator control (Outside/ServiceWaterSystem/PumpRunning)
	// remains the manual-test override for normal operation.
	//
	// Spec justification:
	//   - "The larger auxiliaries such as the service water
	//     pumps [...] are fed directly from the 6600-volt
	//     [engineered safety features] buses."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
	//   - "The onsite emergency diesel generators are
	//     started. [...] The HHSI pumps are started." (SI
	//     actuation list — SWS pumps follow the same ESF
	//     bus-energised path.)
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	//   - "The system auxiliary transformers and the standby
	//     diesel generator constitute the normal and redundant
	//     sources of power for the engineered safety features
	//     system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:91-94
	sws := &p.nextPlant.Outside.ServiceWaterSystem
	swsDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized
	sws.DriveReady = swsDriveReady
	// SWS pumps are fed from the 6600V ESF bus, so they
	// only develop flow when the bus is energised. The
	// auto-start latches PumpRunning on the safety signal
	// (operator intent); the actual flow contribution is
	// gated on PumpRunning && DriveReady in SWS.Step.
	if !sws.PumpRunning &&
		(p.AuxiliaryBuilding.DieselGenerator.StartSignal ==
			DieselStartSafetyInjection ||
			p.AuxiliaryBuilding.DieselGenerator.StartSignal ==
				DieselStartLossOfOffsitePower) {
		sws.PumpRunning = true
	}

	// RCFC step: each running unit integrates heat rejection
	// against the live containment temperature. The cross-
	// building pass has access to the diesel state (the
	// "LOCA sensed" trigger for the low-speed switchover).
	//
	// Spec justification:
	//   - "When a LOCA is sensed, the fan motors are
	//     automatically switched to low speed to provide proper
	//     flow of the steam-air mixture."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-57
	//   - "The cooling coils remove heat from the air with the
	//     fan providing the required air flow rates."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:84-85
	locaSensed := p.AuxiliaryBuilding.DieselGenerator.StartSignal ==
		DieselStartSafetyInjection
	cbTemp := p.ReactorBuilding.ContainmentBuilding.Temperature
	for i := 0; i < RCFCUnitCount; i++ {
		p.nextPlant.AuxiliaryBuilding.RCFCUnits[i] =
			p.nextPlant.AuxiliaryBuilding.RCFCUnits[i].Step(
					locaSensed, cbTemp,
					p.Outside.ServiceWaterSystem.InletTemp,
					p.Outside.ServiceWaterSystem.Flow > 0,
				)
	}
	var rcfcHeatOut Watts
	for i := 0; i < RCFCUnitCount; i++ {
		rcfcHeatOut += Watts(
			p.nextPlant.AuxiliaryBuilding.RCFCUnits[i].HeatTransferRate,
		)
	}
	// CSS spray condensation: spray drops condensing steam from
	// the containment atmosphere remove latent heat at spray mass
	// flow × heat of vaporisation.
	//
	// Spec justification:
	//   - "the steam is condensed and falls to the containment
	//     floor as drops of a borated sodium hydroxide solution."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:4-7
	//   - Pressure limiting alongside the fan coolers:
	//     "independently capable of delivering the necessary flow
	//     to limit containment pressure in conjunction with the
	//     Reactor Containment Fan Cooler System."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:29-32
	spray := p.nextPlant.AuxiliaryBuilding.ContainmentSpraySystem
	var cssHeatOut Watts
	if spray.Running && spray.DriveReady && spray.SprayHeaderOpen {
		massKgPerS := spray.SprayFlow * KilogramsPerGPM
		cssHeatOut = Watts(massKgPerS * float64(LatentHeatOfSteamization))
	}
	// RCS break enthalpy release is deferred until a break-flow
	// state exists (plans/x/possible_bugs.md, containment model
	// split); heatIn carries only the negative spray contribution.
	p.nextPlant.ReactorBuilding.ContainmentBuilding =
		p.nextPlant.ReactorBuilding.ContainmentBuilding.Step(
			-cssHeatOut, rcfcHeatOut,
		)

	// RPS-driven CIS manual flags. The Reactor Protection
	// System is the "brain" that decides when to actuate
	// containment isolation per Table 7-1; the wiring here
	// sets the CIS ManualPhaseA / ManualPhaseB flags so the
	// CIS step can latch the corresponding phases. This
	// mirrors the existing safety-injection propagation path
	// and extends it with RPS-driven containment spray.
	//
	// Spec justification:
	//   - ESFAS Table 7-1: CIS Phase A on SI or manual.
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:78-80
	//   - CIS Phase B on high containment pressure or manual
	//     containment spray.
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:68-71
	rps := p.nextPlant.ReactorBuilding.ReactorProtectionSystem
	cis := &p.nextPlant.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	// The manual flags mirror the RPS ESFAS outputs in both
	// directions so an operator reset of the protection system also
	// releases the isolation demand. The CIS step still requires the
	// underlying condition (SI signal gone, pressure below setpoint)
	// before each phase actually unlatches:
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:68-80
	cis.ManualPhaseA = rps.ContainmentIsolationPhaseAActuated
	cis.ManualPhaseB = rps.ContainmentIsolationPhaseBActuated

	// Containment Isolation System step. Latches Phase A on
	// safety-injection signal or operator manual command;
	// latches Phase B on high containment pressure or operator
	// manual command. The simplified model assumes
	// instantaneous valve closure (no stroke time).
	//
	// Spec justification:
	//   - Phase A actuation: "Containment Isolation Phase A:
	//     Safety injection, Manual."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
	//   - Phase B actuation: "Containment Isolation Phase B:
	//     High containment pressure, Manual actuation of
	//     containment spray."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
	p.nextPlant.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem =
		p.nextPlant.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem.Step(
			locaSensed,
			p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure-AtmosphericPressure,
		)

	// Hydrogen Detection and Suppression step. Generation is
	// active when the safety-injection signal has fired (LOCA
	// detected); recombiners and ventilation activate
	// automatically via the HDS step once the hydrogen
	// concentration crosses their thresholds with hysteresis.
	//
	// Spec justification:
	//   - Hydrogen source: "Under accident conditions [...]
	//     the Zirconium water reaction can result in the
	//     release of hydrogen gas."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-33
	hds := p.nextPlant.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	hds.GenerationActive = locaSensed
	hds = hds.Step()
	p.nextPlant.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression = hds

	// SWS: the service water picks up the heat rejected by the
	// RCFC coils plus the component cooling heat exchangers, and
	// its inlet tracks Outside.RiverTemp (shared intake house).
	// In the simplified model the lumped SWS flow
	// is the design maximum (all three essential pumps running)
	// at construction; the outlet temperature rises with the
	// imposed heat load.
	//
	// Spec justification:
	//   - "The three essential service water pumps for each
	//     unit are typically rated at approximately 20,000
	//     gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	//   - "Component cooling heat exchangers",
	//     "Containment for ventilation coolers" are typical
	//     SWS loads.
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:81-93
	//   - Shared river intake: "serves one circulating water
	//     pump and one service water pump."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:37-38
	//   - "Diesel generator coolers" join the component cooling HXs
	//     as essential loads when the engine runs with cooler flow.
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89
	ccwDuty := p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.HXHeatDuty
	jacketDuty := p.nextPlant.AuxiliaryBuilding.DieselGenerator.JacketHeatDuty
	p.nextPlant.Outside.ServiceWaterSystem =
		p.nextPlant.Outside.ServiceWaterSystem.Step(
			p.Outside.RiverTemp,
			rcfcHeatOut+ccwDuty+jacketDuty,
		)

	// SFPCS: pump auto-start and purification-flow update
	// happen inside AuxiliaryBuilding.Step above, where the
	// upstream-drive readiness signal is plumbed in.
	//
	// Spec justification:
	//   - "Normal operation requires two pumps with the third
	//     pump serving as a standby."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	//   - "passing approximately 100 gpm (23 m3/hr) of each
	//     loop's flow through a filter and demineralizer."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	_ = p.nextPlant.AuxiliaryBuilding.SpentFuelPoolCoolingSystem

	// Flow-modulating valves: assume full-open during the
	// first 30 minutes after actuation. After that the
	// operator controls them, but the simplified model leaves
	// them at full-open for the duration of the run.
	//
	// Spec justification:
	//   - "it is assumed that no operator action can be taken
	//     for 30 minutes, and for this period it is assumed
	//     that these valves will be full open."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:36-39
	if efws.Active {
		efws.SubsystemA.MotorPump.FlowModulatingValve.Open = 1.0
		efws.SubsystemA.TurbinePump.FlowModulatingValve.Open = 1.0
		efws.SubsystemB.MotorPump.FlowModulatingValve.Open = 1.0
		efws.SubsystemB.TurbinePump.FlowModulatingValve.Open = 1.0
	}

	// Reactor-to-turbine trip cascade. The spec's protective feature
	// list includes "Turbine trip following a reactor trip" (item 2)
	// and describes the auto-stop-oil mechanism that closes all steam
	// control valves on any trip signal. We therefore latch the
	// turbine into TurbineTripped the same step the reactor trips,
	// matching the auto-stop-oil "hydraulic fluid dumped to drain"
	// behaviour described in the spec.
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:33-35
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
	if p.ReactorBuilding.ReactorPressureVessel.Trip ||
		p.TurbineBuilding.Turbine.State == TurbineTripped {
		p.TurbineBuilding.Turbine.StopValve.Open = 0
		if p.TurbineBuilding.Turbine.State == TurbineRunning {
			p.TurbineBuilding.Turbine.State = TurbineTripped
		}
	}

	// Load-runback propagation. The spec's protective-layer output
	// for the DEHM is a "controlled selectable rate" runback on
	// dropped rod / high kw/ft / low DNBR / rapid nuclear flux
	// decrease / rod bottom circuitry. We collapse those four
	// signals into a single LoadRunback bit set when the reactor
	// trips (the most aggressive of the four signals); the
	// per-signal wiring is in scope for a later plan.
	//
	// Spec: "Automatic load runback initiated by a dropped rod
	// signal, high kw/ft signal or low departure from nucleate
	// boiling ratio signal."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
	if p.ReactorBuilding.ReactorPressureVessel.Trip {
		p.TurbineBuilding.Turbine.LoadRunback = true
	}

	// Reactor trip also opens the generator breaker so the unit
	// stops exporting power to the grid. Combined with the
	// stop-valve close above, this matches the spec's protective
	// cascade of "Turbine trip following a reactor trip" and the
	// DEHM's automatic load-runback behaviour.
	//
	// Spec: "Turbine trip following a reactor trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:33-35
	if p.ReactorBuilding.ReactorPressureVessel.Trip {
		p.TurbineBuilding.Generator.BreakerClosed = false
	}

	// Grid interface: advance the infinite bus, then latch the
	// grid frequency into the turbine so the droop governor can
	// see it during the next turbine step. Plant.Step is the only
	// writer of GridFrequency; the turbine step only reads.
	//
	// Spec: "computes signals comparing turbine speed and
	// first-stage pressure with reference values established by
	// the dispatcher or initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	p.nextPlant.ReactorBuilding.Grid = p.ReactorBuilding.Grid.Step()
	p.TurbineBuilding.Turbine.GridFrequency = p.nextPlant.ReactorBuilding.Grid.Frequency

	cwPumpRunning := p.Outside.CirculatingWaterPump.Running
	// Condenser flow is gated on both the operator latch
	// (cwPumpRunning) and the upstream-drive readiness
	// (plantDriveReady). A blackout that drops the ESF bus
	// also drops CW flow; the secondary side then has no
	// heat sink and the turbine can no longer accept steam.
	cwFlowing := cwPumpRunning && plantDriveReady
	p.nextPlant.TurbineBuilding = p.TurbineBuilding.StepWithSteam(
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure,
		cwFlowing,
	)

	// Feedwater regulator step. The simplified model collapses
	// the spec's per-SG regulators into a single shared
	// regulator that reads the lumped SteamGenerator (plans/p22_1.txt).
	//
	// The level input is derived from the SG water inventory as
	// a fraction of the design capacity; this is the process
	// variable the regulator drives toward FeedwaterLevelSetpoint.
	//
	// Spec justification:
	//   - "Feedwater flow to each steam generator is controlled
	//     by a feedwater regulator in each feedwater line. The
	//     regulator is controlled by steam generator level,
	//     steam flow and feedwater flow."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
	sg := p.nextPlant.ReactorBuilding.SteamGenerator
	sgLevelPct := 0.0
	if sg.Volume.Float() > 0 {
		sgLevelPct = 100.0 * sg.Water.Float() / sg.Volume.Float()
	}
	reg := p.nextPlant.TurbineBuilding.FeedwaterRegulator
	reg.Level = sgLevelPct
	reg.SteamFlow = sg.SteamFlowRate.Float()
	reg.FeedwaterFlow = sg.SteamFlowRate.Float()
	reg.Running = p.nextPlant.TurbineBuilding.MainFeedwaterPump.Running &&
		!p.nextPlant.TurbineBuilding.Turbine.LoadRunback
	reg = reg.Step()
	p.nextPlant.TurbineBuilding.FeedwaterRegulator = reg

	// Drive the main feedwater control valve to the regulator's
	// demand. The valve position is the regulator's output and
	// gates the effective flow into the SG. RPS tracks
	// FeedwaterIsolationActuated per Table 7-1 (FWI on high SG
	// level, safety injection, or coincidence of reactor trip
	// and low Tcold) but the simplified model does NOT close
	// the valve on FWI here — that wiring requires a
	// feedwater-isolation trip valve beyond the regulator's
	// demand range. FWI is exposed via the RPS field for
	// operator display and downstream coordination.
	//
	// Spec justification:
	//   - ESFAS Table 7-1: Feedwater Isolation triggers.
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:73-76
	p.nextPlant.TurbineBuilding.MainFeedwaterControlValve.Open =
		reg.ValveDemand

	p.nextPlant.Electrical = p.Electrical.Step()

	// Plant electrical systems step (plans/p27_1.txt). The
	// simplified model rolls offsite availability, diesel
	// start timing, ESF bus transfer, inverter power, and
	// station-blackout declaration into a single struct
	// that lives on ReactorBuilding. Its inputs are
	// OffsitePowerAvailable (typically true at construction)
	// and the auto-start signal handled in the Step function
	// below; its outputs feed downstream consumers (e.g. the
	// RPS and EFWS logic can gate on InverterPowerAvailable
	// when the simplified model is extended).
	//
	// Spec justification:
	//   - "The Engineered Safety Features buses are normally
	//     supplied from the system auxiliary transformers.
	//     In the event of total loss of auxiliary power from
	//     off-site sources, auxiliary power required for
	//     safe shutdown will be supplied from [...] diesel
	//     generators."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:28-33
	p.nextPlant.ReactorBuilding.PlantElectricalSystems =
		p.ReactorBuilding.PlantElectricalSystems.Step()

	// Plant control room step (plans/p28_1.txt). The control
	// room is a passive aggregator: its availability is
	// gated on electrical power, its safety-station status
	// mirrors the ESF bus state, and its alarm is the OR
	// of the RPS trip and the hydrogen-alarm signals.
	//
	// Spec justification:
	//   - Power dependency: "No single failure in the power
	//     distribution system will result in degradation of
	//     the control area functions, except for ancillary
	//     or nonessential performance features."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_146.txt:11-16
	//   - Alarm acknowledgement: "Turned off by operator
	//     acknowledgement."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	pes := &p.nextPlant.ReactorBuilding.PlantElectricalSystems
	alarmCondition :=
		p.nextPlant.ReactorBuilding.ReactorProtectionSystem.ReactorTripped ||
			p.nextPlant.ReactorBuilding.ContainmentBuilding.
				HydrogenDetectionSuppression.HydrogenAlarm ||
			p.nextPlant.TurbineBuilding.GeneratorSCCWAlarm ||
			p.nextPlant.TurbineBuilding.GeneratorSCCWTrip
	pcrIn := p.ReactorBuilding.PlantControlRoom
	pcrIn.PowerAvailable = pes.InverterPowerAvailable
	p.nextPlant.ReactorBuilding.PlantControlRoom = pcrIn.Step(
		alarmCondition,
		p.Alarms(),
		pes.ESFBus1Available,
		pes.ESFBus2Available,
	)

	// Plant computer step (plans/p29_1.txt). The computer
	// is a passive aggregator: it derives data-acquisition
	// availability from the control-room power state, drives
	// its SPDS from the aggregate safety-system status, and
	// arms the post-trip review / sequence-of-events
	// recording on a reactor trip.
	//
	// Spec justification:
	//   - Computer mission: "collects data from many
	//     locations within the plant and then processes it
	//     to provide a variety of information."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
	//   - SPDS: "Presents a succinct display account of the
	//     overall plant safety status."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	//   - Post-trip review:
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:1-3
	pcIn := p.ReactorBuilding.PlantComputer
	pcPower := p.nextPlant.ReactorBuilding.
		PlantElectricalSystems.InverterPowerAvailable
	rps = p.nextPlant.ReactorBuilding.ReactorProtectionSystem
	pcAlarm := rps.ReactorTripped ||
		p.nextPlant.ReactorBuilding.ContainmentBuilding.
			HydrogenDetectionSuppression.HydrogenAlarm ||
		p.nextPlant.TurbineBuilding.GeneratorSCCWAlarm ||
		p.nextPlant.TurbineBuilding.GeneratorSCCWTrip
	// Aggregate "safety systems available" is the AND of the
	// RPS being operable, both ESF buses being energised,
	// and the major safeguard systems being active. The
	// simplified model collapses per-system availability
	// flags into this single boolean.
	safetyAvailable := !rps.ReactorTripped &&
		pes.ESFBus1Available && pes.ESFBus2Available
	p.nextPlant.ReactorBuilding.PlantComputer = pcIn.Step(
		pcPower,
		rps.ReactorTripped,
		rps.ReactorTripped && !rps.ManualTrip,
		pcAlarm,
		safetyAvailable,
		p.nextPlant.ReactorBuilding.
			PlantControlRoom.AlarmAcknowledged,
	)

	// Station transformer and house load. The station transformer
	// step reads the generator's terminal voltage (post step)
	// and the grid voltage to decide whether UAT and SAT are
	// energised. The house load step sums fixed demand plus a
	// variable demand that scales with turbine load fraction
	// when the generator is online.
	//
	// Spec: "the plant distribution buses are split and part are
	// supplied by the plant's own generator and the other part
	// are supplied by the system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:54-56
	// "supply power for its own auxiliaries."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
	p.nextPlant.ReactorBuilding.StationTransformer =
		p.nextPlant.ReactorBuilding.StationTransformer.Step(
			float64(p.nextPlant.TurbineBuilding.Generator.Voltage),
			p.nextPlant.ReactorBuilding.Grid.Voltage,
			p.nextPlant.TurbineBuilding.Generator.BreakerClosed,
		)
	loadFraction := 0.0
	if TurbineRatedkW > 0 {
		loadFraction = float64(p.nextPlant.TurbineBuilding.Turbine.RealPower) /
			float64(TurbineRatedkW)
		loadFraction = min(max(loadFraction, 0), 1)
	}
	p.nextPlant.ReactorBuilding.HouseLoad =
		p.nextPlant.ReactorBuilding.HouseLoad.Step(
			p.nextPlant.TurbineBuilding.Generator.BreakerClosed,
			loadFraction,
		)

	// Net generator output after house load. When the breaker is
	// closed, the gross generator output minus house load is what
	// flows to the grid. When the breaker is open, house load is
	// still drawn — from SAT, not the generator — so
	// Generator.RealPower stays at zero (the generator isn't
	// powering anything).
	//
	// Spec: "house loads automatically transferred to the offsite
	// sources of power."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64-66
	if p.nextPlant.TurbineBuilding.Generator.BreakerClosed {
		gross := float64(p.nextPlant.TurbineBuilding.Turbine.RealPower)
		net := gross - p.nextPlant.ReactorBuilding.HouseLoad.TotalMW
		net = max(net, 0)
		p.nextPlant.TurbineBuilding.Generator.RealPower = Watts(net)
		pf := p.nextPlant.TurbineBuilding.Generator.PowerFactor
		if pf <= 0 {
			pf = GeneratorPowerFactor
		}
		p.nextPlant.TurbineBuilding.Generator.ReactivePower =
			net * (1.0/pf - 1.0)
	}

	// Cross-building DriveReady wiring for the AFW pumps. The fluid
	// graph (see simulateFluidFlow) treats `Running && DriveReady` as
	// the gate for effective flow, so each pump's DriveReady must be
	// latched from its upstream before the graph runs.
	//
	// Spec justification:
	//   - DieselAFWPump: "motor driven and turbine driven pumps are
	//     located in the emergency feedwater pump building. The use of
	//     both motor driven and turbine driven pumps satisfies the
	//     requirement that the pumps be powered by diverse power
	//     sources."
	//     (specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:1-5)
	//   - MotorAFWPump: ESF bus is fed by the diesel after LossOfOffsite
	//     (specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:23-36);
	//     until ESF is Energized the motor-driven AFWP cannot deliver flow.
	//   - TurbineAFWPump: "The steam supply line for each turbine driven
	//     pump is connected to the main steam line from a steam
	//     generator. This line is fitted with a steam admission valve
	//     […] arranged to fail-open on loss of air or electrical power."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
	p.nextPlant.Outside.DieselAFWPump.DriveReady =
		p.nextPlant.AuxiliaryBuilding.DieselGenerator.ReadyToLoad
	p.nextPlant.Outside.MotorAFWPump.DriveReady =
		p.nextPlant.Electrical.ESFBusA.Energized
	p.nextPlant.Outside.TurbineAFWPump.DriveReady =
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure > SteamGeneratorShellPressure/100

	// Plumb the CW inlet temperature from the outside loop. The
	// CirculatingWaterSystem.InletTemp field (plans/p31_1.txt)
	// is the source-side temperature after any cooling-tower
	// approach has been applied; in steady state this tracks
	// RiverTemp for once-through systems.
	//
	// Spec justification:
	//   - "The Circulating Water System serves as the major
	//     heat sink for the power plant by providing large
	//     amounts of cooling water for the main condensers."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
	p.nextPlant.TurbineBuilding.Condenser.CirculatingWaterIn =
		p.nextPlant.Outside.CirculatingWaterSystem.InletTemp

	// Plant Operation step (plans/p32_1.txt). Drives the
	// operational-mode state machine, the 5%/min loading
	// ramp, the aux-to-main feedwater transition, and the
	// hot-standby RCP cap. The per-SG steam flow fraction is
	// approximated as SG shell pressure over its design value
	// (920 psia at full load) because the simplified model
	// collapses all four SGs into a single lumped
	// SteamGenerator and does not track per-SG mass flow
	// separately.
	//
	// Spec justification:
	//   - 5%/min ramp: "plant loading is continued to full
	//     power at a rate not to exceed 5 percent of full power
	//     per minute."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
	//   - Aux-to-main: "Before reaching the flow capacity of
	//     the auxiliary feedwater system, equivalent to about
	//     3.5-percent steam flow, the steam generators feedwater
	//     supply is switched to the main feedwater system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:129-133
	//   - Hot-standby RCP cap: "a maximum of two out of four
	//     reactor coolant pumps (RCPs) may be secured."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-108
	runningRCPs := 0
	if p.nextPlant.ReactorBuilding.ReactorCoolantPump.Running {
		runningRCPs = 1
	}
	steamFlowFraction := float64(
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure,
	) / float64(SteamGeneratorShellPressure)
	p.nextPlant.PlantOperation = p.nextPlant.PlantOperation.Step(
		p.nextPlant.PlantOperation.Mode,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.Outlet,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.Pressure,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.TotalThermalPower(),
		CoreRatedThermalPower,
		p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.Running,
		runningRCPs,
		steamFlowFraction,
		p.nextPlant.TurbineBuilding.Turbine.Setpoint,
	)

	// Pressure-driven fluid flow integrates the freshly-stepped parts.
	// Graph nodes hold pointers into nextPlant; the mutations land in
	// nextPlant directly so Update() copies them into p.
	simulateFluidFlow(p.BuildGraphFor(p.nextPlant), TimeStep)
}

func (p *Plant) Update() {
	p.ReactorBuilding = p.nextPlant.ReactorBuilding
	p.AuxiliaryBuilding = p.nextPlant.AuxiliaryBuilding
	p.TurbineBuilding = p.nextPlant.TurbineBuilding
	p.Outside = p.nextPlant.Outside
	p.Electrical = p.nextPlant.Electrical
	p.PlantOperation = p.nextPlant.PlantOperation
}

func MakePlant() Plant {
	p := Plant{nextPlant: &Plant{}}
	p.ReactorBuilding.PlantControlRoom.AlarmSet = make(map[AlarmID]Alarm)
	p.ReactorBuilding.SteamGenerator.Volume = Volume{Val: 100000, Unit: u.Gallon}
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 80000, Unit: u.Gallon}
	// Initialise the RCS to the design full-load operating point so
	// the Doppler / moderator feedback loop starts in a sane state.
	// "Reactor coolant inlet temperature: 621°F (327°C); outlet:
	// 558°F (292°C)" — values here are inverted for RPV-side use
	// (cold leg from SG inlet / hot leg to SG outlet) per Table 3.4-1.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
	p.ReactorBuilding.ReactorPressureVessel.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	p.ReactorBuilding.ReactorPressureVessel.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	p.ReactorBuilding.ReactorPressureVessel.Pressure = RPVDesignPressure

	// Pressurizer design defaults: 60% liquid, 40% steam
	// bubble, pressure at PressurizerHeaterTarget. The RPS
	// reads Pressurizer pressure; without an explicit
	// initialisation the bubble is empty and Pressure
	// collapses to PressurizerBasePressure (1500 psi),
	// below the RPSLowPressurizerPressureTrip (1800 psi)
	// setpoint.
	//
	// HeatersOn is left at its zero value (false) so that
	// tests which override Pressure to a high value are not
	// re-clamped at the heater target. The plant-level
	// pressurizer control loop (planned for a future plan)
	// will manage HeatersOn and SprayOpen.
	p.ReactorBuilding.Pressurizer = Pressurizer{
		Volume:      Volume{Val: 1500, Unit: u.Gallon},
		Water:       Volume{Val: 900, Unit: u.Gallon}, // 60% liquid
		Pressure:    PressurizerHeaterTarget,
		SteamBubble: Volume{Val: 300, Unit: u.Gallon},
	}
	// The PRT starts half full of water to condense incoming
	// relief steam ("which contains sufficient water to condense
	// the steam", chapter_3/page_068.txt:32-34); capacity is a
	// derived figure (see PRTRuptureCapacity).
	p.ReactorBuilding.PressurizedReliefTank = Tank{
		Level:    Volume{Val: PRTRuptureCapacity / 2, Unit: u.Gallon},
		Capacity: Volume{Val: PRTRuptureCapacity, Unit: u.Gallon},
	}
	p.TurbineBuilding.HeaterDrainTank = Tank{
		Level:    Volume{Val: 0, Unit: u.Gallon},
		Capacity: Volume{Val: HeaterDrainTankCapacityGal, Unit: u.Gallon},
		Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
	}
	p.Outside = Outside{
		CondensateStorageTank: Tank{
			Level:    Volume{Val: 300000, Unit: u.Gallon},
			Capacity: Volume{Val: 450000, Unit: u.Gallon},
			Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
		},
		RiverTemp:          Temperature{Val: 75, Unit: u.Fahrenheit},
		AmbientWetBulbTemp: Temperature{Val: OutsideDefaultWetBulbF, Unit: u.Fahrenheit},
	}
	p.AuxiliaryBuilding.DieselGenerator = DieselGenerator{
		Speed: 0,
		Fuel: Tank{
			// Initialise full so the diesel can crank on the
			// first manual start. The state machine
			// (core/diesel.go:47) requires Fuel.Level > 0 to
			// transition Off -> Cranking.
			// Spec minimum "450 gallons at start of day"
			// (core/parts.go:1110-1111); the capacity is sized
			// above that so we leave head-room for the 7-day
			// continuous-rated-load requirement
			// (page_165.txt:120-122).
			Level:    Volume{Val: 500, Unit: u.Gallon},
			Capacity: Volume{Val: 500, Unit: u.Gallon},
			Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
		},
		CoolantTemp: Temperature{Val: 80, Unit: u.Fahrenheit},
	}
	p.Outside.TurbineAFWPump.DriveSource = DriveSteam
	p.Outside.MotorAFWPump.DriveSource = DriveElectric
	p.Outside.DieselAFWPump.DriveSource = DriveDiesel

	// Condenser / steam-system defaults (plans/p9_1.txt).
	// "Under normal operation the air is removed by steam jet air
	// ejectors or vacuum pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
	p.TurbineBuilding.Condenser.VacuumPumpRunning = true
	p.TurbineBuilding.Condenser.Pressure = CondenserDesignVacuum
	p.TurbineBuilding.Turbine.StopValve = Valve{Open: 1.0}
	p.Outside.CirculatingWaterPump = Pump{Running: true}

	// Circulating Water System defaults (plans/p31_1.txt).
	// At construction both CW pumps are running at design
	// flow, the source is the once-through river intake (no
	// cooling tower), the cooling-tower fan mirrors the CW
	// pump state, and the inlet temperature tracks
	// Outside.RiverTemp (set below in this MakePlant).
	//
	// Spec justification:
	//   - "Depending on the plant, the source of circulating
	//     water may be a river, lake, ocean, or cooling
	//     tower."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
	//   - "Circulating water for each main condenser is
	//     supplied by vertical, mixed flow, volute type
	//     circulating water pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:56-58
	p.Outside.CirculatingWaterSystem = CirculatingWaterSystem{
		SourceIsCoolingTower: false,
	}
	p.Outside.CoolingTower = CoolingTower{
		FanRunning: false,
		ApproachF:  CoolingTowerApproachF,
		Basin: Tank{
			Level:    Volume{Val: CoolingTowerBasinCapacityGal, Unit: u.Gallon},
			Capacity: Volume{Val: CoolingTowerBasinCapacityGal, Unit: u.Gallon},
		},
		ColdBasinTemp: Temperature{
			Val: OutsideDefaultWetBulbF + CoolingTowerApproachF,
			Unit: u.Fahrenheit,
		},
		MakeupValve:   Valve{Open: 1.0},
		BlowdownValve: Valve{Open: 1.0},
	}

	// Generator defaults (plans/p12_1.txt).
	//   - BreakerClosed = false at startup; the operator closes
	//     it after synchronisation.
	//   - PowerFactor = 0.9 per design point.
	//   - RatedMVA = 1370 MVA per the spec.
	// Spec: "Rating: 1370 mva"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
	// Spec: "Power factor: 0.9, short circuit ratio: 0.58"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.TurbineBuilding.Generator.PowerFactor = GeneratorPowerFactor
	p.TurbineBuilding.Generator.RatedMVA = GeneratorRatedMVA

	// Engineered Safety Features bus defaults.
	// "The engineered safety features buses are normally energized
	// from system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:81-82
	// Both train breakers default to closed so the buses follow the
	// spec's "automatic transfer of supplies" path
	// (page_165.txt:57-59) out of the box; the operator can open one
	// to isolate a train.
	p.Electrical.ESFBusA.BreakerClosed = true
	p.Electrical.ESFBusB.BreakerClosed = true

	// Offsite-source generators (one per train). At construction
	// both offsite supplies are available and energised, so the
	// ESF buses are powered from offsite; the diesel acts as a
	// redundant backup if offsite is lost.
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:23-36
	p.Electrical.OffsiteSourceA = PowerGenerator{
		Voltage:      DieselRatedVoltage,
		BreakerClosed: true,
	}
	p.Electrical.OffsiteSourceB = PowerGenerator{
		Voltage:      DieselRatedVoltage,
		BreakerClosed: true,
	}
	p.Electrical.OffsiteA = &p.Electrical.OffsiteSourceA
	p.Electrical.OffsiteB = &p.Electrical.OffsiteSourceB

	// Generator excitation and cooling defaults (plans/p30_1.txt).
	// At construction the exciter is not producing field current
	// (rotor not turning), the SCCW pump is not running, all four
	// hydrogen coolers are at full effectiveness, and the
	// design hydrogen pressure is held. The GeneratorExcitation
	// System and GeneratorCoolingSystem are embedded structs so
	// these assignments reach into the PowerGenerator directly.
	//
	// Spec justification:
	//   - "Excitation: Shaft-driven, air-cooled brushless
	//     exciter of the rotating rectifier type."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
	//   - "Hydrogen pressure: 60 or 75 psig"
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:20
	//   - "Demineralized cooling water is provided for the
	//     stator by a closed recirculating water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	p.TurbineBuilding.Generator.RegulatorVoltageSetpoint =
		GeneratorRatedVoltage
	p.TurbineBuilding.Generator.ExciterOutput = 0
	p.TurbineBuilding.Generator.ExciterActive = false
	p.TurbineBuilding.Generator.PilotExciterFrequency = 0
	p.TurbineBuilding.Generator.SCCWPumpRunning = false
	p.TurbineBuilding.Generator.SCCWFlow = 0
	p.TurbineBuilding.Generator.HydrogenPressure =
		GeneratorDesignHydrogenPressurePsig
	for i := range p.TurbineBuilding.Generator.
		HydrogenCoolerEffectiveness {
		p.TurbineBuilding.Generator.
			HydrogenCoolerEffectiveness[i] = 1.0
	}

	// Turbine setpoint defaults: 1.0 (full-load demand) when the
	// operator closes the breaker and 0.0 when off-line.
	// RampRate is set to the spec's "controlled selectable rate"
	// (5% per minute, ≈0.000833/s) at construction time.
	p.TurbineBuilding.Turbine.Setpoint = 0
	p.TurbineBuilding.Turbine.RampRate = TurbineLoadRunbackRate

	// Grid interface defaults (plans/p13_1.txt). At construction
	// the off-site grid is available (SAT can be energised),
	// the grid sits at 60 Hz / 24 kV, and the house load has
	// the design 3% fixed-plus-variable split.
	// Spec: "Output voltage: 24,000 to 26,000 volts, three-phase,
	// 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	p.ReactorBuilding.Grid = Grid{
		Frequency: GridRatedFrequency,
		Voltage:   GridRatedVoltage,
	}
	p.ReactorBuilding.StationTransformer = StationTransformer{}
	p.ReactorBuilding.HouseLoad = HouseLoad{
		FixedMW:    HouseLoadFixed,
		VariableMW: HouseLoadFixed * HouseLoadVariableFraction,
	}

	// CVCS defaults (plans/p14_1.txt). At construction the
	// charging pumps are off, the letdown valve is open, the
	// VCT is at design level, and the RCS is at the
	// hot-zero-power (HZP) boron concentration.
	// Spec: "The operating mode (boration or dilution) and the
	// magnitude of the boron concentration change required is
	// determined by comparison of the measured Tc (cold leg
	// temperature) with the programmed (function of power) Tc."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem = CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
	}
	// Charging/letdown balance is the normal steady-state; the
	// flows are populated from the field on the first Step.

	// RHRS defaults (plans/p15_1.txt). Both pumps are off and
	// the hot-leg isolation valves are closed at construction;
	// the operator aligns the trains and starts the pumps once
	// RCS temp ≤ 350°F and pressure < 400 psig.
	// Spec: "When the reactor coolant temperature has dropped
	// to 350°F (177°C) and the pressure has been reduced to
	// less than approximately 400 psig (28 bar), the RHR
	// equipment is placed in operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	p.AuxiliaryBuilding.ResidualHeatRemovalSystem = RHRS{
		TrainA: RHRSTrain{Pump: Pump{Running: false}},
		TrainB: RHRSTrain{Pump: Pump{Running: false}},
	}

	// SIS defaults (plans/p16_1.txt). The accumulators are
	// full and pressurised at construction; the HHSI pumps
	// are off and aligned to RWST suction; the RWST is full.
	// The model starts the HHSI pumps on the safety-injection
	// actuation signal in Plant.Step.
	//
	// Spec justification:
	//   - "The accumulator tanks are passive components
	//     normally requiring no maintenance, and are located
	//     inside the containment to provide rapid injection of
	//     water following a LOCA."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:11-14
	//   - "The HHSI and RHR pumps are aligned to take suction
	//     directly from the RWST."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	p.ReactorBuilding.SafetyInjectionSystem = SIS{
		Accumulator: Accumulator{
			Pressure: SISAccumulatorPrechargePressure,
			Level:    Volume{Val: SISAccumulatorPerTankCapacity.Float()*
					float64(SISAccumulatorCount), Unit: u.Gallon},
			Capacity: Volume{Val: SISAccumulatorPerTankCapacity.Float()*
					float64(SISAccumulatorCount), Unit: u.Gallon},
		},
		HHSIPumpRunning: false,
		HHSISuction:     SISSuctionRWST,
		RWSTLevel:       Volume{Val: SISRWSTCapacity, Unit: u.Gallon},
	}

	// EFWS defaults (plans/p17_1.txt). At construction the
	// primary tanks are full, both pumps in each subsystem
	// are off, and the EFWS is not active. The auto-start
	// logic in Plant.Step activates the system on low-low SG
	// level or a safety-injection signal.
	//
	// Spec justification:
	//   - "A primary emergency feedwater supply tank, to
	//     which the suction of the emergency feedwater pumps
	//     are normally aligned, is provided in each
	//     subsystem."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:48-51
	//   - "Each tank contains a quantity of condensate
	//     quality water sufficient to allow the plant to be
	//     maintained in hot standby for 13 hours then allow a
	//     5-hour cooldown of the plant to 350°F (176.7°C).
	//     The maximum permissible water temperature in the
	//     primary emergency feedwater supply tank is 120°F
	//     (48.9°C)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-59
	efwsSubsystem := func() EFWSSubsystem {
		return EFWSSubsystem{
			Tank: EFWSTank{
				Level:      EFWSPrimaryTankCapacity,
				Capacity:   EFWSPrimaryTankCapacity,
				Temperature: EFWSPrimaryTankMaxTemp,
				IsPrimary:  true,
			},
			MotorPump: EFWSPump{
				AFWPump: AFWPump{
					Pump: Pump{Running: false},
				},
				FlowModulatingValve: Valve{Open: 0},
				RecirculationValve:  Valve{Open: 1},
			},
			TurbinePump: EFWSPump{
				AFWPump: AFWPump{
					Pump:        Pump{Running: false},
					DriveSource: DriveSteam,
				},
				FlowModulatingValve: Valve{Open: 0},
				RecirculationValve:  Valve{Open: 1},
			},
		}
	}
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem = EFWS{
		SubsystemA: efwsSubsystem(),
		SubsystemB: efwsSubsystem(),
	}

	// CCWS defaults (plans/p18_1.txt). At construction the
	// inlet temperature sits at the steady-state setpoint
	// (105°F), no pumps are running, and the surge tank is
	// full. The simplified model brings pumps online in
	// Plant.Step when the safety-injection actuation signal
	// arrives.
	//
	// Spec justification:
	//   - "The component cooling water inlet temperature to
	//     various components during normal cooldown is permitted
	//     to increase to 120°F (49°C), however it must return to
	//     105°F (40°C) after four hours."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
	//   - "a standby pump in each train of the Safeguards
	//     subsystem is provided. A spare pump is also available
	//     in the Auxiliary subsystem."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:93-96
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem = CCWS{
		PumpRunning:    false,
		InletTemp:      Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
		OutletTemp:     Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
		SurgeTankLevel: CCWSSurgeTankCapacity,
	}

	// CSS defaults (plans/p19_1.txt). At construction the CSS
	// pumps are off, both the CSST and SAT are full, the spray
	// header is closed, and the containment sits at atmospheric
	// pressure. The simplified model starts the CSS in
	// Plant.Step when containment pressure rises above
	// CSSContainmentPressureSetpoint and the diesel reaches
	// ReadyToLoad.
	//
	// Spec justification:
	//   - "The system consists of a containment spray storage
	//     tank (CSST), a spray additive tank (SAT), two pumps,
	//     two heat exchangers, and a set of spray ring headers."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:14-18
	//   - "(T)he storage tanks are sized to provide approximately
	//     30 minutes of spray with spray additive."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-18
	p.AuxiliaryBuilding.ContainmentSpraySystem = ContainmentSpraySystem{
		Running:              false,
		SprayHeaderOpen:      false,
		CSSTLevel:            CSSTCapacity,
		SATLevel:             SATCapacity,
		CSSTLowLevelSetpoint: Volume{Val: CSSLowLevelSetpoint, Unit: u.Gallon},
		SuctionSump:          false,
		SumpPHAdjusted:       false,
		SprayFlow:            0,
	}
	p.ReactorBuilding.ContainmentPressure = AtmosphericPressure

	// RCFC defaults (plans/p20_1.txt). At construction all
	// four RCFC units are running at full speed, satisfying
	// the "minimum of two units" safeguard requirement. The
	// service water coil inlet is at the design service-water
	// temperature (60°F).
	//
	// Spec justification:
	//   - "In a containment, the system includes four fan units
	//     that operate in parallel. A minimum of two units must
	//     function to satisfy design requirements for both
	//     normal operation and post-accident operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:19-24
	//   - "Cooling water is supplied by the essential service
	//     water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
	for i := 0; i < RCFCUnitCount; i++ {
		p.AuxiliaryBuilding.RCFCUnits[i] = RCFCUnit{
			Running:             true,
			LowSpeed:            false,
			HeatTransferRate:    0,
			CoilInletTemp:       p.Outside.RiverTemp,
			CoilOutletTemp:      p.Outside.RiverTemp,
			BackdraftDamperOpen: true,
		}
	}

	// ContainmentBuilding defaults. Initialise at atmospheric
	// pressure and 120°F (typical normal-operation atmosphere
	// temperature); design pressure and free volume are the
	// spec-stated values.
	//
	// Spec justification:
	//   - "Transients resulting from the DBA serve as the basis
	//     for a containment design pressure of 60 psig (4.46
	//     bar)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
	//   - "The internal net free volume approximates 2,000,000
	//     cubic feet (56,600 m³)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_109.txt:11-12
	p.ReactorBuilding.ContainmentBuilding = ContainmentBuilding{
		Pressure:       AtmosphericPressure,
		Temperature:    Temperature{Val: 120.0, Unit: u.Fahrenheit},
		DesignPressure: ContainmentDesignPressure,
		FreeVolume:     ContainmentFreeVolume,
		// CIS defaults (plans/p23_1.txt). At construction
		// neither phase is latched, both manual flags are
		// false, and the design-qualification flags
		// (MissileProtected, SeismicClass1) are recorded
		// as true.
		//
		// Spec justification:
		//   - "missile protected, Seismic Class 1 and
		//     designed for a pressure equal to or greater
		//     than the design pressure of the containment."
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:135-138
		ContainmentIsolationSystem: ContainmentIsolationSystem{
			MissileProtected: true,
			SeismicClass1:    true,
		},
		// HDS defaults (plans/p25_1.txt). At construction the
		// containment atmosphere has 0% hydrogen, no alarm,
		// and no active recombiners or ventilation. Generation
		// becomes active in Plant.Step when the
		// safety-injection signal arrives; recombiners and
		// ventilation activate automatically through the HDS
		// step logic.
		//
		// Spec justification:
		//   - Hydrogen alarm: "Alarms at proper setpoints
		//     are provided."
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:14-15
		//   - Three-function defense: detect, ventilate,
		//     recombine.
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:11-26
		HydrogenDetectionSuppression: HydrogenDetectionSuppression{},
	}

	// SWS defaults (plans/p21_1.txt). At construction all
	// three essential service water pumps are running, the
	// inlet is at the source temperature, and the nonessential
	// headers are aligned (no isolation).
	//
	// Spec justification:
	//   - "The three essential service water pumps for each
	//     unit are typically rated at approximately 20,000
	//     gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	//   - "The nonessential equipment coolers are fed from
	//     separate branch headers that can be isolated from
	//     the main headers under accident conditions."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:23-26
	p.Outside.ServiceWaterSystem = ServiceWaterSystem{
		PumpRunning:           true,
		Flow:                 ServiceWaterDesignFlow,
		InletTemp:            p.Outside.RiverTemp,
		OutletTemp:           p.Outside.RiverTemp,
		NonessentialIsolated: false,
	}

	// SFPCS defaults (plans/p21_1.txt). Two pumps running,
	// header pressure in the normal range (55-75 psig), pool
	// temperature at the design value (100°F), purification
	// flow at 100 gpm.
	//
	// Spec justification:
	//   - "Normal operation requires two pumps with the third
	//     pump serving as a standby."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	//   - "The system pressure is maintained at 55 to 75 psig"
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-46
	p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem = SpentFuelPoolCoolingSystem{
		PumpRunning:       true,
		StandbyRunning:    false,
		HeaderPressure:    SFPCPressureNormalMin,
		PoolTemp:          Temperature{Val: 100.0, Unit: u.Fahrenheit},
		PurificationFlow:  SFPCPurificationFlow,
	}

	// Refueling operations defaults (plans/p24_1.txt). At
	// construction the cycle is complete (Phase 5), no cavity
	// flooding, no head/internals removed, the pit is full of
	// borated water at the refueling-cavity concentration,
	// and the core carries FuelAssembliesPerReactor
	// assemblies. A refueling outage starts by operator
	// action (reset Phase to RefuelingPhasePreparation and
	// set CavityFlooding).
	//
	// Spec justification:
	//   - Pit always full of water:
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:25-29
	//   - Borated water for subcriticality:
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:17-19
	//   - First-core assembly count:
	//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-39
	p.ReactorBuilding.RefuelingOperations = RefuelingOperations{
		Phase:                  RefuelingPhaseComplete,
		CavityFloodDepth:       0,
		CavityFlooded:          false,
		PitWaterLevel:          RefuelingCavityMaxDepth,
		PitBoronConcentration:  RefuelingPitBoronConcentration,
		FuelAssembliesInCore:   FuelAssembliesPerReactor,
		FuelAssembliesInPit:    0,
		FuelAssembliesShipped:  0,
		HeadRemoved:            false,
		InternalsRemoved:       false,
		WeightSafetyTripped:    false,
		FuelTransferred:        false,
	}

	// RPS defaults (plans/p26_1.txt). At construction the
	// reactor is not tripped, no manual trip has been
	// pressed, no ESFAS output is active.
	//
	// Spec justification:
	//   - "The RPS provides for manual initiation of reactor
	//     trip by operator action in the control room."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
	p.ReactorBuilding.ReactorProtectionSystem = ReactorProtectionSystem{}

	// Plant Electrical Systems defaults (plans/p27_1.txt).
	// At construction offsite power is available, both
	// diesel generators are stopped, both ESF buses are
	// energized from offsite, the instrument inverters are
	// powered, and there is no station blackout.
	//
	// Spec justification:
	//   - "The system auxiliary power is supplied from
	//     independent and electrically isolated buses of
	//     the off-site main distribution grid."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:45-47
	//   - "automatic transfer of supplies is provided on
	//     sensing loss of power"
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
	p.ReactorBuilding.PlantElectricalSystems = PlantElectricalSystems{
		OffsitePowerAvailable:   true,
		Diesel1Running:          false,
		Diesel2Running:          false,
		ESFBus1Available:        true,
		ESFBus2Available:        true,
		InverterPowerAvailable:  true,
		StationBlackout:         false,
	}

	// Plant Control Room defaults (plans/p28_1.txt). At
	// construction the control room has electrical power,
	// the PCC and auxiliary panels are operational, all
	// displays are active, control inputs are accepted,
	// and no annunciator alarm is sounding.
	//
	// Spec justification:
	//   - PCC: "all controls, displays and instrumentation
	//     required for normal power operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:23-26
	//   - Operator controls: "Controls have been segregated
	//     into operating areas which optimize the location
	//     of frequently used and important controls."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_146.txt:1-6
	p.ReactorBuilding.PlantControlRoom = PlantControlRoom{
		PowerAvailable:                true,
		PCCAvailable:                  true,
		AuxiliaryPanelsAvailable:      true,
		GraphicDisplaysAvailable:      true,
		DedicatedDisplaysAvailable:    true,
		AnnunciatorAlarm:              false,
		AlarmAcknowledged:             false,
		ControlInputsAvailable:        true,
		SafetySystemStationsAvailable: true,
	}

	// Plant Computer defaults (plans/p29_1.txt). At
	// construction the computer is on, all display formats
	// are active, SPDS is active, post-trip review has not
	// fired, and no alarm is sounding.
	//
	// Spec justification:
	//   - "The computer collects data from many locations
	//     within the plant and then processes it to provide
	//     a variety of information."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
	//   - SPDS: "Presents a succinct display account of the
	//     overall plant safety status."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	p.ReactorBuilding.PlantComputer = PlantComputer{
		DataAcquisitionAvailable:             true,
		MimicDisplaysAvailable:               true,
		BarChartDisplaysAvailable:            true,
		TabularDisplaysAvailable:             true,
		GraphicalDisplaysAvailable:           true,
		DataTrendsAvailable:                  true,
		PolarDisplaysAvailable:               true,
		SPCSActive:                           true,
		PostTripReviewPrinted:                false,
		SequenceOfEventsRecording:            true,
		AlarmActive:                          false,
		BypassedInoperableStatusIndication:   true,
	}

	// Reactor makeup control valve defaults: closed at startup
	// (the operator opens it during cooldown / VCT-level
	// recovery).
	// Spec: "Because of the influx of makeup water from the
	// reactor makeup control system [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
	p.ReactorBuilding.ReactorMakeupControlValve = MakeupValve{
		Open: 0,
		Flow: Volume{Val: 0, Unit: u.Gallon},
	}

	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration =
		CVCSInitialBoron
	// Control rods start fully withdrawn (Insertion=0) for normal
	// operation. A trip forces full insertion (Insertion=1.0) via
	// the clamp in ReactorPressureVessel.Step (plans/x/trip.md).
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0.0

	// Main Feedwater Control System defaults (plans/p22_1.txt).
	// At construction the main feedwater control valve is
	// closed, the regulator is stopped, and the level setpoint
	// sits at FeedwaterLevelSetpoint. Plant.Step latches the
	// regulator's Running state from MainFeedwaterPump.Running.
	//
	// Spec justification:
	//   - "Feedwater flow to each steam generator is controlled
	//     by a feedwater regulator in each feedwater line. The
	//     regulator is controlled by steam generator level,
	//     steam flow and feedwater flow."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
	p.TurbineBuilding.MainFeedwaterControlValve = Valve{Open: 0}
	p.TurbineBuilding.FeedwaterRegulator = FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Running:       false,
		ValveDemand:   0,
	}

	return p
}

// BuildGraph returns a PlantGraph whose node refs point into p. Used
// outside the Step loop (ExportDot, tests) where mutating p is fine.
func (p *Plant) BuildGraph() *PlantGraph { return p.BuildGraphFor(p) }

// BuildGraphFor is the same as BuildGraph but lets the caller target an
// arbitrary *Plant (typically nextPlant during Step). Internal helpers
// use this to avoid mutating the current plant directly.
//
// The connections modelled are the primary loop, the secondary loop,
// and the AFWS suction line, drawn from
// specs/the_westinghouse_pressurized_water_react/glossary.md
// (cold leg, hot leg, hot standby) and
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt
// (AFWS pumps, common pipe header).
func (p *Plant) BuildGraphFor(target *Plant) *PlantGraph {
	g := &PlantGraph{Nodes: map[string]GraphNode{}}

	add := func(id, kind string, ref interface{}) {
		g.Nodes[id] = GraphNode{ID: id, Kind: kind, Ref: ref}
	}
	edge := func(from, to string, valve *Valve, pump *Pump) {
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Valve: valve, Pump: pump})
	}

	add("RPV", "ReactorPressureVessel", &target.ReactorBuilding.ReactorPressureVessel)
	add("Pressurizer", "Pressurizer", &target.ReactorBuilding.Pressurizer)
	add("SG", "SteamGenerator", &target.ReactorBuilding.SteamGenerator)
	add("RCP", "ReactorCoolantPump", &target.ReactorBuilding.ReactorCoolantPump)
	edge("RPV", "SG", nil, &target.ReactorBuilding.ReactorCoolantPump) // hot leg through RCP
	edge("SG", "RPV", nil, nil)                                        // cold leg

	edge("RPV", "Pressurizer", &target.ReactorBuilding.SafetyValve, nil)
	edge("Pressurizer", "RPV", nil, nil)

	add("Condenser", "Condenser", &target.TurbineBuilding.Condenser)
	add("CST", "CondensateStorageTank", &target.Outside.CondensateStorageTank)
	add("CoolingTower", "CoolingTower", &target.Outside.CoolingTower)
	// Hot return water condenser → tower basin; makeup refills
	// the basin. The CW pumps draw from the basin when the
	// tower is the configured source.
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:1-3
	edge("Condenser", "CoolingTower", nil, nil)
	edge("CoolingTower", "CoolingTower",
		&target.Outside.CoolingTower.MakeupValve, nil)
	add("HDT", "HeaterDrainTank", &target.TurbineBuilding.HeaterDrainTank)
	add("LPHeaterDrain", "LowPressureHeaterDrain",
		&target.TurbineBuilding.LowPressureHeater)
	add("HPHeaterDrain", "HighPressureHeaterDrain",
		&target.TurbineBuilding.HighPressureHeater)
	edge("SG", "Condenser", &target.TurbineBuilding.MakeupValve, nil)
	edge("Condenser", "CST", &target.TurbineBuilding.RecirculateValve, nil)
	edge("CST", "HDT", nil, &target.TurbineBuilding.CondensatePump)
	edge("HDT", "SG", nil, &target.TurbineBuilding.MainFeedwaterPump)

	// LP heater drains cascade back to the condenser.
	// "Drains from the four low-pressure feedwater heaters are
	// cascaded back to the condenser."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:14-15
	edge("LPHeaterDrain", "Condenser", nil, nil)

	// HP heater drains route to the deaerator tank (HeaterDrainTank).
	// "Drains from the high-pressure feedwater heaters are routed
	// to the deaerator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-14
	edge("HPHeaterDrain", "HDT", nil, &target.TurbineBuilding.HeaterDrainPump)

	add("DieselAFWPump", "DieselAFWPump", &target.Outside.DieselAFWPump.Pump)
	add("MotorAFWPump", "MotorAFWPump", &target.Outside.MotorAFWPump.Pump)
	add("TurbineAFWPump", "TurbineAFWPump", &target.Outside.TurbineAFWPump.Pump)
	edge("CST", "DieselAFWPump", &target.Outside.DieselAFWPump.CSTValve, nil)
	edge("DieselAFWPump", "SG", &target.Outside.DieselAFWPump.ServiceWaterValve, nil)
	edge("CST", "MotorAFWPump", &target.Outside.MotorAFWPump.CSTValve, nil)
	edge("MotorAFWPump", "SG", &target.Outside.MotorAFWPump.ServiceWaterValve, nil)
	edge("CST", "TurbineAFWPump", &target.Outside.TurbineAFWPump.CSTValve, nil)
	edge("TurbineAFWPump", "SG", &target.Outside.TurbineAFWPump.ServiceWaterValve, nil)

	// Standalone nodes without simulation edges, included so the
	// P&ID graph and Graphviz layout are complete.
	add("SISAccumulators", "SafetyInjectionSystem", nil)
	add("EFWSTank", "EmergencyFeedwaterSystem", &target.AuxiliaryBuilding.EmergencyFeedwaterSystem)
	add("CCWSHX", "ComponentCoolingWaterSystem", &target.AuxiliaryBuilding.ComponentCoolingWaterSystem)
	add("RHRS_A", "ResidualHeatRemovalSystem", &target.AuxiliaryBuilding.ResidualHeatRemovalSystem)
	add("RHRS_B", "ResidualHeatRemovalSystem", &target.AuxiliaryBuilding.ResidualHeatRemovalSystem)
	add("SFP", "SpentFuelPool", &target.AuxiliaryBuilding.SpentFuelPoolCoolingSystem)
	add("Diesel", "DieselGenerator", &target.AuxiliaryBuilding.DieselGenerator)
	add("FuelTank", "FuelTank", nil)
	add("Transformer", "StationTransformer", &target.ReactorBuilding.StationTransformer)
	add("ESF_A", "ESFBus", &target.Electrical.ESFBusA)
	add("ESF_B", "ESFBus", &target.Electrical.ESFBusB)
	add("Grid", "OffsiteGrid", &target.ReactorBuilding.Grid)

	return g
}

// MarshalJSON omits the internal nextPlant pointer to avoid
// circular references. All public state is included.
func (p *Plant) MarshalJSON() ([]byte, error) {
	type plant Plant
	p2 := plant(*p)
	p2.nextPlant = nil
	return json.Marshal(&p2)
}

// UnmarshalJSON restores a Plant from JSON. The internal
// nextPlant pointer is left nil (rebuilt by Step).
func (p *Plant) UnmarshalJSON(data []byte) error {
	type plant Plant
	var p2 plant
	if err := json.Unmarshal(data, &p2); err != nil {
		return err
	}
	*p = Plant(p2)
	p.nextPlant = nil
	return nil
}

// ExportDot writes the plant's graph in Graphviz DOT format.
func (p *Plant) ExportDot(w io.Writer) error {
	return p.BuildGraph().ExportDot(w)
}
