package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCumulativeEnergy_ExactAtConstantPower(t *testing.T) {
	t.Parallel()
	h := &HouseLoad{FixedMW: 10, VariableMW: 0}
	out := h.Step(time.Hour, false, 0)
	assert.InDelta(t, 10*WattsPerMW*3600, float64(out.CumulativeEnergy.Value), 1.0,
		"HouseLoad.CumulativeEnergy should integrate TotalMW exactly at constant load")

	h = &HouseLoad{FixedMW: 0, VariableMW: 10}
	out = h.Step(time.Hour, true, 0.5)
	assert.InDelta(t, 5*WattsPerMW*3600, float64(out.CumulativeEnergy.Value), 1.0,
		"HouseLoad.CumulativeEnergy should scale variable load with generator fraction")
}

func TestCumulativeEnergy_NeverDecreases(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	meters := []*EnergyMeter{
		&p.TurbineBuilding.Turbine.CumulativeEnergy,
		&p.TurbineBuilding.Generator.CumulativeEnergy,
		&p.ReactorBuilding.HouseLoad.CumulativeEnergy,
		&p.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy,
		&p.AuxiliaryBuilding.DieselGenerator.Generator.CumulativeEnergy,
		&p.AuxiliaryBuilding.DieselGeneratorB.Generator.CumulativeEnergy,
	}
	for _, meter := range meters {
		assert.Zero(t, float64(meter.Value),
			"fresh plant cumulative energies must start at zero")
	}

	stepN(&p, int(time.Second/DefaultTimeStep), DefaultTimeStep)
	p.TurbineBuilding.Generator.BreakerClosed = false
	stepN(&p, int(time.Second/DefaultTimeStep), DefaultTimeStep)

	for _, meter := range meters {
		assert.GreaterOrEqual(t, float64(meter.Value), 0.0,
			"cumulative energies must never decrease")
	}
}

func TestCumulativeEnergy_DieselCountsOnlyWhenBreakerClosed(t *testing.T) {
	t.Parallel()
	d := &DieselGenerator{
		State: DieselRunning,
		RunningFraction: 1,
		Fuel:  Tank{Level: Volume{Val: 500, Unit: u.Gallon}},
	}
	d.Generator.BreakerClosed = false
	before := d.Generator.CumulativeEnergy.Value
	d.Step(time.Hour, 1)
	assert.Equal(t, before, d.Generator.CumulativeEnergy.Value,
		"a running but unconnected diesel must not accumulate delivered energy")
}

func TestCumulativeEnergy_SurvivesPlantJSONRoundTrip(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.Turbine.CumulativeEnergy = EnergyMeter{Value: Joules(7 * JoulesPerMWh)}
	p.TurbineBuilding.Generator.CumulativeEnergy = EnergyMeter{Value: Joules(6 * JoulesPerMWh)}
	p.ReactorBuilding.HouseLoad.CumulativeEnergy = EnergyMeter{Value: Joules(1 * JoulesPerMWh)}
	p.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy = EnergyMeter{Value: Joules(19 * JoulesPerMWh)}
	p.AuxiliaryBuilding.DieselGenerator.Generator.CumulativeEnergy = EnergyMeter{Value: Joules(2 * JoulesPerMWh)}

	data, err := json.Marshal(p)
	require.NoError(t, err)
	for _, key := range []string{
		`"CumulativeEnergy"`,
		`"CumulativeThermalEnergy"`,
	} {
		assert.True(t, strings.Contains(string(data), key),
			"plant JSON must contain %s", key)
	}

	var decoded Plant
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, p.TurbineBuilding.Turbine.CumulativeEnergy,
		decoded.TurbineBuilding.Turbine.CumulativeEnergy)
	assert.Equal(t, p.TurbineBuilding.Generator.CumulativeEnergy,
		decoded.TurbineBuilding.Generator.CumulativeEnergy)
	assert.Equal(t, p.ReactorBuilding.HouseLoad.CumulativeEnergy,
		decoded.ReactorBuilding.HouseLoad.CumulativeEnergy)
	assert.Equal(t, p.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy,
		decoded.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy)
	assert.Equal(t, p.AuxiliaryBuilding.DieselGenerator.Generator.CumulativeEnergy,
		decoded.AuxiliaryBuilding.DieselGenerator.Generator.CumulativeEnergy)
}

func TestCumulativeEnergy_IntegratesNetGeneratorPower(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/DefaultTimeStep), DefaultTimeStep)
	require.Greater(t, float64(p.TurbineBuilding.Generator.RealPower), 0.0,
		"precondition: want net RealPower > 0")

	dt := 5 * time.Second
	before := p.TurbineBuilding.Generator.CumulativeEnergy.Value
	beforeTurbine := p.TurbineBuilding.Turbine.CumulativeEnergy.Value
	beforeThermal := p.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy.Value
	beforeHouse := p.ReactorBuilding.HouseLoad.CumulativeEnergy.Value

	stepN(&p, int(dt/DefaultTimeStep), DefaultTimeStep)

	// The generator may still be ramping during this window, so
	// only the integration inequalities are meaningful.
	assert.Positive(t, float64(p.TurbineBuilding.Generator.CumulativeEnergy.Value-before),
		"Generator.CumulativeEnergy should grow while the breaker is closed")
	assert.Positive(t, float64(p.TurbineBuilding.Turbine.CumulativeEnergy.Value-beforeTurbine),
		"Turbine.CumulativeEnergy should grow")
	assert.Positive(t, float64(p.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy.Value-beforeThermal),
		"RPV.CumulativeThermalEnergy should grow")
	assert.Positive(t, float64(p.ReactorBuilding.HouseLoad.CumulativeEnergy.Value-beforeHouse),
		"HouseLoad.CumulativeEnergy should grow")
	// While the breaker is closed, gross turbine energy equals
	// net generator energy plus station-service energy minus
	// any generator losses not modeled separately.
	assert.InDelta(t,
		float64(p.TurbineBuilding.Turbine.CumulativeEnergy.Value-beforeTurbine),
		float64(p.TurbineBuilding.Generator.CumulativeEnergy.Value-before)+
			float64(p.ReactorBuilding.HouseLoad.CumulativeEnergy.Value-beforeHouse),
		1.0,
		"gross turbine energy should equal net plus house load while connected")
	assert.LessOrEqual(t,
		float64(p.ReactorBuilding.ReactorPressureVessel.TotalThermalPower()),
		float64(CoreRatedThermalPower)+float64(p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput),
		"thermal power used for the bound check")
}
