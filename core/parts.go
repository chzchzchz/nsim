package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

type Part struct {
	Name   string
	Damage float32
	Temperature
	// Installation is time of installation since epoch.
	Installation time.Duration
}

type Fluid struct {
	Temperature
	Volume
	Pressure
}

type Tank struct {
	Part
	Level    Volume
	Capacity Volume
	Pressure
}

type Pump struct {
	Part
	// Running is set by the operator between steps; stopped pumps
	// freeze primary-side Inlet tracking in the RPV step (plans/p8_1.txt).
	Running bool
}

type Turbine struct {
	Part
	Velocity

	State TurbineState

	// StopValve is the main-steam stop valve. Open=1 is normal
	// operation; Open=0 is the trip state (auto-closed by reactor
	// trip, overspeed, or operator manual trip).
	//
	// Spec justification:
	//   - "Release of this oil, called auto stop oil, by any one of
	//     the turbine trip devices [...] causes all steam control
	//     valves to close."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
	//   - "Stop valve and check valve in each steam generator outlet
	//     main steam line." (item 3 of the protective features list)
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:66-69
	StopValve Valve

	// DumpValve is the steam dump valve to the condenser. Open=0 is
	// normal operation; Open=1 dumps steam to the condenser during
	// trip or load rejection.
	//
	// Spec justification:
	//   - "Condenser steam dump valves" (item 8 of the protective
	//     features list)
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:62
	//   - "These valves open under a sudden reduction in turbine-
	//     generator load (in excess of 10%) or a turbine trip."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:5-11
	DumpValve Valve

	SteamLossTimer time.Duration
	RealPower      Watts

	// Setpoint is the operator-selected load demand as a
	// fraction of TurbineRatedkW (0..1). "After generator
	// breaker closure, the ability to increase or decrease
	// electrical load at a controlled selectable rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_202.txt:49-51
	// Resolution: 0.5 percent load (GeneratorSetpointResolution).
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:82-83
	Setpoint float64

	// RampRate is the per-second fraction of TurbineRatedkW at
	// which RealPower may change toward Setpoint. Equal to
	// TurbineLoadRunbackRate by default; sized so a full-load
	// runback takes ~20 minutes, matching the typical DEHM
	// "controlled selectable rate" behaviour.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
	RampRate float64

	// LoadRunback is set by the operator or reactor-protection
	// signals (reactor trip, dropped rod, high kw/ft, low DNBR)
	// to drive Setpoint toward zero at RampRate. The spec
	// describes a load-runback initiator list of:
	//   - dropped rod signal
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
	//   - high kw/ft signal
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:29
	//   - low departure from nucleate boiling ratio signal
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:29
	//   - rapid decrease in nuclear flux or rod bottom circuitry
	//     (load-limit path)
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:93-96
	// Plant.Step collapses the four signals into a single bit;
	// the protective-layer output of the DEHM is "shed load at
	// the controlled rate", which this flag represents.
	LoadRunback bool

	// OPCAlarm is set by the turbine step when shaft velocity
	// exceeds TurbineOPCALarmFraction of TurbineRatedSpeed
	// (103%). The OPC controller raises this as a purely
	// informative alarm; no automatic action is taken at this
	// level.
	// "OPC alarm (103%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
	OPCAlarm bool

	// OPCTrip is latched when shaft velocity exceeds
	// TurbineOPCTripFraction of TurbineRatedSpeed (110%). Once
	// latched, the stop valve is held closed until reset.
	// "OPC trip (110%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
	OPCTrip bool

	// ExtractionSteamFlow is the total extraction steam available
	// to the feedwater heaters and MSR reheater. It is computed
	// per step as a fraction of the turbine's equivalent steam
	// flow.
	ExtractionSteamFlow Volume

	// HPExhaustQuality is the steam quality (0..1) at the HP
	// turbine exhaust, before the MSR. The spec notes that
	// moisture is mechanically separated at this point
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:31-35).
	HPExhaustQuality float64

	// HPExhaustPressure is the steam pressure at the HP turbine
	// exhaust (MSR inlet).
	HPExhaustPressure Pressure

	MoistureSeparatorReheater // embedded MSR state
}

// MoistureSeparatorReheater models one combined moisture separator-
// reheater assembly between the HP exhaust and the LP turbines.
// The spec describes these as single assemblies that perform both
// moisture removal and steam reheating.
//
// Spec justification:
//   - "Combination moisture separator-reheater assemblies are
//     provided between the high- and low-pressure elements to dry
//     and superheat the steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_198.txt:43-45
//   - "Typical combined moisture separator-reheaters [...] remove
//     the moisture in the wet steam exhausting from the high-
//     pressure element exhaust and reheat the steam to over 100°F
//     (55°C) superheat."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:54-55
//   - "The wet steam enters the moisture removal section and rises
//     through chevron-type moisture-separators where the water is
//     removed and drained to the feedwater system."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:60-62
//   - "The dried steam then passes through the reheater section
//     where it is reheated by the highest pressure extraction steam
//     and by main steam withdrawn before the throttle valves."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:64-66
type MoistureSeparatorReheater struct {
	Part

	// InletQuality is the mass fraction of steam (0..1) in the
	// wet exhaust from the HP turbine. At full load the HP
	// exhaust is typically 0.85–0.92 quality; it falls at low
	// load as more moisture forms.
	InletQuality float64

	// InletPressure is the pressure of the HP turbine exhaust
	// steam entering the MSR.
	InletPressure Pressure

	// OutletPressure is the pressure leaving the MSR, slightly
	// lower than InletPressure due to pressure drop across the
	// separator and reheater.
	OutletPressure Pressure

	// OutletTemperature is the superheated steam temperature
	// leaving the reheater section. The spec targets >100°F
	// superheat over the saturation temperature at OutletPressure.
	OutletTemperature Temperature

	// DrainFlow is the condensed moisture removed per step,
	// drained to the feedwater system (hotwell).
	DrainFlow Volume

	// ReheatSteamPressure is the pressure of the extraction
	// steam (or main steam before the throttle valves) that
	// supplies the reheater tubes.
	ReheatSteamPressure Pressure

	// ReheatSteamFlow is the mass flow of reheating extraction
	// steam. Set by the turbine step as a fraction of total
	// load.
	ReheatSteamFlow Volume
}

