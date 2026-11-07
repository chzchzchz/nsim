package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

type Part struct {
	Name   string
	Damage float32
	// Temperature is a named field rather than embedded so its
	// JSON methods do not promote into Part-embedding structs,
	// which would serialize them as bare UnitVals.
	Temperature Temperature
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
	// Velocity is the instantaneous shaft speed (RPM). Defaults
	// to zero at construction and is updated per-step by
	// exponential startup / coastdown dynamics in the owning
	// building's Step function (plans/p33_1.txt).
	Velocity Velocity

	// DriveReady is latched true by Plant.Step when the upstream
	// power source that feeds the pump motor is energised. When
	// false, the pump produces zero flow even if Running is set,
	// matching the SWS / CCWS / CSS / HHSI readiness behaviour.
	// See plans/x/uat_sat_gating.md for the wiring rationale.
	DriveReady bool
}

// IsRunning reports whether the pump is effectively running: the
// operator has commanded it on and the upstream drive is ready.
func (p *Pump) IsRunning() bool {
	return p.Running && p.DriveReady
}

// ReactorCoolantPumps holds the reactor coolant pumps, one per
// primary loop, so operators can start them sequentially and secure
// individual machines:
//
//   - "Reactor coolant pumps are started sequentially and pump heat
//     is used to raise the RCS temperature."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37
//   - "a maximum of two out of four reactor coolant pumps (RCPs)
//     may be secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-108
//
// The count follows SteamGeneratorLoopCount: one SG and one RCP per
// loop ("Number and type: 1 vertical, U-tube steam generator with
// integral steam-drum per loop",
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:31-32).
type ReactorCoolantPumps struct {
	LoopA Pump
	LoopB Pump
	LoopC Pump
	LoopD Pump
}

// All returns the per-loop pumps in loop order for iteration by the
// step wiring and fixtures.
func (r *ReactorCoolantPumps) All() []*Pump {
	return []*Pump{&r.LoopA, &r.LoopB, &r.LoopC, &r.LoopD}
}

// ByName resolves a per-loop pump from its bridge path segment
// ("A" .. "D"), returning nil for unknown names.
func (r *ReactorCoolantPumps) ByName(name string) *Pump {
	for i, n := range []string{"A", "B", "C", "D"} {
		if n == name {
			return r.All()[i]
		}
	}
	return nil
}

// SetAllRunning latches every pump's operator start command, the
// master-switch stand-in behind the legacy aggregate control.
func (r *ReactorCoolantPumps) SetAllRunning(running bool) {
	for _, pu := range r.All() {
		pu.Running = running
	}
}

// Velocity is the loop-average shaft speed the lumped hydraulic
// consumers see: with N of four rotors spinning, the primary flow
// fraction scales with their mean.
func (r *ReactorCoolantPumps) Velocity() Velocity {
	sum := 0.0
	for _, pu := range r.All() {
		sum += float64(pu.Velocity)
	}
	return Velocity(sum / float64(len(r.All())))
}

// AnyRunning reports whether any pump carries an operator start
// command.
func (r *ReactorCoolantPumps) AnyRunning() bool {
	return r.RunningCount() > 0
}

