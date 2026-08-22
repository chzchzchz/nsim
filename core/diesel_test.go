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
	p.Outside.ServiceWaterSystem.PumpRunning = true

	p.AuxiliaryBuilding.DieselGenerator = newDiesel(500)
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, 1)
	assert.Equal(t, DieselCranking, p.AuxiliaryBuilding.DieselGenerator.State,
		"after 1 step with start signal")

	// 10 s of simulated time —
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-49.
	stepN(&p, int(10*time.Second/TimeStep))
	assert.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGenerator.State, "after 10 s")
	assert.True(t, p.AuxiliaryBuilding.DieselGenerator.ReadyToLoad, "after 10 s: want ReadyToLoad true")
	assert.GreaterOrEqual(t, p.AuxiliaryBuilding.DieselGenerator.Speed, DieselRatedSpeed,
		"after 10 s: want Speed >= %v, got %v", DieselRatedSpeed, p.AuxiliaryBuilding.DieselGenerator.Speed)
}

func TestDiesel_NoStartWithoutSignal(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = newDiesel(500)

	stepN(&p, 1000)
	assert.Equal(t, DieselOff, p.AuxiliaryBuilding.DieselGenerator.State, "without start signal")
	assert.Equal(t, RPM(0), p.AuxiliaryBuilding.DieselGenerator.Speed, "without start signal: want Speed 0, got %v", p.AuxiliaryBuilding.DieselGenerator.Speed)
}

func TestDiesel_FuelOnlyDecreasesWhileRunning(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = newDiesel(500)
	levelAtRest := p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Float()

	stepN(&p, 1000)
	assert.Equal(t, levelAtRest, p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Float(),
		"at rest: fuel should not change, was %v now %v", levelAtRest, p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Float())

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	// Get to running.
	stepN(&p, int(10*time.Second/TimeStep))
	before := p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Float()
	stepN(&p, int(10*time.Second/TimeStep)) // 10 s running is plenty to observe fuel burn
	after := p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Float()
	assert.Less(t, after, before, "running: fuel must decrease, before %v after %v", before, after)
}

func TestDiesel_TripsOnFuelExhaustion(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = newDiesel(0.0001) // tiny sliver
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, int(10*time.Second/TimeStep))
	assert.Equal(t, DieselTripped, p.AuxiliaryBuilding.DieselGenerator.State, "want Tripped on fuel exhaustion")
}

func TestDiesel_TripsOnCoolantOverTemp(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = false // no cooling
	p.AuxiliaryBuilding.DieselGenerator = newDiesel(500)
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	// Crank to running.
	stepN(&p, int(10*time.Second/TimeStep))
	require.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGenerator.State, "expected Running")

	// Run with no service water; expect a coolant over-temperature
	// trip. Break out of the loop once the trip latches — the
	// coolant heating rate is sized so the trip occurs well within
	// a minute of simulated time, and we only need the transition.
	const maxSim = 10 * time.Minute
	for i := 0; i < int(maxSim/TimeStep); i++ {
		p.Step()
		p.Update()
		if p.AuxiliaryBuilding.DieselGenerator.State == DieselTripped {
			return
		}
	}
	require.Failf(t, "want Tripped on loss of cooling within %v, state=%v",
		"simulated %v, final state=%v", maxSim, p.AuxiliaryBuilding.DieselGenerator.State)
}

// Service Water flowing should pull the coolant back toward ambient.
// Regression for the bug where the diesel step read
// Outside.ServiceWaterSystem.PumpRunning (the operator latch) instead
// of the actual delivered flow, so a PumpRunning=true flag with the
// ESF bus de-energised still cooled the jacket — the SWS pump could
// not develop flow without bus power.
//
// Spec justification:
//   - "Diesel generator coolers" listed as a Service Water System load.
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89
func TestDiesel_CoolantCoolsWhenServiceWaterFlows(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = newDiesel(500)
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	// Crank to running.
	stepN(&p, int(10*time.Second/TimeStep))
	require.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGenerator.State, "expected Running")

	// Force the coolant above ambient so cooling is observable.
	hot := u.NewValue(150, u.Fahrenheit)
	p.AuxiliaryBuilding.DieselGenerator.CoolantTemp = hot
	initial := p.AuxiliaryBuilding.DieselGenerator.CoolantTemp.Float()

	// Step a few tau (dieselCoolingTau = 30 s) so the exponential
	// decay is well past the noise floor. The diesel is also
	// drifting upward while running, but the SW term dominates at
	// this gap; we only assert a drop.
	stepN(&p, int(2*time.Minute/TimeStep))

	final := p.AuxiliaryBuilding.DieselGenerator.CoolantTemp.Float()
	assert.Less(t, final, initial,
		"want coolant to drop with SW flowing (initial=%v final=%v)", initial, final)
}

// Counterpart to TestDiesel_CoolantCoolsWhenServiceWaterFlows:
// PumpRunning=true with the ESF bus de-energised must NOT cool the
// jacket, because the SWS pumps cannot develop flow without bus power.
// We verify both the model invariant (Flow stays at 0 gpm) and the
// physical invariant (coolant does not decay toward ambient during
// the time window the engine is running and heating up).
//
// Spec justification:
//   - "The larger auxiliaries such as the service water pumps [...]
//     are fed directly from the 6600-volt [engineered safety features]
//     buses."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
func TestDiesel_CoolantDoesNotCoolWhenBreakerOpen(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	// Open both ESF breakers so the SWS pump motors are unpowered.
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.AuxiliaryBuilding.DieselGenerator = newDiesel(500)
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	// Crank to running.
	stepN(&p, int(10*time.Second/TimeStep))
	require.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGenerator.State, "expected Running")
	require.Zero(t, p.Outside.ServiceWaterSystem.Flow,
		"sanity: SWS flow must be 0 gpm with both ESF breakers open")

	// Sample Flow over a short window and confirm it stays zero
	// (the gate that drives the diesel cooler).
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		require.Zero(t, p.Outside.ServiceWaterSystem.Flow,
			"SWS flow must stay at 0 gpm with both ESF breakers open")
	}

	// Force the coolant above ambient so heating vs cooling is
	// observable. We sample over a short window so the diesel
	// doesn't trip on its own over-temperature: 8 s of
	// uncooled-running drift is ~40 °F (dieselAmbientRate*100
	// °F/s), well below the 220 °F trip threshold.
	hot := u.NewValue(150, u.Fahrenheit)
	p.AuxiliaryBuilding.DieselGenerator.CoolantTemp = hot
	initial := p.AuxiliaryBuilding.DieselGenerator.CoolantTemp.Float()

	stepN(&p, int(8*time.Second/TimeStep))

	final := p.AuxiliaryBuilding.DieselGenerator.CoolantTemp.Float()
	assert.GreaterOrEqual(t, final, initial,
		"want coolant to NOT drop with SW breaker open (initial=%v final=%v)", initial, final)
}
