package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
)

func newDiesel(fuelGal float64) DieselGenerator {
	return DieselGenerator{
		Fuel: Tank{
			Level:    u.NewValue(fuelGal, u.Gallon),
			Capacity: u.NewValue(500, u.Gallon),
			Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
		},
		CoolantTemp: u.NewValue(80, u.Fahrenheit),
	}
}

// stepN steps the plant via Outside.Step the requested number of times,
// returning the final Outside. This drives production code in core/outside.go
// and core/diesel.go through the same path as the live simulation.
func stepN(p *Plant, n int) {
	for i := 0; i < n; i++ {
		p.Step()
		p.Update()
	}
}

func TestDiesel_FSM_StartAndReachReady(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true

	p.Outside.DieselGenerator = newDiesel(500)
	p.Outside.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, 1)
	if p.Outside.DieselGenerator.State != DieselCranking {
		t.Fatalf("after 1 step with start signal: want Cranking, got %v", p.Outside.DieselGenerator.State)
	}

	// 10 s of simulated time — chapter_11/page_166.txt:50.
	stepN(&p, int(10*time.Second/TimeStep))
	if p.Outside.DieselGenerator.State != DieselRunning {
		t.Fatalf("after 10 s: want Running, got %v", p.Outside.DieselGenerator.State)
	}
	if !p.Outside.DieselGenerator.ReadyToLoad {
		t.Fatal("after 10 s: want ReadyToLoad true")
	}
	if p.Outside.DieselGenerator.Speed < DieselRatedSpeed {
		t.Fatalf("after 10 s: want Speed >= %v, got %v", DieselRatedSpeed, p.Outside.DieselGenerator.Speed)
	}
}

func TestDiesel_NoStartWithoutSignal(t *testing.T) {
	p := MakePlant()
	p.Outside.DieselGenerator = newDiesel(500)

	stepN(&p, 1000)
	if p.Outside.DieselGenerator.State != DieselOff {
		t.Fatalf("without start signal: want Off, got %v", p.Outside.DieselGenerator.State)
	}
	if p.Outside.DieselGenerator.Speed != 0 {
		t.Fatalf("without start signal: want Speed 0, got %v", p.Outside.DieselGenerator.Speed)
	}
}

func TestDiesel_FuelOnlyDecreasesWhileRunning(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = newDiesel(500)
	levelAtRest := p.Outside.DieselGenerator.Fuel.Level.Float()

	stepN(&p, 1000)
	if got := p.Outside.DieselGenerator.Fuel.Level.Float(); got != levelAtRest {
		t.Fatalf("at rest: fuel should not change, was %v now %v", levelAtRest, got)
	}

	p.Outside.DieselGenerator.StartSignal = DieselStartManual
	// Get to running.
	stepN(&p, int(10*time.Second/TimeStep))
	before := p.Outside.DieselGenerator.Fuel.Level.Float()
	stepN(&p, int(time.Hour/TimeStep)) // 1 hr running
	after := p.Outside.DieselGenerator.Fuel.Level.Float()
	if !(after < before) {
		t.Fatalf("running: fuel must decrease, before %v after %v", before, after)
	}
}

func TestDiesel_TripsOnFuelExhaustion(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = newDiesel(0.0001) // tiny sliver
	p.Outside.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, int(10*time.Second/TimeStep))
	if p.Outside.DieselGenerator.State != DieselTripped {
		t.Fatalf("want Tripped on fuel exhaustion, got %v", p.Outside.DieselGenerator.State)
	}
}

func TestDiesel_TripsOnCoolantOverTemp(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = false // no cooling
	p.Outside.DieselGenerator = newDiesel(500)
	p.Outside.DieselGenerator.StartSignal = DieselStartManual

	// Crank to running.
	stepN(&p, int(10*time.Second/TimeStep))
	if p.Outside.DieselGenerator.State != DieselRunning {
		t.Fatalf("expected Running, got %v", p.Outside.DieselGenerator.State)
	}

	// Run a long time without service water.
	stepN(&p, int(1*time.Hour/TimeStep))
	if p.Outside.DieselGenerator.State != DieselTripped {
		t.Fatalf("want Tripped on loss of cooling, got %v", p.Outside.DieselGenerator.State)
	}
}
