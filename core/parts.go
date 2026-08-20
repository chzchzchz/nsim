package core

type Pump struct {
	Running bool
}

type Turbine struct {
	Velocity
}

type Tank struct {
	Level Volume
	Capacity Volume
	Temperature
}

type FuelRods struct {}
type ControlRods struct {}
type Nozzle struct {}

type ReactorPressureVessel struct {
	FuelRods
	ControlRods
	Reactivity float32
	DecayHeat float32
	Inlet Nozzle
	Outlet Nozzle
	Temperature
	Pressure
}

type Pressurizer struct {
	Temperature
	Pressure
	Water		Volume
	Volume
}

type SteamGenerator struct {
	Temperature
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

type PowerGenerator struct {}
type Condenser struct {}
//type Polisher struct {}

// Heats water from 120F to 360F
type LowPressureHeater struct {}
type HighPressureHeater struct {}

type AFWPump struct {
	Pump
	ServiceWaterValve Valve
	CSTValve Valve
}

type DieselEngine struct {
	// speed controlled by discharge pressure
	// and high steam header pressure
	// setpoint is notmally 150 psid
	Speed RPM

	// minimum level is 450 gallons at start of day
	// each tank can support full load for 4 days
	// aux feed pump running for 24 hours
	Fuel Tank
}


type Transformer struct {}

type Pipe struct {
	Pressure
	Velocity
}

type Valve struct {
	Open float64
}
