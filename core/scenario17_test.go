package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 17 full load rejection without reactor trip: a >=10 percent
// turbine load-reference drop must open the condenser steam dump valves,
// which then act as a short-term artificial load while the rod control cuts
// back core power, and reclose automatically once the transient passes -
// without any reactor or turbine trip.
//
//   - "These valves open under a sudden reduction in turbine-generator load
//     (in excess of 10%) or a turbine trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
//   - "The steam dump serves as a short-term artificial load which allows the
//     reactor control system time to automatically cut back power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:27-31
//   - "The dump valves close automatically as steam is admitted to the
//     turbine."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:32-33
func TestLoadRejectionSteamDumpRideThrough(t *testing.T) {
	// Rejection latching is snapshot-based (one operator write past
	// LoadRejectDetectFraction = 0.10), the valves stroke fully in
	// 0.5 s at SteamDumpValveTravelRate = 2.0/s against the 5 s
	// checkpoint, and SteamDumpHoldTime = 60 s counts down linearly
	// in dt; the header rides the SG design clamp through the
	// transient. So the widened tick reproduces the sequence.
	// Checkpoints below sample by simulated time, not step index,
	// so they keep their meaning at any width. Must not run in
	// parallel: overrideTimeStep mutates global simulation state.
	overrideTimeStep(t, 10*time.Millisecond)
	// The at-power fixture is the ride-through's premise: a rejection
	// is only defined against a synchronized machine carrying load.
	// The construction default now idles at hot standby, so the
	// MakePlant surgery this test once used no longer produces a plant
	// with anything to reject.
	p := fullPowerLineup()
	cm := p.GetControlMap()

	require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set("0.05"))

	dumpAt5s := 0.0
	dumpAt60s := 1.0
	maxPressure := 0.0
	// Sample checkpoints one step before each sim-time mark, matching
	// what the original fixed-index samples read at the default tick.
	fiveS := int(5*time.Second/TimeStep) - 1
	sixtyS := int(60*time.Second/TimeStep) - 1
	for i := 0; i < int(73*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		switch i {
		case fiveS:
			dumpAt5s = p.TurbineBuilding.Turbine.DumpValve.Open
		case sixtyS:
			dumpAt60s = p.TurbineBuilding.Turbine.DumpValve.Open
		}
		pp := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		if pp > maxPressure {
			maxPressure = pp
		}
	}

	assert.GreaterOrEqualf(t, dumpAt5s, 0.99,
		"dump valves must be fully open within 5 s of the rejection")
	assert.GreaterOrEqualf(t, dumpAt60s, 0.99,
		"ride-through must hold the dump open for %v", SteamDumpHoldTime)
	assert.LessOrEqualf(t, maxPressure, float64(SteamGeneratorShellDesignPressure),
		"header must stay within SG design pressure")
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
	assert.False(t, p.TurbineBuilding.Turbine.OPCTrip)
	assert.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"unit stays synchronized supplying its own auxiliaries")
	assert.Equal(t, "UAT", p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)

	// Ride-through ends when the grid takes load back: restoring
	// turbine admission is what closes the dumps.
	//
	//   "The dump valves close automatically as steam is admitted to
	//    the turbine."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:32-33
	require.NoError(t,
		cm["TurbineBuilding/Turbine/Setpoint"].Set("1"))
	reclosed := false
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		if p.TurbineBuilding.Turbine.DumpValve.Open < 0.05 {
			reclosed = true
			break
		}
	}
	assert.True(t, reclosed,
		"dump must reclose once the turbine re-admits steam")
}

// A turbine-generator trip from an at-power lineup initiates a
// reactor trip: with the condenser gone as a heat sink, only the
// steam dump banks remain to absorb production, so the RPS must not
// leave the core critical. The trip latches and the rods fall full in.
//
// Spec: "Turbine-generator trip - Trip of the turbinegenerator above
// a certain power level initiates a reactor trip"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:33-34
func TestTurbineTripAtPowerInitiatesReactorTrip(t *testing.T) {
	p := fullPowerLineup()
	p.TurbineBuilding.Turbine.TripByVacuum = true

	trippedAt := -1
	for i := 0; i < 300; i++ {
		stepN(&p, 1)
		if trippedAt < 0 &&
			p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			trippedAt = i
		}
	}
	require.GreaterOrEqualf(t, trippedAt, 0,
		"the turbine trip must scram the reactor at power")
	require.Lessf(t, trippedAt, 10,
		"the reactor trip must land within a few plant steps")
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"the RPS trip must propagate to the rod drop")
	assert.InDelta(t, 1.0,
		float64(p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion),
		1e-6, "rods must be fully inserted after the scram")
}