// TurbineState is the operating state of the main turbine.
//
// Spec justification:
//   - glossary.md turbine trip: "When turbine steam inlet valves are
//     rapidly closed to block steam flow, stopping the turbine."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_245.txt:48-50
//   - auto-stop-oil trip system (lines 28-34) causes all steam
//     control valves to close on any trip signal.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
//   - "turbine speed does not exceed 120 percent of [rated]" (line 40)
//     — see TurbineOverspeedTripRatio below.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:40
type TurbineState int

const (
	TurbineOff TurbineState = iota
	TurbineRunning
	TurbineTripped
)

// Rated values for the main turbine, sized for a large turbo-generator
// to match the implied megawatt scale of
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21 (24-26 kV main generator).
const (
	TurbineRatedSpeed         Velocity = 1800
	TurbineOverspeedTripRatio float64  = 1.20
	TurbineSteamLossDeadline           = 2 * time.Second
	TurbineRatedkW            Watts    = 1_000_000e3
)

// Turbine-generator grid-coupling and protective-control constants
// (plans/p12_1.txt).
//
// Spec justification:
//   - "The two speed channels and OPC redundantly provide OPC alarm
//     (103%), OPC trip (110%), and fast valving functions."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-73
//   - "After generator breaker closure, the ability to increase or
//     decrease electrical load at a controlled selectable rate. The
//     resolution of this controller is 0.5 percent load."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-83
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_202.txt:49-52
//   - Generator rating: "Rating: 1370 mva"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
//   - Generator power factor: "Power factor: 0.9, short circuit ratio:
//     0.58"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
const (
	// TurbineOPCALarmFraction is the shaft-speed fraction of
	// TurbineRatedSpeed at which the OPC channel raises an
	// alarm. "OPC alarm (103%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
	TurbineOPCALarmFraction float64 = 1.03

	// TurbineOPCTripFraction is the shaft-speed fraction of
	// TurbineRatedSpeed at which the OPC channel trips the
	// turbine (closes the stop and governing valves). "OPC trip
	// (110%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
	TurbineOPCTripFraction float64 = 1.10

	// TurbineLoadRunbackRate is the maximum fraction of rated
	// load per second the turbine will shed during a
	// load-runback transient. The spec describes "controlled
	// selectable rate" with "0.5 percent load" resolution
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-83);
	// 5% per minute (≈0.00083/s) is a typical Westinghouse DEHM
	// load-runback rate.
	TurbineLoadRunbackRate float64 = 0.05 / 60.0

	// GeneratorRatedMVA is the apparent-power rating of the main
	// generator. "Rating: 1370 mva"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
	GeneratorRatedMVA float64 = 1370e3

	// GeneratorRatedVoltage is the rated terminal voltage. "Output
	// voltage: 24,000 to 26,000 volts, three-phase, 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	GeneratorRatedVoltage float64 = 25000

	// GeneratorRatedFrequency is the rated electrical frequency.
	// "60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	GeneratorRatedFrequency float64 = 60

	// GeneratorPowerFactor is the design power factor
	// (lagging). "Power factor: 0.9"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
	GeneratorPowerFactor float64 = 0.9

	// GeneratorSetpointResolution is the load-demand controller
	// resolution. "The resolution of this controller is 0.5
	// percent load."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:82-83
	GeneratorSetpointResolution float64 = 0.005
)

type FuelRods struct{ Part }

// ControlRods models the rod cluster control (RCC) assemblies.
// Position=1 means fully withdrawn (normal operation);
// Position=0 means fully inserted (reactor tripped).
//
// Spec justification:
//   - "Rod cluster control (RCC) assemblies are used for reactor
//     startup or shutdown, to follow load changes, and to control
//     small transient changes in reactivity."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:90-93
//   - "extend the full length of the core when fully inserted. Full
//     length RCCs provide operational reactivity control and can shut
//     the reactor down at all times, even with the most reactive RCC
//     stuck out of the core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:108-114
//   - Glossary "rod worth — The reactivity change per unit change in
//     control rod position."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:46-47
type ControlRods struct {
	Part
	Position float64 // 0..1 fraction of withdrawal
}
type Nozzle struct{ Part }