// RunningCount counts the operator start latches across the four
// loops; the hot-standby lineup cap is expressed against this count.
func (r *ReactorCoolantPumps) RunningCount() int {
	n := 0
	for _, pu := range r.All() {
		if pu.Running {
			n++
		}
	}
	return n
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

	// CumulativeEnergy is the time integral of turbine mechanical
	// RealPower, accumulated by Plant.Step. The spec describes
	// turbine output power but does not specify a cumulative-
	// energy counter; this is sim instrumentation.
	CumulativeEnergy EnergyMeter

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

	// LowTavgDeviationHold is set by the turbine step while the
	// measured RCS average temperature sits more than
	// TurbineLowTavgDeviationF below the load-referenced program:
	// the DEHM then holds the load reference instead of raising it,
	// which is how loading stops automatically when a rods-only
	// power increase outruns xenon burnout. The hold releases itself
	// when the deviation recovers; lowering the load reference stays
	// permitted.
	//   "Tavg will continue to drop until turbine loading is stopped
	//    either by manual operation or automatically upon reaching
	//    the low Tavg deviation setpoint signal."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:62-66
	LowTavgDeviationHold bool

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

	// ResetRequest carries the operator's momentary trip-reset
	// command to the next Turbine.Step, which consumes it and
	// returns the machine to service if the rotor has decayed to
	// rest. The spec's trip devices latch by dumping auto stop oil
	// to drain; returning the unit to service re-establishes valve
	// control from the operator's panel:
	//   "The operator may position the valves, when necessary,
	//    through the manual valve controllers."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:114-116
	ResetRequest bool

	// TripByVacuum latches on low condenser vacuum while running;
	// trip device 3 of the protective list.
	// "Low vacuum trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:26
	TripByVacuum bool

	// TripByBearingOil latches when bearing-oil header pressure
	// falls below TurbineLowBearingOilTripPsi while running; trip
	// device 2 of the protective list.
	// "Low bearing oil pressure trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
	TripByBearingOil bool

	// TripByThrustBearing latches when the thrust-bearing failure
	// input is present while running; trip device 4 of the
	// protective list. The spec names the device but prints no
	// setpoint, so the cause arrives as the ThrustBearingWorn
	// failure latch rather than a sensed threshold.
	// "Thrust bearing trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:34
	TripByThrustBearing bool

	// ThrustBearingWorn models excessive wear of the axial-location
	// thrust bearing ("located axially by the thrust bearing",
	// page_200.txt:42-45). It is operator/malfunction-written state;
	// clearing it models repair. It deliberately survives trip reset:
	// returning the machine to service does not repair hardware.
	ThrustBearingWorn bool

	// GoverningValve is the DEHM-positioned governing-valve bank
	// (0..1). Admission = StopValve.Open * GoverningValve.Open.
	// Spec justification:
	//   - "The control signal is transmitted to the electro-hydraulic
	//     actuator on the main steam governing valves and reheat steam
	//     intercept valves."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:41-44
	GoverningValve Valve

	// SpeedDemand is the pre-breaker operator speed reference as a
	// fraction of TurbineRatedSpeed (0..1.05).
	// "(1) Wide range speed control of the turbine from turning gear
	// to rated speed. This control is accomplished by increasing the
	// speed demand (reference) to bring shaft speed to its rated value
	// at a controlled selectable rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-76
	SpeedDemand float64

	// TurningGear engaged holds the rotor at a slow roll with no
	// steam admission.
	TurningGear bool

	// FirstStagePressure proxies the HP first-stage pressure the DEHM
	// controller compares against its reference:
	// "computes signals comparing turbine speed and first-stage
	// pressure with reference values established by the dispatcher or
	// initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	FirstStagePressure Pressure

	// BearingOilPressure (psig) on the turbine-generator lubricating
	// header. The emergency trip system "uses oil from the
	// turbine-generator lubricating system"; loss of that oil trips
	// the turbine.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:24-34
	BearingOilPressure Pressure

	// LubeOilPumpRunning is the motor-driven auxiliary lubricating
	// oil pump, used below LubeOilShaftDriveFraction of rated shaft
	// speed and before startup ("checking the bearing oil pump before
	// startup").
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_203.txt:87-89
	LubeOilPumpRunning bool

	// ExtractionNRVClosed latches when any turbine trip dumps
	// hydraulic air-pilot pressure to the extraction steam
	// nonreturn valves; while closed no extraction steam flows to
	// the heaters or MSR.
	// "The extraction steam nonreturn valves are actuated from the
	// turbine hydraulic fluid system via an air pilot valve which
	// dumps the air pressure to each extraction steam air-operated
	// nonreturn valve."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:100-104
	ExtractionNRVClosed bool

	// LoadDemandRef is the dispatcher load reference, latched from
	// Grid.LoadDemand each step by Plant.Step. The DEHM ramps
	// Setpoint toward it at RampRate while running.
	// "reference values established by the dispatcher"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	LoadDemandRef float64

	// steamDemandGalPerStep is the throttled per-step steam mass the
	// turbine draws from the SG headers during the most recent Step.
	// Read back by Plant.Step through SteamDemandGalPerStep().
	steamDemandGalPerStep float64

	// dumpedGalPerStep is the per-step steam volume the dump valves
	// routed to the condenser during the most recent TurbineBuilding
	// StepWithSteam, in gallons (already MSIV-gated by the Step).
	// Read back through DumpedGalPerStep by the SG->Condenser flow
	// edge so the audit matches the Step's own accounting instead of
	// re-deriving it from DumpValve.Open alone.
	dumpedGalPerStep float64

	// DumpHoldTimer counts down how much longer a trip or load-
	// rejection edge holds the condenser dump valves open before the
	// automatic-reclose logic may shut them.
	DumpHoldTimer time.Duration

	// DumpPressureIntegral is the integral trim of the trip-bypass
	// pressure controller. It accumulates header-pressure error only
	// while the trip latch modulates the bypass and is reset
	// whenever another dump path owns the valves.
	DumpPressureIntegral float64

	// DumpBypassKicked marks the current trip episode as having had
	// its one-shot bypass opening: the valves "open automatically on
	// turbine trip", then the pressure controller throttles them
	// from that starting point.
	// specs/pwr_sim/chapter_3/page_041.txt:37-45
	DumpBypassKicked bool

	// LastSetpoint is the Setpoint observed at the end of the most
	// recent Step. The dump controller compares it against the
	// current Setpoint to detect a between-step sudden load
	// reduction.
	LastSetpoint float64

	// Synchronized mirrors Generator.BreakerClosed, latched by
	// Plant.Step before the turbine step. It selects DEHM mode:
	// speed control toward SpeedDemand before synchronization, load
	// control toward Setpoint after.
	Synchronized bool

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

// SteamDemandGalPerStep is the throttled per-step steam volume the
// turbine drew from the main-steam headers during its most recent
// Step, in gallons. Plant.Step routes this draw against the SG and
// into the condenser mass balance.
func (t *Turbine) SteamDemandGalPerStep() float64 {
	return t.steamDemandGalPerStep
}

// DumpedGalPerStep is the per-step steam volume the dump valves
// routed to the condenser during the most recent StepWithSteam,
// in gallons (already MSIV-gated by the Step).
func (t *Turbine) DumpedGalPerStep() float64 {
	return t.dumpedGalPerStep
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

	// TurbineRollMinSteamFraction is the floor on the rotor-speed
	// scaling of pre-synchronization steam draw: a machine at rest
	// still admits enough steam to self-start once the stop valve
	// opens. Assumption; the source describes wide-range speed
	// control without quoting a minimum admission.
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
	TurbineRollMinSteamFraction = 0.10

	// WattsPerMW converts megawatt-scale inputs (grid dispatch
	// demand) into the Watt-based ratings used at the conversion
	// boundary.
	WattsPerMW float64 = 1e6

	// JoulesPerMWh converts the Joule-based cumulative energies
	// into megawatt-hours for the web UI: one megawatt-hour is
	// one megawatt for one hour, that is, 1e6 W * 3600 s.
	JoulesPerMWh float64 = 1e6 * 3600
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

	// TurbineLowTavgDeviationF is the deviation (°F, measured minus
	// programmed) at which the DEHM's low-Tavg-deviation signal stops
	// turbine loading. Derived convention: the source names the signal
	// without a magnitude; sized above the rod controller's tracking
	// noise during dispatched ramps (observed <= 1 °F) so normal follow
	// never latches it.
	TurbineLowTavgDeviationF float64 = -2.0

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

	// Generator excitation and cooling design constants
	// (plans/p30_1.txt).
	//
	// Spec justification:
	//   - "Hydrogen pressure: 60 or 75 psig"
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:20
	//   - "Four hydrogen coolers are mounted horizontally at
	//     the top of the generator."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:46-47
	//   - "Demineralized cooling water is provided for the
	//     stator by a closed recirculating water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	//   - "Redundancy is provided in the water pumps, water
	//     coolers, and the alarm system so that safe, reliable
	//     operation is assured under all conditions."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:55-61

	// GeneratorSCCWDesignFlow is the design flow rate of the
	// stator coil cooling water circuit (gal/s). The spec does
	// not give a numeric value; 5 gal/s is sized for a 1370 MVA
	// hydrogen-cooled generator's stator water circuit.
	// "Demineralized cooling water is provided for the stator
	// by a closed recirculating water system."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	GeneratorSCCWDesignFlow float64 = 5.0

	// GeneratorNumHydrogenCoolers is the number of hydrogen
	// coolers.
	// "Four hydrogen coolers are mounted horizontally at the
	// top of the generator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:46-47
	GeneratorNumHydrogenCoolers = 4

	// GeneratorDesignHydrogenPressurePsig is the design
	// hydrogen pressure inside the generator enclosure.
	// "Hydrogen pressure: 60 or 75 psig"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:20
	GeneratorDesignHydrogenPressurePsig float64 = 72.5

	// GeneratorSCCWPumpMinFlow is the minimum stable coolant
	// flow below which the SCCW protective trip is asserted
	// (gal/s). The spec does not state a setpoint; 1 gal/s
	// (20% of design) is the simplified model's assumed
	// threshold.
	GeneratorSCCWPumpMinFlow float64 = 1.0

	// GeneratorCooldownTimeConstant is the thermal time
	// constant for stator winding temperature to equilibrate
	// after a coolant flow change (s). The spec gives no number;
	// 60 s is sized for a 1370 MVA stator with significant
	// thermal mass.
	GeneratorCooldownTimeConstant float64 = 60.0

	// GeneratorExciterFieldForcingFraction is the per-step
	// fraction of the voltage error that the simplified
	// voltage regulator closes. Sized at 5%/step so the
	// terminal voltage builds up to rated within ~20 steps
	// when the exciter is active.
	GeneratorExciterFieldForcingFraction float64 = 0.05

	// Turbine startup, trip, and steam-dump constants.
	//
	// Spec justification:
	//   - Low vacuum trip: "Low vacuum trip" (device 3)
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:26
	//   - LP exhaust limit basis: "may be operated with a slight
	//     reduction in load at exhaust pressures as high as eight
	//     inches HgA during emergency operations."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_199.txt:94
	//   - Low bearing oil trip: "Low bearing oil pressure trip"
	//     (device 2)
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
	//   - Dump sizing: "systems to accept 70- to 85-percent steam
	//     dump"
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:59-61
	//   - Dump trigger: "These valves open under a sudden reduction
	//     in turbine-generator load (in excess of 10%) or a turbine
	//     trip."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
	//   - Speed control: "Wide range speed control of the turbine
	//     from turning gear to rated speed."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-76

	// TurbineLowVacuumTripPsia is the condenser back-pressure above
	// which a running turbine trips. Derived: the spec quotes an
	// emergency LP exhaust limit of "eight inches HgA" ≈ 3.9 psia
	// (page_199.txt:94), while the lumped condenser model idles at up
	// to ~5.5 psia under full steam load; the trip plate sits between
	// that operating ceiling and atmospheric so full load never
	// spuriously trips but loss-of-heat-sink does.
	TurbineLowVacuumTripPsia Pressure = 8.0

	// TurbineLowBearingOilTripPsi is the lubricating-header pressure
	// below which a running turbine trips. Derived: typical
	// journal-oil trip plate; the spec states no number.
	TurbineLowBearingOilTripPsi = 15.0

	// BearingOilDesignPressure is the healthy header pressure with
	// either the shaft-driven or motor pump carrying the load.
	// Derived; the spec states no number.
	BearingOilDesignPressure = 25.0

	// LubeOilDecayTimeConstant (s) is how fast the bearing-oil
	// header decays with no pump running. Derived.
	LubeOilDecayTimeConstant = 5.0

	// LubeOilShaftDriveFraction is the shaft-speed fraction of rated
	// above which the shaft-driven lube-oil pump carries the header
	// alone. Derived.
	LubeOilShaftDriveFraction = 0.90

	// TurbineTurningGearRpm is the slow-roll speed held by turning
	// gear. Derived; the spec does not state an rpm.
	TurbineTurningGearRpm Velocity = 5.0

	// TurbineSpeedRampRate is the pre-breaker speed-demand ramp,
	// fraction of rated per second. Derived: full range in ~60 s;
	// controller resolution is "2 rpm".
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:76
	TurbineSpeedRampRate = 1.0 / 60.0

	// SyncFreqToleranceHz and SyncVoltageTolFraction bound when the
	// generator breaker may stay closed. Derived: standard
	// synchronizer check tolerances; the spec does not state numbers.
	SyncFreqToleranceHz    = 0.1
	SyncVoltageTolFraction = 0.10

	// SteamDumpDesignFraction is the dump capacity fraction of
	// design steam flow. The spec's no-trip option quotes "70- to
	// 85-percent steam dump"; the low end is chosen.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:59-60
	SteamDumpDesignFraction = 0.70

	// SteamDumpCapacityGalPerStep is the per-step steam volume the
	// fully-open condenser dump valves pass, at design header
	// conditions.
	SteamDumpCapacityGalPerStep = CondenserDesignSteamLoadGalPer * SteamDumpDesignFraction

	// Trip-bypass modulation gains. On a turbine trip the bypass
	// valves open on the initial pressure rise and then modulate to
	// hold boiler pressure at its setpoint, so the demand is a
	// proportional term on the header-pressure error plus an
	// integral trim that absorbs the residual offset. Tuning
	// assumptions: full opening at a 5 percent pressure error,
	// integral catching up over a few seconds.
	//   "turbine bypass valves open automatically on turbine trip,
	//    then modulate to pass steam to the condenser and hold
	//    boiler pressure at the constant setpoint"
	//   specs/pwr_sim/chapter_3/page_041.txt:37-45
	SteamDumpModulationProportionalGain = 20.0
	SteamDumpModulationIntegralGain     = 2.0

	// LoadRejectDetectFraction is the setpoint drop over one step
	// window that qualifies as a sudden load rejection.
	// "(in excess of 10%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:10
	LoadRejectDetectFraction = 0.10

	// SteamDumpPressureModeMargin is the SG-pressure multiple above
	// which the dump enters pressure-control mode (startup/cooldown).
	// Derived; sized so the mode arms once no-outlet boiling lifts
	// the header even ~1 percent above nominal — the
	// point-of-adding-heat condition where the spec has the dump
	// valves opening automatically. Full-load operation sits at
	// 1.000 of shell pressure and never arms it.
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-26
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:164-170
	SteamDumpPressureModeMargin = 1.005

	// GoverningValveTrimGain is the per-step integral trim of the
	// governing valve on first-stage-pressure error. Derived so valve
	// travel completes over ~10 s at full error.
	GoverningValveTrimGain = 0.02

	// TurbinePowerResponseSeconds (s) is the first-order lag of
	// mechanical power behind admitted steam energy. Derived.
	TurbinePowerResponseSeconds = 2.0

	// SteamDumpValveTravelRate is the dump-valve stroke rate,
	// fraction of full travel per second (derived: full travel in
	// 0.5 s). The spec describes automatic operation without a
	// stated stroke time.
	SteamDumpValveTravelRate = 2.0

	// SteamDumpHoldTime is how long a load-rejection edge holds the
	// dump valves open before the automatic-reclose logic takes
	// over. Derived; the spec says dump "is reduced as rapidly as
	// conditions permit" without a number.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:73-74
	SteamDumpHoldTime = 60 * time.Second

	// While the turbine sits tripped, the dump demand modulates on
	// the RCS average temperature excess above the no-load value
	// instead of pinning full open: "Steam dump is controlled by
	// signals from the RCS (essentially reactor coolant average
	// temperature) [...] the total quantity of steam dumped is
	// minimized", restoring Tavg to the no-load value.
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:70-77,95-96
	// The deadband shuts the dump once Tavg is back within it of
	// no load. The span maps the excess onto valve travel: sized so
	// the post-trip stored-heat error (tens of F, since the primary
	// sheds heat at the 50 F/hr program rate) commands roughly the
	// decay-heat-equivalent bypass fraction of the 70-pct dump
	// capacity; a full-open demand corresponds to a ~300 F excess
	// that only a genuine heat-sink loss produces. Derived; the spec
	// quotes no numbers.
	SteamDumpTripDeadbandF = 2.0
	SteamDumpTripSpanF     = 300.0

	// SteamDumpTripHeaderFloorFraction floors the post-trip dump
	// demand on header pressure: while tripped, the valves throttle
	// to keep the steamline between this fraction of nominal shell
	// pressure and the pressure-mode arm point, so the bypass carries
	// exactly the decay-heat duty ("the total quantity of steam
	// dumped is minimized") without letting the header sag to the
	// 500 psia low-steamline ESFAS setpoint. Derived; sized for
	// comfortable margin above that setpoint.
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:73-77
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-14
	SteamDumpTripHeaderFloorFraction = 0.78

	// AtmosphericDumpDesignFraction is the atmospheric relief
	// capacity as a fraction of rated steam flow. Derived at half
	// the condenser-dump fraction; the spec gives no number.
	AtmosphericDumpDesignFraction = 0.35

	// SteamDumpRestorationMaxLoadFrac bounds the untripped deep-band
	// restoration branch to loads whose governor draw cannot cover
	// fresh decay-plus-pump heat. Derived from the measured descent:
	// the pre-fix arc held 925-975 psia through ff 0.14 and sagged
	// only below that (plans/x/gaps.md item 18), so anything above
	// ~0.2 keeps a servicable turbine sink.
	SteamDumpRestorationMaxLoadFrac = 0.20

	// SteamDumpRestorationHeaderFloorFraction floors the untripped
	// restoration branch on the loading-program band rather than the
	// ESFAS-safe post-trip floor: while the turbine is admitted the
	// bypass may only spend header pressure above ~0.985 of nominal,
	// so carrying park duty can never sag a loading or low-demand
	// steamline. Derived; ~906 psia against the 920 psia shell
	// setpoint, keeping the measured >=900 psia loading band clear.
	SteamDumpRestorationHeaderFloorFraction = 0.985
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
	// (control systems, ESF bus trickle charge, HVAC, lighting)
	// in MW. Sized at ≈30 MW, ≈3% of the 1000 MWe TurbineRatedkW.
	// "supply power for its own auxiliaries."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
	HouseLoadFixed float64 = 30.0

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

	// CVCSBoronSetpointTolerancePpm is the arrival band around
	// BoronTargetConcentration inside which the setpoint drive
	// releases chemistry back to volume control. Derived: one
	// BTRS rate step, so the release cannot chatter on a single
	// tick's increment.
	CVCSBoronSetpointTolerancePpm = 1.0

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
	// of 100 ppm of dissolved boric acid, consumed as percent
	// Δk/k per 100 ppm on the unified reactivity scale
	// (−0.05% = −50 pcm). Negative — boron is a neutron
	// absorber. Sized so a 1000 ppm HZP inventory gives
	// ≈ −500 pcm of boron reactivity, consistent with the
	// spec's "critical boron concentration" and rod-bank
	// withdrawal sequences for a four-loop Westinghouse plant;
	// the spec does not state a worth numerically, and real
	// soluble-boron worths run larger (order −1% per 100 ppm).
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
var CVCSDesignChargingFlow Volume = Volume{Val: 30, Unit: u.Gallon}

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
var CVCSDesignLetdownFlow Volume = Volume{Val: 75, Unit: u.Gallon}

// CVCSLetdownDesignPsia is the primary-system pressure at which the
// letdown orifices pass full design flow. Below it the achievable flow
// falls with the square root of the gauge differential because the
// driving head across the orifice station collapses as the RCS is
// depressurized.
// "Very little letdown flow is possible through the normal letdown
// flow path at low primary system pressures due to the low
// differential pressure across the CVCS letdown orifices."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:135-139
const CVCSLetdownDesignPsia = 2250.0

const (
	// PzrLevelNoLoadPct is the programmed pressurizer level at
	// no-load Tavg, percent of pressurizer volume. The hot-startup
	// initial-condition table pairs Tavg 557°F with a pressurizer
	// 25 percent full.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-66
	PzrLevelNoLoadPct float64 = 25.0

	// PzrLevelFullLoadPct is the programmed pressurizer level at
	// full-load Tavg: "approximately 60 percent of the pressurizer
	// volume is occupied by water" during steady-state operation.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:40-43
	PzrLevelFullLoadPct float64 = 60.0

	// RCSNoLoadTavgF is the no-load anchor shared by the NSSS
	// control programs (°F), the hot-standby reactor coolant
	// temperature.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-53
	RCSNoLoadTavgF float64 = 557.0

	// RCSFullLoadTavgF is the full-load anchor of the NSSS control
	// programs: the midpoint of the rated inlet/outlet design
	// temperatures (Table 3.4-1 values carried by
	// CoreReferenceInletF / CoreReferenceOutletF).
	RCSFullLoadTavgF = (CoreReferenceInletF + CoreReferenceOutletF) / 2

	// PzrLevelCtrlGainGalPerSecPerPct converts pressurizer level
	// error (percentage points) into net CVCS flow demand (gal/s).
	// Derived: saturates the design charging flow at a 10-point
	// error; the spec states the balance law but no tuning figure.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:9-12
	PzrLevelCtrlGainGalPerSecPerPct float64 = 3.0

	// CVCSAutoBoronLowInsertionFrac is the rod-bank insertion below
	// which automatic boron trim dilutes: holding the program there
	// would need positive reactivity the withdrawn bank can no
	// longer supply. Derived threshold, sized to the cracked-rods
	// maneuvering window so chemistry relieves over-withdrawal
	// before it happens ("the control rods are nearly fully
	// withdrawn during all phases of power operations",
	// specs/westinghouse_plant_operations/chapter_19/page_012.txt:12-14).
	CVCSAutoBoronLowInsertionFrac float64 = 0.035

	// CVCSAutoBoronHighInsertionFrac is the rod-bank insertion above
	// which automatic boron trim borates: slow poison swings after
	// maneuvers push the bank deeper, and chemistry carries the
	// deficit so the bank stays near its withdrawal limit and full
	// reserve capability survives the cycle
	// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:109-123).
	CVCSAutoBoronHighInsertionFrac float64 = 0.055

	// CVCSAutoBoronPowerErrBandFrac is the demanded-minus-achieved
	// power deadband inside which the error tier of the automatic
	// boron trim stands down and rod-position rules govern.
	// Assumption; the spec gates the BTRS mode on a "total error"
	// being "large enough" without a figure
	// (specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-65).
	CVCSAutoBoronPowerErrBandFrac float64 = 0.01

	// CVCSAutoBoronPowerErrSpanFrac is the standing power error that
	// drives the automatic exchange at full design rate; errors
	// between band and span pace the rate linearly. Derived: the
	// p51 witnesses pin boron worth at -0.0925 pct/100 ppm
	// effective and rod slew authority at 0.0135 pct/s, so at
	// 0.8 ppm/s a 0.05 span still keeps full-drive chemistry
	// (0.00074 pct/s) an order of magnitude inside rod authority
	// while the walk drift (0.05/60 frac/s) stays inside the same
	// law.
	CVCSAutoBoronPowerErrSpanFrac float64 = 0.05

	// CVCSAutoBoronPositionDriveCapFrac is the fraction of the
	// design BTRS rate the rod-position tiers may command at full
	// depth. Below the low threshold or above the high one the drive
	// ramps from zero; unpaced tiers held the design 0.8 ppm/s for the
	// whole descent, which integrated 571->0 ppm of dilution across
	// the walk and dose past the park crossing in the hold
	// (gaps.md item 18, plans/p48_1.txt, plans/p51_1.txt).
	// Derived: the daily-arc ascent (TestRodAutoControlDailyLoadFollowArc)
	// needs position-tier authority at depth because the error tier
	// stands down there (RodControlAuto off), so the cap is raised to
	// the measured-passing maneuver floor (0.3 = 0.24 ppm/s at full
	// depth). At the cap, 600 s of parked dilution at 0.8*cap ppm/s
	// moves cap*480 ppm; with the effective worth (-0.0925 pct/100 ppm)
	// that is cap*0.444 pct = 0.133 pct at this cap, above the 0.05 pct
	// park-swing guide; the guide yields to maneuver authority and the
	// hold-arc residual is recorded skip-marked instead (gaps.md item
	// 18 stays open).
	CVCSAutoBoronPositionDriveCapFrac float64 = 0.3

	// CVCSAutoBoronPositionSpanFrac is the insertion depth past a
	// position threshold at which the position-tier drive reaches
	// its cap (insertion units, not power fraction). In insertion
	// units so the low tier (full travel 0..0.035) reaches the cap
	// before full withdrawal; the power-error span would never
	// saturate it.
	CVCSAutoBoronPositionSpanFrac float64 = 0.02
)

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
var ReactorMakeupDesignFlow Volume = Volume{Val: 60, Unit: u.Gallon}

// Water-tracking constants for drain tanks and dumped-water paths.
//
// Spec justification:
//   - "Steam from the safety and/or relief valves is piped to the
//     pressurizer relief tank which contains sufficient water to
//     condense the steam. Cold water can be sprayed into the
//     pressurizer relief tank to increase the heat sink capacity.
//     A rupture disc vents the tank to the containment if design
//     pressure is exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-38
//   - "ASME Code safety valves and power operated relief and
//     atmospheric dump valves are located on each steam line
//     upstream of the stop valves."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
//   - "The heater drain tanks are horizontal 13,120-gal tanks.
//     [...] A dump valve which taps off the cross-connection drains
//     the tanks to condenser A on a high level in either tank."
//     specs/westinghouse_tech_manual/chapter_7/page_0636.txt:32-38
const (
	// AtmosphericDumpFraction is the fraction of full-load SG steam
	// flow released per second through one fully open atmospheric
	// dump valve. Derived: relief capacity of order half of
	// SteamGeneratorFullLoadSteamFlow.
	AtmosphericDumpFraction = 0.5

	// HeaterDrainTankCapacityGal is quoted verbatim from the tech
	// manual: "horizontal 13,120-gal tanks".
	HeaterDrainTankCapacityGal = 13120.0

	// HDTHighLevelFraction is derived: the auto dump valve opens
	// above 90% capacity ("drains the tanks to condenser A on a
	// high level in either tank").
	HDTHighLevelFraction = 0.9

	// HDTDumpFlow is the per-second volume through the open HDT
	// dump valve to the condenser. Derived: empties the high-level
	// band ((1-HDTHighLevelFraction)*capacity = 1312 gal) in about
	// 100 s.
	HDTDumpFlow = 13.12 // gal/s

	// FeedPumpSuctionFlow is the per-second main-feedwater draw on
	// the heater drain tank (deaerator stand-in) while
	// MainFeedwaterPump.Running. Derived from the existing
	// full-load condenser steam load constant.
	// "The feedwater booster pumps draw suction from the deaerator
	// water storage tank, which is mounted directly below the
	// deaerator, and supply the main feedwater pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:46-49
	FeedPumpSuctionFlow = CondenserDesignSteamLoadGalPer // gal/s

	// BTRSBedLifeSeconds is derived: continuous boration/dilution
	// saturates the BTRS resin bed in about one plant-hour. The
	// spec gives depletion qualitatively ("until the resin beds are
	// fully depleted as indicated by the boron concentration meter",
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:11-13)
	// but no number.
	BTRSBedLifeSeconds = 3600.0

	// BTRSHotBedF and BTRSColdBedF are the bed temperatures the
	// spec attaches to boration and dilution respectively: "sending
	// the flow from the VCT, at relatively high temperatures (140°F)
	// (60°C), to the thermal regeneration demineralizers" versus
	// "at relatively low temperatures (50°F) (10°C)".
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:13-14,24-26
	BTRSHotBedF  float64 = 140.0
	BTRSColdBedF float64 = 50.0

	// BTRSBedReadyBandF is how close the beds must sit to the active
	// mode's temperature before exchange progresses. Invented
	// tolerance; the source states the conditioning requirement but
	// no readiness band (page_081.txt:41-45). The wide band lets a
	// mode flip begin exchanging while the beds are still
	// conditioning across the 90°F hot/cold gap, so an anticipated
	// reversal does not starve the cascade for the full gap close.
	BTRSBedReadyBandF = 10.0

	// BTRSBedConditioningFPerSec is the isolated-recirculation bed
	// reconditioning rate. Invented; the source gives the 50 F -> 140 F
	// example but no duration (page_081.txt:41-45). Sized so the
	// 90°F gap closes in ~10 s: a reversal during a fast park
	// transient must not idle chemistry for half a minute while
	// fission is falling at percent-per-second rates.
	BTRSBedConditioningFPerSec = 3.0

	// SecondsPerHour converts hourly flow figures (e.g.
	// SteamGeneratorFullLoadSteamFlow, expressed in gal/hr) into
	// per-second rates.
	SecondsPerHour = 3600.0
)

type FuelRods struct{ Part }

// ControlRods models the rod cluster control (RCC) assemblies.
// Insertion=0 means fully withdrawn (normal operation);
// Insertion=1 means fully inserted (reactor tripped).
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
//   - "reactor trip — When control rods are rapidly inserted into the
//     core to quickly take the reactor to a subcritical state."
//     specs/the_westinghouse_pressurized_water_react/glossary.md:142
//   - Glossary "rod worth — The reactivity change per unit change in
//     control rod position."
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:46-47
type ControlRods struct {
	Part
	Insertion float64 // 0..1 fraction of rod insertion (actual)

	// DemandInsertion is the operator's requested rod insertion,
	// written by the bridge Control.Set. Step rate-limits
	// Insertion toward DemandInsertion. This split mirrors the
	// plan's intent that the operator's demand be visible
	// while the actual rod position catches up at the
	// mechanical drive speed.
	//
	// "Withdraw rods to criticality and raise output to power
	// range level."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_223.txt:172
	DemandInsertion float64

	// DroppedBank models one rod bank jammed fully into the core: a
	// mechanical failure, not a demand. While latched the actual
	// insertion carries a floor at RodDroppedBankInsertion that no
	// drive motion or controller withdrawal can recover, so the
	// dropped worth outruns the movable compensation - the condition
	// the automatic load runback's initiator list calls the dropped
	// rod signal.
	//   "Automatic load runback initiated by a dropped rod signal"
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
	// Malfunction class (secondary training source): "One bank of dark
	// rods drops [...], imparting large negative reactivity into the
	// core."
	// specs/pwr_sim/chapter_4/page_049.txt:61-67
	DroppedBank bool

	// AutoControl engages the automatic Tavg-program rod
	// controller, which integrates the program error into
	// DemandInsertion each step. The zero value keeps the demand
	// purely operator-written: "The charging pump flow and
	// low-pressure letdown valve are controlled manually during
	// plant startup and cooldown" is the same manual-lineup
	// convention for the maneuvering systems.
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
	AutoControl bool

	// programTargetFraction is the slew-limited power target the
	// automatic controller walks toward the program point, seeded
	// from live power on first engagement so engaging the
	// controller never steps the plant. programPrimed suppresses
	// that zero-value seed on later calls.
	programTargetFraction float64
	programPrimed         bool

	// ProgrammedTavg is the last computed output of the
	// Tavg program, °F. "The rod control system automatically
	// controls RCS average coolant temperature (Tavg) as a
	// programmed function of turbine load."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
	ProgrammedTavg float64

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

	// EnforceInsertionLimit latches the rod insertion limit: while
	// true, Step clamps DemandInsertion to at most InsertionLimit so
	// resting positions stay withdrawn enough for a trip to add the
	// required shutdown margin. The zero value keeps demand
	// operator-owned. A reactor trip bypasses the clamp (the scram
	// forces full insertion regardless of demand).
	//   "Control rods must be pulled above a certain height, the rod
	//    insertion limit, so that in the event a trip occurs the full
	//    insertion of the rods will add at least a prescribed amount
	//    of negative reactivity."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:30-34
	EnforceInsertionLimit bool

	// InsertionLimit is the maximum allowed rod insertion fraction
	// (0..1) while EnforceInsertionLimit is latched.
	InsertionLimit float64
}

// Step advances Insertion toward DemandInsertion at the rate
// limit (RodInsertionSpeed). The bridge Set path writes the
// operator's request into DemandInsertion; this step rate-limits
// the actual transition so the thermal model sees incremental
// reactivity changes rather than an instantaneous scram. Without
// the limit the operator could drop all rods in one tick,
// which defeats the transient response. A reactor trip bypasses
// this rate limit: the trip clamp in ReactorPressureVessel.Step
// forces Insertion=1.0 directly (see plans/x/trip.md).
//
// While EnforceInsertionLimit is latched, the demand (and the
// controller-integrated demand writers feed it) clamps to
// InsertionLimit so resting positions cannot sink below the
// shutdown-margin envelope; the scram path supersedes it.
//
// The previous-step Insertion is read from the receiver (c.Insertion)
// and the rate-limited target written into the returned copy.
// The clamp at [0,1] mirrors the normalised-kind validation in
// the bridge Set path.
func (c ControlRods) Step(dt time.Duration) ControlRods {
	ret := c
	demand := ret.DemandInsertion
	demand = min(max(demand, 0), 1)
	if ret.EnforceInsertionLimit {
		demand = min(demand, ret.InsertionLimit)
		ret.DemandInsertion = demand
	}
	delta := demand - c.Insertion
	maxStep := RodInsertionSpeed * dt.Seconds()
	ret.Insertion = c.Insertion + min(max(delta, -maxStep), maxStep)
	// The dropped bank sits below every reachable position: no drive
	// motion recovers it, whatever the demand does.
	if ret.DroppedBank {
		ret.Insertion = max(ret.Insertion, RodDroppedBankInsertion)
	}
	return ret
}

type Nozzle struct{ Part }

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

	// CumulativeEnergy is the time integral of RealPower since
	// simulation start. It is sim instrumentation, not a
	// modeled revenue meter: the spec describes generator
	// output power but does not specify a cumulative-energy
	// counter.
	// "monitors output power ... to ensure that the limits of
	// the generator are not exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
	CumulativeEnergy EnergyMeter

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
	// OPC trip, operator manual trip, grid fault, or SCCW-loss
	// trip (plans/p30_1.txt).
	BreakerClosed bool

	// GeneratorMVALimitAlarm latches when apparent power exceeds
	// RatedMVA and the DEHM clamps output back onto the rating
	// circle.
	// "It controls valve changes and monitors output power, the power
	// factor, and speed to ensure that the limits of the generator
	// are not exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
	GeneratorMVALimitAlarm bool

	// GeneratorExcitationSystem embeds the Westinghouse Mark III
	// brushless-exciter / WTA-300 voltage regulator model
	// (plans/p30_1.txt). Reachable as gen.ExciterOutput, etc.
	GeneratorExcitationSystem

	// GeneratorCoolingSystem embeds the dual cooling-circuit
	// model (hydrogen internal gas + stator coil cooling water)
	// (plans/p30_1.txt). Reachable as gen.SCCWFlow, etc.
	GeneratorCoolingSystem
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
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:76-77
//     (the sentence starts at the bottom of page 220; page_221.txt
//     truncates it after "Drains from the four")
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
//     alongside the steam-driven and motor-driven units in the AFW system:
//     "one steam-driven and one diesel-engine-driven auxiliary feedwater
//     pump (each of which is safety related)… There is also a
//     nonsafety-related motor-driven auxiliary feedwater pump."
//     specs/westinghouse_tech_manual/chapter_5/page_0527.txt:30-36
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

	// Cooling state — driven by Outside.ServiceWaterSystem.PumpRunning
	// (specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89).
	CoolantTemp Temperature

	// JacketHeatDuty is the heat rate the jacket-water cooler hands
	// to the Service Water System while the engine runs with cooling
	// flow (W). Zero when the engine is off or the cooler runs dry;
	// Plant.Step sums it into the SWS outlet-temperature rise.
	// "Diesel generator coolers" are a service water load.
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89
	JacketHeatDuty Watts

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
//     main feed pump at full load. The per-SG spec steam flow is
//     3,813,000 lb/hr (480 kg/s):
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//
// FeedwaterMinFlow is the minimum stable flow below which the
// main feed pump discharge recirculates to the deaerator.
// "The discharge from the pumps is automatically recirculated back
// to the deaerator whenever the flow to the high-pressure
// feedwater heaters falls below the minimum."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
//
// Sized as 10% of design flow; the spec does not state a specific
// minimum flow value.
var FeedwaterMinFlow Volume = Volume{Val: 950, Unit: u.Gallon}

// FeedwaterDesignFlow is the design volumetric flow through the
// main feed pump at full load. Known approximation: the value is
// anchored on THREE loops (3 × 480 kg/s = 1440 kg/s ≈ 380 gal/s)
// while SteamGeneratorLoopCount is 4, so the strict four-loop anchor
// would be ~507 gal/s; the delivered flow never approaches the cap at
// either scale and the regulator normalizes against it, so it acts as
// the feedwater-loop tuning scale. Recalibrating to the four-loop
// figure rescales regulator/heater/MSR extraction sizing together and
// is recorded out of scope (scenario-8 G4 ledger).
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
var FeedwaterDesignFlow Volume = Volume{Val: 380, Unit: u.Gallon} // gal/s

// Feedwater regulator design constants (plans/p22_1.txt).
//
// Spec justification:
//   - Three-element control: "Feedwater flow to each steam generator
//     is controlled by a feedwater regulator in each feedwater line.
//     The regulator is controlled by steam generator level, steam
//     flow and feedwater flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
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
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
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

	// ManualControl transfers SG level control from the three-
	// element law to operator positioning of the control valve:
	// ValveDemand then holds the operator's write instead of the
	// computed demand, and the integral freezes so retransfer to
	// automatic does not bump the valve. The unload evolution
	// makes this transfer at house-load level.
	// "reactor control and steam generator water level control are
	// transferred to manual operation"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:156-159
	ManualControl bool
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

// DieselCoolantTripTemp — cooling trip threshold; the spec implies
// Service-Water loss must trip the engine
// (specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89,
// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:8-9).
var DieselCoolantTripTemp = Temperature{Val: 220, Unit: u.Fahrenheit}

// DieselLowFuelLevel is the low-fuel warning threshold, sized
// to fire well before the diesel trips on fuel exhaustion.
// The diesel day-tank is 500 gal at MakePlant
// (core/plant.go:1336-1353); 50 gal is 10% — enough headroom
// for an operator response without nuisance warnings during
// normal rated-load burn.
var DieselLowFuelLevel = Volume{Val: 50, Unit: u.Gallon}

type Transformer struct{ Part }

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
	Open float64
	Flow Volume

	// Auto engages the volume-control-tank low-level initiation:
	// a VCT level at or below VCTLowLevel latches Initiated until
	// the tank is restored to VCTDesignLevel, forcing full makeup
	// flow regardless of the manual Open position. The zero value
	// keeps the historical manual-only valve.
	// "Low-level signals from the volume control tank initiate
	// reactor makeup control or flow from the refueling water
	// storage tank as a backup."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
	// "The reactor coolant makeup is automatically controlled and
	// the boron concentration properly adjusted."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:107-109
	Auto bool

	// Initiated latches while the automatic low-level signal has
	// the makeup control system adding water.
	Initiated bool
}

