package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 7 (grid synchronization and loading), exercised phase by
// phase against production paths instead of replaying the headless
// artifacts in plans/x/scenario7_gridsync:
//
//   - "Manually withdraw rods to 6-15 percent; parallel the
//     turbine-generator on the grid; ramp steam dump valves shut as
//     loading commences [...] and escalate load at <= 5 percent of
//     full power per minute."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:51-66
//   - "Wide range speed control of the turbine from turning gear to
//     rated speed."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
//   - "Speed: 1800 rpm"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:22

// newScenario7Plant seeds the pre-synchronization lineup: hot secondary
// at the shell-pressure anchor, condenser vacuum already established,
// circulating water flowing, rotor stopped with the stop valve shut.
// The stop valve stays shut until the roll begins because a open valve
// on the pressurized header admits steam with no speed demand selected.
func newScenario7Plant(t *testing.T) *Plant {
	t.Helper()
	p := MakePlant()
	// Post low-power ascension start state: the core sits critical at
	// about 8 percent of rated ("rods manually withdrawn to raise
	// reactor power to about 6-15 percent",
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:51-52),
	// seeded through the production equilibrium setter so the slow
	// inventories match the scenario power.
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		Watts(0.08 * float64(CoreRatedThermalPower)))
	p.TurbineBuilding.Condenser.VacuumPumpRunning = true
	p.TurbineBuilding.Condenser.Pressure = 2
	p.Outside.CirculatingWaterPump.Running = true
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Turbine.SpeedDemand = 0
	return &p
}

const scenario7Tick = 100 * time.Millisecond

// The channels asserted here tolerate a 100 ms tick: the speed ramp
// and the dispatcher setpoint walk are linear-in-dt (exact at any
// width), the exciter field forcing carries O(dt/tau) error of about
// five percent of one step (GeneratorExciterFieldForcingFraction =
// 0.05), and the reactor half of the evolution is out of scope because
// the simplified core lacks the stabilizing load-follow feedback
// ("reactor power is maintained equal to load by virtue of feedback
// effects inherent in the physics of a PWR",
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:69-71).
func TestScenario7_WideRangeRollReachesSynchronous(t *testing.T) {
	p := newScenario7Plant(t)
	overrideTimeStep(t, scenario7Tick)

	setControl(t, p, "TurbineBuilding/Turbine/LubeOilPump", "1")
	setControl(t, p, "TurbineBuilding/Turbine/SpeedDemand", "0.33")
	setControl(t, p, "TurbineBuilding/Turbine/StopValve", "1")

	// TurbineSpeedRampRate = 1/60 per second paces the rotor at
	// 1800 rpm / 60 s = 30 rpm/s toward SpeedDemand x rated.
	stepFor(p, 5*time.Second)
	rpm := float64(p.TurbineBuilding.Turbine.Velocity)
	assert.InDelta(t, 150.0, rpm, 25.0,
		"want the roll pacing at the design 30 rpm/s, got %v rpm after 5 s", rpm)

	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"a rolling rotor must be in the running state")
	stepFor(p, 15*time.Second)
	assert.InDelta(t, 0.33*float64(TurbineRatedSpeed),
		float64(p.TurbineBuilding.Turbine.Velocity), 20.0,
		"wide-range control must settle on the selected speed fraction")

	setControl(t, p, "TurbineBuilding/Turbine/SpeedDemand", "1")
	for i := 0; i < int(45*time.Second/scenario7Tick); i++ {
		stepN(p, 1)
		if float64(p.TurbineBuilding.Turbine.Velocity) >=
			float64(TurbineRatedSpeed)-20.0 {
			break
		}
	}
	assert.InDelta(t, float64(TurbineRatedSpeed),
		float64(p.TurbineBuilding.Turbine.Velocity), 20.0,
		"the rotor must reach synchronous speed before the breaker closes")
}

// Paralleling obeys the synchronism check both ways: a close at rated
// speed with the field applied sticks and locks the rotor on the bus,
// an out-of-step close pops immediately.
//
// Spec: DEHM supports "automatic synchronizers" as remote equipment.
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:90-92
func TestScenario7_BreakerHoldsInSyncAndPopsOutOfSync(t *testing.T) {
	run := func(spin Velocity) bool {
		p := newScenario7Plant(t)
		overrideTimeStep(t, scenario7Tick)
		// Seed the slow roll leg directly: the acceleration phase has
		// its own coverage above, and the synchronism check only sees
		// terminal versus grid frequency at the close instant.
		p.TurbineBuilding.Turbine.Velocity = spin
		setControl(t, p, "TurbineBuilding/Turbine/LubeOilPump", "1")
		setControl(t, p, "TurbineBuilding/Turbine/SpeedDemand", "1")
		setControl(t, p, "TurbineBuilding/Turbine/StopValve", "1")
		// Field forcing builds terminal voltage toward rated at five
		// percent of the remaining gap per step; five seconds puts the
		// terminal comfortably inside the ten percent voltage window.
		stepFor(p, 5*time.Second)

		setControl(t, p, "TurbineBuilding/Generator/BreakerClosed", "1")
		stepFor(p, 2*time.Second)
		return p.TurbineBuilding.Generator.BreakerClosed
	}

	assert.True(t, run(TurbineRatedSpeed),
		"a close at synchronous speed with matched voltage must hold")
	assert.False(t, run(TurbineRatedSpeed/2),
		"a close at half speed must be popped by the synchronism check")
}

