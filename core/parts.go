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

	// GridFrequency is the infinite-bus frequency in Hz that
	// the governor droop correction compares against
	// GridRatedFrequency. Plant.Step latches the grid frequency
	// from the Grid cell before calling Turbine.Step; the
	// turbine step computes the droop bias from this field.
	//
	// Spec: "computes signals comparing turbine speed and
	// first-stage pressure with reference values established
	// by the dispatcher or initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	GridFrequency float64

	// DroopBias is the per-step bias applied to Setpoint by the
	// frequency-droop governor. Positive when the grid is
	// under-frequency (turbine must pick up load), negative
	// when over-frequency. Plant.Step reads this back after the
	// turbine step to apply the bias to Setpoint.
	DroopBias float64

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

// Station electrical design constants (plans/p13_1.txt).
//
// Spec justification:
//   - Off-site distribution grid and 6600 V plant distribution bus:
//     "The system auxiliary power is supplied from independent and
//     electrically isolated buses of the off-site main distribution
//     grid. This supply is transformed to the plant electrical
//     distribution bus voltage, normally 6600 volts, by one or
//     more system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:40-44
//   - Generator terminal voltage rating (24–26 kV):
//     "Output voltage: 24,000 to 26,000 volts, three-phase, 60 Hz"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
//   - Steam dump / islanding:
//     "The steam dump system permits rapid automatic reduction of
//     core power without reactor trip and enables the plant to
//     continue operation and supply power for its own auxiliaries.
//     This capability could be of critical importance in the event
//     of large grid system disturbances."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:55-60
const (
	// GridRatedFrequency is the infinite-bus design frequency.
	// The main generator is rated for 1800 rpm / 60 Hz.
	// "Output voltage: 24,000 to 26,000 volts, three-phase, 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	GridRatedFrequency float64 = 60.0

	// GridRatedVoltage is the nominal off-site distribution grid
	// voltage available at the station switchyard. The spec does
	// not state a numeric value; the value chosen here is the
	// generator terminal voltage (24 kV) which is the standard
	// output of a hydrogen-inner-cooled synchronous generator.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	GridRatedVoltage float64 = 24_000.0

	// StationRatedBusVoltage is the plant electrical distribution
	// bus voltage (6600 V). "This supply is transformed to the
	// plant electrical distribution bus voltage, normally 6600
	// volts."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:42-44
	StationRatedBusVoltage float64 = 6600.0

	// TurbineDroop is the governor speed-droop setting (4%). A 4%
	// droop means a 2.4 Hz deviation from 60 Hz causes the
	// generator to swing from no-load to full-load. The spec does
	// not state a numeric droop; 4% is a standard nuclear-turbine
	// governor setting consistent with the "controlled selectable
	// rate" of load changes.
	// "Wide range speed control of the turbine from turning gear
	// to rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-76
	TurbineDroop float64 = 0.04

	// TurbineDroopBiasLimit caps the per-step setpoint bias
	// applied by the droop governor. Without a limit, a large
	// frequency step (e.g. grid separation to 55 Hz) would
	// saturate the setpoint and force the unit into full-load
	// pickup. The ±5% cap is sized for transient droop response
	// without saturating the operator's setpoint.
	TurbineDroopBiasLimit float64 = 0.05

	// HouseLoadFixed is the always-on station auxiliary demand
	// (control systems, ESF bus trickle charge, HVAC, lighting).
	// Sized at ≈30 MW, ≈3% of the 1000 MWe TurbineRatedkW.
	// "supply power for its own auxiliaries."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
	HouseLoadFixed float64 = 30_000.0

	// HouseLoadVariableFraction is the fraction of generator
	// output consumed by variable auxiliaries (RCPs, feedwater
	// pumps, condensate pumps, heater drain pumps) at full load.
	// Sized at 3% of rated.
	HouseLoadVariableFraction float64 = 0.03
)

// CVCS design constants (plans/p14_1.txt).
//
// Spec justification:
//   - "The pressurizer level is controlled by balancing the
//     charging flow to the letdown flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
//   - Boration/dilution flow through the BTRS: "for boration, an
//     increase in the boric acid concentration of the reactor
//     coolant is accomplished by sending the flow from the VCT,
//     at relatively high temperatures (140°F) (60°C), to the
//     thermal regeneration demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:16-19
//   - VCT/BRS overflow: "the volume control tank level
//     controller automatically diverts the outflow from the
//     BTRS into the BRS holdup tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
//   - Charging-pump manual control: "The charging pump flow and
//     low-pressure letdown valve are controlled manually during
//     plant startup and cooldown. [...] The charging flow control
//     then maintains normal charging flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-117
//
// Charging-pump flow and letdown are the primary-level actuators;
// typical charging flow for a four-loop plant is in the range of
// 20–40 gpm per pump. The BTRS demineralizer exchange capacity
// sets the boron-concentration rate of change; 0.8 ppm/s yields
// 48 ppm/min, at the high end of the typical boration-rate range
// and consistent with the spec's "relatively fast" boration
// language for load-follow and xenon-compensation manoeuvres.
const (
	// CVCSBorationRate is the increase in RCS boron
	// concentration per second when the BTRS is in boration
	// mode. Sized at 48 ppm/min (0.8 ppm/s) — the high end of
	// typical boration rates used for xenon-transient
	// compensation.
	CVCSBorationRate float64 = 0.8

	// CVCSDilutionRate is the decrease in RCS boron
	// concentration per second when the BTRS is in dilution
	// mode. Sized symmetrically with CVCSBorationRate.
	CVCSDilutionRate float64 = 0.8

	// VCTDesignLevel is the nominal VCT level (gal). The spec
	// does not state a numeric volume; the value is sized so
	// VCT accumulator dynamics are visible at operator-training
	// timescales.
	VCTDesignLevel float64 = 500.0

	// VCTHighLevel is the VCT level at which the level
	// controller diverts BTRS outflow to the BRS holdup
	// tanks.
	// "the volume control tank level controller automatically
	// diverts the outflow from the BTRS into the BRS holdup
	// tanks."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
	VCTHighLevel float64 = 700.0

	// VCTLowLevel is the VCT level at which the operator
	// needs to initiate makeup from the refueling-water
	// storage tank.
	VCTLowLevel float64 = 300.0

	// VCTBTRSCirculation is the per-second volume of VCT
	// inventory routed through the BTRS recirculation pump
	// during boration/dilution. Sized at 2 gal/s — a typical
	// BTRS pump rating for a four-loop plant.
	// "The BTRS recirculation pump provides the motive
	// force to route flow from the VCT through the
	// demineralizers during boration and dilution modes."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39
	VCTBTRSCirculation float64 = 2.0

	// CVCSInitialBoron is the RCS boron concentration at
	// construction (ppm). 1000 ppm is typical for hot-zero-
	// power (HZP) at beginning-of-life; the value drops over
	// core life as the operator dilutes to compensate for
	// fission-product buildup and fuel depletion.
	// "The operating mode (boration or dilution) and the
	// magnitude of the boron concentration change required
	// is determined by comparison of the measured Tc (cold
	// leg temperature) with the programmed (function of
	// power) Tc."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
	CVCSInitialBoron float64 = 1000.0

	// CoreBoronWorthPer100ppm is the reactivity contribution
	// of 100 ppm of dissolved boric acid. Negative — boron
	// is a neutron absorber. Sized at ≈ −50 pcm per 100 ppm
	// so a 1000 ppm HZP inventory gives ≈ −500 pcm of boron
	// reactivity, which is consistent with the spec's
	// "critical boron concentration" and rod-bank withdrawal
	// sequences for a four-loop Westinghouse plant.
	// "the moderator temperature coefficient is positive at
	// low power (BOL), and [...] the boron concentration
	// changes significantly with fuel depletion."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
	CoreBoronWorthPer100ppm float32 = -0.05
)

// CVCSDesignChargingFlow is the design charging-pump flow rate
// into the RCS cold legs. The spec describes the charging pumps
// in narrative form without stating a numeric gpm; the value
// here is sized at 30 gpm per charging pump (typical for a
// four-loop plant) with two pumps running.
//
// Spec justification:
//   - "The charging pumps take the coolant from the volume
//     control tank [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
//   - "The charging pump flow and low-pressure letdown valve are
//     controlled manually during plant startup and cooldown.
//     [...] The charging flow control then maintains normal
//     charging flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-117
//   - "The pressurizer level is controlled by balancing the
//     charging flow to the letdown flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
var CVCSDesignChargingFlow Volume = u.NewValue(30, u.Gallon)

// CVCSDesignLetdownFlow is the design letdown flow rate from
// the RCS back to the VCT. The spec describes the letdown
// orifice and letdown chiller path in narrative form without
// stating a numeric gpm; the value here is sized at 75 gpm
// (typical for a four-loop plant) to provide a 2.5:1
// letdown-to-charging ratio that matches the design
// letdown-orifice sizing.
//
// Spec justification:
//   - "The charging pumps take the coolant from the volume
//     control tank [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
//   - "The charging pump flow and low-pressure letdown valve
//     are controlled manually [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115
var CVCSDesignLetdownFlow Volume = u.NewValue(75, u.Gallon)

// ReactorMakeupDesignFlow is the per-minute volume of makeup
// water delivered to the VCT when the reactor makeup control
// valve is fully open. Sized at 60 gpm — typical of a
// four-loop Westinghouse plant's RWST-to-VCT makeup line.
//
// Spec justification:
//   - "Because of the influx of makeup water from the reactor
//     makeup control system [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
//   - "The reactor makeup control system dilution is [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:17
var ReactorMakeupDesignFlow Volume = u.NewValue(60, u.Gallon)

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

	// BoronConcentration is the RCS boron concentration in
	// ppm. Plant.Step propagates CVCS.BoronConcentration here
	// so the reactivity model can use it. The reactivity
	// contribution is proportional to (BoronConcentration -
	// CVCSInitialBoron)/100 * CoreBoronWorthPer100ppm — a
	// positive ΔBoron decreases reactivity (boron is a
	// neutron absorber).
	//
	// Spec: "the moderator temperature coefficient is
	// positive at low power (BOL), and [...] the boron
	// concentration changes significantly with fuel
	// depletion."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
	BoronConcentration float64
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

// Feedwater regulator design constants (plans/p22_1.txt).
//
// Spec justification:
//   - Three-element control: "Feedwater flow to each steam generator
//     is controlled by a feedwater regulator in each feedwater line.
//     The regulator is controlled by steam generator level, steam
//     flow and feedwater flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
//   - Pump speed / control valve link: "A signal from the
//     Feedwater Control System also sets the speed of the
//     turbine-driven main feedwater pumps and the position of the
//     motor-driven feedwater pump discharge control valve so as to
//     maintain the main feedwater regulators within their control
//     range."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:73-76
//   - Control-range accommodation: "This setting allows the system
//     to accommodate all operating conditions automatically and
//     provides control margin to accommodate load transients."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:82-85
const (
	// FeedwaterLevelSetpoint is the normal operating steam
	// generator water level (%). The spec describes the "narrow
	// control range"; chosen as 50% — the centre of the narrow
	// range.
	FeedwaterLevelSetpoint = 50.0 // % narrow control range

	// FeedwaterRegulatorGain is the proportional gain of the
	// three-element feedwater regulator (1/s on a fractional
	// valve demand per percent level error). The spec does not
	// state a numerical value; chosen as 0.5 for reasonable
	// damping on a 1 s timescale.
	FeedwaterRegulatorGain = 0.5 // 1/s

	// FeedwaterRegulatorIntegralTime is the integral time
	// constant of the regulator (s). The spec does not state a
	// value; chosen as 10 s for slow steady-state correction
	// without integrator windup during shutdown.
	FeedwaterRegulatorIntegralTime = 10.0 // s

	// FeedwaterSteamFlowGain is the feedforward gain from steam
	// flow (gal/s, normalised to FeedwaterDesignFlow) to valve
	// demand (dimensionless). Chosen as 1.0 so feedwater flow
	// matches steam flow on steady-state load changes.
	FeedwaterSteamFlowGain = 1.0
)

// FeedwaterRegulator models the three-element feedwater regulator
// that maintains steam-generator water level during power
// operation. The regulator receives steam generator level, steam
// flow, and feedwater flow as inputs and computes a valve-position
// demand on the main feedwater control valve.
//
// The simplified model implements a single regulator shared between
// both steam generators (the spec describes one regulator per SG;
// parallel operation is deferred to a higher fidelity band).
//
// Spec justification:
//   - Three-element control: "Feedwater flow to each steam
//     generator is controlled by a feedwater regulator in each
//     feedwater line. The regulator is controlled by steam
//     generator level, steam flow and feedwater flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
//   - Pump speed / control valve link: "A signal from the
//     Feedwater Control System also sets the speed of the
//     turbine-driven main feedwater pumps and the position of the
//     motor-driven feedwater pump discharge control valve so as
//     to maintain the main feedwater regulators within their
//     control range."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:73-76
//   - Control-range accommodation: "This setting allows the
//     system to accommodate all operating conditions
//     automatically and provides control margin to accommodate
//     load transients."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:82-85
type FeedwaterRegulator struct {
	Part

	// ValveDemand is the computed feedwater control valve
	// position (0.0 = closed, 1.0 = fully open). The regulator
	// drives this toward the position that achieves
	// LevelSetpoint.
	ValveDemand float64

	// IntegralError is the running integral of (LevelSetpoint -
	// Level) accumulated while Running is true. The integral
	// freezes when the regulator stops, preventing windup
	// during shutdown.
	IntegralError float64

	// Level is the measured steam generator water level (%).
	// Sourced from the SteamGenerator inventory; the regulator
	// treats it as the process variable.
	Level float64

	// LevelSetpoint is the target SG water level (%) during
	// normal operation. Initialised to FeedwaterLevelSetpoint
	// (50%) at construction; the operator may retune it but the
	// simplified model holds it fixed.
	LevelSetpoint float64

	// SteamFlow is the measured steam flow from this SG
	// (gal/s). Used as feedforward input to anticipate load
	// transients.
	SteamFlow float64

	// FeedwaterFlow is the measured feedwater flow into this SG
	// (gal/s). Used as feedback to close the level loop; the
	// simplified model collapses this to a monitoring input
	// (the PI law acts on Level, not FeedwaterFlow).
	FeedwaterFlow float64

	// Running is true when the regulator is actively
	// controlling the feedwater valve. False when the pump is
	// stopped or the operator has taken manual control.
	Running bool
}

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