// An overpowering core drives the flux channel past its setpoint and
// the reactor trips outright - no second vote, no permissive. The
// neutron-flux feed must read fission power unclamped: the total-power
// helper saturates at rated and would hide every overpower.
//
// Spec: "Neutron flux trip signals ... provided for protection during
// startup, full power, and shutdown operations."
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:56-63
func TestPlant_HighNeutronFluxScramsDirectly(t *testing.T) {
	p := fullPowerLineup()
	p.ReactorBuilding.ReactorPressureVessel.FissionPower =
		Watts(1.25 * float64(CoreRatedThermalPower))

	trippedAt := -1
	for i := 0; i < 50; i++ {
		stepN(&p, 1)
		if trippedAt < 0 &&
			p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			trippedAt = i
			break
		}
	}
	require.GreaterOrEqualf(t, trippedAt, 0,
		"overpowering flux must scram the reactor end to end")
	require.Lessf(t, trippedAt, 5,
		"the flux trip must land within a few plant steps")
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"the scram must propagate to the rod drop")
	assert.InDelta(t, 1.0,
		float64(p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion),
		1e-6, "rods must be fully inserted after the scram")
}

// A pressurizer pressure excursion past the trip setpoint scrams the
// reactor end to end, and the trip answers BEFORE the relieving
// devices: the seeded 2420 psia sits above the trip but below the PORV
// and code-safety setpoints, so the scram - not a primary-coolant
// discharge - is what arrests the heatup.
//
// Spec: "Pressurizer trip signals - High pressure, low pressure,
// and high-level trip signals initiate a reactor trip"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:19-21
func TestPlant_HighPressurizerPressureScramsBeforeRelief(t *testing.T) {
	p := fullPowerLineup()
	p.ReactorBuilding.Pressurizer.Pressure = 2420

	trippedAt := -1
	for i := 0; i < 50; i++ {
		stepN(&p, 1)
		if trippedAt < 0 &&
			p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			trippedAt = i
			break
		}
	}
	require.GreaterOrEqualf(t, trippedAt, 0,
		"high pressurizer pressure must scram the reactor")
	require.Lessf(t, trippedAt, 5,
		"the trip must land within a few plant steps")
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"the scram must propagate to the rod drop")
	assert.False(t, p.ReactorBuilding.Pressurizer.PORVStuckOpen,
		"the PORV must never be challenged on this transient")
	assert.InDelta(t, 1.0,
		float64(p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion),
		1e-6, "rods must be fully inserted after the scram")
}

// Steamline isolation must isolate: the ESFAS output forces the main
// steam stop valves shut, the secondary offtake collapses with the
// turbine dying on its steam-loss deadline, and the upstream paths -
// the atmospheric dump valves - stay available for post-isolation
// cooldown. Clearing the latch returns the valve to operator command.
//
// Spec: "The main steam stop valves automatically close on low steam
// line pressure or on a high-high containment pressure signal [...]"
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:71-75
func TestSteamlineIsolationClosesMSIVAndKillsOfftake(t *testing.T) {
	p := fullPowerLineup()
	p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open = 0.5

	p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationActuated = true
	stepN(&p, 1)
	require.LessOrEqualf(t,
		p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open, 1e-6,
		"isolation must force the MSIV shut within one step")
	require.LessOrEqualf(t,
		float64(p.TurbineBuilding.SteamOfftakeFraction), 1e-9,
		"an isolated shell has no downstream offtake")

	discharging := false
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		stepN(&p, 1)
		if p.TurbineBuilding.Turbine.State == TurbineTripped {
			discharging = true
			break
		}
	}
	assert.Truef(t, discharging,
		"the admission-less machine must trip on the steam-loss deadline")
	assert.Equalf(t, 0.5,
		float64(p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open),
		"the upstream atmospheric dump stays open through isolation")

	p.ReactorBuilding.ReactorProtectionSystem.Reset()
	stepN(&p, 1)
	p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 1
	stepN(&p, 1)
	require.Equalf(t, 1.0,
		float64(p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open),
		"after the reset the operator commands the MSIV again")
}
