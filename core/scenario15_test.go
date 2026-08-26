package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 15 (turbine trip and recovery). The trip-side physics has
// durable coverage in core/plant_test.go and core/turbine_test.go
// (scram on the at-power trip, SG safety valves, bypass modulation,
// feedwater, full-load endpoint); this file pins the operator recovery
// sequence that stitches those pieces together:
//
//   - Above the at-power permissive the turbine-generator trip itself
//     initiates a reactor trip:
//     "Turbine-generator trip - Trip of the turbinegenerator above a
//     certain power level initiates a reactor trip"
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:33-34
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
	// Width note: the latching logic (trip, reset, scram release),
	// the speed ramp, and the dispatcher setpoint walk are exact at any
	// tick width; the scram fold trajectory is width-sensitive but its
	// collapse is verified at the production tick by
	// TestPlant_TurbineTripAtPowerScramsSupersedingStepback, so this
	// sequence test runs at 10 ms and asserts the fold only coarsely.
	// Must not run in parallel.
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
	require.True(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"the at-power machine loss must scram the reactor")
	assert.LessOrEqual(t, float64(p.TurbineBuilding.Generator.RealPower),
		50e6, "generator output must collapse when the breaker opens")

	// Hold window: the bypass replaces the turbine as the steam load
	// while the scram folds the core onto decay heat. The post-scram
	// dump response spends headroom only down to its ESFAS-safe floor
	// and shuts as Tavg restores, so the header never approaches the
	// low-steamline protection channel.
	maxDump := 0.0
	for i := 0; i < int(30*time.Second/scenario15Tick); i++ {
		stepN(&p, 1)
		maxDump = max(maxDump, p.TurbineBuilding.Turbine.DumpValve.Open)
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	park := StepbackPowerFraction * float64(CoreFissionPowerAtRated)
	folded := float64(rpv.FissionPower)
	assert.Less(t, folded, park*0.5,
		"the scram folds the core below the stepback park")
	assert.Less(t, maxDump, 1.0,
		"the post-scram dump modulates rather than pinning wide open")
	sgP := float64(p.ReactorBuilding.SteamGenerator.Pressure)
	assert.Greater(t, sgP, float64(RPSLowSteamlinePressureTrip),
		"header must stay clear of the low-steamline setpoint")
	assert.LessOrEqual(t, sgP, float64(SteamGeneratorShellDesignPressure),
		"header must stay under the SG design clamp")
	assert.True(t, rpv.Trip,
		"the scram holds the rods full in through the hold")
	assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"the reactor trip latches through the hold")

	// Recovery: the resets are honored only once the rotor has decayed
	// below turbineStopThreshold, so coast down first; both trips clear
	// at rest. Rods lead the restore, and only when resumed production
	// holds the header clear of the low-steamline setpoint is steam
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
	// The RPS latch clears only once the machine is shut and its trip
	// can no longer re-declare: reset cannot bypass an active trip.
	setControl(t, &p,
		"ReactorBuilding/ReactorProtectionSystem/Reset", "Reset")
	stepN(&p, 10)
	require.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"the operator reset clears the scram from hot standby")
	require.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip)
	// The protective cascade latched a turbine load runback on the
	// scram; the operator clears it as part of restoring the machine
	// so the dispatcher walk can engage again.
	setControl(t, &p, "TurbineBuilding/Turbine/LoadRunback", "0")
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "0")
	// Hand the rods to automatic control with the park band as the
	// power target: the controller withdraws from the post-scram
	// shutdown state and then holds production where turbine-scale
	// admission is sustainable, instead of letting fully withdrawn
	// rods run the core to rated against a no-load machine.
	setControl(t, &p, "PlantOperation/RodControlAuto", "1")
	setControl(t, &p, "PlantOperation/PowerSetpointTarget",
		fmt.Sprintf("%v", StepbackPowerFraction))
	// Rods lead the restore: withdrawn rods recover power before any
	// steam is re-admitted ("Control rods are manually withdrawn to
	// raise reactor power",
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:52-56).
	// Re-admission waits until production reaches the same park band
	// the stepback holds - the model's established level from which
	// turbine-scale admission is sustainable - so the opening surge
	// cannot crash the header into the low-steamline setpoint.
	for i := 0; i < int(900*time.Second/scenario15Tick); i++ {
		stepN(&p, 1)
		if float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower) >=
			StepbackPowerFraction*float64(CoreFissionPowerAtRated) &&
			float64(p.ReactorBuilding.SteamGenerator.Pressure) >
				float64(RPSLowSteamlinePressureTrip)+50 {
			break
		}
	}
	require.GreaterOrEqual(t,
		float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower),
		StepbackPowerFraction*float64(CoreFissionPowerAtRated),
		"withdrawn rods must restore production to the park band "+
			"before steam is re-admitted")
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
	setControl(t, &p, "Electrical/Grid/LoadDemand", "100")
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
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
}
