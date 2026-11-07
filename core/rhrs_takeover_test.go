package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The chapter-16 heatup holds the primary below the no-load anchor only
// while the RHRS heat balance owns the coolant temperature, and the post-
// RHRS heatup must ride the decay-heat rate rather than an unconditional
// relaxation:
//   - "Primary plant heatup rate is limited to 50°F (28°C) per hour."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
//   - "Primary temperature is maintained through the balanced use of
//     reactor coolant pump heat input and a steam dump system ..."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
//   - RHRS secured before heatup:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:3-4

func coldShutdownPlant() *Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: 200, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 200, Unit: u.Fahrenheit}
	rpv.Pressure = 1600
	rpv.ControlRods.Insertion = 1
	rpv.ControlRods.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	// Decay pools for a shutdown several hours old: the slow pool sets a
	// ~44 F/hr heatup pace once the RHRS is secured (specs/wiki/Decay_heat.txt).
	rpv.DecayHeatSlow = Watts(44e6)
	rpv.DecayHeatFast = Watts(2e6)
	p.ReactorBuilding.SteamGenerator.Pressure = 15
	p.ReactorBuilding.ReactorCoolantPump.Running = false

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pzr.Water = Volume{Val: 1460, Unit: u.Gallon}
	pzr.HeatersOn = false

	rhr := &p.AuxiliaryBuilding.ResidualHeatRemovalSystem
	rhr.Running = true
	rhr.HasTakenOver = true
	for _, tr := range []*RHRSTrain{&rhr.TrainA, &rhr.TrainB} {
		tr.WarmupComplete = true
		tr.SuctionValves[0].Open = 1
		tr.SuctionValves[1].Open = 1
	}
	return &p
}

func outletF(p *Plant) float64 {
	return p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
}

// With both RHR trains secured the only heat input is decay + pump heat,
// so the outlet must rise at that balance's rate and stay under the 50 F/hr
// limit. Wiring check: wantRate is recomputed from the same production
// balance (TotalThermalPower, RCSMassKg, SpecificHeatWater) the takeover
// branch integrates.
func TestRHRSTakeover_SecuredPumpsHeatAtDecayRate(t *testing.T) {
	p := coldShutdownPlant()
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Pump.Running)
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Pump.Running)

	window := 10 * time.Second
	before := outletF(p)
	stepN(p, int(window/time.Millisecond), DefaultTimeStep)
	after := outletF(p)

	wantRate := float64(
		p.ReactorBuilding.ReactorPressureVessel.TotalThermalPower(),
	) / (RCSMassKg * SpecificHeatWater) * 9.0 / 5.0
	gotRate := (after - before) / window.Seconds()
	assert.InDelta(t, wantRate, gotRate, wantRate*0.35,
		"want heatup at the seeded heat-balance rate %v F/s, got %v F/s",
		wantRate, gotRate)
	assert.GreaterOrEqual(t, OpHeatupRateLimit, gotRate,
		"heatup rate must respect the 50 F/hr limit")
}

// With one train running and its suction open the RHRS rejects more than
// the decay input, so the plant cools instead of relaxing toward no-load.
func TestRHRSTakeover_RunningTrainCoolsPlant(t *testing.T) {
	p := coldShutdownPlant()
	p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Pump.Running = true

	window := 10 * time.Second
	before := outletF(p)
	stepN(p, int(window/time.Millisecond), DefaultTimeStep)
	after := outletF(p)

	require.True(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.HasTakenOver,
		"precondition: RHRS takeover should hold while a train rejects heat")
	assert.Less(t, after, before,
		"want one running train to cool the plant, before %v after %v",
		before, after)
}

// A secured-RHRS cold plant must not drift back to the no-load anchor;
// minutes of sim time leave it within a few degrees of its start.
func TestRHRSTakeover_NoRelaxationToNoLoadWhenSecured(t *testing.T) {
	p := coldShutdownPlant()
	stepN(p, int(20*time.Second/time.Millisecond), DefaultTimeStep)
	assert.Less(t, outletF(p), 210.0,
		"want cold plant held near 200 F, got %v", outletF(p))
}
