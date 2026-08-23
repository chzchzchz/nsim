package core

import (
	"encoding/json"
	"fmt"

	u "github.com/bcicen/go-units"
)

// UnitVal pairs a numeric magnitude with a u.Unit. It is the underlying
// struct for the Temperature and Volume defined types; the json
// marshalling below is what allows Plant round-trips through
// encoding/json to preserve magnitudes across process boundaries.
type UnitVal struct {
	Val  float64
	Unit u.Unit
}

func (v UnitVal) Float() float64    { return v.Val }
func (v UnitVal) UnitType() u.Unit  { return v.Unit }
func (v UnitVal) String() string    { return fmt.Sprintf("%g %s", v.Val, v.Unit.Name) }

func (v UnitVal) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Val  float64 `json:"val"`
		Unit string  `json:"unit"`
	}{v.Val, v.Unit.Name})
}

func (v *UnitVal) UnmarshalJSON(data []byte) error {
	var bare float64
	if err := json.Unmarshal(data, &bare); err == nil {
		v.Val = bare
		return nil
	}
	var typed struct {
		Val  float64 `json:"val"`
		Unit string  `json:"unit"`
	}
	if err := json.Unmarshal(data, &typed); err != nil {
		return err
	}
	v.Val = typed.Val
	if typed.Unit == "" {
		return nil
	}
	unit, err := u.Find(typed.Unit)
	if err != nil {
		return fmt.Errorf("unknown unit %q: %w", typed.Unit, err)
	}
	v.Unit = unit
	return nil
}