// applyAutoMakeup runs the volume-control-tank low-level initiation
// while Auto is engaged: a level at or below VCTLowLevel latches
// Initiated, and the latch clears once the tank is restored to its
// design band, so a boundary-straddling level cannot chatter the
// valve. With Auto off the latch is forced clear and the valve stays
// manual.
// "Low-level signals from the volume control tank initiate reactor
// makeup control or flow from the refueling water storage tank as a
// backup."
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
func (m *MakeupValve) applyAutoMakeup(vctLevelGal float64) {
	if !m.Auto {
		m.Initiated = false
		return
	}
	if vctLevelGal <= VCTLowLevel {
		m.Initiated = true
	}
	if m.Initiated && vctLevelGal >= VCTDesignLevel {
		m.Initiated = false
	}
}

// effectiveOpen is the valve position the flow derivation sees: an
// initiated automatic signal forces full makeup regardless of the
// manual position.
func (m *MakeupValve) effectiveOpen() float64 {
	if m.Auto && m.Initiated {
		return max(m.Open, 1)
	}
	return m.Open
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

	// LoadDemand is the dispatcher's load reference for this
	// unit, stored in MW against TurbineRatedkW (bridge kind
	// "mw": a dispatcher value entered in MW is read back in
	// MW) and converted once at the Plant.Step boundary into
	// Turbine.LoadDemandRef, the Setpoint-scale fraction (0..1)
	// the electro-hydraulic control ramps Setpoint toward
	// while synchronized. It is a pure input: the grid does not
	// model other generators or area control error.
	//   "...reference values established by the dispatcher or
	//    initiated at the operator's panel."
	//    specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:36-41
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

	// CumulativeEnergy is the time integral of TotalMW since
	// simulation start. The spec describes auxiliary power
	// flows but does not specify a cumulative counter; this is
	// sim instrumentation, not a revenue meter.
	CumulativeEnergy EnergyMeter
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
	// when ChargingPumpRunning is true; zero otherwise.
	// "The charging pumps take the coolant from the volume
	// control tank [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
	// "system pressure is maintained by charging and letdown
	// flow control."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:96-97
	ChargingFlow Volume

	// ChargingPumpRunning is the operator latch on the charging
	// pumps. The CVCS Step sets ChargingFlow from this field.
	// The pumps are centrifugal and feed from the engineered
	// safety features bus during accident conditions (see
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80),
	// so the auto-start logic in Plant.Step drives this latch
	// independently of the reactor coolant pumps.
	ChargingPumpRunning bool

	// ChargingFlowDemand is the manual throttle on charging flow
	// while the pump latch runs in non-auto mode: 0 (zero value)
	// means unthrottled design flow, (0..1] scales it. Shutting
	// charging stays the pump latch's job.
	// "The charging pump flow and low-pressure letdown valve are
	// controlled manually during plant startup and cooldown."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
	ChargingFlowDemand float64

	// AuxSprayValveOpen is the auxiliary spray valve position
	// (0..1). The valve routes throttled charging flow into the
	// pressurizer steam space for pressure control when normal
	// spray is unavailable with the reactor coolant pumps secured;
	// it requires the charging pump running.
	// "Charging flow is throttled to control pressurizer cooldown
	// rate and raise pressurizer level using auxiliary spray since
	// normal spray is not available when reactor coolant pumps have
	// been secured."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104
	AuxSprayValveOpen float64

	// AuxSprayDrive is the effective auxiliary spray drive (0..1)
	// delivered last step: the valve position gated by the charging
	// pump latch. Plant.Step forwards it to the pressurizer.
	AuxSprayDrive float64

	// AutoBoronDrive scales the armed automatic boron exchange rate
	// this step (0..1]: applyAutoBoronControl writes it from the
	// standing demanded-minus-achieved power error, while position
	// and axial-offset selections run at full rate. CVCS.Step forces
	// the scale to 1 whenever AutoBoronControl is disengaged so
	// operator-driven modes keep the design rates.
	AutoBoronDrive float64

	// LetdownValveOpen is the operator latch on the letdown
	// isolation valve. The CVCS Step sets LetdownFlow from this
	// field; when the VCT high-level condition diverts letdown,
	// the flow lands in BRSHoldupLevel instead of the VCT.
	LetdownValveOpen bool

	// AutoBoronControl engages the automatic boron trim: when the
	// rod bank approaches a travel limit, chemistry relieves it
	// (dilution near full withdrawal, boration under deep
	// insertion) through the existing BTRS mode and pump paths.
	// The zero value keeps Mode operator-owned.
	AutoBoronControl bool

	// LetdownFlow is the flow rate from the RCS letdown tap
	// through the letdown chiller and demineralizers back to
	// the VCT, or into the BRS holdup tanks while diverting.
	// "The charging pumps take the coolant from the volume
	// control tank [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
	LetdownFlow Volume

	// AutoLevelControl engages the automatic pressurizer level
	// controller: the Tavg-referenced program error balances
	// ChargingFlow against LetdownFlow. The zero value keeps the
	// manual lineup because startup and cooldown are manual:
	// "The charging pump flow and low-pressure letdown valve are
	// controlled manually during plant startup and cooldown."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
	AutoLevelControl bool

	// ProgrammedLevel is the last computed output of the
	// Tavg-referenced level program, percent of pressurizer
	// volume. "maintain pressurizer level at a programmed value
	// which varies with RCS average coolant temperature."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:36-40
	ProgrammedLevel float64

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

	// BoronTargetConcentration is the operator-written boron
	// setpoint in ppm. While BoronSetpointTracking is latched, Step
	// selects the boration or dilution mode toward it and ramps
	// BoronConcentration at the BTRS flow rates until it lands in
	// the tolerance band, then releases chemistry back to volume
	// control. The ramp replaces an instantaneous write so a dilution
	// evolution moves on the real flow-rate clock:
	// "A boron dilution is started to bring the boron concentration
	// to the estimated critical concentration."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:36-38
	BoronTargetConcentration float64

	// BoronSetpointTracking engages the drive toward
	// BoronTargetConcentration. The zero value keeps Mode
	// operator-owned; while latched it defers to AutoBoronControl.
	BoronSetpointTracking bool

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

	// BRSHoldupLevel is the accumulated diverted letdown routed to
	// the BRS holdup tanks while the VCT is in its high-level
	// condition. Uncapped pending a spec capacity figure.
	// "If the volume control tank is full, a high-water level
	// signal diverts letdown flow to the Boron Recycle"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:19-21
	BRSHoldupLevel Volume

	// Diverting latches while the VCT high-level condition reroutes
	// letdown into BRSHoldupLevel. It is deliberately separate from
	// Mode: the spec keeps the BTRS cycling through boration and
	// dilution while letdown is diverted ("This operation continues
	// until the resin beds are fully depleted [...]", followed by a
	// mode switch back to dilution),
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:11-16
	// so the divert must not cancel the operator's boron mode.
	Diverting bool

	// BTRSBedSaturation is the fraction (0..1) of BTRS
	// demineralizer resin exchange capacity consumed. At 1.0
	// boration/dilution halt until the bed is regenerated.
	// "This operation continues until the resin beds are fully
	// depleted as indicated by the boron concentration meter."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:11-13
	BTRSBedSaturation float64

	// BTRSBedTemperatureF is the lumped demineralizer-bed
	// temperature. Thermal regeneration makes exchange
	// direction-dependent: boration releases stored boron only from
	// hot beds, dilution stores boron only on cold beds.
	// "sending the flow from the VCT, at relatively high temperatures
	//  (140°F) ... at relatively low temperatures (50°F)"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:12-31
	BTRSBedTemperatureF float64

	// BTRSBedsEstablished latches once a first exchange mode has set
	// the bed state; the zero value means the BTRS has never run, so
	// the first active mode snaps the beds to its own temperature.
	BTRSBedsEstablished bool

	// BTRSLastExchangeMode records the most recent mode that actually
	// exchanged boron. With the loop isolated the recirculation pump
	// conditions the idle beds toward this mode's opposite so the next
	// anticipated mode finds them ready.
	// "after completion of a dilution mode, with demineralizers at
	//  50°F, the BTRS recirculation pump is used to increase the
	//  temperature of the demineralizers to 140°F for the subsequent
	//  boration mode"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:41-45
	BTRSLastExchangeMode CVCSMode
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

	// DriveReady is true when this train's ESF bus is
	// energised — TrainA on BusA, TrainB on BusB.
	// "Loads important to plant safety are split and
	// diversified between independent engineered safety
	// features system switchgear groups."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:33-36
	DriveReady bool

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

	// SolidPlantPressureSetpoint is the selected pressure (psia)
	// the solid-plant pressure control valve maintains while the
	// pressurizer is water-solid; zero disables the path until the
	// operator sets it.
	// "Solid plant primary pressure control utilizes a pressure
	// control valve between the RHR and CVC systems which is set
	// to maintain a selected pressure while charging makes up for
	// any coolant contraction and losses."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:117-122
	SolidPlantPressureSetpoint Pressure

	// SolidPlantPressureValve modulates RHR pump discharge into the
	// CVCS letdown header to hold SolidPlantPressureSetpoint on a
	// water-solid pressurizer. It has authority only while an RHR
	// train provides the forced flow between the two systems.
	SolidPlantPressureValve Valve

	// SolidPlantReliefFlow is the per-second bleed delivered to the
	// CVC letdown path last step (gal/s). Zero unless the valve is
	// open, an RHR train is running, the pressurizer is water-solid,
	// and pressure stands above the setpoint.
	SolidPlantReliefFlow Volume
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

	// RHRSSolidPlantBleedGalPerSec is the full-open bleed capacity
	// of the solid-plant pressure control valve path (gal/s). Sized
	// so small mass/heat inputs on a water-solid plant are trimmed
	// at operator timescales against PressurizerBubbleStiffness
	// (~50 psi/s), well inside the rapid-rise regime the valve
	// exists to prevent; the spec states no capacity figure.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:25-32
	RHRSSolidPlantBleedGalPerSec = 20.0
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

	// IsolationShut closes the normally-open accumulator
	// isolation valves so a planned cooldown can pass below the
	// discharge precharge without injecting into the RCS.
	// "Before reaching the pressure when the safety injection
	// accumulators would discharge, the accumulator isolation
	// valves are closed."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:111-113
	IsolationShut bool
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
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:29-31
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
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:29-31
	SISHHSIShutoffHead float64 = 2800.0

	// SISHHSIShutoffHeadPsia converts the shutoff head to a
	// pressure gate with the fresh-water gradient 0.433 psi/ft.
	// The spec quotes the head in feet only; the gradient is the
	// standard cold-water figure (assumption).
	SISHHSIShutoffHeadPsia = SISHHSIShutoffHead * 0.433

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

	// SISHotLegInjectionFraction is the share of the HHSI
	// discharge redirected to the RCS hot legs once the operator
	// realigns to simultaneous cold- and hot-leg injection. The
	// spec asks for "a substantial portion" without a number;
	// half is the recorded model convention.
	// "a substantial portion of the flow is directed to the RCS
	// hot legs."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:131-133
	SISHotLegInjectionFraction = 0.5
)

// SISAccumulatorPerTankCapacity is the water capacity of one
// accumulator tank (gal). Sized to 1,150 gal, typical of a
// four-loop Westinghouse accumulator tank.
var SISAccumulatorPerTankCapacity Volume = Volume{Val: 1150, Unit: u.Gallon}

