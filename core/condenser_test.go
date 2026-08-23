package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// With vacuum pumps running and no steam load, the condenser
// pressure relaxes toward the design vacuum target.
// "Under normal operation the air is removed by steam jet air
// ejectors or vacuum pumps."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
func TestCondenser_VacuumPumpsRestoreVacuum(t *testing.T) {
	c := Condenser{
		Pressure:           AtmosphericPressure,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		c = c.Step(c.CirculatingWaterIn, true, true)
	}
	assert.Less(t, c.Pressure, AtmosphericPressure,
		"want condenser pressure below atmospheric, got %v", c.Pressure)
	assert.LessOrEqual(t, c.Pressure, CondenserDesignVacuum*2,
		"want condenser near design vacuum (%v), got %v", CondenserDesignVacuum, c.Pressure)
}

// With the CW pumps off, the condenser loses its heat sink and
// the back-pressure rises toward atmospheric within a few seconds.
func TestCondenser_NoCWDegradesToAtmospheric(t *testing.T) {
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		c = c.Step(c.CirculatingWaterIn, true, false)
	}
	assert.Greater(t, c.Pressure, CondenserDesignVacuum*5,
		"want pressure risen from vacuum, got %v", c.Pressure)
	assert.Less(t, c.Pressure, AtmosphericPressure*1.5,
		"want pressure below 1.5× atmospheric, got %v", c.Pressure)
}

// The steam-dump controller opens the dump valves on a turbine trip
// and recloses them automatically once steam is readmitted to the
// turbine.
//
// Spec justification:
//   - "These valves open under a sudden reduction in turbine-
//     generator load (in excess of 10%) or a turbine trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
//   - "The dump valves close automatically as steam is admitted to
//     the turbine."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:32-33
func TestCondenser_TripOpensDumpValveAndReclosesOnAdmission(t *testing.T) {
	tb := TurbineBuilding{
		Condenser: Condenser{
			Pressure:           CondenserDesignVacuum,
			VacuumPumpRunning:  true,
			CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
		},
		Turbine: Turbine{
			State:              TurbineTripped,
			GoverningValve:     Valve{Open: 1},
			LubeOilPumpRunning: true,
			BearingOilPressure: BearingOilDesignPressure,
		},
	}
	for i := 0; i < int(2*time.Second/TimeStep); i++ {
		tb = tb.StepWithSteam(SteamGeneratorShellPressure, true)
	}
	assert.Greater(t, tb.Turbine.DumpValve.Open, 0.9,
		"want DumpValve opened by the trip, got %v", tb.Turbine.DumpValve.Open)

	// Readmit steam: the dump valves ramp closed again.
	tb.Turbine.State = TurbineRunning
	tb.Turbine.StopValve.Open = 1.0
	for i := 0; i < int(2*time.Second/TimeStep); i++ {
		tb = tb.StepWithSteam(SteamGeneratorShellPressure, true)
	}
	assert.Zero(t, tb.Turbine.DumpValve.Open,
		"want DumpValve closed once steam is admitted, got %v",
		tb.Turbine.DumpValve.Open)
}

// Each step accumulates the previous step's CondensingRate into
// Condenser.Level. After several steps with non-zero condensing
// rate, the level rises by the integrated mass.
func TestCondenser_LevelAccumulatesCondensingRate(t *testing.T) {
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	const perStep = 5.0
	const steps = 100
	for i := 0; i < steps; i++ {
		c.CondensingRate = Volume{Val: perStep, Unit: u.Gallon}
		c = c.Step(c.CirculatingWaterIn, true, true)
	}
	want := perStep * float64(steps)
	got := c.Level.Float()
	assert.Positive(t, got, "want Level > 0")
	assert.InDelta(t, want, got, want*0.1,
		"want Level ~%v after %d steps of %v, got %v", want, steps, perStep, got)
}

// HotwellLevel accumulates condensed steam minus the condensate
// pump suction flow. With CW running and incoming steam load, the
// hotwell rises.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser
//     hotwell [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-42
func TestCondenser_HotwellAccumulates(t *testing.T) {
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	const perStep = 5.0
	const steps = 1000
	for i := 0; i < steps; i++ {
		c.CondensingRate = Volume{Val: perStep, Unit: u.Gallon}
		c = c.Step(c.CirculatingWaterIn, true, true)
	}
	assert.Positive(t, c.HotwellLevel.Float(),
		"want HotwellLevel > 0 after %d steps, got %v", steps, c.HotwellLevel.Float())
}