// MakeupValve is the operator-controlled reactor makeup control
// valve between the refueling-water storage tank (RWST) and the
// volume control tank (VCT). Open=1 is full-flow; Open=0 is
// closed. Flow is the per-second volume of makeup water delivered
// to the VCT, derived from the Open fraction and a design full-
// open flow rate.
//
// Spec justification:
//   - "The charging pumps take the coolant from the volume control
//     tank [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
//   - "Because of the influx of makeup water from the reactor
//     makeup control system, the volume control tank level
//     controller automatically diverts the outflow from the BTRS
//     into the BRS holdup tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-8
//   - "The reactor makeup control system dilution is [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:17
type MakeupValve struct {
	Part
	Open  float64
	Flow  Volume
}

// Grid models the infinite bus at the station switchyard. It is a
// one-cell representation of the off-site distribution grid with
// frequency and voltage state.
//
// Spec justification:
//   - "The main power system is made up of the plant main generator
//     and the associated controls necessary to transmit power to
//     the off-site distribution grid."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:17-19
//   - "critical importance in the event of large grid system
//     disturbances."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:59-60
//   - Generator-side voltage/frequency anchor: "Output voltage:
//     24,000 to 26,000 volts, three-phase, 60 Hz"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
type Grid struct {
	Part

	// Frequency is the grid frequency in Hz. Normal steady
	// state is 60 Hz; the operator (or scenario script) can
	// apply a step or ramp to simulate grid disturbances. The
	// DEHM uses this frequency as the speed reference for the
	// turbine governor's droop correction.
	// "Wide range speed control of the turbine from turning
	// gear to rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
	Frequency float64

	// Voltage is the grid-side voltage in volts available at
	// the station switchyard. The System Auxiliary Transformer
	// (SAT) picks up house load when this is non-zero.
	Voltage float64

	// LoadDemand is the external grid's MW demand on this
	// plant. It is a pure input: the grid does not model
	// other generators or area control error.
	LoadDemand float64
}

// StationTransformer models the unit auxiliary transformer (UAT)
// and system auxiliary transformer (SAT) split described in the
// spec.
//
// Spec justification:
//   - "The system auxiliary power is supplied from independent
//     and electrically isolated buses of the off-site main
//     distribution grid. This supply is transformed to the plant
//     electrical distribution bus voltage, normally 6600 volts,
//     by one or more system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:40-44
//   - "Likewise, the unit auxiliary power is transformed from
//     the output of the plant main generator through the unit
//     auxiliary transformer to the plant electrical distribution
//     voltage."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:45-48
//   - "When the plant is operating and supplying power to the
//     main distribution grid and the unit auxiliary transformer,
//     the plant distribution buses are split and part are
//     supplied by the plant's own generator and the other part
//     are supplied by the system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:51-56
//   - Automatic transfer: "Because plant buses are capable of
//     receiving power from either of two supplies and because
//     automatic transfer of supplies is provided on sensing loss
//     of power, reliability of power is assured."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
type StationTransformer struct {
	Part

	// UATEnergized is true when the unit auxiliary transformer
	// is supplied from the main generator (breaker closed AND
	// generator producing rated voltage).
	// "the unit auxiliary power is transformed from the output
	// of the plant main generator through the unit auxiliary
	// transformer"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:45-47
	UATEnergized bool

	// SATEnergized is true when the system auxiliary transformer
	// is supplied from the off-site grid (grid voltage > 0).
	// "The system auxiliary power is supplied from independent
	// and electrically isolated buses of the off-site main
	// distribution grid."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:40-42
	SATEnergized bool

	// UATOutputVoltage is the plant distribution bus voltage
	// when UAT is energized (6600 V design).
	UATOutputVoltage float64

	// SATOutputVoltage is the plant distribution bus voltage
	// when SAT is energized (6600 V design).
	SATOutputVoltage float64

	// HouseLoadSuppliedFrom identifies which transformer is
	// currently supplying the station house load: "UAT",
	// "SAT", or "" when neither is energised (black-out). On
	// turbine trip the spec describes automatic transfer to
	// SAT.
	// "house loads automatically transferred to the offsite
	// sources of power."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64-66
	HouseLoadSuppliedFrom string
}

// HouseLoad is the station's own electrical demand: RCPs, feed
// pumps, condensate pumps, heaters, ESF bus trickle charge, HVAC,
// lighting, and control systems.
//
// Spec justification:
//   - "supply power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
//   - "house loads automatically transferred to the offsite
//     sources of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64-66
type HouseLoad struct {
	Part

	// FixedMW is the always-on demand (control systems, HVAC,
	// lighting, ESF bus trickle charge).
	FixedMW float64

	// VariableMW is the demand that scales with turbine load
	// (RCPs, feedwater pumps, condensate pumps).
	VariableMW float64

	// TotalMW is the computed sum for the current step, in
	// watts. The variable component is added when the
	// generator is online and at the supplied load fraction.
	TotalMW float64
}

// CVCS models the Chemical and Volume Control System, the primary
// chemistry and volume-regulation loop. The spec describes it as
// the "Chemical and Volume Control System (CVCS)" and places it
// between the RCS letdown tap, the volume control tank, the
// charging pumps, and the BTRS demineralizers.
//
// Spec justification:
//   - "The operating mode (boration or dilution) and the magnitude
//     of the boron concentration change required is determined by
//     comparison of the measured Tc (cold leg temperature) with
//     the programmed (function of power) Tc."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
//   - "The boron concentration of the water in the VCT is
//     maintained within a narrow band by the system's control of
//     boration/dilution operations." (paraphrase; the VCT
//     boron-concentration control band is described throughout
//     chapter_7/page_134-135.)
//   - "The pressurizer level is controlled by balancing the
//     charging flow to the letdown flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
//   - Boration path: "for boration, an increase in the boric acid
//     concentration of the reactor coolant is accomplished by
//     sending the flow from the VCT, at relatively high
//     temperatures (140°F) (60°C), to the thermal regeneration
//     demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:16-19
//   - Dilution path: "for dilution, a decrease in the boric acid
//     concentration in the reactor coolant is accomplished by
//     sending the flow from the VCT, at relatively low
//     temperatures (50°F) (10°C), to the thermal regeneration
//     demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:26-30
//   - BTRS recirculation pump: "The BTRS recirculation pump
//     provides the motive force to route flow from the VCT through
//     the demineralizers during boration and dilution modes."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39
//   - VCT is part of CVCS: "the volume control tank (part of the
//     Chemical and Volume Control System)"
//     specs/the_westinghouse_pressurized_water_react/chapter_14/page_189.txt:77
//   - VCT level controller overflow: "the volume control tank
//     level controller automatically diverts the outflow from the
//     BTRS into the BRS holdup tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
//   - BRS collects CVCS letdown: "The BRS normally collects water
//     from the RCS through the Chemical and Volume Control System
//     (CVCS) letdown line."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:32-35
type CVCS struct {
	Part

	// Mode selects the current CVCS operating mode. Set by
	// the operator (ICS) or by the VCT level controller when
	// VCTLevel crosses VCTHighLevel.
	Mode CVCSMode

	// ChargingFlow is the flow rate from the charging pumps
	// into the RCS cold legs (gal). Set to CVCSDesignChargingFlow
	// when a pump is running; zero otherwise.
	// "The charging pumps take the coolant from the volume
	// control tank [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
	// "system pressure is maintained by charging and letdown
	// flow control."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:96-97
	ChargingFlow Volume

	// LetdownFlow is the flow rate from the RCS letdown tap
	// through the letdown chiller and demineralizers back to
	// the VCT (gal/s).
	// "The charging pumps take the coolant from the volume
	// control tank [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
	LetdownFlow Volume

	// BoronConcentration is the RCS boron concentration in
	// ppm. This is the primary state variable that boration
	// and dilution modify. Plant.Step propagates this value
	// into ReactorPressureVessel.ControlRods.BoronConcentration
	// so the reactivity model can use it.
	// "The operating mode (boration or dilution) and the
	// magnitude of the boron concentration change required is
	// determined by comparison of the measured Tc (cold leg
	// temperature) with the programmed (function of power)
	// Tc."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
	BoronConcentration float64

	// VCTLevel is the volume control tank level (gal). It is
	// replenished by reactor makeup control system and
	// depleted by BTRS flow; overflow is diverted to the BRS.
	// "the volume control tank level controller automatically
	// diverts the outflow from the BTRS into the BRS holdup
	// tanks."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
	VCTLevel Volume

	// BTRSRecirculationPumpRunning is true when the BTRS pump
	// is circulating VCT flow through the demineralizers.
	// "The BTRS recirculation pump provides the motive force
	// to route flow from the VCT through the demineralizers
	// during boration and dilution modes."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39
	BTRSRecirculationPumpRunning bool

	// BorationRate is the instantaneous rate of boron
	// concentration increase in ppm per second, positive
	// when the BTRS is in boration mode.
	BorationRate float64

	// DilutionRate is the instantaneous rate of boron
	// concentration decrease in ppm per second, positive when
	// the BTRS is in dilution mode.
	DilutionRate float64
}

// CVCSMode enumerates the CVCS operating modes.
//
// Spec justification:
//   - "The operating mode (boration or dilution) and the magnitude
//     of the boron concentration change required is determined by
//     comparison of the measured Tc (cold leg temperature) with
//     the programmed (function of power) Tc."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
//   - VCT high-level letdown-to-BRS path: "the volume control tank
//     level controller automatically diverts the outflow from the
//     BTRS into the BRS holdup tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
type CVCSMode int

const (
	// CVCSVolumeControl is the normal operating mode. Charging
	// and letdown balance to maintain pressurizer level; no
	// boron concentration change.
	// "The pressurizer level is controlled by balancing the
	// charging flow to the letdown flow."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
	CVCSVolumeControl CVCSMode = iota

	// CVCSBoration is the mode where the BTRS demineralizers
	// release boron into the charging stream, raising RCS
	// boron concentration at CVCSBorationRate.
	// "for boration, an increase in the boric acid
	// concentration of the reactor coolant is accomplished by
	// sending the flow from the VCT, at relatively high
	// temperatures (140°F) (60°C), to the thermal
	// regeneration demineralizers."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:16-19
	CVCSBoration

	// CVCSDilution is the mode where the BTRS demineralizers
	// absorb boron from the charging stream, lowering RCS
	// boron concentration at CVCSDilutionRate.
	// "for dilution, a decrease in the boric acid
	// concentration in the reactor coolant is accomplished by
	// sending the flow from the VCT, at relatively low
	// temperatures (50°F) (10°C), to the thermal regeneration
	// demineralizers."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:26-30
	CVCSDilution

	// CVCSLetdown is the high-VCT-level mode where the
	// level controller has diverted BTRS outflow to the BRS
	// holdup tanks. Charging and letdown are halted so the
	// VCT level can stabilise.
	// "the volume control tank level controller automatically
	// diverts the outflow from the BTRS into the BRS holdup
	// tanks."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
	CVCSLetdown
)

// HeatExchanger models a generic shell-and-tube heat exchanger. The
// RHR heat exchangers are the primary use of this type in the model;
// CCW circulates through the shell side and RCS fluid through the
// tubes, so HeatTransferRate is the per-step watts rejected from
// tube-side fluid to shell-side fluid.
//
// Spec justification:
//   - "transfer heat from the hot reactor coolant fluid to the
//     component cooling water circulating through the RHR heat
//     exchanger shell side."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:19-22
type HeatExchanger struct {
	Part

	// HeatTransferRate is the per-step heat flux rejected
	// from the tube-side fluid to the shell-side fluid.
	HeatTransferRate Watts
}

// RHRSTrain models one independent, redundant subsystem of the
// Residual Heat Removal System. Each train consists of one RHR pump,
// one RHR heat exchanger, suction and discharge isolation valves,
// a bypass loop around the heat exchanger, and a minimum-flow
// recirculation line.
//
// Spec justification:
//   - "The RHRS consists of two independent, redundant mechanical
//     subsystems, each of which receives electrical power from one
//     of two separate and redundant electrical power trains. Each
//     subsystem consists of one RHR pump, one RHR heat exchanger,
//     and the required piping, valves, and instrumentation."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:1-5
//   - "A bypass loop is provided around the RHR heat exchanger to
//     permit control of RCS cooldown while maintaining a constant
//     total pump flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:50-53
//   - "A minimum flow for the RHR pump is provided by a
//     recirculation line from the heat exchanger outlet to the pump
//     suction which remains open in all system operating modes."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:53-57
type RHRSTrain struct {
	Part
	Pump

	// HeatExchanger models the RHR heat exchanger shell-and-tube
	// bundle that transfers decay heat from the RCS fluid to the
	// component cooling water.
	HeatExchanger

	// SuctionValves are the two series motor-operated isolation
	// valves on the hot-leg suction line. Both must be open for
	// the train to take suction from the RCS.
	SuctionValves [2]Valve

	// BypassValve is the flow-control valve in the bypass loop
	// around the RHR heat exchanger. Open=1 means full bypass
	// (no heat rejection); Open=0 means full flow through the
	// HX. The spec describes this as a remote manual throttle
	// valve for cooldown-rate control.
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:58-66
	BypassValve Valve

	// WarmupComplete is true once the mini-flow warmup period
	// has finished and the hot-leg block valves may be opened.
	WarmupComplete bool

	// WarmupTimer tracks the warmup duration.
	WarmupTimer time.Duration

	// ReturnFlow is the total flow returned to the RCS cold legs
	// (gal/s). The bypass controller maintains this constant
	// regardless of BypassValve position.
	ReturnFlow Volume
}

