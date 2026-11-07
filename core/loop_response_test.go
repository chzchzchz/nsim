package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Declaring a loss of offsite power through the bridge control must
// collapse both ESF buses onto the diesel-backed transfer path and
// auto-start the emergency diesel generator without any operator start
// command:
//   - "If conditions occur which result in a loss of normal electrical
//     power to a safeguards electrical bus, backup emergency diesel
//     generators are automatically started and necessary equipment will
//     be automatically sequenced on the buses to safely cool down the
//     primary plant if required."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-109
//   - automatic transfer on sensing loss of power:
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
//   - ready to accept load within 10 s, rated load within 30 s:
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-51
func TestPlant_LoopDeclarationTransfersESFBusesToDiesel(t *testing.T) {
	t.Parallel()
	testDt := 100 * time.Millisecond

	p := MakePlant()
	controls := p.GetControlMap()

	loop := controls["ReactorBuilding/PlantElectricalSystems/OffsitePowerAvailable"]
	require.NotNil(t, loop,
		"want OffsitePowerAvailable registered on the bridge")
	require.NoError(t, loop.Set("0"))

	p.Step(testDt)
	p.Update()

	assert.False(t, p.Electrical.ESFBusA.Energized,
		"bus A must sense the loss immediately")
	assert.False(t, p.Electrical.ESFBusB.Energized,
		"bus B must sense the loss immediately")
	assert.Equal(t, DieselStartLossOfOffsitePower,
		p.AuxiliaryBuilding.DieselGenerator.StartSignal,
		"diesel start signal must latch without an operator write")
	assert.True(t, p.ReactorBuilding.PlantElectricalSystems.StationBlackout,
		"station blackout must be declared before a diesel is running")

	for i := 0; i < 320; i++ {
		p.Step(testDt)
		p.Update()
	}

	d := p.AuxiliaryBuilding.DieselGenerator
	assert.Equal(t, DieselRunning, d.State)
	assert.True(t, d.ReadyToLoad, "diesel must be ready by the 10 s deadline")
	assert.True(t, d.Generator.BreakerClosed)
	assert.True(t, p.Electrical.ESFBusA.Energized,
		"bus A must re-energize from the diesel after transfer")
	assert.True(t, p.Electrical.ESFBusB.Energized,
		"bus B must re-energize from the diesel after transfer")
	assert.InDelta(t, float64(DieselRatedVoltage),
		p.Electrical.ESFBusA.Voltage, 1)

	pes := p.ReactorBuilding.PlantElectricalSystems
	assert.True(t, pes.Diesel1Running,
		"simplified model must mark the diesel running by 30 s")
	assert.True(t, pes.ESFBus1Available,
		"simplified model must complete transfer by 31 s")
	assert.False(t, pes.StationBlackout)

	require.NoError(t, loop.Set("1"))
	p.Step(testDt)
	p.Update()

	assert.True(t, p.Electrical.ESFBusA.Energized,
		"restoring offsite power must hold the buses energized")
	assert.Equal(t, DieselStartLossOfOffsitePower,
		p.AuxiliaryBuilding.DieselGenerator.StartSignal,
		"clearing the diesel latch stays an explicit operator action")
}

// After a reactor trip the condenser dump must restore Tavg toward
// no-load without dragging the SG header into the low-steamline ESFAS
// setpoint: the dump "is reduced as rapidly as conditions permit" and
// "will actuate if necessary and restore Tavg to the no-load value"
// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:70-77,95-96),
// while low-steamline pressure is an SI trigger
// (specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt).
func TestPlant_TrippedDumpRestoresTavgWithoutCollapsingHeader(t *testing.T) {
	t.Parallel()
	testDt := 50 * time.Millisecond

	p := MakePlant()
	controls := p.GetControlMap()

	trip := controls["ReactorBuilding/ReactorPressureVessel/Trip"]
	require.NotNil(t, trip)
	require.NoError(t, trip.Set("1"))

	minHeader := float64(SteamGeneratorShellPressure)

	for i := 0; i < 1200; i++ {
		p.Step(testDt)
		p.Update()
		if h := float64(p.ReactorBuilding.SteamGenerator.Pressure); h < minHeader {
			minHeader = h
		}
	}

	assert.GreaterOrEqual(t, minHeader, 0.7*float64(SteamGeneratorShellPressure),
		"dump must not collapse the header toward the 500 psia SI setpoint")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated,
		"no spurious safety injection during the post-trip restoration")
	assert.Less(t,
		float64(p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()),
		CoreReferenceOutletF,
		"Tavg must glide downward under dump control, not stagnate")
}

