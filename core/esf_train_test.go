package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loopFixture declares a loss of offsite power on a fresh plant whose
// main generator breaker is open, so both ESF buses go dark and each
// train's diesel must answer its own bus.
func loopFixture(t *testing.T) *Plant {
	t.Helper()
	overrideTimeStep(t, 100*time.Millisecond)
	p := MakePlant()
	p.TurbineBuilding.Generator.BreakerClosed = false
	controls := p.GetControlMap()
	loop := controls["ReactorBuilding/PlantElectricalSystems/OffsitePowerAvailable"]
	require.NotNil(t, loop)
	require.NoError(t, loop.Set("0"))
	return &p
}

// Each train's diesel answers only its own ESF bus: killing one diesel
// must drop its bus while the independent train keeps its supply.
//   - "auxiliary power required for safe shutdown will be supplied
//     from physically and electrically independent diesel
//     generators located on the site."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:27-31
//   - "Each power source, diesel generator and offsite, up to the
//     point of connection to the engineered safety features system
//     power buses, is physically and electrically independent."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:30-36
func TestPlant_IndependentDieselsFeedIndependentTrains(t *testing.T) {
	p := loopFixture(t)

	for i := 0; i < 160; i++ {
		p.Step()
		p.Update()
	}
	require.True(t, p.Electrical.ESFBusA.Energized, "precondition: bus A transferred to its diesel")
	require.True(t, p.Electrical.ESFBusB.Energized, "precondition: bus B transferred to its diesel")
	require.True(t, p.AuxiliaryBuilding.DieselGenerator.ReadyToLoad)
	require.True(t, p.AuxiliaryBuilding.DieselGeneratorB.ReadyToLoad)

	// Starve the train-A diesel of fuel: its protective trip must
	// drop bus A while train B keeps running untouched.
	p.AuxiliaryBuilding.DieselGenerator.Fuel.Level =
		Volume{Val: 0, Unit: p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Unit}
	for i := 0; i < 20; i++ {
		p.Step()
		p.Update()
	}

	assert.Equal(t, DieselTripped, p.AuxiliaryBuilding.DieselGenerator.State,
		"the starved train-A diesel must trip")
	assert.False(t, p.Electrical.ESFBusA.Energized,
		"bus A must go dark with its diesel tripped")
	assert.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGeneratorB.State,
		"the independent train-B diesel must be unaffected")
	assert.True(t, p.Electrical.ESFBusB.Energized,
		"bus B must stay energized from its own diesel")
}

// Large ESF motors sequence onto a freshly transferred diesel bus over
// a short period instead of all re-energizing simultaneously: support
// loads return with the bus, the largest pump motors wait one stage.
//   - "Loads are automatically sequenced on the diesel generator
//     buses over a short period of time to preclude the possibility
//     of overload trip due to simultaneous starting loads."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:52-55
func TestPlant_DieselTransferSequencesLargeLoads(t *testing.T) {
	p := loopFixture(t)

	// Step until the diesels pick up their buses (ready 10 s +
	// field flashing margin), then sample inside and past the
	// first sequence stage.
	busUp := false
	var sinceTransfer int
	for i := 0; i < 300 && !busUp; i++ {
		p.Step()
		p.Update()
		busUp = p.Electrical.ESFBusA.Energized &&
			p.Electrical.ESFBusA.OnDiesel
	}
	require.True(t, busUp, "precondition: bus A transferred to its diesel")

	stageDelaySteps := int(esfLoadSequenceStageDelay / TimeStep)
	for sinceTransfer = 0; sinceTransfer < stageDelaySteps/2; sinceTransfer++ {
		p.Step()
		p.Update()
		assert.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.DriveReady,
			"RHR pump must wait out its sequence stage after the transfer")
		assert.True(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.DriveReady,
			"EFWS motor pump is a stage-0 load and returns with the bus")
	}
	for ; sinceTransfer < stageDelaySteps+10; sinceTransfer++ {
		p.Step()
		p.Update()
	}
	assert.True(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.DriveReady,
		"RHR pump must be released once its stage delay elapses")
}

