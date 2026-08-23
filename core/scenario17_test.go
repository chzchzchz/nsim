package core

import (
	"testing"

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
	p := MakePlant()
	cm := p.GetControlMap()

	require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set("1"))
	for i := 0; i < 15000; i++ {
		p.Step()
		p.Update()
	}
	require.NoError(t,
		cm["TurbineBuilding/Generator/BreakerClosed"].Set("true"))
	require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set("0.03"))
	require.NoError(t,
		cm["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"].Set("0.012"))

	dumpAt5s := 0.0
	dumpAt60s := 1.0
	minPressure := 9999.0
	maxPressure := 0.0
	for i := 0; i < 73000; i++ {
		p.Step()
		p.Update()
		switch i {
		case 4999:
			dumpAt5s = p.TurbineBuilding.Turbine.DumpValve.Open
		case 59999:
			dumpAt60s = p.TurbineBuilding.Turbine.DumpValve.Open
		}
		pp := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		if pp < minPressure {
			minPressure = pp
		}
		if pp > maxPressure {
			maxPressure = pp
		}
	}

	assert.GreaterOrEqualf(t, dumpAt5s, 0.99,
		"dump valves must be fully open within 5 s of the rejection")
	assert.GreaterOrEqualf(t, dumpAt60s, 0.99,
		"ride-through must hold the dump open for %v", SteamDumpHoldTime)
	assert.Lessf(t, p.TurbineBuilding.Turbine.DumpValve.Open, 0.05,
		"dump must reclose once the power fold lets the header recover")
	assert.LessOrEqualf(t, maxPressure, float64(SteamGeneratorShellDesignPressure),
		"header must stay within SG design pressure")
	assert.GreaterOrEqualf(t, minPressure, 500.0,
		"header must never sag to the low-steamline SI threshold")
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
	assert.False(t, p.TurbineBuilding.Turbine.OPCTrip)
	assert.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"unit stays synchronized supplying its own auxiliaries")
	assert.Equal(t, "UAT", p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
}
