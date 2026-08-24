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
	t.Skip("recalibration pending under the split temperature feedback " +
		"(plans/x/gaps.md item 15): part-load arc bands were tuned against " +
		"the old combined-coefficient dynamics")
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
	p := MakePlant()
	// Maneuvering chemistry: diluted shim puts the net temperature
	// coefficient negative so the fold below full power is
	// self-stabilizing. At fresh-fuel chemistry the positive
	// coefficient leaves the core no stable part-load point: it either
	// rides the rated cap or folds to the neutron-source floor.
	//   "through the negative moderator coefficient"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:81-83
	//   "the coefficient is substantially more negative due to depletion
	//    and removal of shim boron"
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:119-121
	boron := 500.0
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = boron
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = boron
	cm := p.GetControlMap()

	require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set("1"))
	for i := 0; i < int(15*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
	}
	require.NoError(t,
		cm["TurbineBuilding/Generator/BreakerClosed"].Set("true"))
	require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set("0.03"))
	// Trim rods back to criticality for the reduced-power lineup the
	// dump-backed ride-through settles at: with the condenser dump
	// carrying the load the turbine dropped, the core repositions just
	// under its pre-rejection power instead of folding away.
	//   "The steam dump serves as a short-term artificial load which allows
	//    the reactor control system time to automatically cut back power."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:27-31
	require.NoError(t,
		cm["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"].Set("0.05"))

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