// SISAccumulatorDischargeRate is the maximum flow rate at which
// an accumulator tank discharges into the RCS cold leg (gal/s).
// The spec does not state a numeric per-tank discharge rate; 230
// gpm per tank is the typical Westinghouse four-loop accumulator
// design.
var SISAccumulatorDischargeRate Volume = Volume{Val: 230.0 / 60.0, Unit: u.Gallon}

// SISHHSIDesignFlow is the design discharge flow rate of each
// HHSI pump (gal/s). 970 gpm is the typical rated flow for the
// Westinghouse four-loop HHSI pump.
var SISHHSIDesignFlow Volume = Volume{Val: 970.0 / 60.0, Unit: u.Gallon}

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

	// HHSIDriveReady is true when at least one ESF bus is
	// energised — the HHSI pump motors are fed from the 6600V
	// ESF buses, one per train. The SIS Step gates the actual
	// injection flow on this flag.
	// "The larger auxiliaries such as the [...] safety
	// injection pumps are fed directly from the 6600-volt
	// [engineered safety features] buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
	HHSIDriveReady bool

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

	// Sump is the containment sump inventory (gal): hot-leg break
	// discharge spills into it, and HHSI delivery draws from it once
	// suction is aligned to the sump recirculation lines. Zero value
	// is coherent — an intact plant has spilled nothing.
	// "both the HHSI and RHR pumps for each subsystem take suction
	// directly from one of the two containment sump recirculation
	// lines."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:172-177
	Sump Volume

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

	// HotLegInjection is the manual control-board realignment to
	// simultaneous cold-leg and hot-leg injection, performed
	// about 24 hours into the recirculation phase for long-term
	// core cooling and boron-buildup prevention.
	// "Approximately 24 hours after the switchover to cold-leg
	// recirculation, simultaneous flow to the RCS cold legs and
	// hot legs is established for the purpose of long-term core
	// cooling. [...] Switchover to this phase is performed
	// manually from the control board."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:122-138
	HotLegInjection bool

	// HHSIHotLegFlow is the per-second volume of the HHSI
	// discharge directed to the RCS hot legs while HotLegInjection
	// is armed; the remainder of HHSIInjectionFlow continues to
	// the cold legs.
	HHSIHotLegFlow Volume

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

	// CSTAligned is the train-level alternate-source suction
	// alignment: when true and the primary tank is empty, the
	// subsystem draws from the shared condensate storage tank
	// as its alternate source.
	// "An alternate emergency feedwater source (condensate storage
	// tanks) should also be provided."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:74-78
	CSTAligned bool
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

	// AutoStartElapsed is the time since the automatic-start
	// latch engaged. It gates the full-open flow-modulating-valve
	// window; manual starts do not run the clock because their
	// valves are operator-positioned from the outset.
	AutoStartElapsed time.Duration

	// DeliveredFlow is the SG-side feedwater flow integrated by
	// the most recent Step (gal/s). It equals the tank drawdown
	// rate, so secondary-side inventory stays conserved between
	// the primary tanks and the steam generators even when the
	// flow-modulating valves throttle delivery or a tank runs
	// dry. The pumps take suction from the primary tanks and
	// discharge to the SG feed piping; undelivered discharge
	// recirculates to the tank, so net tank draw is exactly
	// delivery.
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:12-21
	DeliveredFlow Volume

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
//     "Low-low level in 2/4 level channels in any one steam
//     generator" → Motor driven pumps.
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:71-79
//     "Low-low level in 2/4 level channels in any two steam
//     generators" → Turbine drive pumps.
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:82-92
//     "Safety injection" → Motor driven pumps.
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:94-98
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

	// EFWSFullOpenPeriod is the assumed no-operator-action delay
	// after an automatic EFWS start, during which the discharge
	// flow-modulating valves are credited full open.
	//
	// "When operator action can be taken (after an assumed
	// 30-minute delay), the emergency feedwater flow is adjusted
	// by positioning the flow modulating valves to restore and
	// maintain the steam generator water levels within the narrow
	// control range." ... "it is assumed that no operator action
	// can be taken for 30 minutes, and for this period it is
	// assumed that these valves will be full open."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:29-39
	EFWSFullOpenPeriod = 30 * time.Minute

	// EFWSLevelControlClose and EFWSLevelControlReopen bound the
	// post-window level-maintenance band (percent of SG volume):
	// above Close the modulating valves shut, below Reopen they
	// reopen. The source specifies a "narrow control range"
	// without numbers; the band sits under the 90 percent
	// high-level trip with hysteresis so the valves do not
	// chatter. Assumption; the source is silent about the exact
	// band.
	EFWSLevelControlClose  float64 = 85.0
	EFWSLevelControlReopen float64 = 80.0
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
var EFWSPrimaryTankCapacity Volume = Volume{Val: 25000, Unit: u.Gallon}

// EFWSPrimaryTankMaxTemp is the maximum permissible water
// temperature in the primary emergency feedwater supply tank (°F).
// "The maximum permissible water temperature in the primary
// emergency feedwater supply tank is 120°F (48.9°C)."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:56-59
var EFWSPrimaryTankMaxTemp Temperature = Temperature{Val: 120, Unit: u.Fahrenheit}

// EFWSMotorPumpFlow is the design discharge flow rate of each
// motor-driven EFWS pump (gal/s). The spec does not state a
// numeric gpm; the value 150 gpm is a typical rated flow for a
// Westinghouse four-loop motor-driven AFW pump.
var EFWSMotorPumpFlow Volume = Volume{Val: 150.0 / 60.0, Unit: u.Gallon}

// EFWSTurbinePumpFlow is the design discharge flow rate of each
// turbine-driven EFWS pump (gal/s). The spec does not state a
// numeric gpm; the value 200 gpm is higher than the motor-driven
// pump because the turbine governor can track steam demand.
var EFWSTurbinePumpFlow Volume = Volume{Val: 200.0 / 60.0, Unit: u.Gallon}

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

	// DriveReady is true when at least one ESF bus is energised;
	// without bus power the CCW pump motors cannot spin. The Step
	// function uses this to gate the recovery-lag integration.
	// "The CCWS Safeguards subsystem is considered an
	// engineered safeguards system because it is required to
	// remove decay heat during postaccident and to provide
	// cooling water to safeguards equipment."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:135-138
	DriveReady bool

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

	// HXHeatDuty is the lumped heat rate the component cooling
	// loop rejects across the component cooling heat exchangers
	// to the service water (W). Zero when the CCWS pumps are
	// stopped. Summed into the ServiceWaterSystem heat load by
	// Plant.Step.
	// "Component cooling heat exchangers"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:81
	HXHeatDuty Watts
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
var CCWSSurgeTankCapacity Volume = Volume{Val: 5000, Unit: u.Gallon}

// CCWSDesignFlow is the lumped CCW flow rate through all cooled
// components combined (gal/s). The spec does not state a single
// total flow; the value 800 gpm (≈18.5 gal/s) is representative
// of a single safeguards train serving RHR HX, CVCS HX, and RCP
// thermal barriers at full post-accident load.
// "Each safeguards train is sized to supply 100-percent
// post-accident component cooling flow requirements."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:41-42
var CCWSDesignFlow Volume = Volume{Val: 800.0 / 60.0, Unit: u.Gallon}

// CCHXApproachTemp is the lumped temperature rise across the
// CCWS heat exchanger (°F). Used as the single-step outlet
// temperature offset above the inlet so the simplified model
// does not need to track individual cooled-component heat
// loads. The spec does not state an approach temperature; 10°F
// is typical for a shell-and-tube HX with moderate heat duties.
var CCHXApproachTemp Temperature = Temperature{Val: 10.0, Unit: u.Fahrenheit}

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

	// DriveReady is true when at least one ESF bus is energised;
	// the CSS pump motors are fed from the 6600V ESF buses.
	// "Containment spray is initiated automatically in response
	// to a containment high pressure signal. This signal starts
	// the spray pumps, opens the spray header isolation valves
	// "A", and opens the SAT isolation valves "B"."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
	DriveReady bool

	// SprayHeaderOpen is true when the spray header isolation
	// valves "A" are open and spray is being delivered to the
	// ring headers.
	// "opens the spray header isolation valves "A""
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:59-60
	SprayHeaderOpen bool

	// SATIsolationValveBOpen is true when the spray-additive-tank
	// isolation valves "B" are open. Auto-start opens A and B
	// together; with B closed the SAT is isolated, so the loops
	// spray borated water without additive and the sump-pH credit
	// cannot be earned.
	// "opens the SAT isolation valves "B""
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:60-61
	SATIsolationValveBOpen bool

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
	// value of 0.5 means each gal/s of spray condenses 0.5
	// gal/s of steam.
	// "comes in contact with the steam in the containment
	// atmosphere, the steam is condensed and falls to the
	// sump."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:14-20
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
var CSSTCapacity Volume = Volume{Val: 15000, Unit: u.Gallon}

// SATCapacity is the total sodium hydroxide solution capacity of
// the simplified lumped SAT (gal). The Westinghouse plant
// specifies "4000 gal of 30 weight percent NaOH solution"
// specs/westinghouse_tech_manual/chapter_5/page_0492.txt:43;
// 500 gal is sufficient to provide additive for 30 min of spray
// at the chosen flow.
// "the SAT providing sodium hydroxide solution"
// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:62-63
var SATCapacity Volume = Volume{Val: 500, Unit: u.Gallon}

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

	// Temperature is the volume-weighted mean containment
	// atmosphere temperature (°F) derived from the two node
	// temperatures below; it is never integrated directly.
	Temperature Temperature

	// UpperTemperature / LowerTemperature are the integrated
	// node temperatures of the two-node containment atmosphere
	// (plans/x/gap_two_node_containment.md). The spec gives
	// only the total free volume, so the upper/lower split is
	// derived.
	// "A containment ventilation system that circulates the
	// containment atmosphere to remove pockets of high
	// hydrogen concentration after an accident."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:19-21
	UpperTemperature Temperature
	LowerTemperature Temperature

	// UpperHydrogenPercent / LowerHydrogenPercent are the
	// integrated node hydrogen concentrations behind the
	// derived mixture on HydrogenDetectionSuppression.
	UpperHydrogenPercent float64
	LowerHydrogenPercent float64

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

// UpperVolumeFraction is the derived upper-node share of the
// containment free volume (0.5). The Westinghouse PWR manual
// states only the total (2,000,000 ft³, chapter_6/page_109.txt:11-12),
// so the split is an assumption; the AP600 data sheet
// (specs/pwr_sim/appendix_1/page_090.txt) likewise lists a free
// volume with no node split and a different total, so the 0.5
// split follows the primary-source total, not a secondary
// design. (plans/x/gap_two_node_containment.md)
const UpperVolumeFraction float64 = 0.5

// SumpCapacityGal sizes the containment sump level gauge. The
// Westinghouse PWR manual describes the sump functionally
// (containment sump recirculation lines,
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:92-96)
// with no volume; the 15000 gal value is derived from the
// CSST/SAT scale (CSSTCapacity) and, like the CSST sizing, is a
// representative magnitude for the gauge fraction, not a spec
// total. The AP600 and BWRX-300 documents likewise size sumps
// functionally without a stated volume
// (specs/pwr_sim/appendix_1/page_090.txt,
// specs/BWRX-300-General-Description/chapter_8/page_072.txt:31).
var SumpCapacityGal Volume = Volume{Val: 15000, Unit: u.Gallon}

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

	// DriveReady is true when at least one ESF bus is
	// energised — without bus power the pump motors cannot
	// spin. The Step function uses this to gate actual flow.
	// "The larger auxiliaries such as the service water pumps
	// [...] are fed directly from the 6600-volt [engineered
	// safety features] buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
	DriveReady bool

	// Flow is the total essential service water flow rate
	// (gpm). Calculated as ServiceWaterPumpFlow × number of
	// running pumps (up to ServiceWaterEssentialPumpCount),
	// capped at the design maximum.
	Flow float64

	// InletTemp is the service water inlet temperature (°F) as
	// it enters the plant intake. Tracks Outside.RiverTemp;
	// site-specific in a full model.
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

	// LoopAPumpRunning is the running state of the loop-A duty
	// cooler pump. Normal state is both duty loops (A and B)
	// running with the third pump in standby.
	// "Two of three pump-heat-exchanger-filter-demineralizer
	// loops remove decay heat"
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	LoopAPumpRunning bool

	// LoopBPumpRunning is the running state of the loop-B duty
	// cooler pump. See LoopAPumpRunning.
	LoopBPumpRunning bool

	// DriveReady is true when the 480 V MCC that feeds the SFPC
	// pump motors is energised. The spec is silent on the SFPC
	// electrical supply; by exclusion from the explicit ESF load
	// list at
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
	// and the 6600 V vs. 480 V split at
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:77-82,
	// the pumps are inferred to ride on a 480 V MCC downstream of
	// an ESF bus. When false, the pumps spin down to zero flow
	// even if PumpRunning is latched by the standby auto-start.
	DriveReady bool

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

	// Leakage is the latched supply-header leak. Pressure
	// instrumentation detects it and motor-operated valves isolate
	// it: "Detection of leakage which might affect system operation
	// is provided by means of installed pressure, temperature, and
	// flow instrumentation [...] system isolation is provided by
	// means of motor-operated valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:75-83
	Leakage bool

	// CrudBurst is the latched refueling-canal crud burst that robs
	// operators of visual access to the stored fuel: "The refueling
	// water purification pump and filter can be used separately or
	// in conjunction with the spent fuel pool demineralizer to
	// regain refueling canal water clarity after a crud burst."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-90
	CrudBurst bool

	// Turbidity is the pool-water clouding normalized to the design
	// crud burst (0 clear, 100 full slug). Cleanup runs whenever any
	// purification stream is active; the spec is qualitative, so the
	// scale is a model convention (assumption A-4 in
	// plans/x/scenario/34.txt).
	Turbidity float64

	// RWPUPurifying is the operator alignment of the refueling water
	// purification pump and filter beside the spent fuel pool
	// demineralizer: "can be used separately or in conjunction with
	// the spent fuel pool demineralizer to regain refueling canal
	// water clarity".
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-94
	RWPUPurifying bool
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

	// SFPCPressureTimeConstant is the first-order relaxation time of
	// the supply header toward the running-pump pressure target.
	// Assumption (A-5 in plans/x/scenario/34.txt); the spec states
	// only the maintained band and the auto-start setpoint.
	SFPCPressureTimeConstant = 30 * time.Second

	// SFPCCoolingTimeConstant is the pool relaxation time toward the
	// design temperature while cooler loops are running. Assumption
	// (plans/x/scenario/34.txt A-1 note); mirrors the CCWS
	// recovery-lag convention.
	SFPCCoolingTimeConstant = 1 * time.Hour

	// SFPCPoolBoilingTemp is the atmospheric boiling point of the
	// pool water (°F); an uncooled pool approaches it but never
	// exceeds it. "would eventually boil" motivation per
	// plans/x/scenarios.txt item 34 and page_171.txt:24-29.
	SFPCPoolBoilingTemp = 212.0 // °F

	// SFPStoredFuelDecayHeat is the assumed decay-heat load of the
	// stored fuel assembly inventory (W). The spec gives no pool
	// heat-load figure; anchored between a day-old full core
	// (~0.4% of rated, specs/wiki/Decay_heat.txt) and older stored
	// batches (assumption A-1 in plans/x/scenario/34.txt).
	SFPStoredFuelDecayHeat Watts = 10_000_000

	// SFPCLeakPressureDrop is the header depression while a leak is
	// latched (psig). Sized to pull the two-pump header below the
	// 50 psig standby auto-start yet keep the leaked three-pump
	// header pressurized (assumption A-2 in
	// plans/x/scenario/34.txt).
	SFPCLeakPressureDrop = 25.0 // psig

	// SFPCPressureStandbyTarget is the three-running-pump header
	// target (psig), the midpoint of the maintained band.
	SFPCPressureStandbyTarget = (SFPCPressureNormalMin +
		SFPCPressureNormalMax) / 2.0

	// SFPCPressureOneLoopTarget is the header target (psig) with a
	// single duty loop running. Derived/assumption: sized below the
	// 50 psig standby-start setpoint so losing one duty loop sags
	// the header through the pickup point and the third pump
	// recovers the band through real dynamics.
	SFPCPressureOneLoopTarget = 40.0

	// SFPCPurificationCleanupTimeConstant is the turbidity e-folding
	// time for one design purification sidestream; cleanup scales
	// linearly with total active purification flow. Assumption A-3
	// in plans/x/scenario/34.txt.
	SFPCPurificationCleanupTimeConstant = 30 * time.Minute

	// SFPCCrudBurstTurbidityPercent is the turbidity slug a crud
	// burst injects, normalized to the design burst (percent).
	// Model convention, assumption A-4 in plans/x/scenario/34.txt.
	SFPCCrudBurstTurbidityPercent = 100.0

	// SFPRWPUPurificationFlow is the refueling water purification
	// pump flow while aligned beside the demineralizer (gpm).
	// Assumption; the spec gives the RWPU duty but no flow figure
	// (specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-94).
	SFPRWPUPurificationFlow = 100.0 // gpm
)

// SFPCPoolDesignTemp is the spent fuel pool water temperature held
// by the cooling loops (°F). Model seed convention; the spec names
// no pool temperature.
var SFPCPoolDesignTemp = Temperature{Val: 100.0, Unit: u.Fahrenheit}

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
var SFPCPoolVolume Volume = Volume{Val: 500_000, Unit: u.Gallon}