// The declared loss de-energizes the reactor coolant pumps and the
// trip latches: the pumps coast down on their flywheels instead of
// stepping to zero, the diesel-backed buses do not restart them, and
// bringing one back is an explicit operator action.
//   - "Reactor coolant pumps supplied from the offsite electrical
//     source will maintain forced flow cooling to the reactor core."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:66-68
//   - "If offsite power is not available, the long coast down times
//     of the reactor coolant pumps assures adequate flow during the
//     power decay transient."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:80-84
func TestPlant_LoopTripsReactorCoolantPumpsUntilOperatorRestart(t *testing.T) {
	t.Parallel()
	testDt := 100 * time.Millisecond

	p := MakePlant()
	controls := p.GetControlMap()
	require.NoError(t,
		controls["ReactorBuilding/ReactorCoolantPump/Running"].Set("1"),
		"seed the at-power pump lineup through the operator control")
	p.Step(testDt)
	p.Update()
	require.True(t, p.ReactorBuilding.ReactorCoolantPumps.AnyRunning(),
		"precondition: pumps run at power")

	loop := controls["ReactorBuilding/PlantElectricalSystems/OffsitePowerAvailable"]
	require.NotNil(t, loop)
	require.NoError(t, loop.Set("0"))

	p.Step(testDt)
	p.Update()

	assert.False(t, p.ReactorBuilding.ReactorCoolantPumps.AnyRunning(),
		"pumps must trip without an operator write")
	assert.Greater(t, float64(p.ReactorBuilding.ReactorCoolantPump.Velocity), 0.0,
		"the flywheel must coast down, not stop instantly")

	for i := 0; i < 400; i++ {
		p.Step(testDt)
		p.Update()
	}
	require.True(t, p.Electrical.ESFBusA.Energized,
		"precondition for the restart check: buses re-energized")
	assert.False(t, p.ReactorBuilding.ReactorCoolantPumps.AnyRunning(),
		"diesel-backed buses must not restart the pumps")

	require.NoError(t,
		controls["ReactorBuilding/ReactorCoolantPump/Running"].Set("1"))
	p.Step(testDt)
	p.Update()
	assert.True(t, p.ReactorBuilding.ReactorCoolantPumps.AnyRunning(),
		"a supply exists, so the operator restart must latch")
}

// The declared state must actually gate the offsite sources: with
// offsite available the buses sit at the offsite voltage; one step
// after the declaration they read zero
// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-105).
func TestPlant_OffsiteAvailabilityGatesSourceVoltage(t *testing.T) {
	t.Parallel()
	testDt := 100 * time.Millisecond

	p := MakePlant()
	assert.InDelta(t, float64(DieselRatedVoltage),
		p.Electrical.OffsiteSourceA.Voltage, 1)

	controls := p.GetControlMap()
	loop := controls["ReactorBuilding/PlantElectricalSystems/OffsitePowerAvailable"]
	require.NotNil(t, loop)
	require.NoError(t, loop.Set("0"))

	p.Step(testDt)
	p.Update()

	assert.Zero(t, p.Electrical.OffsiteSourceA.Voltage)
	assert.Zero(t, p.Electrical.OffsiteSourceB.Voltage)
	assert.Zero(t, p.Electrical.ESFBusA.Voltage)

	require.NoError(t, loop.Set("1"))
	p.Step(testDt)
	p.Update()

	assert.InDelta(t, float64(DieselRatedVoltage),
		p.Electrical.OffsiteSourceA.Voltage, 1)
	assert.Equal(t, u.Fahrenheit, p.ReactorBuilding.ReactorPressureVessel.Outlet.Unit,
		"sanity: stepping the plant leaves units intact")
}