// RHRS models the complete Residual Heat Removal System. The spec
// describes two independent redundant subsystems.
//
// Spec justification:
//   - "The RHRS consists of two independent, redundant mechanical
//     subsystems, each of which..."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:75
//   - Primary function, design basis, and thresholds in the same
//     chapter.
type RHRS struct {
	Part

	// TrainA and TrainB are the two independent subsystems.
	TrainA RHRSTrain
	TrainB RHRSTrain

	// Running indicates at least one train is operating.
	Running bool

	// HasTakenOver is true once the RHRS has assumed the reactor
	// cooling function from the steam dump system. This gates
	// steam-dump shutdown in Plant.Step.
	HasTakenOver bool
}

// RHRS design constants.
//
// Spec justification:
//   - "reduce the RCS temperature to 140°F (60°C) within 20 hours
//     following reactor shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:31-32
//   - "When the reactor coolant temperature has dropped to 350°F
//     (177°C) and the pressure has been reduced to less than
//     approximately 400 psig (28 bar), the RHR equipment is placed
//     in operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
//   - "One reactor coolant pump is kept running ... until the
//     cooling temperature is below 160°F (71°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:37-40
const (
	// RHRSTempThreshold is the maximum RCS temperature at which
	// the RHRS may be started (350°F = 177°C).
	RHRSTempThreshold = 350.0 // °F

	// RHRSPressureThreshold is the maximum RCS pressure at which
	// the RHRS may be started (400 psig ≈ 28 bar).
	RHRSPressureThreshold = 400.0 // psig

	// RHRSDesignCooldownTemp is the target RCS temperature after
	// 20 hours of RHRS operation (140°F = 60°C).
	RHRSDesignCooldownTemp = 140.0 // °F

	// RHRSDesignCooldownTime is the design cooldown period from
	// 350°F to 140°F with both trains operating.
	RHRSDesignCooldownTime = 20.0 * time.Hour

	// RHRSCoolingTempBelowRCPStop is the RCS temperature below
	// which the last running RCP may be stopped (160°F = 71°C).
	RHRSCoolingTempBelowRCPStop = 160.0 // °F

	// RHRSPumpMinFlowGalPerStep is the minimum recirculation
	// flow through the RHR pump (gal/step).
	RHRSPumpMinFlowGalPerStep = 50.0

	// RHRSTrainDesignFlow is the design flow through one RHR
	// train at full-speed pump operation (gal/s).
	RHRSTrainDesignFlow = 200.0

	// RHRSTrainWarmupTime is the mini-flow warmup period before
	// hot-leg isolation valves may be opened.
	RHRSTrainWarmupTime = 5 * time.Minute

	// RHRSHXApproachTemp is the temperature approach of the RHR
	// heat exchanger (RCS outlet temp - CCW inlet temp, °F).
	RHRSHXApproachTemp = 20.0
)

// Accumulator models one (or, in the simplified model, the combined
// bank of) passive nitrogen-precharged water-storage tank(s) that
// discharge into the RCS cold legs whenever RCS pressure falls
// below the tank precharge pressure. The accumulators are passive:
// no electrical train, no pump motion, no actuation delay.
//
// Spec justification:
//   - "The accumulator tanks are passive components normally
//     requiring no maintenance, and are located inside the
//     containment to provide rapid injection of water following
//     a LOCA."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:11-14
//   - "Similarly, the accumulator discharge will begin
//     automatically as the RCS pressure decreases below the
//     accumulator tank pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
type Accumulator struct {
	Part

	// Pressure is the nitrogen precharge pressure inside the
	// tank (psia). The tank discharges into the RCS cold leg
	// whenever RCS pressure falls below this value.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
	Pressure Pressure

	// Level is the remaining water volume (gal). Decreases at
	// the discharge flow rate whenever Discharging is true.
	Level Volume

	// Capacity is the full water capacity of the tank (gal).
	Capacity Volume

	// Discharging is true while the accumulator is actively
	// discharging into the RCS cold leg (i.e., RCS pressure
	// has fallen below Pressure).
	Discharging bool
}

// SISSuctionSource enumerates the suction alignment for the SIS
// pumps (HHSI and RHR). During the injection phase both pumps draw
// from the RWST; after the RWST low-level signal the pumps
// re-align to containment-sump suction.
//
// Spec justification:
//   - "The HHSI and RHR pumps are aligned to take suction directly
//     from the RWST."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
//   - "both the HHSI and RHR pumps for each subsystem take
//     suction directly from one of the two containment sump
//     recirculation lines."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:173-175
type SISSuctionSource int

const (
	// SISSuctionRWST draws from the refueling water storage
	// tank during the injection phase.
	SISSuctionRWST SISSuctionSource = iota

	// SISSuctionSump draws from the containment sump during
	// the long-term recirculation phase.
	SISSuctionSump
)

// SIS design constants (plans/p16_1.txt).
//
// Spec justification:
//   - SIS subsystem architecture: "two high-head safety injection
//     pumps, two RHR pumps, and four accumulators."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:41-42
//   - Accumulator passive role and precharge-driven discharge:
//     "The accumulator tanks are passive components [...]" and
//     "the accumulator discharge will begin automatically as the
//     RCS pressure decreases below the accumulator tank pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:11-14,
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
//   - HHSI miniflow / deadheading prevention: "A continuous
//     minimum flow path which does not require isolation during
//     the injection phase is provided so that an inadvertent pump
//     start will not result in the deadheading of the HHSI pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_096.txt:26-30
//   - HHSI shutoff head: "will run on miniflow until the RCS
//     pressure falls below the respective shutoff heads of the
//     pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:29-30
//   - Recirculation switchover: "initiated automatically when the
//     RWST level channels indicate an RWST level less than a
//     low-level setpoint in conjunction with the initiation of the
//     engineered safeguards actuation signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:56-60
const (
	// SISAccumulatorPrechargePressure is the nitrogen
	// precharge pressure in each accumulator tank (psia).
	// Discharge into the RCS cold leg begins when RCS pressure
	// drops below this value.
	SISAccumulatorPrechargePressure Pressure = 600.0

	// SISAccumulatorCount is the number of accumulator tanks
	// in the four-loop plant (one per RCS loop).
	// "four accumulators."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:42
	SISAccumulatorCount = 4

	// SISHHSIShutoffHead is the shutoff head of each HHSI pump
	// (ft). Above this head the pump delivers zero flow; the
	// HHSI pump starts and runs on miniflow until RCS pressure
	// falls below this head.
	// "run on miniflow until the RCS pressure falls below the
	// respective shutoff heads of the pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:29-30
	SISHHSIShutoffHead float64 = 2800.0

	// SISHHSIMiniFlowFraction is the fraction of design flow
	// that circulates through the minimum-flow path back to
	// the RWST during the injection phase.
	// "A continuous minimum flow path which does not require
	// isolation during the injection phase is provided so that
	// an inadvertent pump start will not result in the
	// deadheading of the HHSI pump."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_096.txt:26-30
	SISHHSIMiniFlowFraction = 0.15

	// SISRWSTLowLevel is the RWST level (gal) below which the
	// automatic recirculation switchover occurs. The spec
	// describes it as a "low-level setpoint" in the level
	// channels; the numerical value is not stated in the
	// quoted pages and is chosen to be 10% of full RWST
	// capacity.
	// "indicate an RWST level less than a low-level setpoint"
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:56-57
	SISRWSTLowLevel = 5000.0

	// SISRWSTCapacity is the total borated-water capacity of
	// the RWST (gal). The spec does not state a numeric
	// capacity for the RWST; 50,000 gal is sized for a
	// four-loop plant's injection-phase inventory. The EFWS
	// primary-tank sizing language (13-hour hot standby, 5-hour
	// cooldown) appears in
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:53-59,
	// and the 48-hour alternate-supply capacity in
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:90-93,
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:9-12.
	// Neither passage describes the RWST numerically.
	SISRWSTCapacity = 50000.0

	// SISRecirculationSwitchoverDelay is the response time for
	// the protection logic to open the recirculation valves and
	// close the HHSI pump miniflow isolation valves after the
	// RWST low-level signal is generated (s). The spec
	// describes the switchover as automatic; the value is
	// sized for sub-second protection logic and is below
	// physical-fluid transport timescales.
	// "Protection logic is provided to automatically open the
	// recirculation valve in each of the SIS subsystems when
	// the above signals are generated."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:59-60
	SISRecirculationSwitchoverDelay float64 = 2.0
)

// SISAccumulatorPerTankCapacity is the water capacity of one
// accumulator tank (gal). Sized to 1,150 gal, typical of a
// four-loop Westinghouse accumulator tank.
var SISAccumulatorPerTankCapacity Volume = u.NewValue(1150, u.Gallon)

// SISAccumulatorDischargeRate is the maximum flow rate at which
// an accumulator tank discharges into the RCS cold leg (gal/s).
// The spec does not state a numeric per-tank discharge rate; 230
// gpm per tank is the typical Westinghouse four-loop accumulator
// design.
var SISAccumulatorDischargeRate Volume = u.NewValue(230.0/60.0, u.Gallon)

// SISHHSIDesignFlow is the design discharge flow rate of each
// HHSI pump (gal/s). 970 gpm is the typical rated flow for the
// Westinghouse four-loop HHSI pump.
var SISHHSIDesignFlow Volume = u.NewValue(970.0/60.0, u.Gallon)

// SIS models the Safety Injection System's high-head injection
// phase state: the passive accumulator bank, the HHSI pump running
// flags, the active suction source, the RWST inventory, and the
// derived injection flow rate into the RCS cold legs.
//
// Spec justification for the field set:
//   - "The major components are two high-head safety injection
//     pumps, two RHR pumps, and four accumulators."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:41-42
//   - "The receipt of the safety injection signal initiates the
//     following emergency core cooling actions: The onsite
//     emergency diesel generators are started. The HHSI pumps are
//     started."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
//   - "The HHSI and RHR pumps are aligned to take suction directly
//     from the RWST."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
//   - "initiated automatically when the RWST level channels
//     indicate an RWST level less than a low-level setpoint in
//     conjunction with the initiation of the engineered
//     safeguards actuation signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:56-60
type SIS struct {
	Part

	// Accumulator is the four-tank accumulator bank modelled as
	// a single tank with four-fold per-tank capacity for the
	// simplified injection model. Discharge begins automatically
	// when RCS pressure drops below Pressure.
	Accumulator Accumulator

	// HHSIPumpRunning is the running state of the two HHSI
	// pumps (the spec describes them as one pump per
	// independent subsystem, both started by the safety-
	// injection actuation signal). For this fidelity a single
	// flag models both pumps.
	HHSIPumpRunning bool

	// HHSISuction is the active suction source for the HHSI
	// pumps. SISSuctionRWST during the injection phase;
	// SISSuctionSump after the RWST low-level switchover.
	HHSISuction SISSuctionSource

	// RWSTLevel is the remaining borated water volume in the
	// refueling water storage tank (gal). The HHSI and RHR
	// pumps both draw from this common suction source during
	// the injection phase.
	// "The HHSI and RHR pumps are aligned to take suction
	// directly from the RWST."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	RWSTLevel Volume

	// HHSIInjectionFlow is the per-second volume of borated
	// water the HHSI pumps deliver into the RCS cold legs
	// (gal/s). It is derived from HHSIPumpRunning, HHSISuction,
	// and SISHHSIDesignFlow; SISHHSIMiniFlowFraction is routed
	// back to the RWST via the miniflow path during the
	// injection phase.
	HHSIInjectionFlow Volume

	// AccumulatorInjectionFlow is the per-second volume of
	// borated water the accumulator bank delivers into the RCS
	// cold legs (gal/s). It is non-zero only while the
	// accumulator is actively discharging (RCS pressure below
	// the precharge threshold and Level > 0).
	AccumulatorInjectionFlow Volume

	// SwitchoverTimer is the elapsed time since the RWST
	// low-level setpoint was reached. It tracks the SISRecirculationSwitchoverDelay
	// response window before the SISHHSISuction aligns to sump.
	SwitchoverTimer time.Duration

	// SwitchoverComplete is true once the recirculation
	// switchover logic has fired. Latched true after the first
	// switchover to prevent repeated toggling.
	SwitchoverComplete bool
}

