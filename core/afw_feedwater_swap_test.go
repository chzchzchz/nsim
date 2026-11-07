package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Securing or starting the AFW pumps from the control room must have a
// physical effect: the EFWS pump-running controls gate the outside
// pumps that deliver condensate-storage water to the steam generator
// ("the steam generators feedwater supply is switched to the main
// feedwater system; the auxiliary feedwater system is secured",
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:45-50).
func TestEFWSPumpControlDeliversWater(t *testing.T) {
	p := MakePlant()
	// Near-zero core power so the tiny AFW credit is not swamped
	// by full-load boil-off in the default plant seed.
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0
	before := p.ReactorBuilding.SteamGenerator.Water.Float()

	m := GetControlMap(&p)
	c, ok := m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning"]
	require.Truef(t, ok, "want AFW motor pump control registered")
	require.NoErrorf(t, c.Set("1"), "Set MotorPumpRunning")

	for i := 0; i < 2000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	after := p.ReactorBuilding.SteamGenerator.Water.Float()
	assert.Greaterf(t, after, before,
		"SG inventory must grow while an AFW pump is commanded to run")
}

// Starting the main feedwater pump places the main feedwater system in
// service: the regulator un-freezes and, with SG level below its program,
// drives the main feedwater control valve open
// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-76).
func TestMainFeedwaterPumpControlRunsRegulator(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water = Volume{
		Val:  40000,
		Unit: u.Gallon,
	}
	// Stock the drain tank so the pump has suction inventory.
	p.TurbineBuilding.HeaterDrainTank.Level = Volume{
		Val:  5000,
		Unit: u.Gallon,
	}

	m := GetControlMap(&p)
	c, ok := m["TurbineBuilding/MainFeedwaterPump/Running"]
	require.Truef(t, ok, "want main feedwater pump control registered")
	require.NoErrorf(t, c.Set("1"), "Set MainFeedwaterPump.Running")

	mfwBefore := p.ReactorBuilding.SteamGenerator.Water.Float()
	for i := 0; i < 100; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	assert.True(t, p.TurbineBuilding.MainFeedwaterPump.Running)
	assert.True(t, p.TurbineBuilding.FeedwaterRegulator.Running)
	assert.Greater(t, p.TurbineBuilding.FeedwaterRegulator.ValveDemand, 0.0)
	assert.Greater(t, p.TurbineBuilding.MainFeedwaterControlValve.Open, 0.0)
	assert.Greater(t, p.ReactorBuilding.SteamGenerator.Water.Float(), mfwBefore,
		"main feedwater delivery must add SG inventory when level is below program")
}
