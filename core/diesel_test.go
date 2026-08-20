package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, DieselCranking, p.Outside.DieselGenerator.State,
		"after 1 step with start signal")

	// 10 s of simulated time —
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-49.
	stepN(&p, int(10*time.Second/TimeStep))
	assert.Equal(t, DieselRunning, p.Outside.DieselGenerator.State, "after 10 s")
	assert.True(t, p.Outside.DieselGenerator.ReadyToLoad, "after 10 s: want ReadyToLoad true")
	assert.GreaterOrEqual(t, p.Outside.DieselGenerator.Speed, DieselRatedSpeed,
		"after 10 s: want Speed >= %v, got %v", DieselRatedSpeed, p.Outside.DieselGenerator.Speed)
}

func TestDiesel_NoStartWithoutSignal(t *testing.T) {
	p := MakePlant()
	p.Outside.DieselGenerator = newDiesel(500)

	stepN(&p, 1000)
	assert.Equal(t, DieselOff, p.Outside.DieselGenerator.State, "without start signal")
	assert.Equal(t, RPM(0), p.Outside.DieselGenerator.Speed, "without start signal: want Speed 0, got %v", p.Outside.DieselGenerator.Speed)
}

func TestDiesel_FuelOnlyDecreasesWhileRunning(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = newDiesel(500)
	levelAtRest := p.Outside.DieselGenerator.Fuel.Level.Float()

	stepN(&p, 1000)
	assert.Equal(t, levelAtRest, p.Outside.DieselGenerator.Fuel.Level.Float(),
		"at rest: fuel should not change, was %v now %v", levelAtRest, p.Outside.DieselGenerator.Fuel.Level.Float())

	p.Outside.DieselGenerator.StartSignal = DieselStartManual
	// Get to running.
	stepN(&p, int(10*time.Second/TimeStep))
	before := p.Outside.DieselGenerator.Fuel.Level.Float()
	stepN(&p, int(10*time.Second/TimeStep)) // 10 s running is plenty to observe fuel burn
	after := p.Outside.DieselGenerator.Fuel.Level.Float()
	assert.Less(t, after, before, "running: fuel must decrease, before %v after %v", before, after)
}

func TestDiesel_TripsOnFuelExhaustion(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = newDiesel(0.0001) // tiny sliver
	p.Outside.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, int(10*time.Second/TimeStep))
	assert.Equal(t, DieselTripped, p.Outside.DieselGenerator.State, "want Tripped on fuel exhaustion")
}

func TestDiesel_TripsOnCoolantOverTemp(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = false // no cooling
	p.Outside.DieselGenerator = newDiesel(500)
	p.Outside.DieselGenerator.StartSignal = DieselStartManual

	// Crank to running.
	stepN(&p, int(10*time.Second/TimeStep))
	require.Equal(t, DieselRunning, p.Outside.DieselGenerator.State, "expected Running")

	// Run with no service water; expect a coolant over-temperature
	// trip. Break out of the loop once the trip latches — the
	// coolant heating rate is sized so the trip occurs well within
	// a minute of simulated time, and we only need the transition.
	const maxSim = 10 * time.Minute
	for i := 0; i < int(maxSim/TimeStep); i++ {
		p.Step()
		p.Update()
		if p.Outside.DieselGenerator.State == DieselTripped {
			return
		}
	}
	require.Failf(t, "want Tripped on loss of cooling within %v, state=%v",
		"simulated %v, final state=%v", maxSim, p.Outside.DieselGenerator.State)
}
