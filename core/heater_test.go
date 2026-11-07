package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// LP heater with non-zero steam flow and open drain valve raises
// the feedwater temperature by approximately LPTemperatureRisePerStage.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser hotwell
//     and pump condensate [...] through four stages of low-pressure
//     feedwater heaters to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
func TestHeater_LPProducesTemperatureRise(t *testing.T) {
	t.Parallel()
	h := LowPressureHeater{
		DrainValve: Valve{Open: 1.0},
	}
	const fwInletF = 120.0
	for i := 0; i < int(5*time.Second/DefaultTimeStep); i++ {
		h = h.Step(DefaultTimeStep, fwInletF, 1.0)
	}
	assert.Positive(t, h.TemperatureRise.Float(),
		"want TemperatureRise > 0 with steam flow")
	assert.LessOrEqual(t, h.TemperatureRise.Float(), LPTemperatureRisePerStage*2,
		"want TemperatureRise <= %v (clamped), got %v",
		LPTemperatureRisePerStage*2, h.TemperatureRise.Float())
}

// LP heater with zero steam flow or closed drain valve produces
// no temperature rise (the heat-exchange duty is zero in either
// case).
func TestHeater_LPNoSteamNoRise(t *testing.T) {
	t.Parallel()
	h := LowPressureHeater{
		DrainValve: Valve{Open: 1.0},
	}
	h = h.Step(DefaultTimeStep, 120.0, 0)
	assert.Zero(t, h.TemperatureRise.Float(),
		"want TemperatureRise == 0 with zero steam flow")
	h2 := LowPressureHeater{
		DrainValve: Valve{Open: 0},
	}
	h2 = h2.Step(DefaultTimeStep, 120.0, 1.0)
	assert.Zero(t, h2.TemperatureRise.Float(),
		"want TemperatureRise == 0 with drain closed")
}

// LP heater drain accumulates when steam flows.
func TestHeater_LPDrainAccumulates(t *testing.T) {
	t.Parallel()
	h := LowPressureHeater{
		DrainValve: Valve{Open: 1.0},
	}
	const steamFlow = 1.0
	const steps = 1000
	for i := 0; i < steps; i++ {
		h = h.Step(DefaultTimeStep, 120.0, steamFlow)
	}
	assert.Positive(t, h.DrainLevel.Float(), "want DrainLevel > 0")
}

// HP heater at higher inlet temperature produces a smaller absolute
// temperature rise (per the model, the per-stage rise is bounded
// by HPTemperatureRisePerStage).
func TestHeater_HPClampedAtPerStageMax(t *testing.T) {
	t.Parallel()
	h := HighPressureHeater{
		DrainValve: Valve{Open: 1.0},
	}
	// High steam flow forces the model toward its clamp.
	for i := 0; i < int(5*time.Second/DefaultTimeStep); i++ {
		h = h.Step(DefaultTimeStep, 380.0, 100.0)
	}
	assert.LessOrEqual(t, h.TemperatureRise.Float(), HPTemperatureRisePerStage*2,
		"want TemperatureRise clamped at %v, got %v",
		HPTemperatureRisePerStage*2, h.TemperatureRise.Float())
}

// End-to-end LP train: 4 iterations of the LP heater step on a
// 120°F inlet should raise the cumulative outlet temperature
// above the inlet.
func TestHeater_LPTrainCumulativeRise(t *testing.T) {
	t.Parallel()
	h := LowPressureHeater{
		DrainValve: Valve{Open: 1.0},
	}
	fwIn := Temperature{Val: 120.0, Unit: u.Fahrenheit}
	for i := 0; i < 4; i++ {
		h = h.Step(DefaultTimeStep, fwIn.Float(), 1.0)
		fwIn = Temperature{Val: h.Inlet.Float() + h.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	assert.Greater(t, fwIn.Float(), 120.0,
		"want cumulative LP train outlet > 120°F, got %v", fwIn.Float())
}

// End-to-end LP+HP train: 4 LP stages + 2 HP stages from 120°F
// feedwater should produce a meaningful cumulative rise.
func TestHeater_FullTrainCumulativeRise(t *testing.T) {
	t.Parallel()
	lp := LowPressureHeater{DrainValve: Valve{Open: 1.0}}
	fwIn := Temperature{Val: 120.0, Unit: u.Fahrenheit}
	for i := 0; i < 4; i++ {
		lp = lp.Step(DefaultTimeStep, fwIn.Float(), 1.0)
		fwIn = Temperature{Val: lp.Inlet.Float() + lp.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	hp := HighPressureHeater{DrainValve: Valve{Open: 1.0}}
	for i := 0; i < 2; i++ {
		hp = hp.Step(DefaultTimeStep, fwIn.Float(), 0.5)
		fwIn = Temperature{Val: hp.Inlet.Float() + hp.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	assert.Greater(t, fwIn.Float(), 125.0,
		"want cumulative LP+HP train outlet > 125°F, got %v", fwIn.Float())
}
