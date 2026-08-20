package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
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
		CirculatingWaterIn: u.NewValue(CondenserDesignCWIn, u.Fahrenheit),
	}
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		c = c.Step(c.CirculatingWaterIn, true, true)
	}
	if !(c.Pressure < AtmosphericPressure) {
		t.Fatalf("want condenser pressure below atmospheric, got %v", c.Pressure)
	}
	if !(c.Pressure <= CondenserDesignVacuum*2) {
		t.Fatalf("want condenser near design vacuum (%v), got %v",
			CondenserDesignVacuum, c.Pressure)
	}
}

// With the CW pumps off, the condenser loses its heat sink and
// the back-pressure rises toward atmospheric within a few seconds.
func TestCondenser_NoCWDegradesToAtmospheric(t *testing.T) {
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: u.NewValue(CondenserDesignCWIn, u.Fahrenheit),
	}
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		c = c.Step(c.CirculatingWaterIn, true, false)
	}
	if !(c.Pressure > CondenserDesignVacuum*5) {
		t.Fatalf("want pressure risen from vacuum, got %v", c.Pressure)
	}
	if !(c.Pressure < AtmosphericPressure*1.5) {
		t.Fatalf("want pressure below 1.5× atmospheric, got %v", c.Pressure)
	}
}

// With a sustained steam dump into the condenser, the pressure
// rises above SteamDumpOpenThreshold and the turbine's DumpValve
// opens.
func TestCondenser_HighSteamLoadOpensDumpValve(t *testing.T) {
	tb := TurbineBuilding{
		Condenser: Condenser{
			Pressure:           CondenserDesignVacuum,
			VacuumPumpRunning:  true,
			CirculatingWaterIn: u.NewValue(CondenserDesignCWIn, u.Fahrenheit),
			CondensingRate:     u.NewValue(CondenserDesignSteamLoadGalPer*2, u.Gallon),
		},
		Turbine: Turbine{StopValve: Valve{Open: 1.0}, DumpValve: Valve{Open: 0}},
	}
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		tb = tb.StepWithSteam(SteamGeneratorShellPressure, true)
	}
	if !(tb.Condenser.Pressure > SteamDumpOpenThreshold) {
		t.Fatalf("want condenser pressure above dump threshold, got %v",
			tb.Condenser.Pressure)
	}
	if tb.Turbine.DumpValve.Open == 0 {
		t.Fatalf("want DumpValve open, got %v", tb.Turbine.DumpValve.Open)
	}
}

// Each step accumulates the previous step's CondensingRate into
// Condenser.Level. After several steps with non-zero condensing
// rate, the level rises by the integrated mass.
func TestCondenser_LevelAccumulatesCondensingRate(t *testing.T) {
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: u.NewValue(CondenserDesignCWIn, u.Fahrenheit),
	}
	const perStep = 5.0
	const steps = 100
	for i := 0; i < steps; i++ {
		c.CondensingRate = u.NewValue(perStep, u.Gallon)
		c = c.Step(c.CirculatingWaterIn, true, true)
	}
	want := perStep * float64(steps)
	got := c.Level.Float()
	if !(got > 0) {
		t.Fatalf("want Level > 0, got %v", got)
	}
	if got < want*0.9 || got > want*1.1 {
		t.Fatalf("want Level ~%v after %d steps of %v, got %v",
			want, steps, perStep, got)
	}
}
