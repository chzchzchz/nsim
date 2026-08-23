package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

// Diesel generator per-step tunables (not part of the public state, but
// reasonable defaults and reasonable in-test overrides live here).
var (
	// Cranking acceleration toward DieselRatedSpeed, in RPM/s. Sized so
	// rated speed is reached within
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-49's
	// 10 s.
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
	// matching the
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:36-37
	// 7-day (full tank) hint.
	dieselFuelRate = Volume{Val: 500.0/(4*24), Unit: u.Gallon} // gal/hr at 1.0 frac
)

// Step advances the diesel generator by TimeStep. The state machine,
// speed ramp, generator output ramp, fuel integration, and cooling
// integration all run on the 1 ms plant step defined in core/units.go:16.
//
// Spec justification for each block is inline below.
func (d *DieselGenerator) Step(swFlowGPM float64) DieselGenerator {
	ret := *d
	dt := TimeStep

	// 1) State-machine transitions.
	//
	// Any non-None StartSignal causes Off -> Cranking; DieselStartNone
	// returns the engine to Off once speed has decayed.
	switch ret.State {
	case DieselOff:
		if ret.StartSignal != DieselStartNone && ret.Fuel.Level.Float() > 0 {
			ret.State = DieselCranking
			ret.CrankStartTime = 0
			ret.ReadyTime = 0
			ret.ReadyToLoad = false
		}
	case DieselCranking:
		if ret.StartSignal == DieselStartNone && ret.Speed == 0 {
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
		if ret.StartSignal == DieselStartNone {
			ret.State = DieselOff
		}
		if ret.Fuel.Level.Float() <= 0 {
			ret.State = DieselTripped
		}
	case DieselTripped:
		if ret.StartSignal == DieselStartNone && ret.Speed == 0 {
			ret.State = DieselOff
		}
	}

	// 2) Speed ramp.
	switch ret.State {
	case DieselCranking:
		ret.Speed += RPM(float64(dieselCrankAccel) * dt.Seconds())
		ret.Speed = min(ret.Speed, DieselRatedSpeed)
		ret.CrankStartTime += dt
		ret.RunningFraction = 0
	case DieselRunning:
		ret.Speed = DieselRatedSpeed
		ret.ReadyTime += dt
		// Spec deadline
		// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50)
		// is "10 seconds after
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
		ret.Speed = max(ret.Speed, 0)
		ret.RunningFraction = 0
		ret.ReadyToLoad = false
	}

	// 3) Fuel integration — only while running, scaled by RunningFraction.
	if ret.State == DieselRunning {
		burn := float64(dieselFuelRate.Float()) * float64(ret.RunningFraction) * dt.Hours()
		newLevel := ret.Fuel.Level.Float() - burn
		newLevel = max(newLevel, 0)
		ret.Fuel.Level = Volume{Val: newLevel, Unit: u.Gallon}
		if newLevel == 0 {
			ret.State = DieselTripped
		}
	}

	// 4) Cooling integration. Service Water flowing -> approach ambient;
	// Service Water off -> drift upward while running, cool while stopped.
	// The gate is the actual delivered flow in gpm, not the PumpRunning
	// latch: if the ESF breaker is open the pumps cannot develop flow
	// even though PumpRunning was latched, so the jacket cooler goes dry.
	//
	// Spec: "Diesel generator coolers" / "Containment spray pump diesel
	// engine coolers" are Service Water System loads.
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89,
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:84,
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:8-9
	swFlowing := swFlowGPM > 0
	ambient := ret.Fuel.Part.Temperature.Float() // proxy for ambient
	gap := ret.CoolantTemp.Float() - ambient
	if swFlowing {
		ret.CoolantTemp = Temperature{Val: ret.CoolantTemp.Float()-gap*(dt.Seconds()/dieselCoolingTau.Seconds()), Unit: u.Fahrenheit}
	} else {
		if ret.State == DieselRunning {
			ret.CoolantTemp = Temperature{Val: ret.CoolantTemp.Float()+dieselAmbientRate*dt.Seconds()*100, Unit: u.Fahrenheit}
		} else {
			ret.CoolantTemp = Temperature{Val: ret.CoolantTemp.Float()-gap*(dt.Seconds()/dieselCoolingTau.Seconds()), Unit: u.Fahrenheit}
		}
	}

	// 5) Cooling trip.
	if ret.State == DieselRunning && ret.CoolantTemp.Float() >= DieselCoolantTripTemp.Float() {
		ret.State = DieselTripped
	}

	// 6) Generator-side output ramp.
	//
	// "The diesel generators are provided with static-type brushless
	// excitation and field flashing for rapid voltage buildup."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:46-51
	//
	// Voltage and Frequency ramp from 0 to DieselRatedVoltage /
	// DieselRatedFrequency over DieselFieldFlashingTime. BreakerClosed
	// latches once ReadyToLoad is true (i.e. once the diesel can carry
	// load, per
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50).
	// RealPower follows the
	// mechanical RunningFraction scaled by DieselRatedkW.
	switch ret.State {
	case DieselRunning:
		ret.Generator.Voltage += DieselRatedVoltage * (dt.Seconds() / DieselFieldFlashingTime.Seconds())
		ret.Generator.Voltage = min(ret.Generator.Voltage, DieselRatedVoltage)
		ret.Generator.Frequency += DieselRatedFrequency * (dt.Seconds() / DieselFieldFlashingTime.Seconds())
		ret.Generator.Frequency = min(ret.Generator.Frequency, DieselRatedFrequency)
		ret.Generator.RealPower = Watts(float64(DieselRatedkW) * float64(ret.RunningFraction))
		if ret.ReadyToLoad && !ret.Generator.BreakerClosed {
			ret.Generator.BreakerClosed = true
		}
	default:
		ret.Generator.Voltage -= DieselRatedVoltage * (dt.Seconds() / DieselFieldFlashingTime.Seconds())
		ret.Generator.Voltage = max(ret.Generator.Voltage, 0)
		ret.Generator.Frequency -= DieselRatedFrequency * (dt.Seconds() / DieselFieldFlashingTime.Seconds())
		ret.Generator.Frequency = max(ret.Generator.Frequency, 0)
		ret.Generator.RealPower = 0
		ret.Generator.BreakerClosed = false
	}

	return ret
}