type ReactorPressureVessel struct {
	Part
	FuelRods
	ControlRods
	Reactivity float32
	DecayHeat  float32

	// ThermalPower is the per-step heat flux produced by the core
	// and consumed downstream by the SG's HeatInput. Rated when
	// the operator has the reactor running at power; decays via
	// the standard shutdown curve when Trip is set.
	ThermalPower Watts

	// TimeSinceTrip is 0 while running and accumulates after Trip.
	TimeSinceTrip time.Duration

	// Trip is the operator-controlled trip flag. Set true to drop
	// the rods; the per-step output then follows the decay-heat
	// curve.
	Trip bool

	// Inlet is the temperature of coolant entering the core via the
	// cold legs (after the SG). Outlet is the temperature leaving
	// the core via the hot legs (before the SG). These two
	// temperatures drive the Doppler / moderator feedback in Step
	// and are the basis for the average-coolant-temperature
	// reactivity term.
	//
	// Spec justification:
	//   - "Reactor coolant inlet temperature: 621°F (327°C);
	//     outlet: 558°F (292°C)"
	//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
	//   - "The design full load average temperature is about 571°F
	//     (300°C)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:115-116
	Inlet  Temperature
	Outlet Temperature

	InletNozzle  Nozzle
	OutletNozzle Nozzle
	Pressure
}

// Pressurizer models the surge-line volume at the top of the primary
// loop that holds the steam bubble used for pressure control.
//
// Spec justification for the field set:
//   - Pressure, Water, Volume — "The pressurizer is a partial tank that
//     contains saturated water and steam in equilibrium at saturation
//     temperature corresponding to the pressure." (glossary.md
//     pressurizer, p. 169)
//   - HeatersOn — "All pressurizer heaters are energized to start the
//     bubble formation during plant startup."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
//   - SprayOpen — "Pressurizer heater and spray valve control is [...]
//     used to maintain the pressure in accordance with the applicable
//     temperature-pressure curves."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
//   - SteamBubble — "When the pressurizer fluid reaches saturation
//     temperature for the pressure being maintained, steam formation
//     in the pressurizer begins."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23
type Pressurizer struct {
	Part
	Pressure
	Water       Volume
	Volume      Volume
	HeatersOn   bool
	SprayOpen   float64
	SteamBubble Volume
}

// Pressurizer control tunables. The numbers below are sized to give
// spec-anchored behaviour on the 1 ms plant step:
//
//   - PressurizerBasePressure + PressurizerBubbleStiffness * bubble ->
//     ~2250 psia at a ~30% bubble.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:69
//   - PressurizerHeaterRate drives Pressure toward the target on a
//     timescale of a few seconds.
//   - PressurizerPORVTrip is the safety-valve setpoint the step uses to
//     crack the PORV.
//
// Spec justification:
//   - "operation of pressurizer heaters and spray maintains the system
//     pressure in accordance with the applicable temperature-pressure
//     curves." specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
const (
	PressurizerBasePressure    Pressure = 1500
	PressurizerHeaterTarget    Pressure = 2250
	PressurizerHeaterRate      Pressure = 200 // psi/s while HeatersOn
	PressurizerSprayMin        Pressure = 1400
	PressurizerSprayRate       Pressure = 300 // psi/s at SprayOpen == 1
	PressurizerAmbientDecay    Pressure = 5   // psi/s cooldown when idle
	PressurizerBubbleStiffness float64  = 2.5 // psi per gallon of bubble
	PressurizerPORVTrip        Pressure = 2500
)

type SteamGenerator struct {
	Part
	Pressure
	Water  Volume
	Volume Volume

	// SteamFlowRate is the design steam mass-flow the SG is sized
	// for, in volumetric units compatible with Water/Volume.
	// Sized to the spec's 480 kg/s full-load steam flow.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
	SteamFlowRate Volume

	// HeatInput is the primary-to-secondary heat flux in Watts.
	// Caller-supplied (the reactor step is out of scope; p7
	// candidate). When zero the SG does not generate steam on its
	// own; the fluid graph dominates the inventory.
	HeatInput Watts
}

func (s *SteamGenerator) Update() SteamGenerator {
	ret := *s
	// watts := heatTransferred()
	return ret
}

func (s *SteamGenerator) heatTransferred() Watts {
	// Qdot = rate of heat transfer
	// U = heat transfer coefficient
	// A = area of heat transfer
	// deltaT = differential temperature across exchanger
	/*
		U := HeatTransferCoefficient
		A := AreaOfHeatTransfer
		Tavg := (Thot + Tcold) / 2.0
		deltaT := Tavg - Tstm
		Qdot := U*A*deltaT
	*/
	return 0
}

