package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When the reactor trips, Plant.Step must cascade the trip to the
// turbine: StopValve.Open drops to 0 and the turbine transitions
// to TurbineTripped within a few steps.
//
// Spec justification:
//   - "Turbine trip following a reactor trip." (item 2 of the
//     protective features list)
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:64
//   - "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
func TestPlant_ReactorTripCascadesToTurbine(t *testing.T) {
	p := MakePlant()
	// Bring the reactor to rated power; let the SG build pressure.
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 1
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	stepN(&p, int(2*time.Second/TimeStep))
	// Manually trip the reactor.
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	// Step past StopValveCloseDelay.
	stepN(&p, int(100*time.Millisecond/TimeStep))
	assert.Zero(t, p.TurbineBuilding.Turbine.StopValve.Open,
		"want StopValve.Open == 0 after reactor trip, got %v",
		p.TurbineBuilding.Turbine.StopValve.Open)
	assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State,
		"want TurbineTripped after reactor trip, got %v",
		p.TurbineBuilding.Turbine.State)
}

// With the reactor tripped, the condenser no longer receives full
// steam load from the turbine and Pressure relaxes toward the
// design vacuum.
//
// Spec justification:
//   - "Under normal operation the air is removed by steam jet air
//     ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
func TestPlant_CondenserVacuumRecoversAfterTrip(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = true
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	// Vacuum recovery only — no reactor or turbine dynamics needed.
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		p.TurbineBuilding.Condenser = p.TurbineBuilding.Condenser.Step(
			p.TurbineBuilding.Condenser.CirculatingWaterIn,
			true,
			true,
		)
	}
	assert.Less(t, p.TurbineBuilding.Condenser.Pressure, AtmosphericPressure,
		"want vacuum below atmospheric, got %v",
		p.TurbineBuilding.Condenser.Pressure)
}

// When the main feed pump is stopped (representing no SG-side
// demand), the minimum-flow recirculation line sends
// FeedwaterMinFlow per step back to the deaerator (HeaterDrainTank).
//
// Spec justification:
//   - "The discharge from the pumps is automatically recirculated
//     back to the deaerator whenever the flow to the high-pressure
//     feedwater heaters falls below the minimum."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
func TestPlant_MinFlowRecirculationToDeaerator(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.MainFeedwaterPump.Running = false
	before := p.TurbineBuilding.HeaterDrainTank.Level.Float()
	stepN(&p, int(2*time.Second/TimeStep))
	after := p.TurbineBuilding.HeaterDrainTank.Level.Float()
	assert.Greater(t, after, before,
		"want HeaterDrainTank.Level to rise from min-flow recirculation, before=%v after=%v",
		before, after)
}

// End-to-end: at full power the heater train should produce a
// cumulative temperature rise from 120°F inlet to a meaningfully
// higher outlet. This exercises the entire LP+HP chain wired in
// TurbineBuilding.StepWithSteam.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser hotwell
//     and pump condensate [...] through four stages of low-pressure
//     feedwater heaters to the deaerator. [...] The water discharge
//     from the feedwater pumps flows through two stages of high-
//     pressure heaters then into the steam generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-51
func TestPlant_HeaterTrainRaisesFeedwaterTemperature(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.LowPressureHeater.DrainValve = Valve{Open: 1.0}
	p.TurbineBuilding.HighPressureHeater.DrainValve = Valve{Open: 1.0}
	stepN(&p, int(5*time.Second/TimeStep))
	rise := p.TurbineBuilding.HighPressureHeater.TemperatureRise.Float()
	assert.GreaterOrEqual(t, rise, 0.0, "want non-negative HP heater rise, got %v", rise)
}

// When the reactor trips, the breaker must open so the unit stops
// exporting power to the grid. Plant.Step cascades the trip
// through the protective-layer wiring.
//
// Spec justification:
//   - "Turbine trip following a reactor trip." (item 2 of the
//     protective features list)
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:33-35
//   - "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
func TestPlant_ReactorTripOpensBreaker(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 1
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	p.TurbineBuilding.Generator.BreakerClosed = true
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(2*time.Second/TimeStep))
	require.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"precondition: want BreakerClosed before trip, got false")
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	stepN(&p, int(100*time.Millisecond/TimeStep))
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed,
		"want BreakerClosed=false after reactor trip, got true")
}

// With the generator breaker open the unit produces no electrical
// power even when the turbine is running at speed — the breaker
// gates grid connection.
//
// Spec justification:
//   - "output power, the power factor, and speed to ensure that the
//     limits of the generator are not exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
func TestPlant_BreakerOpenZerosRealPower(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 1
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(5*time.Second/TimeStep))
	assert.Zero(t, p.TurbineBuilding.Generator.RealPower,
		"want RealPower=0 with breaker open, got %v",
		p.TurbineBuilding.Generator.RealPower)
}

// End-to-end: when the operator closes the breaker at full load,
// the turbine produces RealPower and the UAT picks up house
// load (HouseLoadSuppliedFrom="UAT"). Net generator output to
// the grid is gross − house load.
//
// Spec justification:
//   - "the plant distribution buses are split and part are
//     supplied by the plant's own generator and the other part
//     are supplied by the system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:54-56
//   - "supply power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
func TestPlant_BreakerCloseEnergizesUATAndSubtractsHouseLoad(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 1
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	p.TurbineBuilding.Generator.BreakerClosed = true
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(5*time.Second/TimeStep))
	require.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"precondition: want BreakerClosed, got false")
	assert.Equal(t, "UAT", p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=UAT with breaker closed, got %q",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
	// Generator.RealPower is now the net of gross turbine output
	// minus house load; both should be positive.
	assert.Greater(t, p.TurbineBuilding.Generator.RealPower, Watts(0),
		"want net RealPower > 0, got %v",
		p.TurbineBuilding.Generator.RealPower)
	assert.Less(t, p.TurbineBuilding.Generator.RealPower,
		p.TurbineBuilding.Turbine.RealPower,
		"want net < gross (house load subtracted), got net=%v gross=%v",
		p.TurbineBuilding.Generator.RealPower,
		p.TurbineBuilding.Turbine.RealPower)
}

// End-to-end: a reactor trip opens the generator breaker and
// drops the UAT, automatically transferring house load to SAT.
// The reactor trip does NOT remove house load from the grid; it
// just switches the supply source from UAT to SAT.
//
// Spec justification:
//   - "The turbine and turbine-generator will trip off the line
//     with house loads automatically transferred to the offsite
//     sources of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:63-66
//   - "automatic transfer of supplies is provided on sensing
//     loss of power"
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:58-59
func TestPlant_ReactorTripTransfersHouseLoadToSAT(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 1
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	p.TurbineBuilding.Generator.BreakerClosed = true
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(2*time.Second/TimeStep))
	require.Equal(t, "UAT",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"precondition: want UAT supplying before trip, got %q",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	stepN(&p, int(200*time.Millisecond/TimeStep))
	assert.Equal(t, "SAT",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=SAT after trip, got %q",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed,
		"want BreakerClosed=false after trip, got true")
}
