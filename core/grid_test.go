package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The grid is an infinite-bus model: the step function does not
// modify Frequency or Voltage. The operator (or scenario script)
// sets those values directly.
//
// Spec justification:
//   - "transmit power to the off-site distribution grid."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:18-19
//   - "critical importance in the event of large grid system
//     disturbances."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:59-60
func TestGrid_StepPreservesFrequencyAndVoltage(t *testing.T) {
	t.Parallel()
	g := Grid{
		Frequency: GridRatedFrequency,
		Voltage:   GridRatedVoltage,
	}
	out := g.Step(DefaultTimeStep)
	assert.Equal(t, GridRatedFrequency, out.Frequency,
		"want Frequency unchanged, got %v", out.Frequency)
	assert.Equal(t, GridRatedVoltage, out.Voltage,
		"want Voltage unchanged, got %v", out.Voltage)
}

// The operator (or scenario script) applies a step disturbance
// to grid frequency by writing to Grid.Frequency between steps;
// the turbine's governor then biases Setpoint to compensate.
// Here we verify the grid holds whatever Frequency the operator
// applies.
func TestGrid_OperatorCanApplyFrequencyStep(t *testing.T) {
	t.Parallel()
	g := Grid{
		Frequency: 60.0,
		Voltage:   GridRatedVoltage,
	}
	g.Frequency = 59.5
	out := g.Step(DefaultTimeStep)
	assert.InDelta(t, 59.5, out.Frequency, 1e-9,
		"want Frequency=59.5 after operator step, got %v", out.Frequency)
	g.Frequency = 60.5
	out = g.Step(DefaultTimeStep)
	assert.InDelta(t, 60.5, out.Frequency, 1e-9,
		"want Frequency=60.5 after operator step, got %v", out.Frequency)
}

// The droop governor responds to a 0.5 Hz grid-frequency drop
// (60.0 → 59.5, −0.83%) by raising Setpoint by ≈ 0.83%/droop ≈
// 20.8% of rated. The per-step bias is capped at
// TurbineDroopBiasLimit = 0.05, so a single step lands at
// Setpoint = 1.0 (already saturated) and the DroopBias field
// records the cap.
//
// Spec justification:
//   - "Wide range speed control of the turbine from turning gear
//     to rated speed."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
//   - The spec does not state a numeric droop; 4% is a standard
//     nuclear-turbine setting.
func TestTurbine_DroopGovernorRaisesSetpointOnUnderFrequency(t *testing.T) {
	t.Parallel()
	tr := Turbine{
		State:         TurbineRunning,
		Velocity:      TurbineRatedSpeed,
		RealPower:     TurbineRatedkW,
		Setpoint:      1.0,
		RampRate:      TurbineLoadRunbackRate,
		GridFrequency: 59.5,
	}
	open := 1.0
	tr = tr.Step(DefaultTimeStep, SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
	assert.InDelta(t, TurbineDroopBiasLimit, tr.DroopBias, 1e-9,
		"want DroopBias capped at TurbineDroopBiasLimit=%v, got %v",
		TurbineDroopBiasLimit, tr.DroopBias)
	assert.InDelta(t, 1.0, tr.Setpoint, 1e-9,
		"want Setpoint=1.0 (saturated by capped bias), got %v", tr.Setpoint)
}

// At 60.5 Hz (over-frequency, +0.83%) the droop governor
// lowers Setpoint by ≈ 0.83%/0.04 ≈ 20.8% of rated. The
// per-step bias is capped at −0.05, so one step lands at
// Setpoint = 0.95.
func TestTurbine_DroopGovernorLowersSetpointOnOverFrequency(t *testing.T) {
	t.Parallel()
	tr := Turbine{
		State:         TurbineRunning,
		Velocity:      TurbineRatedSpeed,
		RealPower:     TurbineRatedkW,
		Setpoint:      1.0,
		RampRate:      TurbineLoadRunbackRate,
		GridFrequency: 60.5,
	}
	open := 1.0
	tr = tr.Step(DefaultTimeStep, SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
	assert.Less(t, tr.Setpoint, 1.0,
		"want Setpoint < 1.0 after over-frequency droop, got %v", tr.Setpoint)
	assert.InDelta(t, 1.0-TurbineDroopBiasLimit, tr.Setpoint, 1e-9,
		"want Setpoint ≈ 1.0 - bias limit = %v, got %v",
		1.0-TurbineDroopBiasLimit, tr.Setpoint)
}

// With grid frequency exactly at the rated value the droop
// governor applies zero bias; Setpoint is unchanged.
func TestTurbine_DroopGovernorZeroBiasAtRatedFrequency(t *testing.T) {
	t.Parallel()
	tr := Turbine{
		State:         TurbineRunning,
		Velocity:      TurbineRatedSpeed,
		RealPower:     TurbineRatedkW,
		Setpoint:      0.75,
		RampRate:      TurbineLoadRunbackRate,
		GridFrequency: GridRatedFrequency,
	}
	open := 1.0
	tr = tr.Step(DefaultTimeStep, SteamGeneratorShellPressure, 0, Temperature{Val: 600}, false, &open)
	assert.InDelta(t, 0.75, tr.Setpoint, 1e-9,
		"want Setpoint unchanged at rated frequency, got %v", tr.Setpoint)
	assert.Zero(t, tr.DroopBias,
		"want DroopBias=0 at rated frequency, got %v", tr.DroopBias)
}