// EFWSTank models the primary emergency feedwater supply tank in
// each EFWS subsystem. The primary tank is sized for 13 hours of
// hot standby plus 5 hours of cooldown, and its water temperature
// is limited to 120°F (48.9°C).
//
// Spec justification:
//   - "A primary emergency feedwater supply tank, to which the
//     suction of the emergency feedwater pumps are normally
//     aligned, is provided in each subsystem. The tanks are
//     safety grade and seismically qualified. Each tank contains
//     a quantity of condensate quality water sufficient to allow
//     the plant to be maintained in hot standby for 13 hours then
//     allow a 5-hour cooldown of the plant to 350°F (176.7°C).
//     The maximum permissible water temperature in the primary
//     emergency feedwater supply tank is 120°F (48.9°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:48-59
type EFWSTank struct {
	Part

	// Level is the remaining water volume (gal).
	Level Volume

	// Capacity is the full water capacity of the tank (gal).
	Capacity Volume

	// Temperature is the water temperature (°F). The spec
	// limits the primary tank to 120°F (48.9°C).
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:56-59
	Temperature Temperature

	// IsPrimary is true for the primary emergency feedwater
	// tank; false for the alternate (condensate storage tank)
	// source. The simplified model only populates primary
	// tanks.
	IsPrimary bool
}

// EFWSPump extends AFWPump with the EFWS-specific valves: the
// flow-modulating valve in the discharge line, the orificed
// recirculation line back to the tank, and the steam admission
// valve for the turbine-driven pump.
//
// Spec justification:
//   - "Normally open, fail-open, air-operated, flow-modulating
//     valves are located in each pump discharge line. These
//     valves will be normally full open when the system is
//     activated."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:1-4
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:94-95
//   - Recirculation line: "Each pump is provided with an orificed
//     recirculation line leading back to the primary emergency
//     feedwater tank."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:17-20
//   - Steam admission valve: "The steam supply line for each
//     turbine driven pump is connected to the main steam line
//     from a steam generator. This line is fitted with a steam
//     admission valve which is a pneumatically operated valve
//     arranged to fail-open on loss of air or electrical power."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
type EFWSPump struct {
	AFWPump

	// FlowModulatingValve is the discharge flow-modulating
	// valve. Open=1 is full flow (assumed for the first 30
	// minutes after actuation); Open=0 is shut.
	FlowModulatingValve Valve

	// RecirculationValve is the orificed recirculation line
	// from the pump discharge back to the primary emergency
	// feedwater tank. Open=1 is normal operating position.
	RecirculationValve Valve

	// SteamAdmissionValve is the steam admission valve on the
	// turbine-driven pump's steam supply line. Fail-open on
	// loss of air or electrical power.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
	SteamAdmissionValve Valve
}

// EFWSSubsystem models one of the two identical EFWS subsystems
// (A and B). Each subsystem has its own primary tank and one
// each of a motor-driven and a turbine-driven pump.
//
// Spec justification:
//   - "The EFWS consists of two identical subsystems each of
//     which receives electrical power from one of two separate
//     Safety Class 1E electrical power trains. Each subsystem
//     consists of a primary emergency feedwater tank, one motor
//     driven emergency feedwater pump, one turbine driven
//     emergency feedwater pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:61-69
type EFWSSubsystem struct {
	Part

	// Tank is the primary emergency feedwater supply tank.
	Tank EFWSTank

	// MotorPump is the motor-driven emergency feedwater pump.
	MotorPump EFWSPump

	// TurbinePump is the turbine-driven emergency feedwater
	// pump.
	TurbinePump EFWSPump

	// MotorPumpRunning and TurbinePumpRunning are the per-step
	// running states set by the auto-start logic in Plant.Step.
	MotorPumpRunning   bool
	TurbinePumpRunning bool
}

// EFWS models the complete Emergency Feedwater System: two
// independent, identical subsystems, each with one motor-driven
// and one turbine-driven pump sharing a primary tank. The two
// pumps in each subsystem are powered from diverse power sources
// (Class 1E electrical train for the motor-driven pump; main
// steam line through a fail-open admission valve for the
// turbine-driven pump) so the system can sustain decay-heat
// removal through either loss-of-offsite-power or main-steam
// failure events.
//
// Spec justification:
//   - Diverse power sources: "The use of both motor driven and
//     turbine driven pumps satisfies the requirement that the
//     pumps be powered by diverse power sources."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:1-5
//   - "The EFWS thereby maintains the capability of the steam
//     generators to remove plant stored heat and core decay heat
//     by converting the emergency feedwater to steam which is
//     then discharged to the condenser or to the atmosphere."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:23-32
type EFWS struct {
	Part

	// SubsystemA and SubsystemB are the two independent
	// subsystems.
	SubsystemA EFWSSubsystem
	SubsystemB EFWSSubsystem

	// Active is true while any EFWS pump is running and
	// delivering water to the steam generators.
	Active bool

	// MotorPumpStartCount and TurbinePumpStartCount are the
	// cumulative count of times the motor/turbine pumps have
	// been started in this run. Exposed for operator-display
	// and instrumentation tests.
	MotorPumpStartCount   int
	TurbinePumpStartCount int
}

// EFWS design constants.
//
// Spec justification:
//   - Primary tank sizing language: "Each tank contains a
//     quantity of condensate quality water sufficient to allow
//     the plant to be maintained in hot standby for 13 hours
//     then allow a 5-hour cooldown of the plant to 350°F
//     (176.7°C). The maximum permissible water temperature in
//     the primary emergency feedwater supply tank is 120°F
//     (48.9°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-59
//   - Two identical subsystems: "The EFWS consists of two
//     identical subsystems each of which receives electrical
//     power from one of two separate Safety Class 1E electrical
//     power trains. Each subsystem consists of a primary
//     emergency feedwater tank, one motor driven emergency
//     feedwater pump, one turbine driven emergency feedwater
//     pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:61-69
//   - Performance requirement: "The system is designed to
//     supply at least the minimum required flow, within one
//     minute of the actuation signal, to at least two effective
//     steam generators [...], and to continue this delivery for
//     an indefinite period without operator action."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:23-28
//   - 30-minute operator delay: "it is assumed that no
//     operator action can be taken for 30 minutes, and for this
//     period it is assumed that these valves will be full open."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:36-39
//   - Auto-start signals:
//       "Low-low level in 2/4 level channels in any one steam
//       generator" → Motor driven pumps.
//       specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:71-79
//       "Low-low level in 2/4 level channels in any two steam
//       generators" → Turbine drive pumps.
//       specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:82-92
//       "Safety injection" → Motor driven pumps.
//       specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:94-98
//   - EFWS secured: "the reactor coolant hot leg temperature
//     is reduced to 350°F (176.7°C) and the RHRS has been
//     activated, the EFWS is secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
//   - EFWS secured (longer-form restatement):
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:91-96
//   - Alternate CST source: "reactor coolant pump operating,
//     for 2 days (48 hours) beginning 18 hours after reactor
//     trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:90-92
const (
	// EFWSDeliveryTime is the time (s) within which the system
	// must supply minimum required flow to at least two
	// effective steam generators after the actuation signal.
	// "supply at least the minimum required flow, within one
	// minute of the actuation signal"
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:24-25
	EFWSDeliveryTime float64 = 60.0 // 1 minute

	// EFWSOperatorDelay is the assumed time (s) before the
	// operator can take action to adjust the flow-modulating
	// valves.
	// "it is assumed that no operator action can be taken for
	// 30 minutes"
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:36-38
	EFWSOperatorDelay float64 = 1800.0 // 30 minutes

	// EFWSAlternateDuration is the duration (s) for which the
	// alternate emergency feedwater source (condensate storage
	// tanks) can sustain the plant: 2 days (48 hours)
	// beginning 18 hours after reactor trip.
	// "reactor coolant pump operating, for 2 days (48 hours)
	// beginning 18 hours after reactor trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:90-92
	EFWSAlternateDuration float64 = 48.0 * 3600.0

	// EFWSAlternateStartDelay is the delay (s) after reactor
	// trip before the alternate source becomes the primary
	// supply.
	EFWSAlternateStartDelay float64 = 18.0 * 3600.0

	// EFWSLevelLowLow is the steam-generator water level (gal)
	// below which the EFWS auto-start logic considers the
	// level "low-low". The spec describes the actuation logic
	// as a 2-out-of-4 level-channel coincidence, not a
	// numeric SG-volume threshold; the value here is sized to
	// be ~30% of the SG water inventory for a four-loop
	// plant.
	EFWSLevelLowLow float64 = 3000.0

	// EFWSSecureTemp is the primary system hot-leg temperature
	// (°F) below which the EFWS is secured and the RHRS takes
	// over decay-heat removal.
	// "the EFWS continues to supply feedwater to the steam
	// generators throughout the cooldown until the primary
	// system hot leg temperature is reduced to 350°F (176.7°C).
	// At this temperature, the Residual Heat Removal System
	// (RHRS) is activated and the EFWS is secured."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:91-96
	// "the reactor coolant hot leg temperature is reduced to
	// 350°F (176.7°C) and the RHRS has been activated, the EFWS
	// is secured."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
	EFWSSecureTemp float64 = 350.0
)

// EFWSPrimaryTankCapacity is the full water capacity of each
// primary emergency feedwater tank (gal). The spec describes the
// required duration (13 h hot standby + 5 h cooldown) but does not
// state a numeric capacity. The value 25,000 gal is sized so a
// single motor-driven pump (150 gpm) depletes the tank in roughly
// 2.8 hours, leaving margin for the operator-driven cooldown rate.
//
// Spec justification:
//   - "Each tank contains a quantity of condensate quality water
//     sufficient to allow the plant to be maintained in hot
//     standby for 13 hours then allow a 5-hour cooldown of the
//     plant to 350°F (176.7°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-56
var EFWSPrimaryTankCapacity Volume = u.NewValue(25000, u.Gallon)

// EFWSPrimaryTankMaxTemp is the maximum permissible water
// temperature in the primary emergency feedwater supply tank (°F).
// "The maximum permissible water temperature in the primary
// emergency feedwater supply tank is 120°F (48.9°C)."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:56-59
var EFWSPrimaryTankMaxTemp Temperature = u.NewValue(120, u.Fahrenheit)

// EFWSMotorPumpFlow is the design discharge flow rate of each
// motor-driven EFWS pump (gal/s). The spec does not state a
// numeric gpm; the value 150 gpm is a typical rated flow for a
// Westinghouse four-loop motor-driven AFW pump.
var EFWSMotorPumpFlow Volume = u.NewValue(150.0/60.0, u.Gallon)

// EFWSTurbinePumpFlow is the design discharge flow rate of each
// turbine-driven EFWS pump (gal/s). The spec does not state a
// numeric gpm; the value 200 gpm is higher than the motor-driven
// pump because the turbine governor can track steam demand.
var EFWSTurbinePumpFlow Volume = u.NewValue(200.0/60.0, u.Gallon)

// CCWS models the Component Cooling Water System — the
// intermediate cooling loop that removes heat from safeguards
// equipment and rejects it to the ultimate heat sink via the
// service water system. The simplified model collapses the
// Safeguards subsystem (two independent trains, two pumps each,
// one HX, one surge tank) and the Auxiliary subsystem (two pumps,
// one HX, one surge tank) into a single lumped component.
//
// Spec justification:
//   - Safeguards subsystem architecture: "The Safeguards
//     subsystem is divided into two separate and redundant
//     safeguards trains and provides cooling water to safeguards
//     equipment such as the residual heat removal heat
//     exchangers and pump, the reactor coolant pump thermal
//     barriers, the containment fan coolers, the containment
//     spray pump heat exchangers, the spent fuel pit heat
//     exchangers, and the positive displacement pump. Each
//     safeguards train is sized to supply 100-percent
//     post-accident component cooling flow requirements."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:33-42
//   - No cross-connection between trains: "to retain total
//     physical separation consisting of two separate, independent
//     safeguards trains with no cross connection between the two
//     trains. Components of each safeguards train (pumps and
//     heat exchanger) are housed in separate safeguards equipment
//     cubicles."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:8-14
//   - Flow path: "The component cooling water flows from the
//     pump, through the shell side of the component cooling heat
//     exchangers, through the components being cooled, and back
//     to the pump. The surge tank is connected to the suction
//     side of the component cooling pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:51-56
//   - Surge-tank function: "They provide a surge volume to
//     accommodate thermal expansion and contraction of system
//     volume during transients, and collects water that may leak
//     into the system from components being cooled."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:60-64
//   - Temperature limit: "The component cooling water inlet
//     temperature to various components during normal cooldown
//     is permitted to increase to 120°F (49°C), however it must
//     return to 105°F (40°C) after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
//   - Standby-pump redundancy: "Failure of a pump will not
//     affect the time required for cooldown because a standby
//     pump in each train of the Safeguards subsystem is
//     provided. A spare pump is also available in the Auxiliary
//     subsystem."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:92-96
//   - Seismic qualification: "The CCWS pumps, heat exchangers
//     of the Safeguards subsystem and its associated piping,
//     valves and instrumentations are seismically qualified as
//     required for an engineered safeguards system."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:116-120
type CCWS struct {
	Part

	// PumpRunning is true when at least one safeguards train or
	// the auxiliary train is running. The spec describes standby
	// pumps in each train; the simplified model collapses
	// running state to a single boolean.
	// "a standby pump in each train of the Safeguards subsystem
	// is provided."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:93-94
	PumpRunning bool

	// InletTemp is the CCW inlet temperature (°F). During normal
	// cooldown this is permitted to rise to 120°F but must
	// recover to 105°F within four hours.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
	InletTemp Temperature

	// OutletTemp is the CCW outlet temperature (°F) after picking
	// up heat from the cooled components. This is the value
	// supplied to the RHRS Step as the ccwInletTemp argument.
	OutletTemp Temperature

	// SurgeTankLevel is the remaining water volume in the CCWS
	// surge tank (gal). The tank provides surge volume for
	// thermal expansion and collects leakage.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:60-64
	SurgeTankLevel Volume
}