// PowerGenerator models the electrical side of a generator set. The
// diesel engine step populates these fields; the main turbine's
// generator populates them in its own step.
//
// Spec justification for the field set:
//   - Voltage / Frequency / RealPower —
//     "Output voltage: 24,000 to 26,000 volts, three-phase, 60 Hz" (main
//     generator reference).
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
//     "two 6600-volt engineered safety features buses" (diesel output).
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-82
//   - BreakerClosed —
//     "controls for the governor, voltage regulator, generator breaker,
//     and synchronization for each diesel generator are located on
//     physically separated portions of the main control board".
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-4
//     The main generator breaker is closed by the operator after
//     synchronisation and reopened on an OPC trip, operator manual
//     trip, or grid fault.
//   - PowerFactor / ReactivePower —
//     "output power, the power factor, and speed to ensure that the
//     limits of the generator are not exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
//     "Power factor: 0.9, short circuit ratio: 0.58"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
//   - RatedMVA —
//     "Rating: 1370 mva"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
//   - RealPower — the running load in Watts, sized by RatedkW *
//     RunningFraction so a downstream load sequencer (planned for the
//     future) can sum load against rated capacity.
type PowerGenerator struct {
	Part

	// Voltage is the terminal voltage in volts. For the main
	// generator the design point is 24–26 kV at 60 Hz
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21).
	Voltage float64

	// Frequency is the terminal electrical frequency in Hz.
	// Tracks shaft speed / pole-pair-count for synchronous
	// generators; the main generator runs at 1800 rpm / 60 Hz
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:22).
	Frequency float64

	// RatedMVA is the apparent-power rating of the generator.
	// "Rating: 1370 mva"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
	RatedMVA float64

	// RealPower is the active power output in watts. When
	// BreakerClosed is false the output is zero regardless of
	// steam flow.
	RealPower Watts

	// ReactivePower is the reactive power output in vars,
	// derived from RealPower and PowerFactor as
	// RealPower * (1/PowerFactor - 1) (lagging convention). When
	// BreakerClosed is false the output is zero.
	ReactivePower float64

	// PowerFactor is the cosine of the phase angle between
	// voltage and current (lagging). The spec's design point is
	// 0.9; the implementation clamps the default to that value.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
	PowerFactor float64

	// BreakerClosed is true when the generator breaker is
	// closed and the generator is connected to the grid. The
	// operator closes it after synchronisation; it opens on an
	// OPC trip, operator manual trip, or grid fault.
	BreakerClosed bool
}

// Condenser models the main condenser (chapter 15.3). The shell-
// side steam condenses on the tube bundle; the latent heat is
// rejected to the circulating water (CW) flowing inside the tubes.
//
// Spec justification for the field set:
//   - Pressure — the condenser operates at sub-atmospheric vacuum
//     under normal conditions: "Under normal operation the air is
//     removed by steam jet air ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
//   - Level — condensate accumulation; the condensate pumps (out of
//     scope for p9) eventually drain it back to the CST.
//     "There is also sufficient surface to condense steam from the
//     steam dump system with an acceptable increase in turbine back
//     pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-18
//   - HotwellLevel — the level in the condenser hotwell from which
//     the condensate pumps take suction. The spec describes "the
//     closed-loop type with deaeration accomplished in the condenser
//     hotwell".
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:38-40
//   - VacuumPumpRunning — operator-controlled flag set from the
//     steam-jet air ejector / vacuum pump subsystem.
//   - CirculatingWaterIn — temperature of the CW water entering the
//     condenser tubes. Set once per step from outside.go's
//     CirculatingWaterPump state.
//   - CWOut — informational outlet temperature of the CW (delta-T
//     across the tube bundle).
//   - CondensingRate — per-step steam mass condensed in gallons; set
//     by TurbineBuilding.StepWithSteam from the turbine's exhaust
//     mass flow, consumed by the condenser step.
type Condenser struct {
	Part
	Pressure
	Level              Volume
	HotwellLevel       Volume
	VacuumPumpRunning  bool
	CirculatingWaterIn Temperature
	CWOut              Temperature
	CondensingRate     Volume
}
type Polisher struct{}

// LowPressureHeater models one stage of the low-pressure feedwater
// heating train. The heater uses extraction steam from the
// corresponding turbine stage to raise the feedwater temperature
// before it reaches the deaerator. Four such stages sit between
// the condenser hotwell and the deaerator; the model below collapses
// them into one stage and lets the plant step iterate four times.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser hotwell
//     and pump condensate [...] through four stages of low-pressure
//     feedwater heaters to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
//   - "Several stages of extraction are used for the feedwater
//     heaters."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:35
//     "A small portion of the steam in the high-pressure turbine
//     is extracted to the highest pressure feedwater heaters [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:26-35
//   - Drain cascade: "Drains from the four low-pressure feedwater
//     heaters are cascaded back to the condenser."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-15
type LowPressureHeater struct {
	Part

	// ExtractionSteamPressure is the pressure of the extraction
	// steam from the corresponding turbine stage. Operator-set
	// from the turbine extraction list (out of scope for p10).
	ExtractionSteamPressure Pressure

	// SteamFlow is the per-step extraction steam mass flow in
	// gallons. Operator-set; the step uses 0 to mean "no steam
	// available", which freezes the temperature rise.
	SteamFlow Volume

	// Inlet is the temperature of the feedwater entering this
	// stage. Set by the previous stage (or by the condenser/CST
	// for stage 1).
	Inlet Temperature

	// TemperatureRise is the temperature increase produced by
	// this heater stage, computed per step from the steam
	// energy balance.
	TemperatureRise Temperature

	// DrainLevel is the condensate level in this heater's drain
	// section. LP heater drains cascade back to the condenser
	// (spec citation above).
	DrainLevel Volume

	// DrainValve gates the drain path. Open=1 allows drain flow;
	// Open=0 blocks it (e.g. during isolation).
	DrainValve Valve
}

// HighPressureHeater models one stage of the high-pressure feedwater
// heater train, located on the main-feed-pump discharge side.
//
// Spec justification:
//   - "The water discharge from the feedwater pumps flows through
//     two stages of high-pressure heaters then into the steam
//     generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
//   - "Drains from the high-pressure feedwater heaters are routed
//     to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-14
type HighPressureHeater struct {
	Part

	// ExtractionSteamPressure is the pressure of the highest-
	// pressure extraction steam (main steam or first-stage HP
	// extraction).
	ExtractionSteamPressure Pressure

	// SteamFlow is the per-step extraction steam flow in gallons.
	SteamFlow Volume

	// Inlet is the feedwater temperature entering this HP heater.
	Inlet Temperature

	// TemperatureRise computed per step from the steam energy
	// balance; the HP train's per-stage rise is smaller than the
	// LP train's because the feedwater is already pre-heated.
	TemperatureRise Temperature

	// DrainLevel in the HP heater drain section.
	DrainLevel Volume

	// DrainValve; open drains to the deaerator tank (spec citation
	// above).
	DrainValve Valve
}

