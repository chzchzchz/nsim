package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

type Pressure float64
type Velocity float64

type Temperature UnitVal
type Volume UnitVal

type Watts float64
type RPM float64

const DefaultTimeStep = time.Millisecond

// TimeStep is the simulated time consumed by every Step call. It
// defaults to DefaultTimeStep and is widened only by tests spanning
// many hours of simulated time (see overrideTimeStep).
//
// WARNING: a wider step lowers integration fidelity. Trips, ramp
// limiters, and valve strokes calibrated for the default 1 ms tick
// can clip or alias, so results may diverge from production behavior
// and make assertions wrong. Use an override only when absolutely
// needed.
var TimeStep = DefaultTimeStep

// Shared unit-conversion factors and fluid properties for the
// lumped thermal models. Water is treated as 1 kg/L, so one
// gallon (≈3.785 L, KilogramsPerGallon in core/parts.go) of
// water weighs KilogramsPerGallon kilograms and one gpm delivers
// KilogramsPerGPM kg/s.
const (
	KilogramsPerGPM   = KilogramsPerGallon / 60.0
	KilogramsPerPound = 0.453592

	// SpecificHeatWater is the specific heat of liquid water
	// (J/kg·K) near ambient conditions; the spec states no value,
	// 4186 is the textbook figure.
	SpecificHeatWater = 4186.0

	// RCSMassKg is the lumped primary-coolant inventory driving
	// the RHRS-takeover heat balance; assumption, the spec quotes
	// no RCS inventory figure.
	RCSMassKg = 3_500_000.0 * KilogramsPerPound
)

func (t Temperature) Float() float64   { return UnitVal(t).Float() }
func (t Temperature) UnitType() u.Unit { return UnitVal(t).UnitType() }
func (v Volume) Float() float64        { return UnitVal(v).Float() }
func (v Volume) UnitType() u.Unit      { return UnitVal(v).UnitType() }

func (t Temperature) MarshalJSON() ([]byte, error) {
	return UnitVal(t).MarshalJSON()
}

func (t *Temperature) UnmarshalJSON(data []byte) error {
	var uv UnitVal
	if err := uv.UnmarshalJSON(data); err != nil {
		return err
	}
	*t = Temperature(uv)
	return nil
}

func (v Volume) MarshalJSON() ([]byte, error) {
	return UnitVal(v).MarshalJSON()
}

func (v *Volume) UnmarshalJSON(data []byte) error {
	var uv UnitVal
	if err := uv.UnmarshalJSON(data); err != nil {
		return err
	}
	*v = Volume(uv)
	return nil
}