// With the unit synchronized and carrying its house loads, a declared
// loss of offsite power does NOT darken the ESF buses: they keep
// drawing from the main generator through the UAT, so the diesels stay
// idle until the unit actually comes off the line.
//   - "the unit auxiliary power is transformed from the output of
//     the plant main generator through the unit auxiliary
//     transformer to the plant electrical distribution voltage."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:44-48
//   - steam-dump house-load capability "enables the plant to continue
//     operation and supply power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:55-58
func TestPlant_IslandedUnitCarriesESFBusesWithoutDieselStart(t *testing.T) {
	overrideTimeStep(t, 100*time.Millisecond)
	p := MakePlant()
	// A synchronized, spinning unit with its exciter active holds
	// terminal voltage (core/plant.go generator-terminal block).
	p.TurbineBuilding.Generator.BreakerClosed = true
	p.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
	p.TurbineBuilding.Generator.ExciterActive = true
	p.TurbineBuilding.Generator.Voltage = GeneratorRatedVoltage

	controls := p.GetControlMap()
	loop := controls["ReactorBuilding/PlantElectricalSystems/OffsitePowerAvailable"]
	require.NotNil(t, loop)
	require.NoError(t, loop.Set("0"))

	for i := 0; i < 30; i++ {
		p.Step()
		p.Update()
	}

	assert.True(t, p.Electrical.ESFBusA.Energized,
		"bus A must stay energized from the islanded unit through the UAT")
	assert.True(t, p.Electrical.ESFBusB.Energized,
		"bus B must stay energized from the islanded unit through the UAT")
	assert.False(t, p.Electrical.ESFBusA.OnDiesel,
		"bus A must be served by the main generator, not its diesel")
	assert.Equal(t, DieselStartNone, p.AuxiliaryBuilding.DieselGenerator.StartSignal,
		"a live bus must not start its diesel")
	assert.Equal(t, DieselStartNone, p.AuxiliaryBuilding.DieselGeneratorB.StartSignal,
		"a live bus must not start its diesel")
	assert.False(t, p.ReactorBuilding.PlantElectricalSystems.StationBlackout)

	p.TurbineBuilding.Generator.BreakerClosed = false
	p.Step()
	p.Update()

	assert.False(t, p.Electrical.ESFBusA.Energized,
		"opening the unit breaker must darken the buses")
	assert.Equal(t, DieselStartLossOfOffsitePower,
		p.AuxiliaryBuilding.DieselGenerator.StartSignal,
		"a darkened bus must latch its diesel start signal")
	assert.Equal(t, DieselStartLossOfOffsitePower,
		p.AuxiliaryBuilding.DieselGeneratorB.StartSignal,
		"a darkened bus must latch its diesel start signal")
}

// The planned-cooldown block gates ONLY the steamline-isolation
// low-steamline trigger: containment-pressure and manual triggers stay
// live while blocked, and an unblocked low header latches isolation.
// Prior to reaching the automatic actuation setpoints during cooldown
// the initiation signals are blocked
// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:107-110);
// extending the documented SI block to the same-setpoint isolation
// output is a modeling assumption (source is silent on a dedicated
// isolation block).
func TestRPS_SteamlineIsolationBlockGatesOnlySteamlineTrigger(t *testing.T) {
	lowHeader := Pressure(RPSLowSteamlinePressureTrip - 100)
	hotContainment := Pressure(RPSHighContainmentPressureTrip + 10)
	nominalSl := Pressure(RPSLowSteamlinePressureTrip + 100)
	nominalArgs := func() (float64, float64, Pressure, float64) {
		return 50, RPSLowTcoldTrip + 50, 0, 0
	}

	lvl, tcold, cont, pw := nominalArgs()
	blocked := ReactorProtectionSystem{SteamlineIsolationBlocked: true}
	r := blocked.Step(lowHeader, PressurizerHeaterTarget, lvl, tcold, cont, pw, 1, 0,
		false)
	assert.False(t, r.SteamlineIsolationActuated,
		"a blocked low-steamline trigger must not isolate the steamlines")

	r = blocked.Step(nominalSl, PressurizerHeaterTarget, lvl, tcold, hotContainment, pw, 1, 0,
		false)
	assert.True(t, r.SteamlineIsolationActuated,
		"high containment pressure must stay live while blocked")

	unblocked := ReactorProtectionSystem{}
	r = unblocked.Step(lowHeader, PressurizerHeaterTarget, lvl, tcold, cont, pw, 1, 0,
		false)
	assert.True(t, r.SteamlineIsolationActuated,
		"an unblocked low header must isolate the steamlines")
}