// CCWS design constants.
//
// Spec justification:
//   - Temperature limit and recovery: "The component cooling
//     water inlet temperature to various components during
//     normal cooldown is permitted to increase to 120°F (49°C),
//     however it must return to 105°F (40°C) after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
//   - "During plant normal cooldown, the standby train of the
//     Safeguards subsystem is placed in operation at
//     approximately four hours after reactor shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:102-104
const (
	// CCWSInletSteadyStateTemp is the design steady-state CCW
	// inlet temperature (°F). Below this value recovery is
	// targeted.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
	CCWSInletSteadyStateTemp = 105.0 // °F

	// CCWSInletMaxCooldownTemp is the maximum permitted CCW
	// inlet temperature during cooldown transients (°F).
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
	CCWSInletMaxCooldownTemp = 120.0 // °F

	// CCWSRecoveryTimeConstant is the first-order-lag time
	// constant (s) used to drive CCW inlet temperature back
	// toward CCWSInletSteadyStateTemp after a transient. Sized
	// so a 120°F → 105°F excursion completes in roughly four
	// hours (5× tau ≈ 4 h).
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
	CCWSRecoveryTimeConstant = 1.0 * time.Hour
)

// CCWSSurgeTankCapacity is the lumped total water volume of the
// CCWS surge tanks (gal). The spec describes three surge tanks
// (one per safeguards train and one for the auxiliary subsystem)
// but does not state a numeric capacity; the value chosen is
// representative of a single surge tank for a four-loop plant.
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:60-67
var CCWSSurgeTankCapacity Volume = u.NewValue(5000, u.Gallon)

// CCWSDesignFlow is the lumped CCW flow rate through all cooled
// components combined (gal/s). The spec does not state a single
// total flow; the value 800 gpm (≈18.5 gal/s) is representative
// of a single safeguards train serving RHR HX, CVCS HX, and RCP
// thermal barriers at full post-accident load.
// "Each safeguards train is sized to supply 100-percent
// post-accident component cooling flow requirements."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:41-42
var CCWSDesignFlow Volume = u.NewValue(800.0/60.0, u.Gallon)

// CCHXApproachTemp is the lumped temperature rise across the
// CCWS heat exchanger (°F). Used as the single-step outlet
// temperature offset above the inlet so the simplified model
// does not need to track individual cooled-component heat
// loads. The spec does not state an approach temperature; 10°F
// is typical for a shell-and-tube HX with moderate heat duties.
var CCHXApproachTemp Temperature = u.NewValue(10.0, u.Fahrenheit)

// ContainmentSpraySystem models the Containment Spray System —
// the engineered safeguard that limits peak containment pressure
// and removes airborne fission products following a LOCA. The
// simplified model collapses the two redundant subsystems into
// a single lumped component.
//
// Spec justification:
//   - CSS purpose: "The Containment Spray System (CSS) (Figure
//     6-4) is an engineered safeguard designed to limit the
//     peak pressure in the reactor containment building to a
//     pressure less than the containment design pressure, in the
//     event of a loss-of-coolant accident or a steam break
//     accident inside the containment. The system also acts to
//     remove airborne fission products, principally iodine, from
//     the containment atmosphere, should they be present due to
//     a fuel cladding break."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:75-85
//   - Subsystem composition: "The system consists of a
//     containment spray storage tank (CSST), a spray additive
//     tank (SAT), two pumps, two heat exchangers, and a set of
//     spray ring headers located in the upper dome of the
//     containment and having nozzles designed to provide
//     adequate containment spray coverage."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:14-20
//   - Pump miniflow line: "A pump miniflow line, containing a
//     heat exchanger and flow limiting orifice, and a test loop
//     with a throttling valve are connected in parallel across
//     each pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:20-24
//   - Independent capability: "Each one of these subsystems is
//     also independently capable of delivering the necessary
//     flow to limit containment pressure in conjunction with the
//     Reactor Containment Fan Cooler System."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:28-32
//   - Actuation: "Initial Injection Mode – Containment spray is
//     initiated automatically in response to a containment high
//     pressure signal. This signal starts the spray pumps, opens
//     the spray header isolation valves "A", and opens the SAT
//     isolation valves "B"."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
//   - Suction alignment during injection: "Suction is provided
//     to the spray pump from the CSST, with the SAT providing
//     sodium hydroxide solution. The SAT drains into each spray
//     pump's suction header from the CSST; this mixing results
//     in a solution pH that is conducive to the entrainment of
//     iodine (airborne fission product)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:61-66
//   - End of injection: "(T)he storage tanks are sized to
//     provide approximately 30 minutes of spray with spray
//     additive, and when the CSST is drained to the "Lo-Lo"
//     setpoint, the spray pumps are manually switched to the
//     recirculation mode of spray operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-21
//   - Sump pH adjustment: "An additional function which is
//     performed by the CSS during its normal operation is the
//     pH adjustment of the containment sump. The sodium
//     hydroxide (spray additive) that is delivered through the
//     injection phase is sufficient to raise the long-term sump
//     pH to a level adequate for inhibiting chloride stress
//     corrosion/cracking of stainless steel components, and for
//     keeping the iodine entrained in the water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:37-49
//   - Manual switchover to recirculation: "The recirculation
//     mode of the CSS will be initiated manually by the
//     operator. To accomplish this, the sump isolation valves
//     "C" are opened, and the spray pump suction isolation
//     valves "D" are closed. The pumps will then deliver spray
//     from the sump to the spray headers. Spray additive is not
//     injected during this mode."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:63-70
//   - CCWS services CSS pump HXs: "containment spray pump heat
//     exchangers" are among the safeguards components cooled by
//     the CCWS.
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:39
type ContainmentSpraySystem struct {
	Part

	// Running is true when at least one CSS pump is running.
	Running bool

	// SprayHeaderOpen is true when the spray header isolation
	// valves "A" are open and spray is being delivered to the
	// ring headers.
	// "opens the spray header isolation valves "A""
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:59-60
	SprayHeaderOpen bool

	// CSSTLevel is the remaining borated water volume in the
	// containment spray storage tank (gal). Decreases at the
	// spray flow rate during the injection phase.
	// "Suction is provided to the spray pump from the CSST"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:61-62
	CSSTLevel Volume

	// SATLevel is the remaining sodium hydroxide solution
	// volume in the spray additive tank (gal). Drains
	// proportionally with the CSST during the injection phase.
	// "the SAT providing sodium hydroxide solution"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:62-63
	SATLevel Volume

	// CSSTLowLevelSetpoint is the CSST level (gal) at which the
	// operator manually switches the CSS to recirculation mode
	// (the "Lo-Lo" setpoint). The simplified model uses this as
	// a low-level alarm that triggers operator-driven switchover.
	// "when the CSST is drained to the "Lo-Lo" setpoint, the
	// spray pumps are manually switched to the recirculation
	// mode of spray operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:18-21
	CSSTLowLevelSetpoint Volume

	// SuctionSump is true when the spray pumps have been
	// switched to sump recirculation mode (valves "C" open,
	// valves "D" closed). Latched true once switchover fires.
	// "The recirculation mode of the CSS will be initiated
	// manually by the operator."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:63-64
	SuctionSump bool

	// SumpPHAdjusted is true once the CSS has delivered enough
	// spray additive during the injection phase to raise the
	// long-term containment sump pH to an adequate level. This
	// is a one-time event that latches true when CSSTLevel drops
	// below CSSTLowLevelSetpoint.
	// "the sodium hydroxide (spray additive) that is delivered
	// through the injection phase is sufficient to raise the
	// long-term sump pH to a level adequate for inhibiting
	// chloride stress corrosion/cracking of stainless steel
	// components, and for keeping the iodine entrained in the
	// water."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:40-49
	SumpPHAdjusted bool

	// SprayFlow is the rate of spray solution delivery to the
	// containment (gal/s). Zero when Running is false or
	// SprayHeaderOpen is false; CSSDesignFlow when both are
	// true.
	SprayFlow float64
}

// CSS design constants.
//
// Spec justification:
//   - Additive duration: "(T)he storage tanks are sized to
//     provide approximately 30 minutes of spray with spray
//     additive."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-18
//   - Lo-Lo setpoint: "when the CSST is drained to the "Lo-Lo"
//     setpoint, the spray pumps are manually switched to the
//     recirculation mode of spray operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:18-21
//   - Sump pH adjustment: "the sodium hydroxide (spray
//     additive) that is delivered through the injection phase
//     is sufficient to raise the long-term sump pH to a level
//     adequate for inhibiting chloride stress corrosion/
//     cracking of stainless steel components, and for keeping
//     the iodine entrained in the water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:40-49
//   - CCWS safeguards loads include CSS pump heat exchangers.
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:39
const (
	// CSSSprayAdditiveDuration is the approximate time the CSST
	// and SAT can supply spray with additive (min). The spec
	// states "approximately 30 minutes"; the constant matches
	// the spec language.
	// "(T)he storage tanks are sized to provide approximately
	// 30 minutes of spray with spray additive."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-18
	CSSSprayAdditiveDuration = 30.0 // minutes

	// CSSDesignFlow is the lumped spray flow rate from the CSS
	// into the containment (gal/min). The spec does not state
	// an explicit spray flow rate; 500 gpm is representative of
	// a two-pump safeguards train delivering borated water
	// through ring headers.
	CSSDesignFlow float64 = 500.0 // gpm per subsystem, simplified to lumped

	// CSSLowLevelSetpoint is the CSST level (gal) at which the
	// operator manually switches the CSS to recirculation mode
	// (Lo-Lo setpoint). The spec does not state a numerical
	// value; 10% of CSSTCapacity (1500 gal) is a typical low-low
	// setpoint for the injection-phase CSST.
	// "when the CSST is drained to the "Lo-Lo" setpoint, the
	// spray pumps are manually switched to the recirculation
	// mode of spray operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:18-21
	CSSLowLevelSetpoint = 1500.0 // gal (Lo-Lo)

	// CSSMiniflowFraction is the fraction of pump discharge
	// that returns to the CSST via the continuous miniflow path
	// when the spray header is closed. The spec describes a
	// miniflow line with heat exchanger and flow limiting
	// orifice; simplified to 10% recirculation.
	// "A pump miniflow line, containing a heat exchanger and
	// flow limiting orifice"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:20-22
	CSSMiniflowFraction = 0.10

	// CSSSprayEffectiveness is the fraction of incident steam
	// mass flow condensed by a unit of spray flow
	// (dimensionless). The spec does not state a pressure-
	// reduction coefficient; this constant is a simulation
	// parameter that must be calibrated against plant data. A
	// value of 0.5 means each gal/min of spray condenses 0.5
	// gal/min of steam.
	// "comes in contact with the steam in the containment
	// atmosphere, the steam is condensed and falls to the
	// containment floor as drops of a borated sodium hydroxide
	// solution."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:8-11
	CSSSprayEffectiveness = 0.5 // simulation parameter, must be calibrated

	// CSSContainmentPressureSetpoint is the containment pressure
	// (psig) that automatically starts the CSS. The spec
	// describes "containment high pressure" without a numerical
	// setpoint; 50 psig is representative of the pressure rise
	// during a design-basis LOCA before spray actuation.
	// "Initial Injection Mode – Containment spray is initiated
	// automatically in response to a containment high pressure
	// signal."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-57
	CSSContainmentPressureSetpoint Pressure = 50.0 // psig, approximate
)

// CSSTCapacity is the total borated water capacity of the
// simplified lumped CSST (gal). The spec does not state a total
// capacity; 15000 gal is sized to provide 30 min of spray at
// CSSDesignFlow.
// "Suction is provided to the spray pump from the CSST, with the
// SAT providing sodium hydroxide solution."
// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:61-63
var CSSTCapacity Volume = u.NewValue(15000, u.Gallon)

// SATCapacity is the total sodium hydroxide solution capacity of
// the simplified lumped SAT (gal). The spec does not state a
// capacity; 500 gal is sufficient to provide additive for 30 min
// of spray at the chosen flow.
// "the SAT providing sodium hydroxide solution"
// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:62-63
var SATCapacity Volume = u.NewValue(500, u.Gallon)

// RCFCUnit models one of the four parallel fan units in the
// Reactor Containment Fan Cooler System. Each unit contains a
// fan motor, cooling coils, inlet/outlet dampers, and a
// gravity-actuated backdraft damper. The simplified model
// tracks running state and coil heat transfer.
//
// Spec justification:
//   - RCFC purpose: "The Reactor Containment Fan Cooler System
//     (RCFC) is designed to remove heat from the containment
//     building during both normal operation and in the event of
//     a loss-of-coolant accident (LOCA). The RCFC is an
//     engineered safeguard system."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:79-83
//   - Four units, minimum two required: "In a containment, the
//     system includes four fan units that operate in parallel.
//     A minimum of two units must function to satisfy design
//     requirements for both normal operation and post-accident
//     operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:19-24
//   - Normal flow path: "During normal operation, air is drawn
//     from the upper part of the containment, through the return
//     air ductwork, and continues through the normal flow inlet
//     damper into the roughing filter plenum. The air continues
//     its flow through the cooling coils and discharged by the
//     fan into the ventilation system distribution ductwork."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:25-31
//   - Post-LOCA flow path: "During the post LOCA operating
//     mode, the air flow is routed from the return air ductwork,
//     through the emergency flow inlet damper, and into the
//     filtration package plenum. Here the air flows through
//     moisture separators, HEPA filters, and discharges through
//     the emergency flow outlet dampers into the roughing
//     filter plenum and then into the cooling coils."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:47-54
//   - Low-speed switchover: "When a LOCA is sensed, the fan
//     motors are automatically switched to low speed to provide
//     proper flow of the steam-air mixture."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-57
//   - Backdraft damper: "A gravity actuated backdraft damper is
//     installed in the ventilation system discharge ductwork of
//     each fan. These dampers serve to isolate units from the
//     ventilation system when the fan is not in use and to
//     protect each unit from damage due to reverse flow during
//     a LOCA pressure transient."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:71-77
//   - Service water supply: "Cooling water is supplied by the
//     essential service water system."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
type RCFCUnit struct {
	Part

	// Running is true when the fan motor is energized and the
	// unit is moving air.
	Running bool

	// LowSpeed is true when the fan motor has been switched to
	// low speed in response to a LOCA signal. During normal
	// operation LowSpeed is false (full speed).
	// "When a LOCA is sensed, the fan motors are automatically
	// switched to low speed to provide proper flow of the
	// steam-air mixture."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-57
	LowSpeed bool

	// HeatTransferRate is the rate of heat rejection from the
	// containment atmosphere to the service water system via
	// the cooling coils (W).
	HeatTransferRate float64

	// CoilInletTemp is the service water inlet temperature to
	// the RCFC cooling coils (°F). Assumed equal to the
	// essential service water temperature.
	// "Cooling water is supplied by the essential service water
	// system."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
	CoilInletTemp Temperature

	// CoilOutletTemp is the service water outlet temperature
	// (°F) after picking up heat from the containment
	// atmosphere.
	CoilOutletTemp Temperature

	// BackdraftDamperOpen is true when the gravity-actuated
	// backdraft damper is open, allowing normal forward flow.
	// The damper closes automatically on reverse flow to
	// protect the unit.
	// "A gravity actuated backdraft damper is installed in the
	// ventilation system discharge ductwork of each fan."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:71-73
	BackdraftDamperOpen bool
}