// Boron Recycle System constants.
//
// Spec justification:
//   - Evaporator duty: "use of the BTRS reduces the average
//     evaporator duty from approximately 10 gpm (38 l/m) to less
//     than 2 gpm (8 l/m) per unit."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:71-74
const (
	// BRSSEvaporatorDutyGPM is the reclaim-train processing rate
	// (gpm), the without-BTRS average evaporator duty. The scenario
	// that exercises reclamation runs precisely when boron changes
	// exceed BTRS capacity, so the higher figure applies. Like the
	// CVCS design flows it integrates per second of simulated time
	// (see VCTBTRSCirculation for the same convention).
	BRSSEvaporatorDutyGPM = 10.0

	// BRSSProductWeightPercentPpm is the evaporator concentrate grade
	// in ppm by mass (4 weight percent).
	// "The evaporative process produces a batch of
	// 4-weight-percent boric acid solution."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:6-7
	BRSSProductWeightPercentPpm = 40000.0

	// BRSSRadiationTauSec is the first-order response time of the
	// lumped distillate radiation monitor to evaporator carryover.
	// The spec describes the monitor's function, not its response;
	// modeled value.
	BRSSRadiationTauSec = 30.0

	// BRSSRadiationTripFrac is the normalized monitor level that
	// constitutes a high radiation signal; DistillateRadiation reads
	// against it.
	BRSSRadiationTripFrac = 0.95

	// BRSSRadiationReleaseFrac is the decayed monitor level below
	// which the auto-divert releases once the carryover source has
	// cleared; the hysteresis gap prevents divert chatter.
	BRSSRadiationReleaseFrac = 0.05
)

// BoronRecycleSystem models the Boron Recycle System — the subsystem
// that collects borated reactor coolant effluent (CVCS letdown diverted
// by makeup-control boron changes and heatup overflow) and reclaims it,
// by demineralization, gas stripping, and evaporation, into 4-weight-
// percent boric acid plus clean distillate for the reactor makeup water
// storage tank. The lumped model tracks the panel start latch, the feed
// draw from the CVCS holdup tanks, product accounting, and the distillate
// radiation monitor with its auto-divert.
//
// Spec justification:
//   - Purpose: "The BRS provides a means of recycling borated reactor
//     coolant so as to minimize activity releases."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:16-18
//   - Reclaim means: "The BRS, by means of demineralization, gas
//     stripping, and evaporation, reclaims the boric acid and the
//     primary water."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:49-51
//   - Feed train: letdown coolant passes through the recycle evaporator
//     feed demineralizers where lithium and fission products are removed
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:76-80;
//     hydrogen and residual fission gases leave in the stripping column
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:3-5.
type BoronRecycleSystem struct {
	Part

	// FeedPumpRunning is the operator latch on the recycle evaporator
	// feed pump(s) from the boron recycle panel; it is the motive
	// force moving holdup-tank liquid through the demineralizers,
	// stripping column, and evaporator shell.
	// "borated liquid is then pumped to the recycle evaporator package
	// by one of the recycle evaporator feed pumps"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:1-3
	FeedPumpRunning bool

	// Carryover is the latched evaporator-carryover condition that
	// contaminates the distillate with radioactivity. The spec does
	// not name a cause for high distillate activity; the monitor's
	// existence implies one ("To provide that the reactor makeup water
	// storage tank will not become contaminated"), so this is the
	// modeling device exercising that protection.
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:25-28
	Carryover bool

	// FeedFlow is the last-step processing rate (gpm) drawn from the
	// holdup tanks (plus recirculated distillate) into the evaporator
	// package.
	FeedFlow float64

	// HoldupDrawn is the holdup-tank volume consumed last step. The
	// plant step debits CVCS.BRSHoldupLevel by this amount so the
	// holdup tanks keep a single writer.
	HoldupDrawn Volume

	// DistillateRadiation is the lumped scintillation-counter signal
	// normalized against the trip level (BRSSRadiationTripFrac).
	DistillateRadiation float64

	// DistillateDiverted is true while the high-radiation signal has
	// the distillate routed back to the feed demineralizer instead of
	// the reactor makeup water storage tank.
	// "A high radiation level signal at the monitor directs distillate
	// back to the recycle evaporator feed demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:28-30
	DistillateDiverted bool

	// Recirculating is the distillate inventory returned to the train
	// for reprocessing while Diverted is latched.
	Recirculating Volume

	// BoricAcidProduced accumulates the 4 wt% concentrate delivered to
	// the boric acid tanks through the concentrates filter.
	// "Concentrates at 4-weight-percent boric acid are normally pumped
	// to the boric acid tanks through the recycle evaporator
	// concentrates filter."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:53-55
	BoricAcidProduced Volume

	// DistillateDelivered accumulates the clean distillate pumped to
	// the reactor makeup water storage tank.
	// "Distillate from the evaporator is pumped to the evaporator
	// condensate demineralizer and then to the reactor makeup water
	// storage tank."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:20-22
	DistillateDelivered Volume
}

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
// phase timer advances ProductionStep seconds per step.
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

	// GenerationActive is the training-mode fallback of the
	// zirconium-water reaction: it pins the hydrogen source at
	// HydrogenGenerationRate regardless of inventory/thermal state.
	// It is set only by the operator/training harness, never by
	// physics, and is documented as not a production LOCA trigger.
	// Production generation is driven by the derived
	// hydrogenSourceRate HDS.Step receives from Plant.Step
	// (plans/x/gap_hydrogen_kinetics.md §1).
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

	// PocketExchangePerHour is the inter-region transport
	// rate of the two-node containment atmosphere (% per
	// hour of gap per hour). It drives both the hydrogen
	// node gap (hds.go) and the temperature node gap
	// (rcfc.go); the Westinghouse PWR manual describes the
	// ventilation function ("remove pockets of high
	// hydrogen concentration", chapter_6/page_117.txt:19-21)
	// with no rate, and neither the AP600 data sheet
	// (specs/pwr_sim/appendix_1/page_090.txt) nor BWRX-300
	// (specs/BWRX-300-General-Description/chapter_3/) state
	// an inter-region rate either, so 0.2/h is a derived
	// hourscale assumption carried over from the retired
	// pre-G3 magnitude (plans/x/gap_two_node_containment.md,
	// plans/x/big_gaps.md G3).
	PocketExchangePerHour float64 = 0.2

	// NaturalExchangePerHour is the same inter-region
	// transport at ventilation off (buoyancy only), weaker
	// than PocketExchangePerHour but nonzero so a secured
	// plant stratifies rather than freezes.
	NaturalExchangePerHour float64 = 0.05

	// HydrogenGenerationRate is the volumetric production
	// rate of hydrogen in the containment atmosphere during
	// early LOCA from the Zirconium water reaction (% per
	// hour). The spec does not state a rate; 1.0 %/hour is
	// representative of the early-phase generation spike.
	HydrogenGenerationRate float64 = 1.0

	// HydrogenInventoryGateFraction is the fraction of
	// drainable primary inventory (BreakDischarged net of
	// injection credit, over RCSDrainableInventoryGal) above
	// which H2 generation is allowed even when the core is at
	// or below saturation. Below it and subcooled, the derived
	// rate is zero. plans/x/gap_hydrogen_kinetics.md §1.
	HydrogenInventoryGateFraction float64 = 0.5

	// HydrogenSuperheatGateF is the superheat above which H2
	// generation engages on steam existence alone (steam at
	// hot clad); at or below it, generation requires ≥50%
	// drainable lost. Post-blowdown coolant tracks Tsat in
	// this tree (applyBlowdownCooling), so superheat never
	// reaches large positive values after a blowdown; the
	// inventory leg carries blowdown-driven cases.
	// plans/x/gap_hydrogen_kinetics.md §1.
	HydrogenSuperheatGateF float64 = 5.0

	// HydrogenHeatNormW normalises the decay-heat ramp term:
	// coreHeatW/HydrogenHeatNormW reads ~0.49 at post-trip
	// initial decay heat (3800 MWt rated, 6.5% shutdown decay
	// fraction = ~247 MW), decaying toward ~0.11 by 4h (spec
	// 1.5% decay at 1h). At full power the term saturates
	// (3.8 GW/5e8 = 7.6), so any gate opening at power goes
	// straight to the 1.0 %/hour ceiling; the ramp only exists
	// post-trip. plans/x/gap_hydrogen_kinetics.md §1.
	HydrogenHeatNormW = 5e8

	// HydrogenRecombinerHysteresis is the volume percent
	// above the alarm setpoint at which the recombiners
	// activate; below (setpoint + hysteresis) they stay
	// active to prevent cycling near the setpoint.
	HydrogenRecombinerHysteresis float64 = 0.5

	// BallooningF is the lumped fuel-ballooning onset: the
	// "first stage of core damage" at ~1,100 K (1,520 °F).
	// plans/p65_1.txt Tier-A (sourced from
	// specs/wiki/Nuclear_meltdown.txt, Kuan/Haskin).
	BallooningF = 1520.0

	// CladFailureF is the 10 CFR 50.46 limit (a) peak-clad
	// temperature cap: "Peak clad temp. < 2200°F"
	// (specs/westinghouse_tech_manual/chapter_2/page_0209.txt:25-30).
	// Owned by p65_1 (plans/p65_1.txt); the CoreDamageFrac
	// saturation point reuses it. plans/p65_3.txt must not
	// redeclare it.
	CladFailureF = 2200.0

	// CladThermalMassCpJPerDegF is the lumped hot-rod thermal
	// mass (J/°F) behind the clad-heatup integrator. Tier-B
	// derived (plans/p65_1.txt Heating law): post-trip decay
	// heat 6.5% × 3,800 MWt ≈ 247 MW divided by the sourced
	// pre-damage heatup band midpoint (~0.65 K/s, ~1.17 °F/s;
	// specs/wiki/Nuclear_meltdown.txt) gives ≈2.5e8 J/°F.
	CladThermalMassCpJPerDegF = 2.5e8
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
	// It votes the reactor trip only: Table 7-1's ESFAS trigger
	// lists carry their own manual pushbuttons ("Manual safety
	// injection" under Safety Injection), and a reactor trip is
	// described as avoiding unnecessary ESFAS actuation.
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-14
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:11-13
	ManualTrip bool

	// SafetyInjectionBlocked is the operator block on SI
	// initiation signals armed during planned cooldowns before
	// process variables reach the automatic actuation setpoints.
	// It gates only the SI ESFAS output; reactor-trip channels
	// are untouched. The block persists until the operator clears
	// it through the bridge control.
	// "Prior to reaching the automatic safety injection setpoints
	// during cooldown and depressurization, the safety injection
	// initiation signals are blocked."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	SafetyInjectionBlocked bool

	// SteamlineIsolationBlocked is the matching planned-cooldown
	// block for the steamline-isolation ESFAS output, whose low
	// steamline pressure trigger shares SI's setpoint: an
	// intentional atmospheric-dump cooldown legitimately carries
	// the header through it. It gates only that trigger — high
	// containment pressure and manual initiation stay live. The
	// chapter documents the blocking practice for the shared
	// setpoint's SI signal; extending it to the same-trigger
	// isolation output is a modeling assumption (source is silent
	// on a dedicated isolation block).
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:107-110
	SteamlineIsolationBlocked bool

	// ManualSI is true when the operator has pressed a manual
	// safety-injection (or manual steamline-isolation / Phase A)
	// pushbutton; it carries the "manual" entries of the ESFAS
	// trigger lists, which are separate signals from the manual
	// reactor trip.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-21
	ManualSI bool

	// ChannelTest isolates one monitored input for calibration
	// and testing: while isolated, that measurement is forced
	// away from its trip setpoints before any voting or ESFAS
	// trigger evaluates, so a stimulus injected into the tested
	// channel cannot actuate protections. The zero value runs all
	// channels in service.
	// "The protection system normally uses 2/4 logic but reverts
	// to a safe 2/3 logic when part of the system is being tested
	// or is involved in maintenance."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
	ChannelTest RPSChannelTest

	// FlowEstablished is the low-flow trip's process memory: it sets
	// once measured forced flow has reached the trip setpoint and
	// stays set, so the trip function answers a DEGRADED flow - a loss
	// from an established lineup, not the startup ramp of pumps that
	// were never up. Modeling convention; the spec documents the trip
	// function but not its interlock memory ("low flow ... provide
	// trips",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:80-87).
	FlowEstablished bool

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
	// (%) above which a reactor trip signal is generated.
	// Assumption; the source lists the trip signal without a
	// numeric setpoint (specs/the_westinghouse_pressurized_water_react/
	// chapter_7/page_138.txt:19-21). Sized above the full-load
	// programmed level (60 percent) with surge margin, mirroring
	// the SG hi-channel's relation to its own operating band.
	RPSHighPressurizerLevelTrip float64 = 80.0

	// RPSLowSteamGeneratorLevelTrip is the steam generator
	// level (%) below which a trip signal is generated.
	RPSLowSteamGeneratorLevelTrip float64 = 15.0

	// RPSHighSteamGeneratorLevelTrip is the steam generator
	// level (%) above which a trip signal is generated.
	RPSHighSteamGeneratorLevelTrip float64 = 90.0

	// RPSLowSteamlinePressureTrip is the steamline pressure
	// (psia) below which a trip signal is generated. The SG shell
	// channel is absolute, so the spec's 585 psig limit converts
	// to ~600 psia.
	//
	// "Before raising primary system pressure above 1900 psig
	// (132 bar), steam generator pressure must be greater than
	// 585 psig (41 bar) or a low steamline pressure safety
	// injection will occur."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:150-153
	// (cooldown twin:
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:50-53)
	RPSLowSteamlinePressureTrip Pressure = 600.0

	// RPSLowTcoldTrip is the reactor coolant cold-leg
	// temperature (°F) below which a trip signal is generated.
	RPSLowTcoldTrip float64 = 530.0

	// RPSLowFlowTripFraction is the forced-coolant-flow fraction of
	// rated below which a reactor-trip signal is generated while the
	// at-power permissive is satisfied. The spec lists the function
	// but not its numeric setpoint:
	// "Primary coolant trip signals - Departure from nucleate boiling
	// ratio, kw/ft, low flow, and reactor coolant pump under-speed
	// trip signals provide trips."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:80-87
	// 0.85 of rated flow is an assumption typical of Westinghouse
	// low-flow trips; with one of four machines lost the flywheel
	// coastdown crosses it about 9 s after the trip.
	RPSLowFlowTripFraction float64 = 0.85

	// RPSAtPowerPermissiveFraction is the reactor power fraction
	// below which the "at-power" reactor trips are not enabled
	// (the P-7 permissive). Cold-condition lineups and startups
	// operate far below this level with low Tcold expected, so
	// low-Tcold trip contributions are blocked until the plant
	// exceeds it.
	// "When reactor power increases above 10%, verify the 'Nuclear
	// At-Power' Permissive P-10 light illuminates and the
	// 'At-Power' Permissive P-7 light extinguishes."
	// specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-5
	RPSAtPowerPermissiveFraction float64 = 0.10

	// RPSHighNeutronFluxTripFraction is the neutron-flux fraction of
	// rated full-power flux that declares the overpower reactor trip
	// outright. Derived: the primary document lists the flux trip
	// among the reactor trip signals without a number; the 120
	// percent value is adopted from the modeling source.
	//   "High neutron flux trip - trip setpoint = 120 % of Neutron
	//    Flux at full power"
	//   specs/pwr_sim/chapter_2/page_025.txt:32
	RPSHighNeutronFluxTripFraction float64 = 1.20

	// RPSHighPressurizerPressureTrip is the pressurizer pressure
	// (psia) above which the overpower-pressure channel declares the
	// reactor trip outright. Derived: the primary document lists the
	// channel without a number; the value sits inside the model's
	// own relief escalation order - above the full-power operating
	// band, below the PORV and code-safety setpoints - so the scram
	// arrests the heatup before any relieving device lifts.
	//   "Pressurizer trip signals - High pressure, low pressure,
	//    and high-level trip signals initiate a reactor trip"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:19-21
	RPSHighPressurizerPressureTrip Pressure = 2385.0

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

	// Plant electrical systems design constants.
	//
	// Spec justification:
	//   - 6600-volt engineered safety features buses.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-81
	//   - Diesel generators: 10 seconds to speed, 30 seconds to
	//     rated load; 7 days fuel capacity.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:131-136
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:118-122
	//   - 125-volt DC battery system, 118-volt AC instrument
	//     buses.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:124-129
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-27
	//   - Automatic transfer on sensing loss of power.
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59

	// ESFSystemVoltage is the nominal voltage of the
	// engineered safety features buses (volts).
	// "two 6600-volt engineered safety features buses"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
	ESFSystemVoltage = 6600.0

	// ESFSystemCount is the number of engineered safety
	// features buses (2).
	// "All of the engineered safety system loads are fed from
	// two 6600-volt engineered safety features buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
	ESFSystemCount = 2

	// DieselStartupTime is the time for a diesel generator
	// to reach rated speed and be ready to begin accepting
	// load (seconds).
	// "reach rated speed and be ready to begin accepting load
	// within 10 seconds"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-50
	DieselStartupTime = 10.0

	// DieselRatedLoadTime is the time for a diesel generator
	// to carry rated load after receipt of a start signal
	// (seconds).
	// "capable of carrying rated load within 30 seconds after
	// receipt of a start signal"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50-51
	DieselRatedLoadTime = 30.0

	// DieselFuelCapacityDays is the minimum number of days of
	// continuous rated-load operation from the onsite fuel
	// supply.
	// "sufficient to allow continuous rated load operation for
	// a minimum of seven days"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:118-122
	DieselFuelCapacityDays = 7.0

	// DCSystemVoltage is the nominal voltage of the 125-volt
	// DC battery system (volts).
	// "two physically separate and electrically isolated
	// sources of 125-volt dc power"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:124-126
	DCSystemVoltage = 125.0

	// DCSystemCount is the number of physically separate
	// 125-volt DC sources.
	// "two physically separate and electrically isolated
	// sources of 125-volt dc power"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:124-126
	DCSystemCount = 2

	// InverterVoltage is the nominal AC output voltage of the
	// instrument inverters (volts).
	// "The 118-volt ac instrument power supply is divided
	// into four buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-23
	InverterVoltage = 118.0

	// InverterFrequency is the nominal output frequency of
	// the instrument inverters (Hz).
	InverterFrequency = 60.0

	// InverterBusCount is the number of 118-volt instrument
	// buses (one per protection channel).
	// "divided into four buses. These four buses provide
	// power for reactor protection and engineered safety
	// features instrumentation and control channels I, II,
	// III, IV, respectively."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-27
	InverterBusCount = 4

	// AutoTransferDelay is the assumed delay for automatic
	// bus transfer on sensing loss of offsite power
	// (seconds). The spec states automatic transfer is
	// provided on sensing loss of power, but does not
	// specify a numerical delay.
	// "automatic transfer of supplies is provided on sensing
	// loss of power"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
	AutoTransferDelay = 1.0

	// Plant control room design constants.
	//
	// Spec justification:
	//   - "Over a dozen graphical display devices (CRTs and
	//     flat panel plasma displays) are located in the
	//     operations area of the control room."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:39-45
	//   - "Key process variables are presented via dedicated
	//     displays."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:71-75
	//   - "Horns, buzzers, bells, etc. Used to attract
	//     attention to a significant situation. Turned off by
	//     operator acknowledgement."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:64-67
	//   - "Auxiliary control panels contain [...] the safety
	//     systems and functions which are arranged in
	//     stations each dedicated to a single train of
	//     safeguard systems and equipment."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:61-67

	// ControlRoomGraphicDisplayCount is the number of
	// graphical display devices at the PCC.
	// "Over a dozen graphical display devices (CRTs and
	// flat panel plasma displays) are located in the
	// operations area of the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:39-42
	ControlRoomGraphicDisplayCount = 12

	// ControlRoomDedicatedDisplayCount is the number of
	// dedicated displays for key process variables. The
	// spec does not state an exact number; 24 is assumed.
	// "key process variables are presented via dedicated
	// displays."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:71-75
	ControlRoomDedicatedDisplayCount = 24

	// ControlRoomAnnunciatorWindowCount is the number of
	// annunciator windows in the control room. The spec
	// does not state an exact number; 120 is assumed.
	// "A grouping of status lights to indicate malfunction
	// conditions."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:69-72
	ControlRoomAnnunciatorWindowCount = 120

	// ControlRoomAlarmAckDelay is the assumed operator
	// alarm-acknowledgement delay (seconds). The spec
	// states the alarm is "Turned off by operator
	// acknowledgement" without a numerical deadline.
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:64-67
	ControlRoomAlarmAckDelay = 5.0

	// ControlRoomSafetyStationCount is the number of safety
	// system stations on the auxiliary panels. The spec
	// describes stations per safeguard train without a
	// fixed count; four is the simplified model's
	// representation (SIS A/B, EFWS A/B).
	// "arranged in stations each dedicated to a single
	// train of safeguard systems and equipment."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:65-67
	ControlRoomSafetyStationCount = 4

	// Plant computer system design constants.
	//
	// Spec justification:
	//   - "The computer collects data from many locations
	//     within the plant and then processes it to provide
	//     a variety of information."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
	//   - Table 9-1 lists computer information devices and
	//     formats (mimic, bar chart, tabular, graphical, data
	//     trends, polar graphical, mixed, video, typer, line
	//     printer, bell, pen recorder, lights, meters, magnetic
	//     tape).
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:104-202
	//   - "Provides more effective display format such as
	//     mimic diagrams, graphs, and bar charts."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_153.txt:11-12
	//   - "Safety Parameter Display System – Presents a
	//     succinct display account of the overall plant
	//     safety status."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35

	// PlantComputerScanRate is the assumed analog input
	// scan rate (samples per second). The spec does not
	// state a numerical rate; 100 Hz is a typical value.
	// "Analog/digital conversions of signals from a variety
	// of analog sensors."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:45-46
	PlantComputerScanRate = 100

	// PlantComputerDisplayFormatCount is the number of
	// display formats supported (from Table 9-1).
	PlantComputerDisplayFormatCount = 12

	// PlantComputerMimicDisplayCount is the number of
	// mimic displays available (system diagram + key
	// parameters). The spec does not state a count; 16 is
	// assumed.
	// "the mimic display is defined as a display combining
	// a diagrammatic representation of a system and the
	// values of key parameters in the system."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:23-26
	PlantComputerMimicDisplayCount = 16

	// PlantComputerSPDSRefreshRate is the assumed refresh
	// rate of the Safety Parameter Display System (Hz).
	// "Safety Parameter Display System – Presents a
	// succinct display account of the overall plant safety
	// status."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	PlantComputerSPDSRefreshRate = 1.0

	// PlantComputerAlarmProcessingDelay is the assumed
	// delay for the computer to process and display an
	// alarm (seconds). The spec does not state a numerical
	// value; 0.5 s is typical for an analog scan-to-display
	// latency.
	// "Alarming – Checks for individual parameters
	// exceeding limits."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:32-33
	PlantComputerAlarmProcessingDelay = 0.5

	// PlantComputerSequenceOfEventsBufferSize is the
	// assumed buffer size for sequence of events
	// recording (events). The spec does not state a count;
	// 1000 is assumed.
	// "Post-trip review – Prints a history of parameter
	// values before and after a trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:1-3
	PlantComputerSequenceOfEventsBufferSize = 1000
)

