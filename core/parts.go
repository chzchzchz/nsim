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
	Level Volume
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
}

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
	Water Volume
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

type PowerGenerator struct{ Part }
type Condenser struct{ Part }
type Polisher struct{}

// Heats water from 120F to 360F
type LowPressureHeater struct{ Part }
type HighPressureHeater struct{ Part }

type AFWPump struct {
	Pump
	ServiceWaterValve Valve
	CSTValve          Valve
}

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

	// Operator-controlled between steps. Latched true until the start
	// sequence completes or an explicit operator stop clears it.
	StartRequested bool

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
const (
	// Chapter_11/page_166.txt:50-51 — rated speed reached within 10 s.
	DieselRatedSpeed RPM = 1800
	// Chapter_11/page_166.txt:50 — ready to accept load within 10 s.
	DieselReadyDeadline = 10 * time.Second
	// Chapter_11/page_166.txt:50 — capable of carrying rated load
	// within 30 s.
	DieselRatedLoadDeadline = 30 * time.Second
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