// ContainmentBuilding models the reactor containment building —
// the pressure and temperature boundary that contains the RCS,
// safeguards equipment, and containment atmosphere during normal
// and accident conditions. The simplified model tracks lumped
// pressure and temperature, with sinks from CSS spray
// condensation and RCFC heat removal.
//
// Spec justification:
//   - Containment purpose: "This chapter discusses the design
//     of the Reactor Containment Building (hereafter referred
//     as the containment) that will contain and control any
//     release of radioactivity to the environment under
//     normal, upset, or emergency conditions."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:43-51
//   - Design pressure: "Transients resulting from the DBA
//     serve as the basis for a containment design pressure of
//     60 psig (4.46 bar)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
//   - Free volume: "The internal net free volume approximates
//     2,000,000 cubic feet (56,600 m³)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_109.txt:11-12
type ContainmentBuilding struct {
	Part

	// Pressure is the absolute containment pressure (psia).
	Pressure Pressure

	// Temperature is the lumped containment atmosphere
	// temperature (°F).
	Temperature Temperature

	// DesignPressure is the containment design pressure (psig).
	// 60 psig per the spec.
	// "Transients resulting from the DBA serve as the basis
	// for a containment design pressure of 60 psig (4.46
	// bar)."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
	DesignPressure Pressure

	// FreeVolume is the lumped free volume of the containment
	// atmosphere (ft³). Stated in the spec as 2,000,000 ft³
	// for a four-loop prestressed-concrete containment.
	// "The internal net free volume approximates 2,000,000
	// cubic feet (56,600 m³)."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_109.txt:11-12
	FreeVolume float64

	// ContainmentIsolationSystem models the CIS — the
	// engineered safeguard that isolates all containment
	// penetrations via Phase A and Phase B isolation. The
	// simplified model assumes the double-barrier principle
	// holds and tracks a single pair of phased state booleans.
	//
	// Spec justification:
	//   - Purpose: "The Containment Isolation System provides
	//     the means of isolating the various fluid systems
	//     passing through the containment walls as required to
	//     prevent the release of radioactivity to the outside
	//     environment."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:92-96
	//   - Double barrier: "Leakage from the containment through
	//     these pipes or ducts which penetrate the containment
	//     is minimized by use of a double barrier."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:126-128
	ContainmentIsolationSystem ContainmentIsolationSystem

	// HydrogenDetectionSuppression models the containment
	// hydrogen monitoring and suppression system — the
	// defence-in-depth function that prevents explosive
	// hydrogen/oxygen accumulation in the containment
	// atmosphere following a LOCA.
	//
	// Spec justification:
	//   - Chapter 6 introduction: "controlling hydrogen to
	//     avoid an explosive mixture."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:52-55
	//   - Three-function defense (detect / ventilate /
	//     recombine):
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:11-26
	HydrogenDetectionSuppression HydrogenDetectionSuppression
}

// ContainmentIsolationSystem models the Containment Isolation
// System — the engineered safeguard that closes isolation valves
// on all containment penetrations to prevent radioactive release
// during a Design Basis Accident. The simplified model tracks two
// phased states (Phase A and Phase B) with a double-barrier
// assumption per penetration: each penetration has at least two
// independent isolation barriers so that failure of a single
// active component does not result in loss of ability to isolate
// the containment. The second barrier is implicit (blind flange or
// closed piping) and not tracked independently.
//
// Spec justification:
//   - Purpose: "The Containment Isolation System provides the
//     means of isolating the various fluid systems passing through
//     the containment walls as required to prevent the release of
//     radioactivity to the outside environment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:92-96
//   - Double barrier: "Leakage from the containment through these
//     pipes or ducts which penetrate the containment is minimized
//     by use of a double barrier. This double barrier ensures
//     that failure of a single active component along a leakage
//     path will not result in loss of ability to isolate the
//     containment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:126-132
//   - Phase A actuation: "Containment Isolation Phase A:
//     Safety injection, Manual."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
//   - Phase B actuation: "Containment Isolation Phase B:
//     High containment pressure, Manual actuation of containment
//     spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
//   - Isolation barrier qualification: "missile protected, Seismic
//     Class 1 and designed for a pressure equal to or greater than
//     the design pressure of the containment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:136-138
type ContainmentIsolationSystem struct {
	Part

	// PhaseAIsolated is true when the Phase A isolation signal
	// has been received (safety injection or manual). Closes
	// isolation valves on engineered-safety-features
	// penetrations. Latched until the operator resets
	// ManualPhaseA and the safety-injection signal clears.
	PhaseAIsolated bool

	// PhaseBIsolated is true when the Phase B isolation signal
	// has been received (high containment pressure or manual
	// actuation of containment spray). Closes additional
	// non-safeguards penetrations. Latched until the operator
	// resets ManualPhaseB and containment pressure falls below
	// CISPhaseBSetpoint.
	PhaseBIsolated bool

	// ManualPhaseA is set by the operator to manually
	// initiate Phase A isolation. Latched state clears when
	// both ManualPhaseA is reset AND the safety-injection
	// signal clears (Phase A is operator-confirmed; there is
	// no automatic restoration).
	ManualPhaseA bool

	// ManualPhaseB is set by the operator to manually
	// initiate Phase B isolation. Latched state clears when
	// both ManualPhaseB is reset AND containment pressure
	// falls below CISPhaseBSetpoint.
	ManualPhaseB bool

	// MissileProtected and SeismicClass1 record the design
	// qualification criteria for the isolation barriers. They
	// are not per-step dynamics but document the design basis
	// in code.
	// "missile protected, Seismic Class 1 and designed for a
	// pressure equal to or greater than the design pressure of
	// the containment."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:135-138
	MissileProtected bool
	SeismicClass1    bool
}

// RCFC design constants.
//
// Spec justification:
//   - Four units, two required: "In a containment, the system
//     includes four fan units that operate in parallel. A
//     minimum of two units must function to satisfy design
//     requirements for both normal operation and post-accident
//     operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:19-24
//   - Low-speed switchover: "When a LOCA is sensed, the fan
//     motors are automatically switched to low speed."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-56
//   - Service water supply: "Cooling water is supplied by the
//     essential service water system."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
const (
	// RCFCUnitCount is the total number of RCFC fan units in
	// the containment.
	// "the system includes four fan units that operate in
	// parallel"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:19-20
	RCFCUnitCount = 4

	// RCFCUnitsRequired is the minimum number of units that
	// must be running to satisfy design requirements for both
	// normal operation and post-accident operation.
	// "A minimum of two units must function to satisfy design
	// requirements"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:20-22
	RCFCUnitsRequired = 2

	// RCFCNormalSpeed is the normal full-speed air flow rate
	// per unit (cfm). The spec does not state a numerical
	// value; 15,000 cfm is representative of a large
	// containment fan cooler serving a four-loop plant.
	RCFCNormalSpeed float64 = 15000.0 // cfm per unit

	// RCFCLowSpeed is the post-LOCA low-speed air flow rate
	// per unit (cfm). The spec states fans switch to low speed
	// but does not give a ratio; 50% of normal speed is a
	// typical reduction.
	// "When a LOCA is sensed, the fan motors are automatically
	// switched to low speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-56
	RCFCLowSpeed float64 = 7500.0 // cfm per unit

	// RCFCServiceWaterFlow is the service water flow rate per
	// RCFC unit (gpm). The spec does not state a value; 200 gpm
	// is representative of a large shell-and-tube coil.
	// "Cooling water is supplied by the essential service water
	// system."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
	RCFCServiceWaterFlow float64 = 200.0 // gpm per unit

	// RCFCDesignHeatRejection is the design heat rejection per
	// unit (W) at the design temperature difference of 50°F
	// between containment air and service water. The spec does
	// not state a value; sized so 4 units at 50°F ΔT give
	// roughly 2 MW total post-LOCA heat removal (covering the
	// 1-2 MW of CSS-condensed steam plus residual decay heat).
	RCFCDesignHeatRejection float64 = 5e5 // W per unit at design ΔT

	// RCFCDesignDeltaT is the design temperature difference
	// between containment air and the RCFC coil inlet (°F).
	// Used to scale Q_RCFC linearly with the live ΔT.
	RCFCDesignDeltaT float64 = 50.0 // °F

	// RCFCCoilApproachTemp is the temperature approach of the
	// RCFC cooling coil (°F). The spec does not state a value;
	// 15°F is typical for a finned-tube coil with air-side heat
	// transfer.
	RCFCCoilApproachTemp float64 = 15.0

	// RCFCSWInletTemp is the assumed service water inlet
	// temperature to the RCFC coil (°F). The spec describes
	// service water supply without a numerical value; 60°F is
	// typical for a deep-lake / cooling-tower service water
	// supply at design conditions.
	RCFCSWInletTemp float64 = 60.0
)

// ContainmentDesignPressure is the spec-stated containment
// design pressure (psig, 60 psig) and the related
// ContainmentFreeVolume (ft³, 2,000,000 ft³) used to
// initialise the ContainmentBuilding.
//
// Spec justification:
//   - "Transients resulting from the DBA serve as the basis for
//     a containment design pressure of 60 psig (4.46 bar)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
//   - "The internal net free volume approximates 2,000,000
//     cubic feet (56,600 m³)."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_109.txt:11-12
const (
	ContainmentDesignPressure Pressure = 60.0 // psig
	ContainmentFreeVolume     float64  = 2_000_000.0
)

// Containment Isolation System design constants (plans/p23_1.txt).
//
// Spec justification:
//   - Phase A actuation: "Containment Isolation Phase A:
//     Safety injection, Manual."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
//   - Phase B actuation: "Containment Isolation Phase B:
//     High containment pressure, Manual actuation of containment
//     spray."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
//   - Isolation barrier design pressure: "missile protected,
//     Seismic Class 1 and designed for a pressure equal to or
//     greater than the design pressure of the containment."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:135-138
const (
	// CISPhaseBSetpoint is the containment pressure (psig)
	// that triggers automatic Phase B isolation. The spec does
	// not state a numerical value; chosen as 50 psig,
	// representative of the pressure rise during a design-basis
	// LOCA before spray actuation but below the 60 psig
	// containment design pressure documented in p20.
	// "High containment pressure" is the automatic trigger for
	// Phase B.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:65
	CISPhaseBSetpoint Pressure = 50.0 // psig, approximate
)

// ServiceWaterSystem models the Service Water System — the
// ultimate heat sink for the plant. Cooling water is drawn from
// a river, lake, ocean, or cooling tower and supplied to all
// essential equipment coolers. The simplified model collapses
// the six physical pumps into a single lumped flow source with
// an assumed constant inlet temperature.
//
// Spec justification:
//   - Purpose and redundancy: "The Service Water System supplies
//     all the cooling water for the plant, including that
//     required for emergency shutdown. Consequently, high
//     reliability and redundancy is required."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
//   - Pump sizing: "The three essential service water pumps for
//     each unit are typically rated at approximately 20,000 gpm
//     (4500 m3/hr)."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:44-48
//   - Strainers: "The discharge of the service water pumps
//     passes through two strainers typically with 1/8-inch (0.3
//     cm) openings to a common header."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:51-54
//   - Essential loads: "Component cooling heat exchangers",
//     "Containment for ventilation coolers", "Diesel generator
//     coolers" are typical SWS loads.
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:79-93
//   - Nonessential isolation: "The nonessential equipment
//     coolers are fed from separate branch headers that can be
//     isolated from the main headers under accident conditions
//     to conserve water."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:23-26
type ServiceWaterSystem struct {
	Part

	// PumpRunning is true when at least one essential service
	// water pump is running. The simplified model collapses the
	// three physical pumps into a single boolean; the design
	// flow of ServiceWaterPumpFlow × 3 is reached when this
	// flag is set.
	PumpRunning bool

	// Flow is the total essential service water flow rate
	// (gpm). Calculated as ServiceWaterPumpFlow × number of
	// running pumps (up to ServiceWaterEssentialPumpCount),
	// capped at the design maximum.
	Flow float64

	// InletTemp is the service water inlet temperature (°F) as
	// it enters the plant intake. Assumed constant; site-
	// specific in a full model.
	InletTemp Temperature

	// OutletTemp is the mixed service water outlet temperature
	// (°F) after picking up heat from all essential equipment
	// coolers.
	OutletTemp Temperature

	// NonessentialIsolated is true when the nonessential headers
	// have been isolated from the main header to conserve water
	// during accident conditions.
	// "The nonessential equipment coolers are fed from separate
	// branch headers that can be isolated from the main headers
	// under accident conditions to conserve water."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:23-26
	NonessentialIsolated bool
}