// RPSChannelTest selects which monitored input is isolated for
// calibration/testing. The bridge exposes these as enum strings.
type RPSChannelTest int

const (
	RPSNoChannelInTest RPSChannelTest = iota
	RPSSteamlineChannelInTest
	RPSSGLevelChannelInTest
	RPSTcoldChannelInTest
	RPSContainmentChannelInTest
	RPSPressurizerChannelInTest
	RPSFlowChannelInTest
)

// name returns the bridge enum string for the selection.
func (t RPSChannelTest) name() string {
	switch t {
	case RPSSteamlineChannelInTest:
		return "Steamline"
	case RPSSGLevelChannelInTest:
		return "SGLevel"
	case RPSTcoldChannelInTest:
		return "Tcold"
	case RPSContainmentChannelInTest:
		return "Containment"
	case RPSPressurizerChannelInTest:
		return "Pressurizer"
	case RPSFlowChannelInTest:
		return "Flow"
	default:
		return "None"
	}
}

// PlantElectricalSystems models the plant electrical power
// distribution, including offsite power, diesel generators,
// 6600-volt engineered safety features buses, automatic
// transfer between sources, and the 118-volt instrument
// inverters that feed reactor protection and ESF
// instrumentation channels I-IV. The simplified model tracks
// power availability and diesel-start timing as boolean and
// timer scalars; it does not model transformer dynamics,
// load sequencing, or battery discharge.
//
// Spec justification:
//   - Diversity of reliable power sources: "physically and
//     electrically isolated so that any single failure will
//     affect only one source."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:9-14
//   - ESF buses normally from offsite, diesel backup:
//     "normally supplied from the system auxiliary
//     transformers. In the event of total loss of auxiliary
//     power from off-site sources, auxiliary power required
//     for safe shutdown will be supplied from [...] diesel
//     generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:28-33
//   - Diesel start timing: "reach rated speed and be ready
//     to begin accepting load within 10 seconds, and be
//     capable of carrying rated load within 30 seconds after
//     receipt of a start signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-51
//   - Inverter power: four 118V buses for RPS and ESF
//     channels I-IV: "The 118-volt ac instrument power
//     supply is divided into four buses. These four buses
//     provide power for reactor protection and engineered
//     safety features instrumentation and control channels
//     I, II, III, IV, respectively."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-27
type PlantElectricalSystems struct {
	Part

	// OffsitePowerAvailable is true when the plant is
	// receiving power from the off-site main distribution
	// grid through the system auxiliary transformers.
	// "The system auxiliary power is supplied from
	// independent and electrically isolated buses of the
	// off-site main distribution grid."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:45-47
	OffsitePowerAvailable bool

	// Diesel1Running is true when diesel generator 1 has
	// reached rated load and is ready to accept ESF bus
	// load.
	Diesel1Running bool

	// Diesel2Running is true when diesel generator 2 has
	// reached rated load and is ready to accept ESF bus
	// load.
	Diesel2Running bool

	// Diesel1StartTime is the elapsed time in seconds since
	// diesel generator 1 received a start signal. Reset to
	// zero when offsite power returns.
	Diesel1StartTime float64

	// Diesel2StartTime is the elapsed time in seconds since
	// diesel generator 2 received a start signal. Reset to
	// zero when offsite power returns.
	Diesel2StartTime float64

	// ESFBus1Available is true when Engineered Safety
	// Features bus 1 is powered (from offsite or from diesel
	// 1 after AutoTransferDelay).
	ESFBus1Available bool

	// ESFBus2Available is true when Engineered Safety
	// Features bus 2 is powered (from offsite or from diesel
	// 2 after AutoTransferDelay).
	ESFBus2Available bool

	// InverterPowerAvailable is true when the 118V
	// instrument-bus inverter supply is available for
	// reactor protection and ESF instrumentation. The
	// simplified model collapses the four channel-buses
	// into a single boolean that mirrors any one of them
	// being powered.
	// "The 118-volt ac instrument power supply is divided
	// into four buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-23
	InverterPowerAvailable bool

	// StationBlackout is true when both offsite power and
	// both diesel generators are unavailable.
	StationBlackout bool
}

// PlantControlRoom models the plant control room operator
// interface. The simplified model tracks display and
// control availability, alarm state, and operator
// acknowledgement. It does not model human factors,
// cognitive load, panel layout, or decision-support
// algorithms.
//
// Spec justification:
//   - Control room purpose: "display and control hardware
//     needed to operate a nuclear power plant in a safe,
//     efficient, and reliable manner."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:4-9
//   - ACR function: "optimizes the generation and display of
//     useful plant information to enable rapid
//     understanding of plant condition."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:16-20
//   - NSSS-BOP integration: "effective man-machine
//     interface which is consistent for all plant systems."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:1-5
//   - PCC scope: "all controls, displays and instrumentation
//     required for normal power operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:23-40
//   - Auxiliary panels: "safety systems and functions which
//     are arranged in stations each dedicated to a single
//     train of safeguard systems and equipment."
//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:61-67
type PlantControlRoom struct {
	Part

	// PowerAvailable is true when the control room has
	// electrical power for displays and controls (from
	// p27's PlantElectricalSystems).
	PowerAvailable bool

	// PCCAvailable is true when the Primary Control Center
	// is operational.
	// "The PCC is the location for all controls, displays
	// and instrumentation required for normal power
	// operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:23-26
	PCCAvailable bool

	// AuxiliaryPanelsAvailable is true when the auxiliary
	// control panels are operational.
	// "The auxiliary control panels contain all the
	// controls, displays and instrumentation less
	// frequently used and not mounted at the PCC."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:61-65
	AuxiliaryPanelsAvailable bool

	// GraphicDisplaysAvailable is true when CRT and flat
	// panel graphic displays are operational.
	// "Over a dozen graphical display devices (CRTs and
	// flat panel plasma displays) are located in the
	// operations area of the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:39-42
	GraphicDisplaysAvailable bool

	// DedicatedDisplaysAvailable is true when dedicated
	// displays for key process variables are operational.
	// "Key process variables are presented via dedicated
	// displays."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_143.txt:71-75
	DedicatedDisplaysAvailable bool

	// AnnunciatorAlarm is true when an annunciator panel
	// is signalling an abnormal condition.
	// "Horns, buzzers, bells, etc. Used to attract
	// attention to a significant situation."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:64-66
	AnnunciatorAlarm bool

	// AlarmAcknowledged is true when the operator has
	// acknowledged the annunciator alarm.
	// "Turned off by operator acknowledgement."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67

	// SimTimeNs is the control room's simulated clock: the
	// accumulated plant time at the start of the last Step.
	// Plant.Step calls Step exactly once per tick, so the
	// clock tracks simulation time and stamps each alarm's
	// activation moment in AlarmSet.
	SimTimeNs         int64
	AlarmAcknowledged bool

	// AlarmSet is the per-condition acknowledgement map keyed
	// by AlarmID. Step() merges incoming alarms into this map;
	// ResetAlarm() / AcknowledgeAll() flip the Acknowledged
	// flag on individual entries.
	// "Turned off by operator acknowledgement."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	AlarmSet map[AlarmID]Alarm

	// ControlInputsAvailable is true when operator
	// control inputs (switches, pushbuttons, keyboards)
	// are operational.
	// "Controls have been segregated into operating
	// areas which optimize the location of frequently
	// used and important controls."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_146.txt:1-6
	ControlInputsAvailable bool

	// SafetySystemStationsAvailable is true when
	// auxiliary panel safety system stations are
	// operational.
	// "arranged in stations each dedicated to a single
	// train of safeguard systems and equipment."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:65-67
	SafetySystemStationsAvailable bool
}

// PlantComputer models the plant computer system for data
// collection, processing, display formatting, and malfunction
// detection. The simplified model tracks data acquisition
// availability, display format availability, supervision
// status, and alarm state. It does not model scan rates,
// calibration procedures, or detailed human factors.
//
// Spec justification:
//   - Computer mission: "collects data from many locations
//     within the plant and then processes it to provide a
//     variety of information."
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
//   - Display formats (Table 9-1): mimic, bar chart, tabular,
//     graphical, data trends, polar graphical, mixed.
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:104-202
//     and specs/the_westinghouse_pressurized_water_react/chapter_9/page_153.txt:6-12
//   - Reactor control and protection supervision:
//     "enhance the reactor control and protection systems by
//     reporting on their status, by duplicating or checking
//     some of their calculations."
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_155.txt:3-8
//   - NSSS process supervision: "provide data about the
//     operation of the nuclear power plant."
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_155.txt:34-36
//   - Safety Parameter Display System: "Presents a succinct
//     display account of the overall plant safety status."
//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
type PlantComputer struct {
	Part

	// DataAcquisitionAvailable is true when the computer is
	// powered and operational.
	// "The plant interrogates the plant's parameters by:
	// Analog/digital conversions of signals from a variety
	// of analog sensors."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:43-46
	DataAcquisitionAvailable bool

	// MimicDisplaysAvailable is true when the mimic display
	// format is available (diagram + key parameters).
	// "the mimic display is defined as a display combining
	// a diagrammatic representation of a system and the
	// values of key parameters in the system."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:23-26
	MimicDisplaysAvailable bool

	// BarChartDisplaysAvailable is true when bar chart format
	// is available.
	// "Provides a sense of relative values."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:123-126
	BarChartDisplaysAvailable bool

	// TabularDisplaysAvailable is true when tabular format
	// is available for large data sets.
	// "Effective method of presenting large quantities of
	// data."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:128-131
	TabularDisplaysAvailable bool

	// GraphicalDisplaysAvailable is true when graphical
	// format is available.
	// "Aids in comprehension of relationships."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:133-136
	GraphicalDisplaysAvailable bool

	// DataTrendsAvailable is true when data trend format is
	// available.
	// "Aids in comprehension of time related changes."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:138-140
	DataTrendsAvailable bool

	// PolarDisplaysAvailable is true when polar graphical
	// format is available.
	// "Shape changes indicate abnormal situations."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_151.txt:142-145
	PolarDisplaysAvailable bool

	// SPCSActive is true when the Safety Parameter Display
	// System is active and displaying overall plant safety
	// status.
	// "Safety Parameter Display System – Presents a succinct
	// display account of the overall plant safety status."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	SPCSActive bool

	// PostTripReviewPrinted is true when the post-trip review
	// has been printed.
	// "Post-trip review – Prints a history of parameter
	// values before and after a trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:1-3
	PostTripReviewPrinted bool

	// SequenceOfEventsRecording is true when sequence of
	// events recording is active.
	// "Sequence of Events Recording – Records the sequence
	// of events during a transient."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:5-7
	SequenceOfEventsRecording bool

	// AlarmActive is true when the computer has detected a
	// parameter exceeding its limit.
	// "Alarming – Checks for individual parameters exceeding
	// limits."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:32-33
	AlarmActive bool

	// BypassedInoperableStatusIndication is true when the
	// Bypassed and Inoperable Status Indication display is
	// active.
	// "Bypassed and Inoperable Status Indication (BISI) –
	// Indicates which pieces of equipment have been
	// bypassed or are inoperable."
	// specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:9-12
	BypassedInoperableStatusIndication bool
}

// GeneratorExcitationSystem models the Westinghouse Mark III
// brushless exciter and its Type WTA-300 voltage regulator
// (chapter 15.2). The simplified model collapses the rotating
// rectifier bridge to a dimensionless ExciterOutput (0..1)
// that scales GeneratorRatedVoltage into the actual terminal
// voltage. It does not model the regulator's solid-state
// transfer function or individual rectifier diodes.
//
// Spec justification:
//   - Exciter type: "Excitation: Shaft-driven, air-cooled
//     brushless exciter of the rotating rectifier type."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
//   - All high-power components on the shaft: "all high-power
//     components are mounted on the shaft. This eliminates the
//     need for troublesome carbon brushes."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:64-69
//   - Voltage regulator: "The Type WTA-300 voltage regulating
//     system is of the latest design incorporating all solid
//     state circuitry."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:5-7
//   - Rectifier output: "The exciter output is thus rectified
//     and the resultant direct current is carried by rotating
//     components on the shaft to the main generator field
//     winding."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:21-26
//   - Pilot exciter: "The high-frequency permanent magnet
//     generator provides power to the automatic voltage
//     regulator which regulates and controls the output of the
//     exciter."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:27-30
//   - Brushless-exciter parts: "consists of three basic parts:
//     a permanent magnet pilot exciter, a main ac exciter, and
//     a rectifier wheel."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:14-17
type GeneratorExcitationSystem struct {
	Part

	// ExciterOutput is the exciter DC output fraction (0..1)
	// scaling GeneratorRatedVoltage into the field voltage
	// applied to the main generator rotor winding.
	ExciterOutput float64

	// ExciterActive is true when the exciter is producing
	// field current. Cannot be active when the shaft is not
	// turning (the brushless exciter is shaft-driven).
	ExciterActive bool

	// RegulatorVoltageSetpoint is the voltage the WTA-300
	// regulates toward. Fixed at GeneratorRatedVoltage for
	// the simplified model; a real DEHM would fold in
	// power-factor correction and VAR demand.
	RegulatorVoltageSetpoint float64

	// PilotExciterFrequency is the high-frequency permanent-
	// magnet pilot-exciter output frequency, proportional to
	// shaft speed. Diagnostic indicator only; not load-bearing
	// for the simplified model.
	// "The high-frequency permanent magnet generator provides
	// power to the automatic voltage regulator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:27-30
	PilotExciterFrequency float64
}

