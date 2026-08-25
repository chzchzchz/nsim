package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 36 (diesel generator test run paralleled to the auxiliary
// power source) durable assertions, per plans/x/scenario/36.txt. The
// anchors come from chapter 11:
//
//   - Fast start: "reach rated speed and be ready to begin accepting
//     load within 10 seconds, and be capable of carrying rated load
//     within 30 seconds after receipt of a start signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:46-51
//   - Parallel testing: "Automatic synchronizing equipment is provided
//     to permit parallel operation of each diesel with the normal
//     auxiliary power source during testing and exercising periods."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:65-68
//   - Normal supply: "The Engineered Safety Features buses are normally
//     supplied from the system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:23-25

// startScenario36Diesel arms the exercising/test start through the
// registered control (the operator lever behind page_166.txt:65-68) on a
// fresh plant whose service water cools the running engine. The widened
// tick keeps the deadline accumulators exact while making thousands of
// simulated seconds cheap; tests using it must not run in parallel.
func startScenario36Diesel(t *testing.T, signal string) *Plant {
	t.Helper()
	overrideTimeStep(t, 10*time.Millisecond)
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	cm := GetControlMap(&p)
	require.NoError(t, cm["AuxiliaryBuilding/DieselGenerator/StartSignal"].Set(signal))
	return &p
}

// A test start against the normal lineup cranks to rated speed, latches
// ready-to-load inside the 10 s deadline, and only then does the
// automatic synchronizer close the breaker - leaving the ESF bus served
// by its normal offsite source, paralleled beside the diesel.
func TestScenario36_TestStartParallelsWithLiveBus(t *testing.T) {
	p := startScenario36Diesel(t, "4")

	stepN(p, 900) // 9 s at the 10 ms tick: before the deadline
	d := &p.AuxiliaryBuilding.DieselGenerator
	assert.Equal(t, DieselRunning, d.State, "rated speed reached before the deadline")
	assert.False(t, d.ReadyToLoad, "ready-to-load must wait for the 10 s deadline")
	assert.False(t, d.Generator.BreakerClosed,
		"the synchronizer must not close onto a live bus before ready")

	stepN(p, 150) // 10.5 s total
	assert.True(t, d.ReadyToLoad, "ready inside the 10 s deadline")
	assert.True(t, d.Generator.BreakerClosed,
		"automatic synchronizing closes the breaker once synchronized")
	assert.InDelta(t, DieselRatedVoltage, d.Generator.Voltage, 1,
		"terminal voltage held at nominal across the close")
	busA := &p.Electrical.ESFBusA
	assert.True(t, busA.Energized, "the bus never drops during the parallel test")
	assert.False(t, busA.OnDiesel,
		"a paralleled test leaves the bus on its normal auxiliary source")
	assert.InDelta(t, float64(DieselRatedkW), float64(d.Generator.RealPower),
		0.01*float64(DieselRatedkW),
		"carrying rated load per the governor fraction")
}

// The emergency path survives the synchronizer: with every upstream
// source gone the bus is dead, so the breaker closes dead-bus the moment
// the engine is ready to load and the bus transfers onto the diesel.
func TestScenario36_DeadBusClosesAtReadyWithoutSync(t *testing.T) {
	p := startScenario36Diesel(t, "4")
	p.ReactorBuilding.PlantElectricalSystems.OffsitePowerAvailable = false

	stepN(p, 900) // 9 s at the 10 ms tick: before the deadline
	require.False(t, p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed,
		"no close before ready even on a dead bus")

	stepN(p, 150) // 10.5 s total
	assert.True(t, p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed,
		"dead-bus close fires as soon as the engine is ready to load")
	assert.Equal(t, DieselStartTest,
		p.AuxiliaryBuilding.DieselGenerator.StartSignal,
		"the exercising signal persists; the close does not depend on the cause")
	assert.True(t, p.Electrical.ESFBusA.Energized, "the bus transfers to the diesel")
	assert.True(t, p.Electrical.ESFBusA.OnDiesel,
		"an emergency transfer puts the diesel behind the bus")
}

// A closed breaker dragged out of its synchronization band pops,
// mirroring the main-generator out-of-step protection.
func TestScenario36_OutOfSyncBreakerPops(t *testing.T) {
	p := startScenario36Diesel(t, "4")
	stepN(p, 1100) // 11 s at the 10 ms tick: paralleled
	require.True(t, p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed,
		"precondition: paralleled")

	p.AuxiliaryBuilding.DieselGenerator.Generator.Voltage = 0
	stepN(p, 1)
	assert.False(t, p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed,
		"out-of-sync closed breaker must pop while the bus is live")
}

// Regression for the synchronizer gate itself: ready but outside the
// sync band against a live bus must hold the breaker OPEN, and the close
// waits until the terminal quantities re-enter the band. The pre-fix
// code closed unconditionally at ready-to-load, so this test fails on
// it. Voltage is sagged just before the deadline to simulate a
// regulator that has not settled.
func TestScenario36_BreakerWaitsForSyncBand(t *testing.T) {
	p := startScenario36Diesel(t, "4")
	d := &p.AuxiliaryBuilding.DieselGenerator

	stepN(p, 999) // one tick before the 10 s deadline
	d.Generator.Voltage = DieselRatedVoltage * 0.5

	stepN(p, 1) // deadline crossed while the sag dominates the ramp
	assert.True(t, d.ReadyToLoad, "ready latches on schedule")
	assert.False(t, d.Generator.BreakerClosed,
		"an unsynchronized generator must not close onto a live bus")

	closed := false
	for i := 0; i < 300; i++ { // voltage ramp re-enters the band well inside 3 s
		stepN(p, 1)
		if d.Generator.BreakerClosed {
			closed = true
			break
		}
	}
	assert.True(t, closed,
		"the synchronizer closes once the band is re-entered")
	assert.GreaterOrEqual(t, d.Generator.Voltage,
		DieselRatedVoltage*(1-SyncVoltageTolFraction),
		"close may only happen inside the sync band")
}

// Clearing the test signal ends the run without ever disturbing the
// ESF buses: the engine stops, the breaker opens, the offsite feed
// continues uninterrupted.
func TestScenario36_ClearingSignalEndsTestCleanly(t *testing.T) {
	p := startScenario36Diesel(t, "4")
	stepN(p, 1100) // 11 s at the 10 ms tick: paralleled
	require.True(t, p.Electrical.ESFBusA.Energized, "precondition: bus energized")

	cm := GetControlMap(p)
	require.NoError(t, cm["AuxiliaryBuilding/DieselGenerator/StartSignal"].Set("0"))
	stepN(p, 5)

	d := &p.AuxiliaryBuilding.DieselGenerator
	assert.NotEqual(t, DieselRunning, d.State, "engine stops when the signal clears")
	assert.False(t, d.Generator.BreakerClosed, "breaker opens with the engine stopped")
	assert.True(t, p.Electrical.ESFBusA.Energized,
		"the normal auxiliary feed rides through test termination")
}
