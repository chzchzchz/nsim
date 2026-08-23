package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stepFor(p *Plant, d time.Duration) {
	for i := 0; i < int(d/TimeStep); i++ {
		p.Step()
		p.Update()
	}
}

func setControl(t *testing.T, p *Plant, path, value string) {
	t.Helper()
	cm := GetControlMap(p)
	c, ok := cm[path]
	require.True(t, ok, "control %s not registered", path)
	require.NoError(t, c.Set(value))
}

// Scenario 4 (secondary side spin-up): pulling condenser vacuum via
// the operator control drives back-pressure to design vacuum before
// any steam is admitted.
//
// "When the reactor coolant temperature exceeds 400F, steam is drawn
// from the steam generators to warm up the secondary plant, develop a
// vacuum in the main condenser, and roll the turbine."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:171-177
//
// "Under normal operation the air is removed by steam jet air
// ejectors or vacuum pumps."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:21-22
func TestScenario4_VacuumPumpControlEstablishesVacuum(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = false
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	// E1 requires vacuum established BEFORE any steam admission; a
	// steam load holds the back-pressure up by design
	// (condenser.go load-factor term), so secure admission first.
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	setControl(t, &p, "TurbineBuilding/Condenser/VacuumPump", "1")
	stepFor(&p, 5*time.Second)

	assert.True(t, p.TurbineBuilding.Condenser.VacuumPumpRunning)
	// Physics-forced: established vacuum sits below atmospheric and
	// below the low-vacuum trip plate.
	assert.Less(t, float64(p.TurbineBuilding.Condenser.Pressure), float64(AtmosphericPressure))
	assert.Less(t, float64(p.TurbineBuilding.Condenser.Pressure), float64(TurbineLowVacuumTripPsia))
	// Scenario acceptance band (plans/scenario/4.txt E1); the 3 psia
	// figure encodes the modeled 5 s vacuum recovery, not spec text.
	assert.LessOrEqual(t, float64(p.TurbineBuilding.Condenser.Pressure), 3.0,
		"want vacuum established below the 3 psia acceptance band, got %v",
		p.TurbineBuilding.Condenser.Pressure)
}

// Rolling the turbine into an evacuated condenser must not latch the
// low-vacuum trip.
//
// "Low vacuum trip"
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:26
func TestScenario4_RollingWithVacuumHoldsBackpressure(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = false
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	setControl(t, &p, "TurbineBuilding/Condenser/VacuumPump", "1")
	stepFor(&p, 5*time.Second)

	setControl(t, &p, "TurbineBuilding/Turbine/SpeedDemand", "0.05")
	setControl(t, &p, "TurbineBuilding/Turbine/StopValve", "1")
	stepFor(&p, 10*time.Second)

	// Fixed-horizon read is valid: TripByVacuum latches while stepping
	// and clears only on an operator reset, so an end-state false
	// proves no crossing happened anywhere in the window.
	assert.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State)
	assert.False(t, p.TurbineBuilding.Turbine.TripByVacuum,
		"rolling against an evacuated condenser must not trip on low vacuum")
	assert.Greater(t, float64(p.TurbineBuilding.Turbine.Velocity), 0.0)
}

// Admitting steam without the vacuum lineup latches the low-vacuum
// trip: the condenser sits at atmospheric back-pressure, above the
// trip plate.
func TestScenario4_AdmittingSteamWithoutVacuumTrips(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = false
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	setControl(t, &p, "TurbineBuilding/Turbine/SpeedDemand", "0.05")
	setControl(t, &p, "TurbineBuilding/Turbine/StopValve", "1")
	stepFor(&p, 10*time.Second)

	assert.True(t, p.TurbineBuilding.Turbine.TripByVacuum)
	assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State)
}
