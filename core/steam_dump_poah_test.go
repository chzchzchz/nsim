package core

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 6 low-power ascension: holding ~1 percent power with the
// turbine shut must lift the SG header above nominal and arm the steam
// dump's pressure-control mode without any operator command — "an
// increasing primary Tavg with the steam dump valves automatically
// opening" (specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:167-170;
// pressure mode usage specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-26).
// The header must stay far above the low-steamline SI threshold while
// cycling.
func TestSteamDumpPressureModeArmsAtPOAH(t *testing.T) {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.FissionPower = CoreRatedThermalPower / 100
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	maxDump, minP, maxP := 0.0, 9999.0, 0.0
	for i := 0; i < 15000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		d := p.TurbineBuilding.Turbine.DumpValve.Open
		pp := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		if d > maxDump {
			maxDump = d
		}
		if pp < minP {
			minP = pp
		}
		if pp > maxP {
			maxP = pp
		}
	}

	assert.Greaterf(t, maxDump, 0.0,
		"steam dump must open automatically at POAH with no steam admission")
	assert.GreaterOrEqual(t, minP, 500.0,
		"header must never sag to the low-steamline SI threshold")
	assert.LessOrEqual(t, maxP, float64(SteamGeneratorShellDesignPressure),
		"header must stay within SG design pressure")
}

// The un-tripped pressure-control case must modulate on header error
// rather than pinning the valves: "These valves are also utilized in
// the pressure control mode during plant startup and plant cooldown"
// (specs/the_westinghouse_pressurized_water_react/chapter_15/
// page_216.txt:24-26). Walking demand down in sub-rejection steps arms
// the case on a small persistent surplus. The discriminator is valve
// stroke travel over the settle window, measured on both laws: the
// pre-modulation pin cycled full-stroke between open and shut for 120
// valve-fractions of travel per minute; the shared PI law settles and
// holds for 2. The asserted bound sits far from both.
//
// The widened tick only stretches valve travel and thermal lag, both of
// which move slower than the sampled checkpoints..
func TestSteamDumpPressureModeModulatesUnderSmallSurplus(t *testing.T) {
	testDt := 10 * time.Millisecond
	p := fullPowerLineup()
	cm := p.GetControlMap()

	for _, sp := range []string{"0.91", "0.82", "0.73", "0.64", "0.55"} {
		require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set(sp))
		for i := 0; i < int(20*time.Second/DefaultTimeStep); i++ {
			p.Step(testDt)
			p.Update()
		}
	}

	travel, maxP := 0.0, 0.0
	prev := -1.0
	for i := 0; i < int(60*time.Second/DefaultTimeStep); i++ {
		p.Step(testDt)
		p.Update()
		d := float64(p.TurbineBuilding.Turbine.DumpValve.Open)
		if prev >= 0 {
			travel += math.Abs(d - prev)
		}
		prev = d
		pp := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		if pp > maxP {
			maxP = pp
		}
	}

	assert.LessOrEqual(t, travel, 10.0,
		"a settled pressure-control modulation must not stroke the valves full-travel")
	assert.Less(t, p.TurbineBuilding.Turbine.DumpValve.Open, 0.99,
		"a small surplus must not saturate the valves fully open")
	assert.LessOrEqual(t, maxP, 960.0,
		"the held header stays well inside SG design pressure")
}