// GeneratorCoolingSystem models the dual independent cooling
// circuits for the main generator (chapter 15.2):
//
//  1. Hydrogen gas circulated by a shaft-driven blower
//     through four hydrogen coolers.
//  2. Stator coil cooling water (SCCW) closed demineralized-
//     water loop with redundant pumps and coolers.
//
// The simplified model tracks flow availability and per-cooler
// effectiveness; it does not model H₂ circulation kinetics,
// rotor hot-spot temperatures, or SCCW chemistry.
//
// Spec justification:
//   - H₂ circulation: "The hydrogen gas is circulated by a
//     blower mounted on the generator rotor."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:27-28
//   - Four H₂ coolers: "Four hydrogen coolers are mounted
//     horizontally at the top of the generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:46-47
//   - Two sections per cooler: "The cooler consists of two
//     separate sections to permit temporary operation at
//     reduced load with one section out of service."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:47-49
//   - SCCW: "Demineralized cooling water is provided for the
//     stator by a closed recirculating water system."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
//   - SCCW redundancy: "Redundancy is provided in the water
//     pumps, water coolers, and the alarm system so that safe,
//     reliable operation is assured under all conditions."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:55-61
type GeneratorCoolingSystem struct {
	Part

	// HydrogenCoolerEffectiveness is the per-cooler heat-
	// rejection effectiveness (0..1) for each of the four
	// hydrogen coolers. A value of 0 means the cooler is
	// fully degraded; 1.0 means design heat rejection.
	// Four independent values support the spec's two-section
	// per cooler redundancy.
	// "Four hydrogen coolers are mounted horizontally at
	// the top of the generator. The cooler consists of two
	// separate sections to permit temporary operation at
	// reduced load with one section out of service."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:46-49
	HydrogenCoolerEffectiveness [4]float64

	// SCCWFlow is the demineralized water flow rate through
	// the stator coil cooling water circuit (gal/s). Zero
	// means the SCCW pump is not running; non-zero indicates
	// the cooling circuit is active.
	SCCWFlow float64

	// SCCWPumpRunning is true when at least one SCCW pump is
	// running. The spec describes redundant pumps; this flag
	// represents at least one pump in service.
	// "Redundancy is provided in the water pumps, water
	// coolers, and the alarm system."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:55-57
	SCCWPumpRunning bool

	// HydrogenPressure is the containment pressure of the
	// hydrogen gas inside the generator enclosure (psig).
	// The spec quotes 60 or 75 psig as design values.
	// "Hydrogen pressure: 60 or 75 psig"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:20
	HydrogenPressure float64
}

// CirculatingWaterSystem models the Circulating Water System
// (CWS) — the direct coolant for the main condenser tube
// bundle. The simplified model tracks pump-running count,
// total flow, source inlet temperature, and outlet
// temperature after the condenser. It does not model intake
// house hydraulics, ice-melting recirculation, or detailed
// cooling-tower fill dynamics.
//
// Spec justification:
//   - CWS mission: "major heat sink for the power plant by
//     providing large amounts of cooling water for the main
//     condensers."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
//   - Pump type: "vertical, mixed flow, volute type
//     circulating water pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:56-58
//   - Source diversity: "Depending on the plant, the source of
//     circulating water may be a river, lake, ocean, or
//     cooling tower."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
//   - Condenser coupling: "The condenser is usually a
//     single-pass deaerating type with straight-through tubes."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-20
type CirculatingWaterSystem struct {
	Part

	// PumpsInService is the number of CW pumps currently
	// running. The simplified model treats this as the count
	// of pumps that the operator has selected online; it
	// does not model individual pump failures.
	PumpsInService int

	// FlowGPM is the total circulating water flow rate
	// (gal/min) delivered to the main condenser tube bundle.
	// Computed as PumpsInService ×
	// CirculatingWaterPumpFlowGPM.
	FlowGPM float64

	// InletTemp is the CW temperature entering the condenser
	// tubes (°F). Set from the cooling source
	// (river/lake/ocean) or cooling-tower cold-water basin.
	InletTemp Temperature

	// OutletTemp is the CW temperature leaving the condenser
	// tubes (°F), elevated by the latent heat of
	// condensation. The condenser Step writes this value from
	// the heat-balance calculation.
	OutletTemp Temperature

	// SourceIsCoolingTower is true when the CW source is a
	// cooling tower rather than a once-through
	// river/lake/ocean intake. When true, InletTemp is the
	// cooling-tower cold-basin temperature
	// (wet-bulb + ApproachF).
	SourceIsCoolingTower bool
}

// PlantOperationalMode represents the current operational mode of the
// plant. The simplified model collapses the full startup/shutdown
// procedure into three modes linked by condition-based transitions.
//
// Spec justification:
//   - Cold vs. hot startup: "Startup from a cold condition is initiated
//     following a shutdown that required the Reactor Coolant System
//     (RCS) to be cooled down and depressurized ... Startup from a hot
//     condition refers to a return to power operation following a
//     shutdown not requiring depressurization."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
//   - Hot standby definition: "the plant is in the hot standby
//     condition" when primary temperature is maintained through RCP
//     heat and steam dump, at saturation pressure for Tavg.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:9-16
//   - Power operation: following the loading sequence described in
//     normal operation.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:135-150
type PlantOperationalMode int

const (
	// ColdShutdown — RCS <140°F, <400 psig, pressurizer
	// water-solid. No criticality, RCPs off, RHR may be in
	// service.
	ColdShutdown PlantOperationalMode = iota

	// HotStandby — RCS 547–557°F, 2250 psia, no-load
	// pressurizer level, steam bubble present. Control rods
	// inserted. Max 2 of 4 RCPs running. RHRS may be in
	// service.
	HotStandby

	// PowerOperation — plant loaded above 15% of rated,
	// turbine generator online and producing power. Full
	// control systems in automatic.
	PowerOperation
)

// LeadLagMode selects which demand the operator drives directly
// during power maneuvering: the turbine load setpoint or the reactor
// power setpoint.
type LeadLagMode int

const (
	// TurbineLead — load changes are made on the turbine-
	// generator with the NSSS following steam demand; zero value
	// preserves the manual lineup the plant ran before the RPC
	// layer existed.
	//   "Load changes ... are initially made on the turbine-
	//    generator. The control system of the NSSS then adjusts
	//    the power level of the reactor to follow the power
	//    demand of the turbine."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_230.txt:126-129
	TurbineLead LeadLagMode = iota

	// ReactorLead — the operator drives the reactor power
	// setpoint and the turbine load demand follows achieved
	// power (the mirrored coordination of the citation above).
	ReactorLead
)

// Plant Operation design constants (plans/p32_1.txt).
//
// Spec justification:
//   - Loading rate: "5 percent of full power per minute."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:112
//   - Heatup/cooldown rate: "50°F (28°C) per hour."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:153-154
//   - Auxiliary to main feedwater: "about 3.5-percent steam flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:129-130
//   - Hot standby RCP limit: "maximum of two out of four reactor
//     coolant pumps may be secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-105
//   - Startup times: cold = 13 h, hot = 4 h (Table 16-1 totals).
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_223.txt:191
//   - Cooldown time: "approximately 20 hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:59-61
//   - Load follow operating range: 15%–100% of rated power.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_230.txt:62-65
const (
	// OpPowerRaiseRate is the maximum fraction of rated power
	// per second the plant may raise power during startup or
	// load follow. Spec: "5 percent of full power per
	// minute."
	//   per minute → per second: 0.05 / 60 = 0.0008333...
	OpPowerRaiseRate = 0.05 / 60.0

	// OpHeatupRateLimit is the maximum RCS temperature
	// increase rate during heatup (°F/s). Spec: "50°F (28°C)
	// per hour."
	//   50 / 3600 = 0.01389 °F/s.
	OpHeatupRateLimit = 50.0 / 3600.0

	// OpCooldownRateLimit is the maximum RCS temperature
	// decrease rate during cooldown (°F/s). Spec: "50°F/hr
	// (28°C/hr)."
	OpCooldownRateLimit = -OpHeatupRateLimit

	// SecondaryImbalanceTau is the first-order filter time constant
	// (seconds) for the signed SG draw-versus-supplied-heat
	// imbalance before rectification into primary cooling.
	// Assumption; the source is silent about bookkeeping dynamics.
	// Sized above control-loop lag noise, far below cooldown legs.
	SecondaryImbalanceTau = 20.0

	// OpAuxToMainFeedwaterThreshold is the per-SG steam flow
	// fraction above which the feedwater supply switches from
	// auxiliary to main. Spec: "about 3.5-percent steam flow."
	OpAuxToMainFeedwaterThreshold = 0.035

	// StepbackPowerFraction is the neutron power level the plant
	// control system runs back to on a turbine trip before holding,
	// preserving xenon reactivity margin for a full-power recovery.
	// "The reactor neutron power will be reduced quickly to 60 % by
	// rapid insertion of control rods - this is known as reactor
	// power 'stepback'."
	//   specs/pwr_sim/chapter_3/page_041.txt:28-29
	StepbackPowerFraction = 0.60

	// StepbackHoldBand is the fraction of rated fission power around
	// the stepback target inside which the controller stops driving
	// rods in and holds measured reactivity at zero instead.
	// Assumption; the source describes the target level but not a
	// hysteresis band.
	StepbackHoldBand = 0.02

	// StepbackControlGain scales the stepback controller's
	// reactivity tracking: rod demand moves at the full mechanical
	// drive speed at a 0.1 percent delta-k/k error between measured
	// reactivity and the approach bias. Assumption; the source gives
	// no controller tuning.
	StepbackControlGain = 0.5

	// StepbackApproachBias is the subcritical offset (percent
	// delta-k/k) the controller commands per unit of remaining power
	// descent: it kicks the fold off at full drive speed and shrinks
	// toward zero as neutron power nears the park, so no stored rod
	// worth can carry the core past the target. Assumption; sized to
	// the pinned-full-power reactivity excess.
	StepbackApproachBias = 0.12

	// StepbackMaxHold caps how long the automatic stepback holds the
	// park before handing back to the operator: xenon growth on the
	// model's compressed timescale eventually exhausts rod trim
	// authority (gap A2 of plans/x/scenario/15.txt).
	// Assumption.
	StepbackMaxHold = 180 * time.Second

	// StepbackInitiationFlowFrac is the forced-flow fraction below
	// which a degraded-coolant-flow condition arms the stepback in
	// addition to the turbine-trip trigger: "The loss of one RC pump
	// will immediately initiate reactor power stepback. As the
	// coolant flow for one loop is decreasing rapidly, the low
	// coolant flow in that loop will exceed the trip setpoint and
	// will scram the reactor."
	//   specs/pwr_sim/chapter_4/page_051.txt:44-47
	// Assumption; the source sequences the runback ahead of the trip
	// but gives no initiation threshold, so the value sits above the
	// low-flow trip setpoint (RPSLowFlowTripFraction) to preserve
	// that ordering.
	StepbackInitiationFlowFrac = 0.93

	// StepbackXenonBoronGain is the soluble-boron dilution (ppm)
	// that offsets one unit of xenon-135 growth above its rated-power
	// equilibrium: xenon worth -2.7 pct per unit Xe (core/reactor.go
	// CoreXenonEquilibriumWorthPct) divided by boron worth -0.05 pct
	// per 100 ppm (core/parts.go CoreBoronWorthPer100ppm).
	StepbackXenonBoronGain = 5400.0

	// StepbackBoronFloorPpm keeps some soluble shim in hand during a
	// long stepback hold. Assumption.
	StepbackBoronFloorPpm = 600.0

	// OpHotStandbyMaxRCPs is the maximum number of reactor
	// coolant pumps permitted to run in HotStandby mode before
	// RHRS is in service. Spec: "maximum of two out of four
	// reactor coolant pumps (RCPs) may be secured."
	OpHotStandbyMaxRCPs = 2

	// OpColdStartupTotalHours is the estimated total cold
	// startup time. Spec: 13.0 hours (Table 16-1, cold column
	// total).
	OpColdStartupTotalHours = 13.0

	// OpHotStartupTotalHours is the estimated total hot
	// startup time. Spec: 4.0 hours (hot column total in
	// Table 16-1).
	OpHotStartupTotalHours = 4.0

	// OpCooldownTotalHours is the estimated plant cooldown
	// time. Spec: "approximately 20 hours."
	OpCooldownTotalHours = 20.0

	// OpPowerOperationMinFraction is the minimum reactor
	// power fraction for PowerOperation mode. Spec: "15
	// percent."
	OpPowerOperationMinFraction = 0.15

	// OpMaxRCPs is the total number of reactor coolant pumps
	// in the four-loop plant.
	OpMaxRCPs = 4
)

// Reactor power control (RPC) constants.
const (
	// RodControlMaxRatePerSec caps the operator-entered ramp
	// rate at 0.8 % of full power per second (0.008 fraction/s).
	//   "limited by the maximum rate of 0.8 % of full power per
	//    second"
	//   specs/pwr_sim/chapter_2/page_024.txt:3-5
	RodControlMaxRatePerSec float64 = 0.008

	// RodControlDeadbandF is the first deadband around Tref,
	// 0.5 °C expressed in Fahrenheit (0.5 * 9/5). Inside it the
	// rods hold; outside it they trim one bank's worth of speed
	// toward the reference.
	//   regions defined with -0.5 < DT < 0.5
	//   (specs/pwr_sim/chapter_2/page_021.txt:13-16);
	//   withdrawal below Tref, insertion above it
	//   (specs/pwr_sim/chapter_2/page_022.txt:6-10)
	RodControlDeadbandF float64 = 0.9

	// RodControlSecondDeadbandF is the second deadband, 1 °C in
	// Fahrenheit (1.8 F). Past it two banks move at once, modeled
	// as a doubled trim rate.
	//   specs/pwr_sim/chapter_2/page_022.txt:24-27
	RodControlSecondDeadbandF float64 = 1.8

	// RodControlTrimRatePerSec is the base rate at which the RPC
	// slews rod demand toward its computed target, doubled past the
	// second deadband (two banks at once). It is a small fraction
	// of the mechanical drive speed for two reasons: the gray banks
	// are fine-worth regulators moved one at a time at a controlled
	// rate
	//   "gray rods would be withdrawn, one bank at a time"
	//   specs/pwr_sim/chapter_2/page_022.txt:8-9;
	//   bank worths -6.25/-5/-3/-1.75 mk
	//   specs/pwr_sim/chapter_2/page_022.txt:15-17
	// and the modeled power-defect chain rescales fission power
	// every step with no delayed-neutron smoothing, so the trim
	// must inject reactivity no faster than the coolant-
	// temperature feedback can absorb.
	RodControlTrimRatePerSec float64 = 1e-4

	// RodControlSteerOffsetInsertion is the rod-demand offset a
	// full-scale deadband error commands on top of the reactivity-
	// nulling position. One deadband width maps to 0.9/63 = 0.0143
	// points of rated power through the Tref program span; at the
	// ~2.8e-5 travel per point critical-line slope of the lumped
	// ABCD bank this is ~4e-5 of insertion - about one plateau-to-
	// plateau increment, so sustained deviations steer power back
	// onto the program while nulling keeps the chain critical.
	RodControlSteerOffsetInsertion float64 = 4e-5
)

