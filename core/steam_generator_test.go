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
		Volume:    u.NewValue(1000, u.Gallon),
		Water:     u.NewValue(1000, u.Gallon),
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		sg = sg.Step()
	}
	assert.Positive(t, sg.Pressure, "Pressure should rise with HeatInput")
	assert.LessOrEqual(t, sg.Pressure, SteamGeneratorShellPressure,
		"Pressure overshot target: %v > %v", sg.Pressure, SteamGeneratorShellPressure)
}

// With HeatInput == 0 and zero initial Pressure, no phantom steam
// generation occurs.
func TestSteamGenerator_NoHeatInputNoSteam(t *testing.T) {
	sg := SteamGenerator{
		Volume: u.NewValue(1000, u.Gallon),
		Water:  u.NewValue(1000, u.Gallon),
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		sg = sg.Step()
	}
	assert.Zero(t, sg.Pressure, "Pressure should stay at 0 with no HeatInput")
}

// Sustained full HeatInput against a small Water inventory: Water
// depletes; once Water is gone Pressure decays back toward zero.
func TestSteamGenerator_WaterDepletionDecaysPressure(t *testing.T) {
	sg := SteamGenerator{
		Volume:    u.NewValue(1000, u.Gallon),
		Water:     u.NewValue(0.001, u.Gallon), // tiny sliver
		Pressure:  SteamGeneratorShellPressure,
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		sg = sg.Step()
	}
	assert.Zero(t, sg.Water.Float(), "Water should be fully depleted")
	assert.Less(t, sg.Pressure, SteamGeneratorShellPressure,
		"Pressure should decay once Water is gone, got %v", sg.Pressure)
}

// A pressure above the design pressure is clamped — no run-away.
func TestSteamGenerator_PressureClampedAtDesign(t *testing.T) {
	sg := SteamGenerator{
		Volume:    u.NewValue(1000, u.Gallon),
		Water:     u.NewValue(1000, u.Gallon),
		Pressure:  SteamGeneratorShellDesignPressure + 500,
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	for i := 0; i < 1000; i++ {
		sg = sg.Step()
	}
	assert.LessOrEqual(t, sg.Pressure, SteamGeneratorShellDesignPressure,
		"Pressure should be clamped at %v, got %v", SteamGeneratorShellDesignPressure, sg.Pressure)
}
