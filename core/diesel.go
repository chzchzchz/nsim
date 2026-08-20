package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

// Diesel generator per-step tunables (not part of the public state, but
// reasonable defaults and reasonable in-test overrides live here).
var (
	// Cranking acceleration toward DieselRatedSpeed, in RPM/s. Sized so
	// rated speed is reached within chapter_11/page_166.txt:50's 10 s.
	dieselCrankAccel RPM = 300
	// Coasting friction in 1/s once tripped / stopped.
	dieselCoastRate = 3.0
	// Cooling thermal time constant in seconds.
	dieselCoolingTau = 30 * time.Second
	// Cooling rate to ambient (per second of the gap) when no Service
	// Water is flowing.
	dieselAmbientRate = 0.05
	// Fuel burn rate at full load — sized so 500 gal of fuel (the day
	// tank set in core/plant.go:50-54) supports ~4 days of rated load,
	// matching the chapter_11/page_166.txt:36-37 7-day (full tank) hint.
	dieselFuelRate = u.NewValue(500.0/(4*24), u.Gallon) // gal/hr at 1.0 frac
)

// Step advances the diesel generator by TimeStep. The state machine,
// speed ramp, fuel integration, and cooling integration all run on the
// 1 ms plant step defined in core/units.go:16.
func (d *DieselGenerator) Step(swRunning bool) DieselGenerator {
	ret := *d
	dt := TimeStep

	// 1) Transitions.
	switch ret.State {
	case DieselOff:
		if ret.StartRequested && ret.Fuel.Level.Float() > 0 {
			ret.State = DieselCranking
			ret.CrankStartTime = 0
			ret.ReadyTime = 0
			ret.ReadyToLoad = false
		}
	case DieselCranking:
		if !ret.StartRequested && ret.Speed == 0 {
			ret.State = DieselOff
			break
		}
		if ret.Speed >= DieselRatedSpeed {
			ret.State = DieselRunning
		}
		if ret.Fuel.Level.Float() <= 0 {
			ret.State = DieselTripped
		}
	case DieselRunning:
		if !ret.StartRequested {
			ret.State = DieselOff
		}
		if ret.Fuel.Level.Float() <= 0 {
			ret.State = DieselTripped
		}
	case DieselTripped:
		if !ret.StartRequested && ret.Speed == 0 {
			ret.State = DieselOff
		}
	}

	// 2) Speed ramp.
	switch ret.State {
	case DieselCranking:
		ret.Speed += RPM(float64(dieselCrankAccel) * dt.Seconds())
		if ret.Speed > DieselRatedSpeed {
			ret.Speed = DieselRatedSpeed
		}
		ret.CrankStartTime += dt
		ret.RunningFraction = 0
	case DieselRunning:
		ret.Speed = DieselRatedSpeed
		ret.ReadyTime += dt
		// Spec deadline (chapter_11/page_166.txt:50) is "10 seconds after
		// receipt of a start signal", so check the total elapsed time
		// since the start signal latched, not just Running time.
		if !ret.ReadyToLoad && ret.CrankStartTime+ret.ReadyTime >= DieselReadyDeadline {
			ret.ReadyToLoad = true
		}
		// Default the running fraction to 1.0 if the operator didn't
		// pin it lower between steps; the engine still only burns fuel
		// proportional to RunningFraction.
		if ret.RunningFraction <= 0 {
			ret.RunningFraction = 1
		}
	default:
		ret.Speed -= RPM(float64(ret.Speed) * dieselCoastRate * dt.Seconds())
		if ret.Speed < 0 {
			ret.Speed = 0
		}
		ret.RunningFraction = 0
		ret.ReadyToLoad = false
	}

	// 3) Fuel integration — only while running, scaled by RunningFraction.
	if ret.State == DieselRunning {
		burn := float64(dieselFuelRate.Float()) * float64(ret.RunningFraction) * dt.Hours()
		newLevel := ret.Fuel.Level.Float() - burn
		if newLevel < 0 {
			newLevel = 0
		}
		ret.Fuel.Level = u.NewValue(newLevel, u.Gallon)
		if newLevel == 0 {
			ret.State = DieselTripped
		}
	}

	// 4) Cooling integration. Service Water flowing -> approach ambient;
	// Service Water off -> drift upward while running, cool while stopped.
	ambient := ret.Fuel.Part.Temperature.Float() // proxy for ambient
	gap := ret.CoolantTemp.Float() - ambient
	if swRunning {
		ret.CoolantTemp = u.NewValue(ret.CoolantTemp.Float()-gap*(dt.Seconds()/dieselCoolingTau.Seconds()), u.Fahrenheit)
	} else {
		if ret.State == DieselRunning {
			ret.CoolantTemp = u.NewValue(ret.CoolantTemp.Float()+dieselAmbientRate*dt.Seconds()*100, u.Fahrenheit)
		} else {
			ret.CoolantTemp = u.NewValue(ret.CoolantTemp.Float()-gap*(dt.Seconds()/dieselCoolingTau.Seconds()), u.Fahrenheit)
		}
	}

	// 5) Cooling trip.
	if ret.State == DieselRunning && ret.CoolantTemp.Float() >= DieselCoolantTripTemp.Float() {
		ret.State = DieselTripped
	}

	return ret
}