// PlantOperation models the Chapter 16 operational layer that sits on
// top of the physics models and enforces rate limits, mode
// transitions, and gating conditions described in the startup,
// normal-operation, shutdown, and load-follow sections of the spec.
//
// Spec justification:
//   - Startup sequence: precritical checks, CVCS/RCS fill, CCWS in
//     service, pressurizer heatup, letdown flow, RCP start,
//     criticality, power raise.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:76-116
//   - Rate limits: 5%/min loading, 50°F/hr heatup, 50°F/hr cooldown.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:153-154
//   - Aux-to-main feedwater transition: "about 3.5-percent steam
//     flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:129-130
//   - Hot standby RCP limit: "maximum of two out of four reactor
//     coolant pumps may be secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-105
//   - Load follow: 5%/min ramp, 10% step, 12-3-6-3 cycle, 15%–100%
//     operating range.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_230.txt:62-87
//   - Full load rejection: "capable of accepting complete load
//     rejection from the maximum rated power level."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:50-53
type PlantOperation struct {
	Part

	// Mode is the current operational mode. Transitions are
	// gated by operator actions and by physics conditions
	// (temperature, pressure, criticality).
	Mode PlantOperationalMode

	// StartupPhase tracks which phase of the cold or hot
	// startup sequence has been completed. Advances as the
	// operator satisfies gating conditions. Simplified model
	// uses a float so that phases can be partially completed
	// by clock time or condition progress; the set of
	// recognized phases is the StartupPhase enum.
	StartupPhase float64

	// RampingUp is true when the plant is currently ramping
	// power upward at the 5%/min design rate. The
	// PlantOperation step enforces the rate limit when this
	// flag is true.
	RampingUp bool

	// RampingDown is true when the plant is currently ramping
	// power downward (shutdown or load rejection).
	// Rate-limited by the 50°F/hr cooldown equivalent on
	// power, or directly by the cooldown rate on temperature.
	RampingDown bool

	// AuxFeedwaterInUse is true when the steam generators are
	// being supplied by the auxiliary feedwater system (EFWS,
	// p17). Switches to false when per-SG steam flow exceeds
	// OpAuxToMainFeedwaterThreshold.
	AuxFeedwaterInUse bool

	// HotStandbyRCPLimitActive is true when HotStandby mode
	// is active and RHRS is not yet in service, capping the
	// number of running RCPs at two.
	HotStandbyRCPLimitActive bool

	// LoadFollowActive is true when the plant is in
	// load-follow mode (automatically tracking a varying load
	// demand).
	LoadFollowActive bool

	// TavgRateLimited is the RCS average coolant temperature
	// change rate after applying the 50°F/hr heatup/cooldown
	// limit (°F/s). Positive when heating, negative when
	// cooling.
	TavgRateLimited float64

	// LeadLagMode selects which demand the operator drives
	// directly. TurbineLead preserves the manual lineup: turbine
	// Setpoint writes are operator actions and rods are manual
	// unless RodControlAuto engages the reactor power control.
	// ReactorLead gives the operator the power setpoint and rate;
	// the turbine demand follows achieved power.
	//   "MODE will indicate whether the reactor is under TURBINE
	//    LEADING to REACTOR LEADING control, this status can also
	//    be changed here."
	//   specs/pwr_sim/chapter_2/page_023.txt:27-28
	LeadLagMode LeadLagMode

	// RodControlAuto engages the RPC rod drive. While false,
	// manual DemandInsertion writes remain the single rod
	// authority.
	RodControlAuto bool

	// PowerSetpointDemand is the operator's target reactor power
	// (fraction of rated fission power); PowerRateDemand is the
	// entered ramp rate (fraction of rated per second), clamped
	// to RodControlMaxRatePerSec.
	//   "Enter 'reactor power SP target' = 90 %; enter 'power
	//    rate' = 0.08 %/sec"
	//   specs/pwr_sim/chapter_3/page_036.txt:37-40
	PowerSetpointDemand float64
	PowerRateDemand     float64

	// DemandedPowerSetpoint is the integrator output the Tref
	// program reads. Derived observables (Tref, power error) are
	// instantaneous aggregates of stored state, computed on read,
	// not stored.
	DemandedPowerSetpoint float64
	// StepbackActive latches while the plant control system is
	// holding the reactor-power stepback that follows a turbine
	// trip: neutron power run back to StepbackPowerFraction by
	// rod insertion with xenon margin preserved for recovery.
	// While latched the controller owns ControlRods.DemandInsertion;
	// the latch releases when the turbine returns to Running or a
	// reactor trip supersedes it.
	//   "the plant control system is designed with the following
	//    control actions: The reactor neutron power will be reduced
	//    quickly to 60 % by rapid insertion of control rods"
	//   specs/pwr_sim/chapter_3/page_041.txt:24-29
	StepbackActive bool

	// StepbackDemand is the rod insertion demand the active
	// stepback controller holds (normalised 0..1).
	StepbackDemand float64

	// StepbackFailed models the inserted malfunction that impairs
	// the plant control system's power-reduction response: while set,
	// no new stepback can be initiated and the shutdown system alone
	// answers the process conditions.
	//   "This malfunction event impairs the first line of protective
	//    action initiated by the reactor control system ... However,
	//    the reactor shutdown system (SDS) is always poised to act,
	//    should those alarm limits reach the trip setpoint."
	//   specs/pwr_sim/chapter_4/page_051.txt:5-9
	StepbackFailed bool

	// StepbackFlowArmed is the runback's flow latch: it sets once
	// forced flow has reached StepbackInitiationFlowFrac and stays
	// set, so degraded-flow arming answers a loss of established flow
	// rather than a pump startup ramp passing through the band on its
	// way up. Modeling convention paired with the RPS flow latch
	// (same page_051.txt:44-47 sequence).
	StepbackFlowArmed bool

	// StepbackCVCSDilute requests the CVCS dilution mode while the
	// stepback is latched, so soluble-boron dilution carries the
	// post-stepback xenon buildup instead of rod margin.
	StepbackCVCSDilute bool

	// StepbackElapsed accumulates hold time while the stepback is
	// latched; the controller hands back to the operator at
	// StepbackMaxHold.
	StepbackElapsed time.Duration
}

// Stepback latches and drives the automatic turbine-trip runback.
// Arming requires a tripped turbine with the core still critical
// above the target level; a reactor trip supersedes the runback, and
// the latch releases once the turbine returns to service so operator
// demand regains the rods. While active the controller drives measured
// reactivity toward a shrinking subcritical bias: rod motion answers
// immediately while neutron power lags on the point-kinetics
// multiplier, so reactivity tracking folds the core onto the park
// without the stored-worth overshoot a power-error integral produces.
// Xenon growth after the power cut is carried by soluble-boron dilution
// (the load-follow compensation duty) so rod margin is never the
// binding limit during the hold.
// Stepback latches and drives the automatic reactor-power runback.
// Arming requires a degraded first-line control trigger - a tripped
// turbine or forced flow fallen below the initiation band - with the
// core still critical above the target level; a reactor trip supersedes
// the runback, and the latch releases once both triggers clear so
// operator demand regains the rods. While active the controller drives
// measured reactivity toward a shrinking subcritical bias: rod motion
// answers immediately while neutron power lags on the point-kinetics
// multiplier, so reactivity tracking folds the core onto the park
// without the stored-worth overshoot a power-error integral produces.
// Xenon growth after the power cut is carried by soluble-boron dilution
// (the load-follow compensation duty) so rod margin is never the
// binding limit during the hold. The inserted stepback-failure
// malfunction blocks new arming, leaving the shutdown system as the
// remaining protection ("the reactor shutdown system (SDS) is always
// poised to act", specs/pwr_sim/chapter_4/page_051.txt:8-9); the flow
// trigger models the pump-trip response "The loss of one RC pump will
// immediately initiate reactor power stepback"
// (specs/pwr_sim/chapter_4/page_051.txt:44-46).
func (po PlantOperation) Stepback(dt time.Duration, tstate TurbineState, flowFrac float64, rpv ReactorPressureVessel) PlantOperation {
	ret := po
	if rpv.Trip {
		ret.StepbackActive = false
		return ret
	}
	// Degraded established flow only: the latch sets once forced flow
	// has reached the initiation band (so a pump startup ramp passing
	// through the band on its way up cannot arm the runback) and stays
	// set - a loss of established flow then arms and HOLDS the runback
	// for the episode. Zero flow is natural circulation (never armed),
	// mirroring the low-flow trip's latch in rps.go.
	flowDegraded := ret.StepbackFlowArmed &&
		flowFrac < StepbackInitiationFlowFrac
	if flowFrac >= StepbackInitiationFlowFrac {
		ret.StepbackFlowArmed = true
	}
	turbineTripped := tstate == TurbineTripped
	if !ret.StepbackActive {
		if ret.StepbackFailed || (!turbineTripped && !flowDegraded) ||
			float64(rpv.FissionPower) <=
				StepbackPowerFraction*float64(CoreFissionPowerAtRated) {
			return ret
		}
		ret.StepbackActive = true
		ret.StepbackDemand = rpv.ControlRods.Insertion + RodInsertionSpeed
	} else if !turbineTripped && !flowDegraded {
		// Both triggers consumed: the trip latch is cleared (State
		// Off) or the machine is back in service (Running) and flow
		// has recovered - the operator regains rod demand.
		ret.StepbackActive = false
		return ret
	}
	errFrac := (float64(rpv.FissionPower) -
		StepbackPowerFraction*float64(CoreFissionPowerAtRated)) /
		float64(CoreFissionPowerAtRated)
	// Track measured reactivity toward a shrinking subcritical bias
	// instead of raw power: the point-kinetics multiplier answers
	// sustained subcriticality within about a second, so driving rod
	// demand from the power error itself dives far past the park
	// before the neutron population responds. Rod motion moves
	// reactivity immediately, giving the loop an honest signal, and
	// the bias vanishing at the target leaves no stored worth to
	// overshoot with.
	//   "Insertion demand re-zeroing MEASURED reactivity"
	//   specs/pwr_sim/chapter_3/page_041.txt:27-35
	rhoTarget := -StepbackApproachBias * errFrac
	rate := max(min(StepbackControlGain*(float64(rpv.Reactivity)-rhoTarget),
		RodInsertionSpeed), -RodInsertionSpeed)
	demand := ret.StepbackDemand + rate*dt.Seconds()
	ret.StepbackDemand = min(max(demand, 0), 1)
	// Dilute for the whole hold: at fresh-fuel chemistry a part-load
	// park does not self-arrest (the fold would otherwise sink toward
	// the neutron-source floor), and xenon growth after the power cut
	// only adds to the deficit. This is the same remedy the
	// load-rejection scenario applies by hand.
	//   "Dilute to maneuvering chemistry so the fold self-arrests"
	//   core/scenario17_test.go (93fa73c); plans/x/scenario/15.txt gap A2
	ret.StepbackCVCSDilute = true
	ret.StepbackElapsed += dt
	if ret.StepbackElapsed >= StepbackMaxHold {
		// Xenon growth on the model's compressed timescale eventually
		// exhausts rod trim authority (gap A2); hand back to the
		// operator rather than let the core sink to the source floor.
		ret.StepbackActive = false
	}
	return ret
}

// Step advances the PlantOperation layer for one tick. It enforces
// mode-dependent gating (RCP cap, aux/main feedwater path, ramping
// flags) and the 5%/min power-raise limit that downstream turbines,
// rod-control, and steam-dump valve logic respect.
//
// Spec justification:
//   - Mode-dependent RCP cap: "a maximum of two out of four reactor
//     coolant pumps (RCPs) may be secured. Proper loop flow and
//     desired loop temperatures are maintained with the other RCPs
//     until the Residual Heat Removal System (RHRS) is in service."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-108
//   - 5%/min raise: "plant loading is continued to full power at a
//     rate not to exceed 5 percent of full power per minute."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
//   - Aux-to-main switchover: "Before reaching the flow capacity of
//     the auxiliary feedwater system, equivalent to about 3.5-percent
//     steam flow, the steam generators feedwater supply is switched
//     to the main feedwater system."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:129-133
func (po *PlantOperation) Step(dt time.Duration,
	mode PlantOperationalMode,
	rcsTemp Temperature,
	rcsPressure Pressure,
	reactorPower Watts,
	ratedPower Watts,
	rhsInService bool,
	runningRCPCount int,
	steamFlowFraction float64,
	targetLoadFraction float64,
) PlantOperation {
	ret := *po
	ret.Mode = mode

	switch ret.Mode {
	case ColdShutdown:
		ret.RampingUp = false
		ret.RampingDown = false
		ret.LoadFollowActive = false
		ret.HotStandbyRCPLimitActive = false
		ret.AuxFeedwaterInUse = true
		ret.StartupPhase = 0
		ret.TavgRateLimited = 0

	case HotStandby:
		ret.RampingUp = false
		ret.RampingDown = false
		ret.LoadFollowActive = false
		ret.HotStandbyRCPLimitActive = !rhsInService
		ret.AuxFeedwaterInUse = true
		ret.TavgRateLimited = 0

	case PowerOperation:
		ret.HotStandbyRCPLimitActive = false
		ret.AuxFeedwaterInUse =
			steamFlowFraction < OpAuxToMainFeedwaterThreshold
		// TavgRateLimited is the per-second temperature
		// change rate the plant is requesting, scaled by the
		// normalised power demand delta. The sign tracks
		// heatup (positive) vs. cooldown (negative); the
		// magnitude is clamped to OpHeatupRateLimit in both
		// directions.
		//
		// Spec: "Primary plant heatup rate is limited to
		// 50°F (28°C) per hour."
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
		normalisedPowerDelta := targetLoadFraction -
			float64(reactorPower)/float64(ratedPower)
		ret.TavgRateLimited = ClampRate(
			0,
			normalisedPowerDelta*OpHeatupRateLimit,
			OpHeatupRateLimit,
			OpCooldownRateLimit,
			dt,
		)
		powerDemandDelta := targetLoadFraction*float64(ratedPower) -
			float64(reactorPower)
		absRate := OpPowerRaiseRate * float64(ratedPower) *
			dt.Seconds()
		switch {
		case powerDemandDelta > absRate:
			ret.RampingUp = true
			ret.RampingDown = false
			reactorPower += Watts(absRate)
		case powerDemandDelta < -absRate:
			ret.RampingUp = false
			ret.RampingDown = true
			reactorPower += Watts(-absRate)
		default:
			ret.RampingUp = false
			ret.RampingDown = false
			reactorPower += Watts(powerDemandDelta)
		}
		ret.StartupPhase = 100 // fully raised
	}

	return ret
}

// fissionFraction normalizes fission power against its rated value,
// clamped to [0, 1].
func fissionFraction(fissionPower Watts) float64 {
	return min(max(float64(fissionPower)/
		float64(CoreFissionPowerAtRated), 0), 1)
}

// referenceTempFor evaluates the Tref-versus-power characteristic at
// an explicit demand. The anchors are the same full-load coolant
// temperatures the core thermal model relaxes toward, so the
// controller's reference is the plant's own temperature program by
// construction.
//
//	"From the DEMANDED POWER SETPOINT, a reference reactor coolant
//	 temperature (TREF) is obtained from the 'Tref versus power'
//	 characteristic curve."
//	specs/pwr_sim/chapter_2/page_024.txt:13-14
//	specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
func (po *PlantOperation) referenceTempFor(demand float64) float64 {
	return CoreReferenceInletF +
		(CoreReferenceOutletF-CoreReferenceInletF)*demand
}

func (po *PlantOperation) referenceTempF() float64 {
	return po.referenceTempFor(po.DemandedPowerSetpoint)
}

// boronDemandPower is the demand the auto-boron comparison keys on:
// the walked integrator under reactor-leading RPC, otherwise the
// turbine's own setpoint. The integrator shadows achieved power in
// turbine lead and would zero the demanded-minus-achieved tier.
// "The rod control system automatically adjusts the power level of
// the reactor to match the power demanded by the turbine."
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:69-71
func (po *PlantOperation) boronDemandPower(turbineSetpoint float64) float64 {
	if po.LeadLagMode == ReactorLead {
		return po.DemandedPowerSetpoint
	}
	return turbineSetpoint
}

// StepRPC advances the reactor power control (RPC) layer for one
// tick. The demanded power setpoint integrates toward the operator's
// target at the entered rate; the Tref program converts it to a
// reference coolant temperature; and when auto rod control is
// engaged, rod demand is steered so outlet temperature tracks the
// reference within the deadband.
//
// The rod drive is a supervisory servo. Its base target is the
// insertion that nulls measured reactivity - the modeled
// power-defect chain rescales fission power every step with no
// delayed-neutron smoothing, so holding any point of the program
// requires continuous reactivity balance, not motionless rods. On
// top of that, the deadband error biases the target: Tavg below
// Tref beyond the deadband withdraws (region B), above it inserts
// (region D), and past the second deadband the slew rate doubles
// (two banks at once).
//
// Spec justification:
//   - Rate limit and demanded-setpoint ramp:
//     specs/pwr_sim/chapter_2/page_024.txt:3-12
//   - Tref curve and dT comparison:
//     specs/pwr_sim/chapter_2/page_024.txt:13-16
//   - Deadband rod direction and second-deadband doubled speed:
//     specs/pwr_sim/chapter_2/page_022.txt:6-10, :24-27
//   - RPC runs in reactor-leading mode:
//     specs/pwr_sim/chapter_2/page_023.txt:11-14
func (po *PlantOperation) StepRPC(rpv *ReactorPressureVessel,
	turbine *Turbine, dt time.Duration) {

	pf := fissionFraction(rpv.FissionPower)
	if !po.RodControlAuto {
		// Bumpless: the integrator shadows measured power so
		// engaging auto later introduces no reactivity step.
		po.DemandedPowerSetpoint = pf
		return
	}

	switch po.LeadLagMode {
	case ReactorLead:
		rate := min(max(po.PowerRateDemand, 0), RodControlMaxRatePerSec)
		delta := min(max(po.PowerSetpointDemand, 0), 1) -
			po.DemandedPowerSetpoint
		maxDelta := rate * dt.Seconds()
		po.DemandedPowerSetpoint = min(max(
			po.DemandedPowerSetpoint+
				min(max(delta, -maxDelta), maxDelta), 0), 1)
		// The turbine follows the reactor's walked demand: the
		// ACTUAL (load) setpoint equals the accepted reactor power
		// setpoint target under reactor-leading RPC, so steam draw
		// tracks the program rather than achieved power - following
		// pf instead would make every fission dip cut steam flow and
		// fold the core further.
		// specs/pwr_sim/chapter_2/page_023.txt:64-71
		turbine.Setpoint = po.DemandedPowerSetpoint
	default:
		// Turbine lead: the demand path is the turbine's own
		// setpoint, so the integrator shadows measured power.
		po.DemandedPowerSetpoint = pf
	}

	if rpv.Trip {
		// A scram owns the rods; the drive stands down until
		// the operator resets and re-engages.
		return
	}

	// The rod loop's reference keys on the demand the plant actually
	// has: the walked integrator under reactor lead, otherwise the
	// turbine's setpoint. Evaluating at the shadowed integrator in
	// turbine lead would zero dT by construction and leave the rods
	// inert while chemistry carried the whole load chase.
	//   "The rod control system automatically adjusts the power level
	//    of the reactor to match the power demanded by the turbine."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:69-71
	dT := rpv.Outlet.Float() -
		po.referenceTempFor(po.boronDemandPower(turbine.Setpoint))
	bias := min(max(dT/RodControlDeadbandF, -1), 1)
	rodWorthPctPerInsertion := -float64(CoreRodWorthABCD) * 100
	target := rpv.ControlRods.DemandInsertion +
		float64(rpv.Reactivity)/rodWorthPctPerInsertion +
		bias*RodControlSteerOffsetInsertion
	rate := RodControlTrimRatePerSec * dt.Seconds()
	if dT > RodControlSecondDeadbandF || dT < -RodControlSecondDeadbandF {
		rate *= 2
	}
	step := min(max(target-rpv.ControlRods.DemandInsertion,
		-rate), rate)
	rpv.ControlRods.DemandInsertion =
		min(max(rpv.ControlRods.DemandInsertion+step, 0), 1)
}

// ClampRate limits the rate of change of a value to a per-second
// ceiling while preserving sign. Used to enforce the 50°F/hr
// heatup/cooldown and the 5%/min load-raise design limits described
// in chapter 16.
//
// Spec justification:
//   - "Primary plant heatup rate is limited to 50°F (28°C) per
//     hour."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
//   - "plant loading is continued to full power at a rate not to
//     exceed 5 percent of full power per minute."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
func ClampRate(current, desiredDelta, posLimit, negLimit float64, dt time.Duration) float64 {
	maxDeltaPos := posLimit * dt.Seconds()
	maxDeltaNeg := negLimit * dt.Seconds()
	delta := desiredDelta
	delta = min(delta, maxDeltaPos)
	delta = max(delta, maxDeltaNeg)
	return delta / dt.Seconds()
}