// SpentFuelPoolCoolingSystem models the Spent Fuel Pool Cooling
// System — the subsystem that removes residual decay heat from
// irradiated fuel stored in the spent fuel pool and maintains
// water clarity and purity. The simplified model tracks pump
// state, header pressure, and pool temperature.
//
// Spec justification:
//   - Purpose: "The Spent Fuel Pool Cooling System is designed
//     to remove residual decay heat generated by irradiated
//     fuel stored in the spent fuel pool. The system also
//     maintains the clarity and purity of the spent fuel
//     cooling water and the refueling water."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:124-129
//   - Redundancy: "Normal operation requires two pumps with the
//     third pump serving as a standby."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
//   - Pressure control: "The system pressure is maintained at
//     55 to 75 psig (4.8 to 6.2 bar) in the main supply header
//     and the third pump automatically starts at a header
//     pressure of 50 psig (4.5 bar)."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-48
//   - CCWS cooling: "Component cooling water cools the heat
//     exchanger."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:55-57
//   - Purification: "passing approximately 100 gpm (23 m3/hr) of
//     each loop's flow through a filter and demineralizer."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
type SpentFuelPoolCoolingSystem struct {
	Part

	// PumpRunning is true when at least one SFPCS pump is
	// running. Normal state is two pumps running; third is
	// standby.
	PumpRunning bool

	// StandbyRunning is true when the third standby pump has
	// automatically started due to low header pressure.
	// "the third pump automatically starts at a header pressure
	// of 50 psig."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:47-48
	StandbyRunning bool

	// HeaderPressure is the SFPCS main supply header pressure
	// (psig). Normal range 55-75 psig; standby starts at 50
	// psig.
	// "The system pressure is maintained at 55 to 75 psig (4.8
	// to 6.2 bar) in the main supply header"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-47
	HeaderPressure float64

	// PoolTemp is the spent fuel pool water temperature (°F).
	PoolTemp Temperature

	// PurificationFlow is the flow rate through the filter and
	// demineralizer (gpm). Approximately 100 gpm per loop.
	// "passing approximately 100 gpm (23 m3/hr) of each loop's
	// flow through a filter and demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	PurificationFlow float64
}

// Service Water System design constants.
//
// Spec justification:
//   - Three essential pumps per unit, ~20,000 gpm each:
//     "A typical system design will provide six pumps, three
//     each for essential and nonessential service. The three
//     essential service water pumps for each unit are typically
//     rated at approximately 20,000 gpm (4500 m3/hr)."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:44-48
//   - Strainers: "two strainers typically with 1/8-inch (0.3
//     cm) openings to a common header."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:52-54
const (
	// ServiceWaterEssentialPumpCount is the number of essential
	// service water pumps per unit.
	// "three each for essential and nonessential service"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:45
	ServiceWaterEssentialPumpCount = 3

	// ServiceWaterPumpFlow is the design flow rate per essential
	// service water pump (gpm). "Approximately 20,000 gpm" per
	// the spec.
	// "rated at approximately 20,000 gpm (4500 m3/hr)"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:47-48
	ServiceWaterPumpFlow = 20000.0 // gpm per pump

	// ServiceWaterDesignFlow is the total essential service
	// water flow when all three pumps are running (gpm).
	ServiceWaterDesignFlow = ServiceWaterPumpFlow *
		ServiceWaterEssentialPumpCount // 60,000 gpm

	// ServiceWaterSourceTemp is the assumed constant service
	// water source temperature (°F). The spec does not state a
	// value; 70°F is typical for a river or lake in temperate
	// climates.
	ServiceWaterSourceTemp = 70.0 // °F
)

// Spent Fuel Pool Cooling System design constants.
//
// Spec justification:
//   - Two pumps running, one standby: "Normal operation requires
//     two pumps with the third pump serving as a standby."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
//   - Pressure control: "maintained at 55 to 75 psig (4.8 to 6.2
//     bar) in the main supply header and the third pump
//     automatically starts at a header pressure of 50 psig."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-48
//   - Purification: "approximately 100 gpm (23 m3/hr) of each
//     loop's flow through a filter and demineralizer."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
const (
	// SFPCPressureNormalMin is the minimum normal SFPCS header
	// pressure (psig). "55 to 75 psig" per the spec.
	// "maintained at 55 to 75 psig"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-46
	SFPCPressureNormalMin = 55.0 // psig

	// SFPCPressureNormalMax is the maximum normal SFPCS header
	// pressure (psig).
	SFPCPressureNormalMax = 75.0 // psig

	// SFPCStandbyStartPressure is the header pressure (psig) at
	// which the standby pump automatically starts.
	// "the third pump automatically starts at a header pressure
	// of 50 psig (4.5 bar)"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:47-48
	SFPCStandbyStartPressure = 50.0 // psig

	// SFPCPurificationFlow is the design flow rate through the
	// SFPCS filter and demineralizer (gpm).
	// "passing approximately 100 gpm (23 m3/hr) of each loop's
	// flow through a filter and demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	SFPCPurificationFlow = 100.0 // gpm
)

// SFPCPoolVolume is the spent fuel pool water volume (gal).
// The spec does not state a numeric value; 500,000 gal is sized
// for a four-loop refueling cavity plus storage.
//
// Spec justification:
//   - "The design incorporates redundant active components.
//     System piping is arranged so that failure of any pipeline
//     does not drain the spent fuel pool below the top of the
//     stored fuel elements."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:132-136
var SFPCPoolVolume Volume = u.NewValue(500_000, u.Gallon)

// ServiceWaterInletTemp is the design inlet temperature of the
// service water at the plant intake (°F). Used to initialise the
// lumped service-water inlet and the RCFC coil inlet.
//
// Spec justification:
//   - "Depending on the plant, the source of circulating water
//     may be a river, lake, ocean, or cooling tower."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
var ServiceWaterInletTemp Temperature = u.NewValue(
	ServiceWaterSourceTemp, u.Fahrenheit,
)

// RefuelingOperations models the refueling operation cycle for
// the plant. The simplified model tracks cavity flooding depth,
// spent fuel pit water level and boron concentration, fuel
// assembly counts in core / pit / shipped, and the five-phase
// state machine (preparation → reactor disassembly → fuel
// handling → reactor reassembly → preoperational checks /
// startup). It does not model refueling-machine kinematics,
// stud-tensioner hydraulics, individual tools, or radiation-dose
// tracking; those are deferred to a higher fidelity band.
//
// Spec justification:
//   - Spent fuel handled underwater: "Spent fuel is handled
//     underwater from the time it leaves the reactor vessel
//     until it is placed in a cask for shipment from the site."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:13-16
//   - Water functions: "provides an effective, economical, and
//     transparent radiation shield as well as a reliable medium
//     for decay heat removal. Boric acid is added to the water
//     to ensure subcritical conditions during refueling."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:16-19
//   - Two facility types: "the refueling cavity and fuel
//     transfer canal, which are flooded only during the
//     refueling shutdown, and (2) the fuel storage pit, which is
//     kept full of water and is always accessible to operating
//     personnel."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:23-29
//   - Cavity flood depth: "filled to a depth of approximately 24
//     feet (731 cm) of water when a fuel assembly is being
//     transferred between the reactor and the fuel transfer
//     system."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:43-46
//   - Five-phase cycle: "The refueling operation is divided
//     into five major phases: preparation, reactor disassembly,
//     fuel handling, reactor reassembly, and preoperational
//     checks, tests, and startup."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
//   - Pit capacity: "Fuel assembly storage racks to accommodate
//     a minimum of 1-1/3 times the number of assemblies in a
//     core are located on the pit floor."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_176.txt:32-34
//   - 110% weight safety interlock on the refueling-machine
//     gripper: "excess of 110 percent of a fuel assembly
//     weight automatically stops the winch drive from moving in
//     the up direction."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_178.txt:109-111
//   - First-core assembly count: "Of the 193 fuel assemblies
//     contained in a fourloop reactor first core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-39
type RefuelingOperations struct {
	Part

	// Phase is the current refueling phase: 0 = preparation,
	// 1 = reactor disassembly, 2 = fuel handling, 3 = reactor
	// reassembly, 4 = preoperational checks / startup, 5 =
	// complete.
	Phase int

	// PhaseTimer tracks elapsed time in the current phase
	// (seconds). Reset to zero when Phase advances.
	PhaseTimer float64

	// CavityFloodDepth is the current water depth in the
	// refueling cavity (feet). 0 during normal operation;
	// ramps to ~24 feet during flooding.
	CavityFloodDepth float64

	// CavityFlooding is true when the cavity is currently
	// being flooded (depth rising). False when draining or
	// idle.
	CavityFlooding bool

	// CavityDraining is true when the cavity is currently
	// being drained (depth falling). False when flooding or
	// idle.
	CavityDraining bool

	// CavityFlooded is true when the refueling cavity contains
	// sufficient water for fuel transfer (≥
	// RefuelingCavityMaxDepth).
	CavityFlooded bool

	// PitWaterLevel is the spent fuel pit water level (feet).
	// The simplified model tracks the steady full state; an
	// empty pit would be a leak event outside scope.
	PitWaterLevel float64

	// PitBoronConcentration is the boric acid concentration in
	// the spent fuel pit water (ppm). Matches the refueling
	// cavity concentration to ensure subcriticality.
	PitBoronConcentration float64

	// FuelAssembliesInCore is the number of fuel assemblies
	// currently in the reactor vessel.
	FuelAssembliesInCore int

	// FuelAssembliesInPit is the number of spent fuel
	// assemblies in the spent fuel storage pit.
	FuelAssembliesInPit int

	// FuelAssembliesShipped is the number of spent fuel
	// assemblies removed from the site in casks.
	FuelAssembliesShipped int

	// HeadRemoved is true when the reactor vessel head has
	// been lifted off during reactor disassembly.
	HeadRemoved bool

	// InternalsRemoved is true when the reactor vessel upper
	// internals have been lifted out during reactor
	// disassembly.
	InternalsRemoved bool

	// WeightSafetyTripped is the refueling-machine 110% weight
	// interlock latch. True when the suspended load exceeded
	// RefuelingWeightSafetyFactor × nominal fuel assembly
	// weight; the winch cannot move in the up direction until
	// the operator clears it.
	WeightSafetyTripped bool

	// FuelTransferred is a one-shot latch set true when the
	// mid-phase batch transfer of FuelAssembliesPerRefuelingOutage
	// assemblies has been performed during the fuel-handling
	// phase. Reset only on a full cycle reset.
	FuelTransferred bool
}

// Refueling operations design constants (plans/p24_1.txt).
//
// Spec justification:
//   - Cavity flood depth: "filled to a depth of approximately 24
//     feet (731 cm) of water."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:43-45
//   - Spent fuel pit always full of water: "the fuel storage
//     pit, which is kept full of water and is always accessible
//     to operating personnel."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:25-29
//   - Borated water for subcriticality: "Boric acid is added to
//     the water to ensure subcritical conditions during
//     refueling."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:17-19
//   - 110% weight safety interlock on the refueling machine
//     gripper: "excess of 110 percent of a fuel assembly weight
//     automatically stops the winch drive from moving in the up
//     direction."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_178.txt:109-111
//   - First-core assembly count: "Of the 193 fuel assemblies
//     contained in a fourloop reactor first core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-39
const (
	// RefuelingCavityMaxDepth is the depth to which the
	// refueling cavity is filled during fuel transfer (feet).
	// "filled to a depth of approximately 24 feet (731 cm) of
	// water"
	// specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:43-44
	RefuelingCavityMaxDepth float64 = 24.0

	// RefuelingCavityFloodRate is the rate at which the cavity
	// fills during the flooding phase (feet per minute). The
	// spec does not state a value; 1 ft/min gives a ~24 minute
	// fill, a reasonable estimate for a four-loop plant.
	RefuelingCavityFloodRate float64 = 1.0

	// RefuelingCavityDrainRate is the rate at which the cavity
	// drains during the draining phase (feet per minute). The
	// spec does not state a value; matches the flood rate.
	RefuelingCavityDrainRate float64 = 1.0

	// RefuelingPitBoronConcentration is the boric acid
	// concentration held in the spent fuel pit water (ppm).
	// The spec does not state a numeric value; 1000 ppm is
	// representative of the refueling-cavity concentration
	// typically used in Westinghouse PWRs.
	RefuelingPitBoronConcentration float64 = 1000.0

	// RefuelingWeightSafetyFactor is the load fraction of
	// nominal fuel assembly weight at which the winch
	// auto-stops in the up direction. The spec states "110
	// percent of a fuel assembly weight".
	// specs/the_westinghouse_pressurized_water_react/chapter_13/page_178.txt:109-111
	RefuelingWeightSafetyFactor float64 = 1.10
)

