package core

import (
	"io"

	u "github.com/bcicen/go-units"
)

func (r *ReactorBuilding) Step() ReactorBuilding {
	ret := *r
	ret.Pressurizer = ret.Pressurizer.Step(&ret.PORV)
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
	ret.ChemicalVolumeControlSystem = ret.ChemicalVolumeControlSystem.Step(
		ret.ReactorCoolantPump.Running,
		0,
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
	ret.SteamGenerator.HeatInput = rpv.ThermalPower
	ret.SteamGenerator = ret.SteamGenerator.Step()
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
		VCTLevel:           VCTDesignLevel,
		BoronConcentration: CVCSInitialBoron,
		LetdownValveOpen:   true,
	}
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration =
		CVCSInitialBoron

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