// AFWPump is the auxiliary feedwater pump described in chapter 5.2 of
// the spec.
//
// Spec justification for DriveSource / DriveReady:
//   - The EFWS has three diverse power sources for the AFW pumps —
//     "motor driven and turbine driven pumps are located in the
//     emergency feedwater pump building. The use of both motor driven
//     and turbine driven pumps satisfies the requirement that the pumps
//     be powered by diverse power sources."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:1-5
//   - A pump labelled "diesel-driven" must not run when its diesel is
//     not running. DriveReady latches that condition without forcing the
//     pump type to know about diesel internals.
type AFWPump struct {
	Pump

	// DriveSource names the upstream that supplies motive force.
	DriveSource DriveSource

	// DriveReady is latched by the upstream drive (diesel, ESF bus,
	// steam header) and gates the Pump.Running field's effectiveness.
	DriveReady bool

	ServiceWaterValve Valve
	CSTValve          Valve
}

// DieselStartCause names the source of the start signal that asked the
// onsite emergency diesel generator to run. The engine itself does not
// care which cause tripped; downstream consumers (a future load
// sequencer, if one is added) do.
//
// Spec justification:
//   - DieselStartSafetyInjection —
//     "The onsite emergency diesel generators are started." (item 1 in the
//     SIS actuation list)
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:18-22
//   - DieselStartLossOfOffsitePower —
//     "In the event of total loss of auxiliary power from off-site
//     sources, auxiliary power required for safe shutdown will be
//     supplied from physically and electrically independent diesel
//     generators located on the site."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:28-36
//   - DieselStartManual / DieselStartTest — operator action from the
//     main control board; the spec locates "controls for the governor,
//     voltage regulator, generator breaker, and synchronization for each
//     diesel generator […] on physically separated portions of the main
//     control board".
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:2-4
type DieselStartCause int

const (
	DieselStartNone DieselStartCause = iota
	DieselStartManual
	DieselStartSafetyInjection
	DieselStartLossOfOffsitePower
	DieselStartTest
)

// DriveSource names the upstream that supplies motive force to a pump.
//
// Spec justification:
//   - DriveDiesel — the diesel-driven auxiliary feedwater pump described
//     alongside the motor-driven and turbine-driven units in the EFWS:
//     "The use of both motor driven and turbine driven pumps satisfies
//     the requirement that the pumps be powered by diverse power sources."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:1-5
//   - DriveSteam — the turbine-driven AFW pump whose "steam supply line
//     for each turbine driven pump is connected to the main steam line
//     from a steam generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
//   - DriveElectric — the motor-driven AFW pump fed from the "Safety
//     Class 1E electrical power" trains.
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:71-82
type DriveSource int

const (
	DriveElectric DriveSource = iota
	DriveSteam
	DriveDiesel
)

type DieselGenerator struct {
	Part

	// speed controlled by discharge pressure
	// and high steam header pressure
	// setpoint is normally 150 psid
	Speed RPM

	// minimum level is 450 gallons at start of day
	// each tank can support full load for 4 days
	// aux feed pump running for 24 hours
	Fuel Tank

	// Operator-controlled between steps. DieselStartNone is the resting
	// state; any non-None value is treated as a request to run. Latched
	// across steps until an explicit DieselStartNone clears it.
	StartSignal DieselStartCause

	// Per-spec state machine.
	State DieselState

	// Simulated time accumulated in the current state, used to meet
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50-51
	// deadlines (10 s ready, 30 s rated).
	CrankStartTime time.Duration
	ReadyTime      time.Duration

	// Rated-load readiness latch
	// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50).
	ReadyToLoad bool

	// RunningFraction is the fraction of rated load currently served,
	// 0..1. Set by the operator between steps.
	RunningFraction float32

	// Cooling state — driven by ServiceWaterPump.Running
	// (specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89).
	CoolantTemp Temperature

	// Generator-side output. See core/parts.go:89 for the PowerGenerator
	// definition; this field is the "generator" half of the diesel
	// generator. Voltage / Frequency are ramped up by the diesel step
	// once rated speed is reached.
	Generator PowerGenerator
}

// DieselState is the operational state of the onsite emergency
// diesel generator
// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:96-122).
type DieselState int

const (
	// DieselOff is the resting state; Speed == 0.
	DieselOff DieselState = iota
	// DieselCranking is the air-over-piston start transient
	// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:77-78).
	DieselCranking
	// DieselRunning means rated speed has been reached and the unit is
	// ready to accept load.
	DieselRunning
	// DieselTripped means a protective trip has latched
	// (overspeed, loss of fuel, or cooling failure).
	DieselTripped
)

