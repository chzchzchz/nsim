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
	Running bool
}

type Turbine struct {
	Part
	Velocity

	State          TurbineState
	SteamLossTimer time.Duration
	RealPower      Watts
}

// TurbineState is the operating state of the main turbine.
//
// Spec justification:
//   - glossary.md turbine trip: "When turbine steam inlet valves are
//     rapidly closed to block steam flow, stopping the turbine."
//     specs/.../back_matter/page_245.txt:48-50
//   - chapter_15/page_205.txt:28-34 — the auto-stop-oil trip system
//     causes all steam control valves to close on any trip signal.
//   - chapter_15/page_205.txt:40 — "turbine speed does not exceed 120
//     percent of [rated]" — see TurbineOverspeedTripRatio below.
type TurbineState int

const (
	TurbineOff TurbineState = iota
	TurbineRunning
	TurbineTripped
)

// Rated values for the main turbine, sized for a large turbo-generator
// to match the implied megawatt scale of
// specs/.../chapter_15/page_208.txt:21 (24-26 kV main generator).
const (
	TurbineRatedSpeed         Velocity = 1800
	TurbineOverspeedTripRatio float64  = 1.20
	TurbineSteamLossDeadline           = 2 * time.Second
	TurbineRatedkW            Watts    = 1_000_000e3
)

type FuelRods struct{ Part }
type ControlRods struct{ Part }
type Nozzle struct{ Part }

type ReactorPressureVessel struct {
	Part
	FuelRods
	ControlRods
	Reactivity float32
	DecayHeat  float32
	Inlet      Nozzle
	Outlet     Nozzle
	Pressure
}

type Pressurizer struct {
	Part
	Pressure
	Water Volume
	Volume
}

type SteamGenerator struct {
	Part
	Pressure
	Water  Volume
	Volume Volume
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

// PowerGenerator models the electrical side of a generator set. The diesel
// engine step populates these fields; the main turbine's generator
// populates them in its own step (not yet implemented).
//
// Spec justification for the field set:
//   - Voltage / Frequency —
//     "two 6600-volt engineered safety features buses" (diesel output).
//     specs/.../chapter_11/page_165.txt:73-82
//     "Output voltage: 24,000 to 26,000 volts, three-phase, 60 Hz" (main
//     generator reference).
//     specs/.../chapter_15/page_208.txt:21
//   - BreakerClosed —
//     "controls for the governor, voltage regulator, generator breaker,
//     and synchronization for each diesel generator are located on
//     physically separated portions of the main control board".
//     specs/.../chapter_11/page_166.txt:1-4
//   - RealPower — the running load in Watts, sized by RatedkW *
//     RunningFraction so a downstream load sequencer (planned for the
//     future) can sum load against rated capacity.
type PowerGenerator struct {
	Part
	Voltage       float64
	Frequency     float64
	RealPower     Watts
	BreakerClosed bool
}
type Condenser struct {
	Part
	Pressure
}
type Polisher struct{}

// Heats water from 120F to 360F
type LowPressureHeater struct{ Part }
type HighPressureHeater struct{ Part }

// AFWPump is the auxiliary feedwater pump described in chapter 5.2 of
// the spec.
//
// Spec justification for DriveSource / DriveReady:
//   - The EFWS has three diverse power sources for the AFW pumps —
//     "motor driven and turbine driven pumps are located in the
//     emergency feedwater pump building. The use of both motor driven
//     and turbine driven pumps satisfies the requirement that the pumps
//     be powered by diverse power sources."
//     specs/.../chapter_5/page_099.txt:89-93
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
//     specs/.../chapter_11/page_165.txt:28-36
//   - DieselStartManual / DieselStartTest — operator action from the
//     main control board; the spec locates "controls for the governor,
//     voltage regulator, generator breaker, and synchronization for each
//     diesel generator […] on physically separated portions of the main
//     control board".
//     specs/.../chapter_11/page_166.txt:2-4
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
//     specs/.../chapter_5/page_099.txt:89-93
//   - DriveSteam — the turbine-driven AFW pump whose "steam supply line
//     for each turbine driven pump is connected to the main steam line
//     from a steam generator."
//     specs/.../chapter_5/page_099.txt:42-47
//   - DriveElectric — the motor-driven AFW pump fed from the "Safety
//     Class 1E electrical power" trains.
//     specs/.../chapter_5/page_099.txt:71-82
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
	// chapter_11/page_166.txt:50-51 deadlines (10 s ready, 30 s rated).
	CrankStartTime time.Duration
	ReadyTime      time.Duration

	// Rated-load readiness latch (chapter_11/page_166.txt:50).
	ReadyToLoad bool

	// RunningFraction is the fraction of rated load currently served,
	// 0..1. Set by the operator between steps.
	RunningFraction float32

	// Cooling state — driven by ServiceWaterPump.Running
	// (chapter_12/page_169.txt:89).
	CoolantTemp Temperature

	// Generator-side output. See core/parts.go:89 for the PowerGenerator
	// definition; this field is the "generator" half of the diesel
	// generator. Voltage / Frequency are ramped up by the diesel step
	// once rated speed is reached.
	Generator PowerGenerator
}

// DieselState is the operational state of the onsite emergency
// diesel generator (chapter_11/page_165.txt:96-122).
type DieselState int

const (
	// DieselOff is the resting state; Speed == 0.
	DieselOff DieselState = iota
	// DieselCranking is the air-over-piston start transient
	// (chapter_11/page_166.txt:77-78).
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
//     specs/.../chapter_11/page_166.txt:46-51
const (
	DieselRatedSpeed        RPM     = 1800
	DieselReadyDeadline             = 10 * time.Second
	DieselRatedLoadDeadline         = 30 * time.Second
	DieselRatedVoltage      float64 = 6600
	DieselRatedFrequency    float64 = 60
	DieselRatedkW           Watts   = 4000e3
	DieselFieldFlashingTime         = 1 * time.Second
)

// DieselCoolantTripTemp — cooling trip threshold; the spec implies
// Service-Water loss must trip the engine (chapter_12/page_169.txt:89,
// page_171.txt:8-9).
var DieselCoolantTripTemp = u.NewValue(220, u.Fahrenheit)

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
