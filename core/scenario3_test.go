package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 3 (controlled primary heatup) secures the RHRS, starts the
// reactor coolant pumps, and rides decay heat up to the no-load
// condition without exceeding the heatup-rate limit:
//
//   - RHRS secured before the temperature rise, RCPs started:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:1-9,
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37
//   - "Primary plant heatup rate is limited to 50°F (28°C) per hour."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
//   - No-load endpoint ~547 F / 2250 psia:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:178-180

func scenario3ColdPlant() *Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: 138, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 138, Unit: u.Fahrenheit}
	rpv.Pressure = 1600
	rpv.Trip = true
	rpv.ControlRods.Insertion = 1
	rpv.ControlRods.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	// Decay pools for a shutdown several hours old pace the post-RHRS
	// heatup just under the 50 F/hr limit (specs/wiki/Decay_heat.txt).
	rpv.DecayHeatSlow = Watts(44e6)
	rpv.DecayHeatFast = Watts(2e6)

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pzr.Water = Volume{Val: 1460, Unit: u.Gallon}
	pzr.Pressure = 1600
	pzr.HeatersOn = false

	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.AutoLevelControl = false
	cvcs.ChargingPumpRunning = false
	cvcs.LetdownValveOpen = false

	rhr := &p.AuxiliaryBuilding.ResidualHeatRemovalSystem
	rhr.Running = true
	rhr.HasTakenOver = true
	for _, tr := range []*RHRSTrain{&rhr.TrainA, &rhr.TrainB} {
		tr.WarmupComplete = true
		tr.SuctionValves[0].Open = 1
		tr.SuctionValves[1].Open = 1
	}
	rhr.TrainA.Pump.Running = true
	return &p
}

// With one RHR train rejecting heat the cold plant holds; securing the
// train and starting the lumped RCP hands the temperature over to the
// decay-plus-pump-heat balance, which must begin heating and stay under
// the 50 F/hr limit.
func TestScenario3_SecuredRHRSHeatupUnderLimit(t *testing.T) {
	p := scenario3ColdPlant()

	window := 5 * time.Second
	before := outletF(p)
	stepFor(p, window, DefaultTimeStep)
	require.LessOrEqual(t, outletF(p), before,
		"a running RHR train must keep the cold plant from heating")

	setControl(t, p,
		"AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning", "0")
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Pump.Running)
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Pump.Running)
	// First stage of the sequential start: loop A only, so the
	// partial lineup is what paces the early heatup
	// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37).
	setControl(t, p, "ReactorBuilding/ReactorCoolantPumps/A/Running", "1")
	require.Equal(t, 1,
		p.ReactorBuilding.ReactorCoolantPumps.RunningCount())

	rideStart := outletF(p)
	// Wiring check: recomputed from the same production balance
	// (TotalThermalPower, RCSMassKg, SpecificHeatWater) the takeover
	// branch integrates.
	wantRate := float64(
		p.ReactorBuilding.ReactorPressureVessel.TotalThermalPower(),
	) / (RCSMassKg * SpecificHeatWater) * 9.0 / 5.0
	ride := 20 * time.Second
	stepFor(p, ride, DefaultTimeStep)
	gotRate := (outletF(p) - rideStart) / ride.Seconds()

	assert.Greater(t, gotRate, 0.0,
		"heatup must begin once the RHRS is secured")
	assert.LessOrEqual(t, gotRate, OpHeatupRateLimit,
		"heatup rate %v F/s must respect the 50 F/hr limit", gotRate)
	assert.InDelta(t, wantRate, gotRate, wantRate*0.35,
		"want heatup at the seeded heat-balance rate %v F/s, got %v F/s",
		wantRate, gotRate)
	// One spinning rotor of four puts the loop-average telemetry near
	// a quarter of rated: the staged lineup must be visible.
	assert.InDelta(t,
		0.25*float64(RCPRatedSpeed),
		float64(p.ReactorBuilding.ReactorCoolantPump.Velocity),
		0.1*float64(RCPRatedSpeed),
		"want one-pump lineup near a quarter of rated speed, got %v",
		p.ReactorBuilding.ReactorCoolantPump.Velocity)
}

// Final pressurization from the post-transit state: heaters drive
// pressure onto the no-load setpoint while Tavg holds the no-load band
// and the PORV stays shut throughout.
func TestScenario3_FinalPressurizationReachesNoLoadBands(t *testing.T) {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	// Post-transit condition from the scenario execution: Tavg just
	// under the no-load anchor with the core shut down.
	rpv.Inlet = Temperature{Val: 552.5, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 552.5, Unit: u.Fahrenheit}
	rpv.Trip = true
	rpv.ControlRods.Insertion = 1
	rpv.ControlRods.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatSlow = Watts(44e6)
	rpv.DecayHeatFast = Watts(2e6)

	pzr := &p.ReactorBuilding.Pressurizer
	pzr.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pzr.Water = Volume{Val: 375, Unit: u.Gallon}
	pzr.Pressure = 1600
	pzr.HeatersOn = false

	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.AutoLevelControl = false
	cvcs.ChargingPumpRunning = false
	cvcs.LetdownValveOpen = false

	setControl(t, &p, "ReactorBuilding/Pressurizer/HeatersOn", "1")
	maxPORV, maxOutletF := 0.0, 0.0
	for i := 0; i < int(6*time.Second/time.Millisecond); i++ {
		stepN(&p, 1, DefaultTimeStep)
		maxPORV = max(maxPORV, p.ReactorBuilding.PORV.Open)
		maxOutletF = max(maxOutletF, outletF(&p))
	}

	assert.GreaterOrEqual(t, float64(p.ReactorBuilding.Pressurizer.Pressure),
		2225.0, "final pressure must reach 2250 +/- 25 psia, got %v",
		p.ReactorBuilding.Pressurizer.Pressure)
	assert.LessOrEqual(t, float64(p.ReactorBuilding.Pressurizer.Pressure), 2275.0)
	outlet := outletF(&p)
	assert.GreaterOrEqual(t, outlet, 537.0,
		"final Tavg must stay inside 547 +/- 10 F, got %v", outlet)
	assert.LessOrEqual(t, outlet, 557.0)
	assert.LessOrEqual(t, maxOutletF, 557.0,
		"pressurization must not push Tavg out of the no-load band")
	assert.Equal(t, 0.0, maxPORV, "the PORV must never lift in transit")
}