// Rated speed and trip thresholds for the emergency diesel generator.
//
// Spec justification:
//   - DieselRatedSpeed, DieselReadyDeadline, DieselRatedLoadDeadline —
//     "reach rated speed and be ready to begin accepting load within 10
//     seconds, and be capable of carrying rated load within 30 seconds
//     after receipt of a start signal to the diesel engine."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:46-51
const (
	DieselRatedSpeed        RPM     = 1800
	DieselReadyDeadline             = 10 * time.Second
	DieselRatedLoadDeadline         = 30 * time.Second
	DieselRatedVoltage      float64 = 6600
	DieselRatedFrequency    float64 = 60
	DieselRatedkW           Watts   = 4000e3
	DieselFieldFlashingTime         = 1 * time.Second
)

// SteamGenerator shell-side operating pressure and the latent heat of
// vaporisation used by the heat-transfer step.
//
// Spec justification:
//   - SteamGeneratorShellPressure — "Full load pressure, shell side:
//     920 psia (63 bar)" for the 2-loop plant from Table 3.4-1
//     (specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,
//     value lines 80-82).
//   - SteamGeneratorShellDesignPressure — design pressure on the
//     shell side is 1000 psia per the plan p6_1 trip-threshold
//     choice. (Table 3.4-1 quotes 1200 psia for the same row;
//     the plan uses 1000 as the trip point; the discrepancy is
//     noted in plans/x/x.txt verification.)
//   - LatentHeatOfSteamization — water at ~558 F to saturated steam
//     at 920 psia. The spec does not state a value; this is the
//     textbook number from standard steam tables.
const (
	SteamGeneratorShellPressure       Pressure = 920
	SteamGeneratorShellDesignPressure Pressure = 1000
	LatentHeatOfSteamization          Watts    = 2.26e6
)

// AtmosphericPressure is the standard sea-level pressure the
// condenser back-pressure relaxes toward when the vacuum pumps and
// circulating-water pumps are unavailable. Not stated explicitly in
// the Westinghouse PWR spec but used to bound the no-CW condenser
// degradation.
const AtmosphericPressure Pressure = 14.7

// Condenser and main-steam system constants (plans/p9_1.txt).
//
// Spec justification:
//   - CondenserDesignVacuum — "Under normal operation the air is
//     removed by steam jet air ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
//     The number below approximates a well-operated condenser with
//     a modest air in-leakage.
//   - CondenserDesignCWIn — design circulating-water inlet
//     temperature of 75°F (24°C), matching the RiverTemp default
//     used for the Service Water system (core/plant.go).
//   - CondenserCWRise — design temperature rise of the CW across
//     the tube bundle, sized to give a sensible heat-rejection
//     value. The spec does not state a value.
//   - CondenserDesignCWFlowGPM — per-condenser CW flow sized for a
//     large 4-loop plant (essential CW pump rating "approximately
//     20,000 gpm").
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:39-43
//   - CondenserDesignSteamLoadGalPerStep — full-load steam
//     volumetric flow per step, derived from
//     SteamGeneratorFullLoadSteamFlow × 4 loops / steps-per-second.
//     The per-SG spec value is 3,813,000 lb/hr (480 kg/s).
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:49-56
//   - SteamDumpOpenThreshold — condenser pressure above which the
//     dump valves open automatically to relieve inertial heat. Not
//     stated in the spec; sized above design vacuum but well below
//     atmospheric.
//   - StopValveCloseDelay — hydraulic time for the auto-stop-oil
//     system to dump fluid pressure and close all stop valves
//     after a trip signal. The spec describes mechanical-hydraulic
//     dumping without a stated delay.
//     "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
//   - CondenserVacuumTimeConstant / CondenserNoCWDegradeTimeConstant
//     — sized for plausible ~2 s vacuum recovery and a few seconds
//     of no-CW degradation; not stated in the spec.
const (
	CondenserDesignVacuum            Pressure = 1.5
	CondenserDesignCWIn              float64  = 75
	CondenserCWRise                  float64  = 15
	CondenserDesignCWFlowGPM         float64  = 20000
	CondenserDesignSteamLoadGalPer   float64  = 100
	SteamDumpOpenThreshold           Pressure = 5.0
	StopValveCloseDelay                       = 50 * time.Millisecond
	CondenserVacuumTimeConstant      float64  = 2.0
	CondenserNoCWDegradeTimeConstant float64  = 5.0
)

