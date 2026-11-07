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
type Joules float64
type RPM float64

// EnergyMeter is a reusable simulator-owned cumulative-energy
// accumulator. Energy is stored in joules and reported in
// megawatt-hours by the bridge. The caller controls when a
// device's power is delivered; Add never infers a breaker or
// operating state.
type EnergyMeter struct {
	Value Joules
}

// Add integrates instantaneous power over dt through the common
// watt-second conversion.
func (m *EnergyMeter) Add(power Watts, dt time.Duration) {
	m.Value += Joules(float64(power) * dt.Seconds())
}

// MWh returns an energy in megawatt-hours.
func (j Joules) MWh() float64 {
	return float64(j) / JoulesPerMWh
}

// MWh returns the accumulated energy in megawatt-hours.
func (m EnergyMeter) MWh() float64 {
	return m.Value.MWh()
}

// DefaultTimeStep is the production tick (1 ms). Every Step method
// takes an explicit dt time.Duration; production callers pass this
// constant. Tests pass a wider explicit dt only for long-span physics.
const DefaultTimeStep = time.Millisecond

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
