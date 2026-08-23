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

const TimeStep = time.Millisecond

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