// Feedwater heating train design constants (plans/p10_1.txt).
//
// Spec justification:
//   - LPTemperatureRisePerStage / HPTemperatureRisePerStage — no
//     per-stage temperature numbers are stated in the spec; the
//     heater model uses a per-stage rise that sums to the
//     condensate-to-deaerator temperature rise on the LP side and
//     the deaerator-to-SG rise on the HP side. The full train's
//     rise is sized to give ~120°F → ~440°F feedwater at full
//     load, consistent with the closed-loop feedwater heating
//     train described in:
//     "The condensate pumps take suction from the condenser
//     hotwell and pump condensate [...] through four stages of
//     low-pressure feedwater heaters to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
//     "The water discharge from the feedwater pumps flows through
//     two stages of high-pressure heaters then into the steam
//     generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
//   - ExtractionSteamEnthalpy — the latent heat of saturated steam
//     at a typical mid-range LP extraction pressure (~60–80 psia).
//     The spec does not state a number; ~1.16 MJ/kg is the textbook
//     value at that pressure.
const (
	LPTemperatureRisePerStage float64 = 70
	HPTemperatureRisePerStage float64 = 20
	ExtractionSteamEnthalpy   Watts   = 1.16e6
	KilogramsPerGallon        float64 = 3.785
)
//   - LPTemperatureRisePerStage / HPTemperatureRisePerStage — no
//     per-stage temperature numbers are stated in the spec; the
//     heater model uses a per-stage rise that sums to the
//     condensate-to-deaerator temperature rise on the LP side and
//     the deaerator-to-SG rise on the HP side. The full train's
//     rise is sized to give ~120°F → ~440°F feedwater at full
//     load, consistent with the closed-loop feedwater heating
//     train described in:
//     "The condensate pumps take suction from the condenser
//     hotwell and pump condensate [...] through four stages of
//     low-pressure feedwater heaters to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
//     "The water discharge from the feedwater pumps flows through
//     two stages of high-pressure heaters then into the steam
//     generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
//   - ExtractionSteamEnthalpy — the latent heat of saturated steam
//     at a typical mid-range LP extraction pressure (~60–80 psia).
//     The spec does not state a number; ~1.16 MJ/kg is the textbook
//     value at that pressure.
//   - FeedwaterMinFlow — the minimum stable flow below which the
//     main feed pump discharge recirculates to the deaerator.
//     "The discharge from the pumps is automatically recirculated
//     back to the deaerator whenever the flow to the high-pressure
//     feedwater heaters falls below the minimum."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
//   - FeedwaterDesignFlow — the design volumetric flow through the
//     main feed pump at full load. Anchored on the per-SG steam
//     flow of 3,813,000 lb/hr (480 kg/s):
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//     three SGs at full load give 3 × 480 kg/s = 1440 kg/s ≈
//     9531 gal/s, converted below.
// FeedwaterMinFlow is the minimum stable flow below which the
// main feed pump discharge recirculates to the deaerator.
// "The discharge from the pumps is automatically recirculated back
// to the deaerator whenever the flow to the high-pressure
// feedwater heaters falls below the minimum."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
//
// Sized as 10% of design flow; the spec does not state a specific
// minimum flow value.
var FeedwaterMinFlow Volume = u.NewValue(950, u.Gallon)

// FeedwaterDesignFlow is the design volumetric flow through the
// main feed pump at full load. Anchored on the per-SG steam flow
// of 3,813,000 lb/hr (480 kg/s) for three SGs.
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
// 3 × 480 kg/s = 1440 kg/s ≈ 1440 / 3.785 ≈ 380 gal/s = ~22,800
// gal/min. The per-step value below is per-step at 1 ms: 380 gal/s
// × 0.001 s/step ≈ 0.38 gal/step; the constant is sized in gal/s
// for the per-step integration in heater.go.
var FeedwaterDesignFlow Volume = u.NewValue(380, u.Gallon) // gal/s

// MSR and extraction steam design constants (plans/p11_1.txt).
//
// Spec justification:
//   - "A small portion of the steam in the high-pressure turbine
//     is extracted to the highest pressure feedwater heaters."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:31-32
//   - Reheat target: "reheat the steam to over 100°F (55°C)
//     superheat."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:55
//   - The typical four-cylinder turbine-generator is rated
//     ≈1500 MWe.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_197.txt:16-17
const (
	// MSRMoistureRemovalEfficiency is the fraction of liquid
	// water removed per pass through the chevron separators.
	MSRMoistureRemovalEfficiency = 0.95

	// MSRReheatTarget is the target steam temperature rise
	// across the reheater section at design extraction steam
	// flow.
	MSRReheatTarget = 100.0 // °F

	// MSRDesignExtractionFraction is the fraction of HP steam
	// flow diverted to the MSR reheater and the highest-pressure
	// feedwater heaters at full turbine load.
	MSRDesignExtractionFraction = 0.12

	// MSRSixthStageExtractionFraction is the fraction of HP
	// steam flow diverted to the sixth-stage (lowest-pressure)
	// extraction for one representative LP feedwater heater.
	MSRSixthStageExtractionFraction = 0.03

	// HPExhaustQualityFullLoad is the expected steam quality
	// at the HP exhaust at rated turbine power.
	HPExhaustQualityFullLoad = 0.90

	// HPExhaustQualityLowLoad is the expected steam quality
	// at the HP exhaust at low turbine power (more moisture).
	HPExhaustQualityLowLoad = 0.70
)

// SteamGeneratorFullLoadSteamFlow is the per-SG steam flow at rated
// load, sized for 480 kg/s of saturated steam at 920 psia. The spec
// quotes 3,813,000 lb/hr (480 kg/s) per steam generator.
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//
// We convert kg/s -> lb/hr (× 2.2046 × 3600) and approximate the
// specific volume of saturated steam at 920 psia as ~0.47 ft^3/lb,
// giving a volumetric flow in ft^3/hr which we then express in
// gallons. The exact conversion is not load-bearing — the value
// drives the boil-rate integration only.
var SteamGeneratorFullLoadSteamFlow = u.NewValue(480*2.2046*3600*0.47/7.4805, u.Gallon)

// SteamGeneratorFullLoadHeatInput is the primary-to-secondary heat
// flux that vaporises 480 kg/s of water at LatentHeatOfSteamization.
// Sized to match the spec's 480 kg/s full-load steam flow
// (specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56).
var SteamGeneratorFullLoadHeatInput Watts = Watts(480 * float64(LatentHeatOfSteamization))

// DieselCoolantTripTemp — cooling trip threshold; the spec implies
// Service-Water loss must trip the engine
// (specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89,
// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:8-9).
var DieselCoolantTripTemp = u.NewValue(220, u.Fahrenheit)