// Refueling phase design constants (plans/p24_1.txt).
//
// Spec justification:
//   - Five phases: "The refueling operation is divided into
//     five major phases: preparation, reactor disassembly, fuel
//     handling, reactor reassembly, and preoperational checks,
//     tests, and startup."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
const (
	// RefuelingPhasePreparation is the phase index for the
	// preparation phase.
	RefuelingPhasePreparation = 0

	// RefuelingPhaseDisassembly is the phase index for the
	// reactor disassembly phase.
	RefuelingPhaseDisassembly = 1

	// RefuelingPhaseFuelHandling is the phase index for the
	// fuel handling phase.
	RefuelingPhaseFuelHandling = 2

	// RefuelingPhaseReassembly is the phase index for the
	// reactor reassembly phase.
	RefuelingPhaseReassembly = 3

	// RefuelingPhaseStartup is the phase index for the
	// preoperational checks / startup phase.
	RefuelingPhaseStartup = 4

	// RefuelingPhaseComplete marks completion of the cycle.
	RefuelingPhaseComplete = 5
)

// Refueling phase durations (hours). The spec does not state
// exact per-phase times; the table below uses typical values for
// a four-loop plant. Phase durations are wall-clock hours; the
// phase timer advances TimeStep seconds per step.
//
// Spec justification:
//   - Five phases listed (timing not specified per-phase).
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
var RefuelingPhaseDurations = [6]float64{
	4 * 3600,  // preparation
	12 * 3600, // reactor disassembly
	18 * 3600, // fuel handling
	14 * 3600, // reactor reassembly
	8 * 3600,  // preoperational checks / startup
	0,         // complete (terminal)
}

// FuelAssembliesPerReactor is the nominal number of fuel
// assemblies in a four-loop Westinghouse first core. The same
// value applies in steady state after each refueling outage (the
// refueling cycle replaces some assemblies but does not change
// the total count).
//
// Spec justification:
//   - "Of the 193 fuel assemblies contained in a fourloop
//     reactor first core."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-39
const FuelAssembliesPerReactor = 193

// FuelAssembliesPerRefuelingOutage is the typical number of
// assemblies replaced during a single refueling outage (≈1/3 of
// the core). The spec describes the standard 1/3-core
// replacement strategy; the simplified model collapses to 64.
//
// Spec justification:
//   - First-core composition (65 low, 64 intermediate, 64 high
//     enrichment): "Of the 193 fuel assemblies contained in a
//     fourloop reactor first core, approximately 65 are of a
//     low enrichment [...], 64 are an intermediate enrichment
//     [...], and 64 are a high enrichment [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-41
const FuelAssembliesPerRefuelingOutage = 64

// HydrogenDetectionSuppression models the Hydrogen Detection and
// Suppression System for containment atmosphere safety. The
// simplified model tracks hydrogen volume percent, recombiner
// removal rate, ventilation removal rate, and a detection alarm
// flag. It does not model catalytic kinetics, spatial gradients,
// or the Zr-water reaction chemistry.
//
// Spec justification:
//   - Chapter 6 introduction: containment systems protect
//     integrity by "controlling hydrogen to avoid an explosive
//     mixture."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:52-55
//   - Hydrogen source: "Under accident conditions, coolant
//     radioactivity and the Zirconium water reaction can result
//     in the release of hydrogen gas to the containment
//     atmosphere. Eventually the gas could accumulate to an
//     explosive hydrogen/oxygen concentration."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-38
//   - Three-function defense (detect): "A hydrogen detection
//     system that can detect and report in a continuous manner
//     the hydrogen content in the containment atmosphere.
//     Alarms at proper setpoints are provided."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:11-15
//   - Three-function defense (ventilate): "A containment
//     ventilation system that circulates the containment
//     atmosphere to remove pockets of high hydrogen
//     concentration after an accident."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:19-21
//   - Three-function defense (recombine): "Hydrogen recombiners
//     that burn the free hydrogen. The hydrogen/oxygen
//     combination results in water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:25-26
type HydrogenDetectionSuppression struct {
	Part

	// HydrogenVolumePercent is the current hydrogen volume
	// concentration in the containment atmosphere (%). 0
	// under normal operation; rises during LOCA.
	HydrogenVolumePercent float64

	// HydrogenAlarm is true when the hydrogen concentration
	// exceeds HydrogenAlarmSetpoint.
	HydrogenAlarm bool

	// RecombinerActive is true when the hydrogen recombiners
	// are operating.
	RecombinerActive bool

	// VentilationActive is true when the containment
	// ventilation system is circulating to remove hydrogen.
	VentilationActive bool

	// GenerationActive is true when the Zirconium-water
	// reaction is producing hydrogen (the early-LOCA phase).
	GenerationActive bool
}

// Hydrogen detection and suppression design constants
// (plans/p25_1.txt).
//
// Spec justification:
//   - Hydrogen alarm requirement: "Alarms at proper setpoints
//     are provided."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:14-15
//   - Recombiner function: "Hydrogen recombiners that burn the
//     free hydrogen."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:25-26
//   - Ventilation function: "A containment ventilation system
//     that circulates the containment atmosphere to remove
//     pockets of high hydrogen concentration after an accident."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:19-21
//   - Hydrogen source: "Under accident conditions [...] the
//     Zirconium water reaction can result in the release of
//     hydrogen gas."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-33
const (
	// HydrogenAlarmSetpoint is the volume percent that
	// triggers the high alarm. The spec does not state a
	// numeric threshold; 2% is a conservative design value
	// well below the lower flammability limit.
	HydrogenAlarmSetpoint float64 = 2.0

	// HydrogenExplosiveLimit is the volume percent above
	// which the mixture is considered explosive. The spec
	// does not state a numeric threshold; 4% is the standard
	// engineering value for the lower flammability limit of
	// hydrogen in air.
	HydrogenExplosiveLimit float64 = 4.0

	// HydrogenNumRecombiners is the number of hydrogen
	// recombiners installed in the containment. The spec does
	// not state a count; four is assumed, matching the
	// "minimum of two units must function" pattern used for
	// the RCFC system (plans/p20_1.txt).
	HydrogenNumRecombiners int = 4

	// HydrogenRecombinerRemovalRate is the volumetric removal
	// rate per active recombiner (% per hour). The spec does
	// not state a rate; 0.5 %/hour per recombiner is
	// representative of a large catalytic recombiner.
	HydrogenRecombinerRemovalRate float64 = 0.5

	// HydrogenVentilationRemovalRate is the volumetric
	// removal rate of the containment ventilation system
	// (% per hour). The spec does not state a rate; 0.2
	// %/hour is representative of containment air
	// circulation through a filter train.
	HydrogenVentilationRemovalRate float64 = 0.2

	// HydrogenGenerationRate is the volumetric production
	// rate of hydrogen in the containment atmosphere during
	// early LOCA from the Zirconium water reaction (% per
	// hour). The spec does not state a rate; 1.0 %/hour is
	// representative of the early-phase generation spike.
	HydrogenGenerationRate float64 = 1.0

	// HydrogenRecombinerHysteresis is the volume percent
	// above the alarm setpoint at which the recombiners
	// activate; below (setpoint + hysteresis) they stay
	// active to prevent cycling near the setpoint.
	HydrogenRecombinerHysteresis float64 = 0.5
)

// ReactorProtectionSystem models the Reactor Protection System
// (RPS) and Engineered Safety Features Actuation System (ESFAS)
// for the plant. The simplified model implements trip logic
// with 2/4 voting and ESFAS output signals. It does not model
// detailed nuclear instrumentation channels, in-core
// instrumentation, automatic testing facilities, preventive
// turbine runbacks, or digital rod position indication.
//
// Spec justification:
//   - RPS functions: "Monitors the plant for abnormal conditions
//     and alerts the operator to take appropriate action;
//     Automatically provides reactor trip (shutdown) signals and
//     engineered safeguards actuation signals when plant
//     conditions, as monitored by nuclear instrumentation and
//     process instrumentation, reach the plant safety limits."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:55-72
//   - Trip purpose: "Whenever a direct process or calculated
//     variable exceeds a setpoint, the reactor will be shut
//     down: to prevent fuel rod damage for an anticipated
//     transient (Condition II); to limit core damage for
//     infrequent faults (Condition III); and to protect the
//     reactor coolant pressure boundary for limiting fault
//     conditions (Condition IV)."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:1-7
//   - Manual trip: "The RPS provides for manual initiation of
//     reactor trip by operator action in the control room."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
//   - Turbine trip interlock: "The RPS initiates a turbine trip
//     signal whenever reactor trip occurs."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:10-11
//   - 2/4 voting logic: "The protection system normally uses 2/4
//     logic but reverts to a safe 2/3 logic when part of the
//     system is being tested or is involved in maintenance. A 2/4
//     logic means that a trip occurs when at least 2 signals out
//     of 4 indicate a trip condition."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:116-121
//   - ESFAS description: "The protection system detects
//     Condition II, III and IV faults and actuates the
//     Engineered Safety Features. Table 7-1 lists the safety
//     systems that may be actuated together with the events that
//     trigger the actuation."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:127-131
//   - Trip breaker 2/4 configuration: "The trip breakers are
//     arranged to provide a 2/4 configuration."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
//   - ESFAS Table 7-1 (Safety Injection, Steamline Isolation,
//     Turbine Trip, Start Auxiliary Feed Pumps, Block Steam Dump,
//     Containment Vent Isolation, Open Accumulator Isolation
//     Valves, Containment Spray, Containment Isolation Phase B,
//     Feedwater Isolation, Containment Isolation Phase A).
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81
type ReactorProtectionSystem struct {
	Part

	// ReactorTripped is true when the RPS has initiated a
	// reactor trip (scram). Latches true until manually
	// reset.
	ReactorTripped bool

	// ManualTrip is true when the operator has pressed a
	// manual trip pushbutton.
	// "The RPS provides for manual initiation of reactor trip
	// by operator action in the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
	ManualTrip bool

	// TripSignalCount tracks how many of the redundant trip
	// channels are demanding a trip. The simplified model
	// collapses the four channels into a single integer and
	// applies 2/4 voting: a trip is declared when the count
	// reaches RPSTripVotingThreshold.
	// "A 2/4 logic means that a trip occurs when at least 2
	// signals out of 4 indicate a trip condition."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
	TripSignalCount int

	// ESFAS outputs. Each is set true when the RPS commands
	// the corresponding safeguard to actuate. The wiring in
	// Plant.Step propagates these flags to the destination
	// systems (DieselGenerator.StartSignal for SI, etc.).
	// Table 7-1 lists the engineered safety features and
	// their triggering events.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81

	// SafetyInjectionActuated: SI on low pressurizer
	// pressure, high containment pressure, low steamline
	// pressure, low Tcold, or manual safety injection.
	SafetyInjectionActuated bool

	// SteamlineIsolationActuated: SLI on the same five
	// triggers plus high steam-pressure rate of change.
	SteamlineIsolationActuated bool

	// ContainmentSprayActuated: CSS on high containment
	// pressure or manual.
	ContainmentSprayActuated bool

	// ContainmentIsolationPhaseAActuated: CIS Phase A on
	// safety injection or manual.
	ContainmentIsolationPhaseAActuated bool

	// ContainmentIsolationPhaseBActuated: CIS Phase B on
	// high containment pressure or manual containment spray.
	ContainmentIsolationPhaseBActuated bool

	// AuxiliaryFeedPumpsStartActuated: AFWP start on safety
	// injection or low steam generator level.
	AuxiliaryFeedPumpsStartActuated bool

	// FeedwaterIsolationActuated: FWI on high steam
	// generator level, safety injection, or coincidence of
	// reactor trip and low Tcold.
	FeedwaterIsolationActuated bool

	// TurbineTripActuated: turbine trip on reactor trip,
	// high steam generator level, or safety injection.
	TurbineTripActuated bool
}

// Reactor Protection System design constants (plans/p26_1.txt).
//
// Spec justification:
//   - Trip table (Table 7-1) lists the engineered safety
//     features and their triggering events.
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:6-81
//   - 2/4 voting: "trip occurs when at least 2 signals out of 4
//     indicate a trip condition."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
//   - Trip breaker 2/4 configuration: "The trip breakers are
//     arranged to provide a 2/4 configuration."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
const (
	// RPSLowPressurizerPressureTrip is the pressurizer
	// pressure (psig) below which a trip signal is generated.
	RPSLowPressurizerPressureTrip Pressure = 1800.0

	// RPSHighPressurizerLevelTrip is the pressurizer level
	// (%) above which a trip signal is generated.
	RPSHighPressurizerLevelTrip float64 = 80.0

	// RPSLowSteamGeneratorLevelTrip is the steam generator
	// level (%) below which a trip signal is generated.
	RPSLowSteamGeneratorLevelTrip float64 = 15.0

	// RPSHighSteamGeneratorLevelTrip is the steam generator
	// level (%) above which a trip signal is generated.
	RPSHighSteamGeneratorLevelTrip float64 = 90.0

	// RPSLowSteamlinePressureTrip is the steamline pressure
	// (psig) below which a trip signal is generated.
	RPSLowSteamlinePressureTrip Pressure = 500.0

	// RPSLowTcoldTrip is the reactor coolant cold-leg
	// temperature (°F) below which a trip signal is generated.
	RPSLowTcoldTrip float64 = 530.0

	// RPSHighContainmentPressureTrip is the containment
	// pressure (psig) above which a trip signal is generated.
	// Equal to the containment design pressure so a
	// containment overpressure coincides with a trip.
	RPSHighContainmentPressureTrip Pressure = ContainmentDesignPressure

	// RPSTripVotingThreshold is the number of out-of-4 trip
	// signals required to declare a reactor trip (2/4 logic).
	RPSTripVotingThreshold = 2

	// RPSNumTripChannels is the total number of redundant
	// trip signal channels (4).
	// "trip occurs when at least 2 signals out of 4 indicate a
	// trip condition"
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:120-121
	RPSNumTripChannels = 4
)


