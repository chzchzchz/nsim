package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		p.Step()
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