// Reactor core sizing. The spec gives steam-flow and pressure data
// (specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56,
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:80-82)
// but does not state a plant-wide rated thermal power. The number
// below is sized so that four SGs at SteamGeneratorFullLoadHeatInput
// match the standard 4-loop plant scale (~3.4 GWt).
const (
	CoreRatedThermalPower Watts = 3_800_000_000
)

// Decay-heat model constants.
//
// Spec justification:
//   - The spec does not state a decay-heat curve; the model follows
//     the textbook two-branch approximation of ANSI/ANS-5.1 (1971).
//     The fast branch (half-life ~ 40 s) drops the reactor from
//     rated to the ~6.5% shutdown level; the slow branch (e-folding
//     ~ 20 h) tracks the residual heat the Residual Heat Removal
//     System is designed to remove. The 20-hour cooldown target is
//     cited in
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_083.txt:104-105
//     ("reduce the RCS temperature to 140°F (60°C) within 20 hours
//     following reactor shutdown").
const (
	CoreDecayHeatShutdownFraction float64 = 0.065 // 6.5% of rated
	CoreDecayHeatMinFraction      float64 = 0.005 // long tail ~0.5%
	CoreDecayHeatTimeConstant             = 20 * time.Hour
	CoreDecayHeatFastTimeConstant         = 50 * time.Second

	// Reactivity contributions. ABCD is the absolute delta-k at
	// full rod insertion, scaled so withdrawing the rods from 0 to
	// 1 shifts reactivity by 45 pcm in the positive direction.
	CoreRodWorthABCD float32 = -0.045

	// Design full-load average coolant temperature (°F); the
	// reference point at which the simplified Doppler term
	// contributes zero reactivity.
	//   "The design full load average temperature is about
	//   571°F (300°C)."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:115-116
	CoreDopplerReferenceTempF float32 = 571
)

// Primary-side thermal model constants (plans/p8_1.txt).
//
// Spec justification:
//   - CoreReferenceInletF / CoreReferenceOutletF — Table 3.4-1 lists
//     "Reactor coolant inlet temperature: 621°F; outlet: 558°F" from
//     the steam-generator tube-side perspective. Walking the loop back
//     to the RPV (coolant enters the core cold via the SG cold leg and
//     leaves hot via the hot leg) means RPV-side Inlet=558°F and
//     Outlet=621°F.
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
//   - RPVDesignPressure — "designed for full system pressure of 2500
//     psia (172.3 bar a)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_054.txt:20-21
//     The reactor coolant pump design data table repeats "Design
//     pressure 2500 psia (172 bar)".
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_069.txt:84-91
//   - RCP design parameters: "Design capacity 97,600 gpm (6.15
//     m3/sec)" / "Design head 280 ft (85.3 m)" — cited for context
//     but not directly used in the model.
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:124-130
//   - CoreDopplerPcmPerDegF — combined Doppler + moderator
//     temperature coefficient of reactivity for this model. The spec
//     describes the constituent effects separately:
//     Doppler — "This defect is due to an increase in width of
//     uranium-238 resonance peaks in the epithermal energy range."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:89-96
//     Power defect — "The negative reactivity associated with an
//     increase in reactor core power due to temperature feedback
//     effects from an increase in fuel and moderator temperature."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
//     Temperature coefficient of reactivity — "The ratio of the
//     change in reactivity to the change in temperature when the
//     reactivity change is only due to a temperature change."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_245.txt:18-20
//     Moderator temperature coefficient — varies from positive at
//     beginning-of-life ("on the order of +7 pcm/°F",
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:54-55)
//     to negative at end-of-life ("the coefficient is substantially
//     more negative due to depletion and removal of shim boron",
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:119-121);
//     the simplified model uses a representative negative value.
const (
	// Reference full-load RPV-side temperatures (°F).
	CoreReferenceInletF  float64 = 558
	CoreReferenceOutletF float64 = 621

	// Primary loop design pressure (psia).
	RPVDesignPressure Pressure = 2500

	// PrimaryThermalTimeConstant is the thermal time constant of
	// the core coolant + fuel + internals in seconds. Sized to give
	// plausible ~1 s outlet response to a step in ThermalPower at
	// the 1 ms plant step. Not derived from a spec number; the spec
	// describes the steady-state temperatures only.
	PrimaryThermalTimeConstant float64 = 90

	// PrimaryInletTimeConstant is slower than the outlet because
	// the cold leg traverses the entire primary loop before
	// returning to the RPV. Roughly 3× the RPV-side thermal time
	// constant.
	PrimaryInletTimeConstant float64 = 270

	// PrimaryPressureTimeConstant is the rate at which the RPV
	// pressure tracks its design pressure in this simplified model.
	PrimaryPressureTimeConstant float64 = 300

	// CoreInletBlendFraction is the fraction of the power level
	// that contributes to the inlet mixing model; at zero power the
	// inlet is effectively decoupled from the outlet.
	CoreInletBlendFraction float64 = 0.8

	// CoreDopplerPcmPerDegF is the PWR net temperature coefficient
	// of reactivity (combined Doppler + moderator temperature
	// effect) expressed in pcm per degree Fahrenheit, applied to
	// the outlet temperature. A negative value means rising outlet
	// temperature reduces reactivity (stable).
	CoreDopplerPcmPerDegF float64 = -1.0
)

type Transformer struct{ Part }

type Pipe struct {
	Part
	Pressure
	Velocity
}

type Valve struct {
	Part
	Open float64
}