// A close at synchronous speed locks the rotor on the infinite bus and
// dispatch escalation walks the load at the controlled selectable rate.
func TestScenario7_SynchronizedUnitEscalatesAtControlledRate(t *testing.T) {
	p := newScenario7Plant(t)
	overrideTimeStep(t, scenario7Tick)
	p.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
	setControl(t, p, "TurbineBuilding/Turbine/LubeOilPump", "1")
	setControl(t, p, "TurbineBuilding/Turbine/SpeedDemand", "1")
	setControl(t, p, "TurbineBuilding/Turbine/StopValve", "1")
	stepFor(p, 5*time.Second)
	setControl(t, p, "TurbineBuilding/Generator/BreakerClosed", "1")
	stepFor(p, 1*time.Second)
	require.True(t, p.TurbineBuilding.Generator.BreakerClosed)

	// Dispatcher demand drives the DEHM setpoint walk at
	// TurbineLoadRunbackRate = 5 percent of rated per minute
	// (core/parts.go). The full walk from zero to rated demand takes
	// 1.0 / TurbineLoadRunbackRate = 1200 s of simulated time, which
	// costs about a second of wall time at the coarse tick because
	// the walk is linear in dt.
	setControl(t, p, "ReactorBuilding/Grid/LoadDemand", "1")
	maxDump := 0.0
	maxPowerFrac := 0.0
	minSgP := 2000.0
	turb := &p.TurbineBuilding.Turbine
	reactorTripped := false
	turbineTripped := false
	walkedOneMinute := 0.0
	for i := 0; i < int(1320*time.Second/scenario7Tick); i++ {
		stepN(p, 1)
		if i == int(60*time.Second/scenario7Tick)-1 {
			walkedOneMinute = turb.Setpoint
		}
		maxDump = max(maxDump, turb.DumpValve.Open)
		frac := float64(turb.RealPower) / float64(TurbineRatedkW)
		maxPowerFrac = max(maxPowerFrac, frac)
		sgP := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		minSgP = min(minSgP, sgP)
		if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			reactorTripped = true
		}
		if turb.State == TurbineTripped {
			turbineTripped = true
		}
		assert.InDelta(t, float64(TurbineRatedSpeed),
			float64(turb.Velocity), 0.01,
			"the paralleled machine rides the bus speed lock")
	}

	// One simulated minute of walk must land on one increment of the
	// controlled selectable rate: faster would outrun the DEHM limiter,
	// slower would fail to reach full demand inside the window.
	assert.InDelta(t, TurbineLoadRunbackRate*60.0, walkedOneMinute,
		TurbineLoadRunbackRate*60.0*0.35,
		"one minute of dispatcher walk must escalate about one increment")
	// The escalation must reach full-power demand with the generator
	// inside 90 percent of the target, with no spurious trips and the
	// header held in the loading band throughout.
	assert.InDelta(t, 1.0, turb.Setpoint, 0.01,
		"the dispatcher walk must reach full-power demand")
	endFrac := float64(turb.RealPower) / float64(TurbineRatedkW)
	assert.GreaterOrEqual(t, endFrac, 0.9,
		"want output within 90 percent of the full demand at run end")
	assert.LessOrEqual(t, maxPowerFrac, turb.Setpoint+1e-9,
		"output may never exceed the setpoint ceiling")
	assert.False(t, reactorTripped, "want no spurious reactor trip")
	assert.False(t, turbineTripped, "want no spurious turbine trip")
	assert.GreaterOrEqual(t, minSgP, 900.0,
		"the header must hold the loading-program band while load rises")
	// The dump valve belongs to the pressure-mode controller: with the
	// core producing more than the early demand draws, it carries the
	// surplus open (its spec'd startup duty: "These valves are also
	// utilized in the pressure control mode during plant startup",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-25),
	// then parks shut once demand outdraws production late in the walk.
	assert.Equal(t, 1.0, maxDump,
		"the bypass must carry the early production surplus open")
	assert.Equal(t, 0.0, turb.DumpValve.Open,
		"once loading outdraws production the dump must park shut")
}
