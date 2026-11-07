package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// Full-load HeatInput drives Pressure up toward SteamGeneratorShellPressure
// within ~1 s of simulated time.
func TestSteamGenerator_FullHeatInputRaisesPressure(t *testing.T) {
	sg := SteamGenerator{
		Volume:    Volume{Val: 1000, Unit: u.Gallon},
		Water:     Volume{Val: 1000, Unit: u.Gallon},
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	for i := 0; i < int(time.Second/DefaultTimeStep); i++ {
		sg = sg.Step(DefaultTimeStep, 0)
	}
	assert.Positive(t, sg.Pressure, "Pressure should rise with HeatInput")
	assert.LessOrEqual(t, sg.Pressure, SteamGeneratorShellPressure,
		"Pressure overshot target: %v > %v", sg.Pressure, SteamGeneratorShellPressure)
}

// With HeatInput == 0 and zero initial Pressure, no phantom steam
// generation occurs.
func TestSteamGenerator_NoHeatInputNoSteam(t *testing.T) {
	sg := SteamGenerator{
		Volume: Volume{Val: 1000, Unit: u.Gallon},
		Water:  Volume{Val: 1000, Unit: u.Gallon},
	}
	for i := 0; i < int(time.Second/DefaultTimeStep); i++ {
		sg = sg.Step(DefaultTimeStep, 0)
	}
	assert.Zero(t, sg.Pressure, "Pressure should stay at 0 with no HeatInput")
}

// Sustained full HeatInput against a small Water inventory: Water
// depletes; once Water is gone Pressure decays back toward zero.
func TestSteamGenerator_WaterDepletionDecaysPressure(t *testing.T) {
	sg := SteamGenerator{
		Volume:    Volume{Val: 1000, Unit: u.Gallon},
		Water:     Volume{Val: 0.001, Unit: u.Gallon}, // tiny sliver
		Pressure:  SteamGeneratorShellPressure,
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
		sg = sg.Step(DefaultTimeStep, 0)
	}
	assert.Zero(t, sg.Water.Float(), "Water should be fully depleted")
	assert.Less(t, sg.Pressure, SteamGeneratorShellPressure,
		"Pressure should decay once Water is gone, got %v", sg.Pressure)
}

// A pressure above the design pressure is clamped — no run-away.
func TestSteamGenerator_PressureClampedAtDesign(t *testing.T) {
	sg := SteamGenerator{
		Volume:    Volume{Val: 1000, Unit: u.Gallon},
		Water:     Volume{Val: 1000, Unit: u.Gallon},
		Pressure:  SteamGeneratorShellDesignPressure + 500,
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	for i := 0; i < 1000; i++ {
		sg = sg.Step(DefaultTimeStep, 0)
	}
	assert.LessOrEqual(t, sg.Pressure, SteamGeneratorShellDesignPressure,
		"Pressure should be clamped at %v, got %v", SteamGeneratorShellDesignPressure, sg.Pressure)
}

// Offtake exactly matched to production holds the header at the shell
// pressure regardless of the absolute power level: the pressure-mode
// dump band and nominal operation both stay reachable.
//
// "Steam flow per steam generator: 3,813,000 lb/hr (480 kg/sec)"
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
func TestSteamGenerator_BalancedDemandHoldsShellPressure(t *testing.T) {
	for _, supply := range []float64{1.0, 0.5} {
		sg := SteamGenerator{
			// Design-scale inventory: the matched-demand window
			// must not end in boil-dry now that evaporation
			// really debits the stock at the rated rate.
			Volume:    Volume{Val: 100000, Unit: u.Gallon},
			Water:     Volume{Val: 100000, Unit: u.Gallon},
			Pressure:  SteamGeneratorShellPressure,
			HeatInput: Watts(supply * float64(SteamGeneratorFullLoadHeatInput)),
		}
		for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
			sg = sg.Step(DefaultTimeStep, supply)
		}
		assert.InDelta(t, float64(SteamGeneratorShellPressure),
			float64(sg.Pressure), 1e-6,
			"matched demand at supply %v must hold shell pressure", supply)
	}
}

// Drawing more steam than the available heat can raise sags the
// header toward what production actually holds; a decay-heat-only
// core serving house-load draw must not read as an infinite steam
// source pinned at the shell pressure.
func TestSteamGenerator_UndersupplySagsHeader(t *testing.T) {
	sg := SteamGenerator{
		Volume:    Volume{Val: 1000, Unit: u.Gallon},
		Water:     Volume{Val: 1000, Unit: u.Gallon},
		Pressure:  SteamGeneratorShellPressure,
		HeatInput: Watts(0.01 * float64(SteamGeneratorFullLoadHeatInput)),
	}
	for i := 0; i < int(30*time.Second/DefaultTimeStep); i++ {
		sg = sg.Step(DefaultTimeStep, 0.04)
	}
	want := float64(SteamGeneratorShellPressure) * 0.01 / 0.04
	assert.InDelta(t, want, float64(sg.Pressure), want*0.05,
		"header should sag toward the production-held target %v", want)
}
