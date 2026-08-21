package core

import (
	"io"

	u "github.com/bcicen/go-units"
)

func (r *ReactorBuilding) Step() ReactorBuilding {
	ret := *r
	ret.Pressurizer = ret.Pressurizer.Step(&ret.PORV)
	// Reactor makeup control valve: the operator sets Open
	// (0..1); Flow is the per-step volume delivered to the
	// VCT. Derived once per step so downstream consumers see
	// a stable value.
	//
	// Spec: "Because of the influx of makeup water from the
	// reactor makeup control system [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
	ret.ReactorMakeupControlValve.Flow = u.NewValue(
		ret.ReactorMakeupControlValve.Open*
			ReactorMakeupDesignFlow.Float(),
		u.Gallon,
	)
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
	letdownValveOpen := ret.ChemicalVolumeControlSystem.LetdownFlow.Float() > 0
	ret.ChemicalVolumeControlSystem = ret.ChemicalVolumeControlSystem.Step(
		ret.ReactorCoolantPump.Running,
		letdownValveOpen,
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
		ret.ChemicalVolumeControlSystem.BoronConcentration
	rpv := ret.ReactorPressureVessel.Step(ret.ReactorCoolantPump.Running)
	ret.ReactorPressureVessel = rpv

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
	dt := TimeStep
	rcsOutletF := ret.ReactorPressureVessel.Outlet.Float()
	rcsPressure := float64(ret.Pressurizer.Pressure)
	ret.ComponentCoolingWaterSystem = ret.ComponentCoolingWaterSystem.Step()
	ret.ResidualHeatRemovalSystem = ret.ResidualHeatRemovalSystem.Step(
		ret.ReactorPressureVessel.Outlet,
		rcsPressure,
		ret.ComponentCoolingWaterSystem.OutletTemp,
	)
	if ret.ResidualHeatRemovalSystem.HasTakenOver {
		rejectionW := float64(
			ret.ResidualHeatRemovalSystem.TrainA.HeatExchanger.HeatTransferRate,
		) + float64(
			ret.ResidualHeatRemovalSystem.TrainB.HeatExchanger.HeatTransferRate,
		)
		decayW := float64(ret.ReactorPressureVessel.ThermalPower)
		netW := decayW - rejectionW
		rcsMassKg := 3_500_000.0 * 0.453592
		deltaTK := netW / (rcsMassKg * 4186.0) * dt.Seconds()
		ret.ReactorPressureVessel.Outlet = u.NewValue(
			rcsOutletF+deltaTK*9.0/5.0, u.Fahrenheit,
		)
		ret.SteamGenerator.HeatInput = 0
	} else {
		ret.SteamGenerator.HeatInput = rpv.ThermalPower
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
	rhrsActive := ret.ResidualHeatRemovalSystem.Running &&
		ret.ResidualHeatRemovalSystem.HasTakenOver
	ret.EmergencyFeedwaterSystem = ret.EmergencyFeedwaterSystem.Step(
		ret.ReactorPressureVessel.Outlet, rhrsActive,
	)
	ret.EFWSFeedwaterFlow = ret.EmergencyFeedwaterSystem.EFWSFeedwaterFlow()

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
	cssRunning := ret.ContainmentSpraySystem.Running
	if cssRunning && ret.ContainmentSpraySystem.SprayHeaderOpen {
		// Spray condenses steam: effectiveness × spray flow
		// × (P - P_atm) drag toward atmospheric.
		excess := csP - float64(AtmosphericPressure)
		if excess > 0 {
			csP -= CSSSprayEffectiveness *
				ret.ContainmentSpraySystem.SprayFlow * excess *
				dt.Seconds()
			if csP < float64(AtmosphericPressure) {
				csP = float64(AtmosphericPressure)
			}
		}
	}
	ret.ContainmentPressure = Pressure(csP)
	ret.ContainmentSpraySystem = ret.ContainmentSpraySystem.Step()

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
	if ret.Generator.BreakerClosed && rated {
		ret.Generator.Voltage = GeneratorRatedVoltage
		ret.Generator.Frequency = GeneratorRatedFrequency
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
		ret.Condenser.CondensingRate = u.NewValue(
			steamMassFromPower(ret.Turbine.RealPower),
			u.Gallon,
		)
	} else {
		ret.Condenser.CondensingRate = u.NewValue(0, u.Gallon)
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
	fwIn := u.NewValue(120, u.Fahrenheit)
	lpSteam := ret.Turbine.ExtractionSteamFlow.Float() * MSRSixthStageExtractionFraction
	hpSteam := ret.Turbine.ExtractionSteamFlow.Float() * MSRDesignExtractionFraction
	lp := ret.LowPressureHeater
	for i := 0; i < 4; i++ {
		lp = lp.Step(fwIn.Float(), lpSteam)
		fwIn = u.NewValue(lp.Inlet.Float()+lp.TemperatureRise.Float(),
			u.Fahrenheit)
	}
	ret.LowPressureHeater = lp

	// MSR moisture drain to hotwell (feedwater system).
	// Spec: "where the water is removed and drained to the
	// feedwater system."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:62-63
	msrDrain := ret.Turbine.MoistureSeparatorReheater.DrainFlow.Float()
	ret.Condenser.HotwellLevel = u.NewValue(
		ret.Condenser.HotwellLevel.Float()+msrDrain,
		u.Gallon,
	)

	// HP train: two stages.
	hp := ret.HighPressureHeater
	for i := 0; i < 2; i++ {
		hp = hp.Step(fwIn.Float(), hpSteam)
		fwIn = u.NewValue(hp.Inlet.Float()+hp.TemperatureRise.Float(),
			u.Fahrenheit)
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
	ret.HeaterDrainTank.Level = u.NewValue(newHDTLevel, u.Gallon)

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
		ret.HeaterDrainTank.Level = u.NewValue(newHDTLevel, u.Gallon)
	}

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
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return frac * CondenserDesignSteamLoadGalPer
}

// Plant represents all state relevant to the facility.
type Plant struct {
	ReactorBuilding
	TurbineBuilding
	Outside
	Electrical
	nextPlant *Plant
}

func (p *Plant) Step() {
	// Run Outside.Step first so the diesel generator state is in
	// nextPlant by the time Electrical.Step reads it. The cross-building
	// source pointers must point at nextPlant (where the fresh state is
	// being written), not at p (the current state being read).
	p.nextPlant.Outside = p.Outside.Step()
	p.Electrical.DieselGenA = &p.nextPlant.Outside.DieselGenerator
	p.Electrical.DieselGenB = &p.nextPlant.Outside.DieselGenerator

	p.nextPlant.ReactorBuilding = p.ReactorBuilding.Step()

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
	if p.Outside.DieselGenerator.StartSignal == DieselStartSafetyInjection {
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
	efws := &p.nextPlant.ReactorBuilding.EmergencyFeedwaterSystem
	if !efws.Active {
		sgWaterLow := p.ReactorBuilding.SteamGenerator.Water.Float() <
			EFWSLevelLowLow
		// The simplified model uses a single combined
		// SteamGenerator field, so "low-low in any one SG"
		// reduces to a single boolean and "low-low in any
		// two SGs" becomes equivalent to "low-low in any
		// one SG". The per-loop SGs are not modelled
		// separately at this fidelity band.
		if p.Outside.DieselGenerator.StartSignal ==
			DieselStartSafetyInjection || sgWaterLow {
			if !efws.SubsystemA.MotorPumpRunning {
				efws.SubsystemA.MotorPumpRunning = true
				efws.MotorPumpStartCount++
			}
			if !efws.SubsystemB.MotorPumpRunning {
				efws.SubsystemB.MotorPumpRunning = true
				efws.MotorPumpStartCount++
			}
			efws.Active = true
		}
		if sgWaterLow {
			// Single-SG low-low simultaneously satisfies
			// the "two SG" condition in the simplified
			// model, so the turbine pump starts too.
			if !efws.SubsystemA.TurbinePumpRunning {
				efws.SubsystemA.TurbinePumpRunning = true
				efws.TurbinePumpStartCount++
			}
			if !efws.SubsystemB.TurbinePumpRunning {
				efws.SubsystemB.TurbinePumpRunning = true
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
	// Motor-driven pump DriveReady tracks the ESF bus (wired
	// by the existing AFWPump.DriveReady path through the
	// diesel). When the diesel reaches RatedToLoad the bus is
	// energised; the motor-driven pump's DriveReady follows.
	if p.Outside.DieselGenerator.ReadyToLoad {
		efws.SubsystemA.MotorPump.DriveReady = true
		efws.SubsystemB.MotorPump.DriveReady = true
	}

	// CCWS pumps derive power from the ESF bus; the diesel
	// energises them through the same path as the EFWS motor
	// pumps. Set PumpRunning when the diesel is ready so the
	// CCWS Step applies the recovery-lag to the inlet.
	//
	// Spec justification:
	//   - "The CCWS Safeguards subsystem is considered an
	//     engineered safeguards system because it is required
	//     to remove decay heat during postaccident and to
	//     provide cooling water to safeguards equipment."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:135-138
	if p.Outside.DieselGenerator.ReadyToLoad {
		p.nextPlant.ReactorBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	}

	// CSS auto-start: containment spray is initiated in
	// response to a containment high-pressure signal. The
	// cross-building pass has access to the live
	// ContainmentPressure (computed by ReactorBuilding.Step
	// above) and the diesel state. With pumps ready, the CSS
	// transitions to the Initial Injection Mode (Running=true,
	// SprayHeaderOpen=true, SAT isolation valves "B" open).
	//
	// Spec justification:
	//   - "Initial Injection Mode – Containment spray is
	//     initiated automatically in response to a containment
	//     high pressure signal. This signal starts the spray
	//     pumps, opens the spray header isolation valves "A",
	//     and opens the SAT isolation valves "B"."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
	css := &p.nextPlant.ReactorBuilding.ContainmentSpraySystem
	if !css.Running &&
		p.ReactorBuilding.ContainmentPressure > CSSContainmentPressureSetpoint &&
		p.Outside.DieselGenerator.ReadyToLoad {
		css.Running = true
		css.SprayHeaderOpen = true
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
	locaSensed := p.Outside.DieselGenerator.StartSignal ==
		DieselStartSafetyInjection
	cbTemp := p.ReactorBuilding.ContainmentBuilding.Temperature
	for i := 0; i < RCFCUnitCount; i++ {
		p.nextPlant.ReactorBuilding.RCFCUnits[i] =
			p.nextPlant.ReactorBuilding.RCFCUnits[i].Step(
				locaSensed, cbTemp,
			)
	}
	var rcfcHeatOut Watts
	for i := 0; i < RCFCUnitCount; i++ {
		rcfcHeatOut += Watts(
			p.nextPlant.ReactorBuilding.RCFCUnits[i].HeatTransferRate,
		)
	}
	p.nextPlant.ReactorBuilding.ContainmentBuilding =
		p.nextPlant.ReactorBuilding.ContainmentBuilding.Step(
			Watts(0), rcfcHeatOut,
		)

	// SWS: the service water picks up the heat rejected by the
	// RCFC coils. In the simplified model the lumped SWS flow
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
	p.nextPlant.ReactorBuilding.ServiceWaterSystem =
		p.nextPlant.ReactorBuilding.ServiceWaterSystem.Step(rcfcHeatOut)

	// SFPCS: pump auto-start and purification-flow update.
	//
	// Spec justification:
	//   - "Normal operation requires two pumps with the third
	//     pump serving as a standby."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	//   - "passing approximately 100 gpm (23 m3/hr) of each
	//     loop's flow through a filter and demineralizer."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	p.nextPlant.ReactorBuilding.SpentFuelPoolCoolingSystem =
		p.nextPlant.ReactorBuilding.SpentFuelPoolCoolingSystem.Step()

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
	p.nextPlant.TurbineBuilding = p.TurbineBuilding.StepWithSteam(
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure,
		cwPumpRunning,
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
	// gates the effective flow into the SG.
	p.nextPlant.TurbineBuilding.MainFeedwaterControlValve.Open =
		reg.ValveDemand

	p.nextPlant.Electrical = p.Electrical.Step()

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
		if loadFraction < 0 {
			loadFraction = 0
		}
		if loadFraction > 1 {
			loadFraction = 1
		}
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
		if net < 0 {
			net = 0
		}
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
		p.nextPlant.Outside.DieselGenerator.ReadyToLoad
	p.nextPlant.Outside.MotorAFWPump.DriveReady =
		p.nextPlant.Electrical.ESFBusA.Energized
	p.nextPlant.Outside.TurbineAFWPump.DriveReady =
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure > SteamGeneratorShellPressure/100

	// Plumb the CW inlet temperature from the outside loop. The
	// CirculatingWaterPump.Temperature field models the CW supply
	// header; in steady state this tracks RiverTemp.
	p.nextPlant.TurbineBuilding.Condenser.CirculatingWaterIn =
		p.Outside.CirculatingWaterPump.Temperature

	// Pressure-driven fluid flow integrates the freshly-stepped parts.
	// Graph nodes hold pointers into nextPlant; the mutations land in
	// nextPlant directly so Update() copies them into p.
	simulateFluidFlow(p.BuildGraphFor(p.nextPlant), TimeStep)
}

func (p *Plant) Update() {
	p.ReactorBuilding = p.nextPlant.ReactorBuilding
	p.TurbineBuilding = p.nextPlant.TurbineBuilding
	p.Outside = p.nextPlant.Outside
	p.Electrical = p.nextPlant.Electrical
}

func MakePlant() Plant {
	p := Plant{nextPlant: &Plant{}}
	p.ReactorBuilding.SteamGenerator.Volume = u.NewValue(100000, u.Gallon)
	p.ReactorBuilding.SteamGenerator.Water = u.NewValue(80000, u.Gallon)
	// Initialise the RCS to the design full-load operating point so
	// the Doppler / moderator feedback loop starts in a sane state.
	// "Reactor coolant inlet temperature: 621°F (327°C); outlet:
	// 558°F (292°C)" — values here are inverted for RPV-side use
	// (cold leg from SG inlet / hot leg to SG outlet) per Table 3.4-1.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
	p.ReactorBuilding.ReactorPressureVessel.Inlet = u.NewValue(CoreReferenceInletF, u.Fahrenheit)
	p.ReactorBuilding.ReactorPressureVessel.Outlet = u.NewValue(CoreReferenceOutletF, u.Fahrenheit)
	p.ReactorBuilding.ReactorPressureVessel.Pressure = RPVDesignPressure
	p.TurbineBuilding.HeaterDrainTank = Tank{
		Level:    u.NewValue(0, u.Gallon),
		Capacity: u.NewValue(13120, u.Gallon),
		Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
	}
	p.Outside = Outside{
		CondensateStorageTank: Tank{
			Level:    u.NewValue(300000, u.Gallon),
			Capacity: u.NewValue(450000, u.Gallon),
			Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
		},
		DieselGenerator: DieselGenerator{
			Speed: 0,
			Fuel: Tank{
				Level: u.NewValue(0, u.Gallon),
				// Can run feed pump for 24 hours
				// Can run generators for 4 days
				Capacity: u.NewValue(500, u.Gallon),
				Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
			},
			CoolantTemp: u.NewValue(80, u.Fahrenheit),
		},
		RiverTemp: u.NewValue(75, u.Fahrenheit),
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
	p.ReactorBuilding.ChemicalVolumeControlSystem = CVCS{
		Mode:               CVCSVolumeControl,
		VCTLevel:           u.NewValue(VCTDesignLevel, u.Gallon),
		BoronConcentration: CVCSInitialBoron,
	}
	// Letdown is flowing at construction (set LetdownFlow so
	// the Step function's letdownValveOpen sentinel works).
	p.ReactorBuilding.ChemicalVolumeControlSystem.LetdownFlow = CVCSDesignLetdownFlow

	// RHRS defaults (plans/p15_1.txt). Both pumps are off and
	// the hot-leg isolation valves are closed at construction;
	// the operator aligns the trains and starts the pumps once
	// RCS temp ≤ 350°F and pressure < 400 psig.
	// Spec: "When the reactor coolant temperature has dropped
	// to 350°F (177°C) and the pressure has been reduced to
	// less than approximately 400 psig (28 bar), the RHR
	// equipment is placed in operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	p.ReactorBuilding.ResidualHeatRemovalSystem = RHRS{
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
			Level:    u.NewValue(
				SISAccumulatorPerTankCapacity.Float()*
					float64(SISAccumulatorCount),
				u.Gallon,
			),
			Capacity: u.NewValue(
				SISAccumulatorPerTankCapacity.Float()*
					float64(SISAccumulatorCount),
				u.Gallon,
			),
		},
		HHSIPumpRunning: false,
		HHSISuction:     SISSuctionRWST,
		RWSTLevel:       u.NewValue(SISRWSTCapacity, u.Gallon),
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
	p.ReactorBuilding.EmergencyFeedwaterSystem = EFWS{
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
	p.ReactorBuilding.ComponentCoolingWaterSystem = CCWS{
		PumpRunning:    false,
		InletTemp:      u.NewValue(CCWSInletSteadyStateTemp, u.Fahrenheit),
		OutletTemp:     u.NewValue(CCWSInletSteadyStateTemp, u.Fahrenheit),
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
	p.ReactorBuilding.ContainmentSpraySystem = ContainmentSpraySystem{
		Running:              false,
		SprayHeaderOpen:      false,
		CSSTLevel:            CSSTCapacity,
		SATLevel:             SATCapacity,
		CSSTLowLevelSetpoint: u.NewValue(CSSLowLevelSetpoint, u.Gallon),
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
		p.ReactorBuilding.RCFCUnits[i] = RCFCUnit{
			Running:             true,
			LowSpeed:            false,
			HeatTransferRate:    0,
			CoilInletTemp:       u.NewValue(RCFCSWInletTemp, u.Fahrenheit),
			CoilOutletTemp:      u.NewValue(RCFCSWInletTemp, u.Fahrenheit),
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
		Temperature:    u.NewValue(120.0, u.Fahrenheit),
		DesignPressure: ContainmentDesignPressure,
		FreeVolume:     ContainmentFreeVolume,
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
	p.ReactorBuilding.ServiceWaterSystem = ServiceWaterSystem{
		PumpRunning:           true,
		Flow:                 ServiceWaterDesignFlow,
		InletTemp:            ServiceWaterInletTemp,
		OutletTemp:           ServiceWaterInletTemp,
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
	p.ReactorBuilding.SpentFuelPoolCoolingSystem = SpentFuelPoolCoolingSystem{
		PumpRunning:       true,
		StandbyRunning:    false,
		HeaderPressure:    SFPCPressureNormalMin,
		PoolTemp:          u.NewValue(100.0, u.Fahrenheit),
		PurificationFlow:  SFPCPurificationFlow,
	}

	// Reactor makeup control valve defaults: closed at startup
	// (the operator opens it during cooldown / VCT-level
	// recovery).
	// Spec: "Because of the influx of makeup water from the
	// reactor makeup control system [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
	p.ReactorBuilding.ReactorMakeupControlValve = MakeupValve{
		Open: 0,
		Flow: u.NewValue(0, u.Gallon),
	}

	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration =
		CVCSInitialBoron

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

	return g
}

// ExportDot writes the plant's graph in Graphviz DOT format.
func (p *Plant) ExportDot(w io.Writer) error {
	return p.BuildGraph().ExportDot(w)
}
