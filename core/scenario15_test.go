package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 15 (turbine trip and recovery). The trip-side physics has
// durable coverage in core/plant_test.go and core/turbine_test.go
// (automatic stepback park, SG safety valves, bypass modulation,
// feedwater, full-load endpoint); this file pins the operator recovery
// sequence that stitches those pieces together:
//
//   - Reset precedes steam re-admission because the auto-stop-oil latch
//     force-holds the stop valves while tripped ("release of auto stop
//     oil [...] causes all steam control valves to close",
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34;
//     "The operator may position the valves, when necessary, through
//     the manual valve controllers",
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:114-116).
//   - Reactor rods lead the restore before steam is re-admitted;
//     "RCC assemblies follow load changes."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_038.txt:46-48
//   - Re-paralleling at synchronous speed and reloading at the
//     controlled selectable rate.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:52-53,
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:65-66
const scenario15Tick = 10 * time.Millisecond

func TestScenario15_RecoveryAfterTurbineTrip(t *testing.T) {
	// Width note: the latching logic (trip, reset, stepback release),
	// the speed ramp, and the dispatcher setpoint walk are exact at any
	// tick width; the stepback fold trajectory is width-sensitive but
	// its park band is verified at the production tick by
	// TestPlant_TurbineTripStepbackHoldsSixtyPercent, so this sequence
	// test runs at 10 ms and asserts the fold only coarsely. Must not
	// run in parallel.
	overrideTimeStep(t, 10*time.Millisecond)

	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepFor(&p, 5*time.Second)
	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State)

	// Trip the machine: stop/governor valves shut, breaker opens, MW
	// collapse, and the automatic stepback latches.
	setControl(t, &p, "TurbineBuilding/Turbine/StopValve", "0")
	setControl(t, &p, "TurbineBuilding/Generator/BreakerClosed", "0")
	stepFor(&p, 3*time.Second)
	require.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State)
	require.True(t, p.PlantOperation.StepbackActive)
	assert.LessOrEqual(t, float64(p.TurbineBuilding.Generator.RealPower),
		50e6, "generator output must collapse when the breaker opens")

	// Hold window: the bypass replaces the turbine as the steam load
	// while the stepback walks the core toward the park level.
	maxDump := 0.0
	for i := 0; i < int(30*time.Second/scenario15Tick); i++ {
		stepN(&p, 1)
		maxDump = max(maxDump, p.TurbineBuilding.Turbine.DumpValve.Open)
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	park := StepbackPowerFraction * float64(CoreFissionPowerAtRated)
	folded := float64(rpv.FissionPower)
	assert.Less(t, folded, float64(CoreFissionPowerAtRated)*0.9,
		"the stepback must be folding the core during the hold")
	assert.Greater(t, folded, park*0.5,
		"the fold must not overshoot far past the park level")
	assert.GreaterOrEqual(t, maxDump, 0.5,
		"the modulating bypass must carry the steam while tripped")
	sgP := float64(p.ReactorBuilding.SteamGenerator.Pressure)
	assert.GreaterOrEqual(t, sgP, 700.0,
		"header must stay clear of the low-steamline trip")
	assert.LessOrEqual(t, sgP, float64(SteamGeneratorShellDesignPressure))
	assert.False(t, rpv.Trip)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)

	// Recovery: the reset is honored only once the rotor has decayed
	// below turbineStopThreshold, so coast down first; reset clears
	// the trip at rest, rods restore first, and only then is steam
	// re-admitted for the roll.
	for i := 0; i < int(90*time.Second/scenario15Tick); i++ {
		if float64(p.TurbineBuilding.Turbine.Velocity) < 1e-3 {
			break
		}
		stepN(&p, 1)
	}
	require.Less(t, float64(p.TurbineBuilding.Turbine.Velocity), 1e-3,
		"the rotor must reach rest within the coast-down window")
	setControl(t, &p, "TurbineBuilding/Turbine/Reset", "Reset")
	stepN(&p, 10)
	require.Equal(t, TurbineOff, p.TurbineBuilding.Turbine.State)
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "0")
	setControl(t, &p, "TurbineBuilding/Turbine/Setpoint", "0")
	setControl(t, &p, "TurbineBuilding/Turbine/SpeedDemand", "1")
	setControl(t, &p, "TurbineBuilding/Turbine/StopValve", "1")
	for i := 0; i < int(80*time.Second/scenario15Tick); i++ {
		stepN(&p, 1)
		if float64(p.TurbineBuilding.Turbine.Velocity) >=
			float64(TurbineRatedSpeed)-20 {
			break
		}
	}

	// Field forcing rebuilds terminal voltage before the close, and the
	// close itself must land on synchronous speed: the speed ramp pins
	// the rotor exactly on the demand target, so poll to that pin
	// before throwing the breaker (an out-of-tolerance close pops).
	for i := 0; i < int(10*time.Second/scenario15Tick); i++ {
		stepN(&p, 1)
		if p.TurbineBuilding.Generator.Voltage >= 0.95*24000 &&
			float64(p.TurbineBuilding.Turbine.Velocity) >=
				float64(TurbineRatedSpeed)-0.01 {
			break
		}
	}
	setControl(t, &p, "TurbineBuilding/Generator/BreakerClosed", "1")
	stepFor(&p, 2*time.Second)
	require.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"a synchronized close must hold after the recovery")
	assert.InDelta(t, float64(TurbineRatedSpeed),
		float64(p.TurbineBuilding.Turbine.Velocity), 0.01,
		"the restored machine rides the bus speed lock")

	// Reload escalates at the selectable rate; output follows the
	// demand ladder up while the bypass hands the steam load back.
	// Dispatcher demand reaches the turbine only under the
	// load-follow permissive ("to act as a permissive network so
	// that unsafe demands from the computer [...]",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:10-12),
	// and it is entered in MW: 100 MW against the 1000 MW machine is
	// the 0.10 setpoint fraction this phase demands. Without the
	// permissive LoadDemandRef stays zero, Setpoint == 0 leaves the
	// steam-derived output uncapped by contract, and the walk never
	// starts.
	setControl(t, &p, "PlantOperation/LoadFollow", "1")
	setControl(t, &p, "ReactorBuilding/Grid/LoadDemand", "100")
	maxSgP := 0.0
	for i := 0; i < int(140*time.Second/scenario15Tick); i++ {
		stepN(&p, 1)
		maxSgP = max(maxSgP, float64(p.ReactorBuilding.SteamGenerator.Pressure))
		assert.LessOrEqual(t, float64(p.TurbineBuilding.Turbine.RealPower),
			p.TurbineBuilding.Turbine.Setpoint*float64(TurbineRatedkW)+1e-6,
			"output may never exceed the loading ceiling")
	}
	turb := &p.TurbineBuilding.Turbine
	assert.GreaterOrEqual(t, turb.Setpoint, TurbineLoadRunbackRate*120.0*0.75,
		"the dispatcher walk must escalate toward the entered demand")
	assert.Greater(t, float64(turb.RealPower), 10e6,
		"the unit must be picking up load again by the end of the walk")
	assert.LessOrEqual(t, maxSgP, float64(SteamGeneratorShellDesignPressure),
		"the bypass must keep the header under the design cap")
	assert.False(t, rpv.Trip)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
}
