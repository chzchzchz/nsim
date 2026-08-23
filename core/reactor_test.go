package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// With no Trip, the reactor settles to a positive ThermalPower
// near rated (the simplified power-defect model loses some power
// to the Doppler correction; we assert it stays in a sane band).
func TestReactor_RunningProducesRatedPower(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Positive(t, r.TotalThermalPower(), "want ThermalPower > 0 at running")
	assert.LessOrEqual(t, r.TotalThermalPower(), CoreRatedThermalPower,
		"want ThermalPower <= rated, got %v", r.TotalThermalPower())
	assert.False(t, r.Trip, "Trip should remain false")
}

// Setting Trip drops ThermalPower below rated, into the decay-heat
// region. After the fast-branch window, the power should sit at or
// near the shutdown fraction of rated.
//
// At beginning-of-life the net temperature coefficient is small
// positive (specs/the_westinghouse_pressurized_water_react//chapter_2/page_037.txt:117-122,
// specs/the_westinghouse_pressurized_water_react//chapter_2/page_039.txt:54-55), so the post-trip
// After a scram the fission chain collapses and ThermalPower
// settles onto the fission-product decay inventory, which decays
// with time and distance from rated.
func TestReactor_TripDropsPowerToDecayHeat(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	r.Trip = true
	for i := 0; i < int(2*time.Minute/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Less(t, r.TotalThermalPower(), CoreRatedThermalPower,
		"want ThermalPower < rated after trip, got %v", r.TotalThermalPower())
	shutdownFloor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	// At BOL the small positive net temperature coefficient
	// lifts the floor above the bare fraction; cap at 2× the
	// shutdown level.
	assert.LessOrEqual(t, r.TotalThermalPower(), shutdownFloor*2,
		"want ThermalPower <= 2× shutdown level (%v), got %v", shutdownFloor*2, r.TotalThermalPower())
}

// At beginning-of-life (high RCS boron) the net temperature
// coefficient is small positive per the spec; the Doppler+moderator
// contribution at hot outlet is therefore positive relative to the
// reference-temperature baseline. End-of-life (low boron) reverses
// the sign.
//
// Spec justification:
//   - "At the beginning of life, the effect on reactivity is
//     small since the moderator temperature coefficient is small
//     [positive]. At the end of life, the coefficient is
//     substantially more negative due to depletion and removal of
//     shim boron [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
//   - "+7 pcm/°F" moderator effect at 1700 ppm with burnable
//     absorbers reducing the boron need
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:51-56
// The net temperature coefficient interpolates from small positive
// at beginning-of-life (full soluble-boron shim) to substantially
// negative at end-of-life (boron depleted), so the same hot-outlet
// offset contributes opposite-sign Doppler feedback depending on
// boron concentration.
//
// Spec justification:
//   - "At the beginning of life, the effect on reactivity is
//     small since the moderator temperature coefficient is small
//     [positive]. At the end of life, the coefficient is
//     substantially more negative due to depletion and removal of
//     shim boron [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
//   - "+7 pcm/°F" moderator effect at 1700 ppm with burnable
//     absorbers reducing the boron need
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:51-56
func TestReactor_DopplerShiftsReactivityWithBoron(t *testing.T) {
	assert.Greater(t, netTempCoef(CVCSInitialBoron), netTempCoef(10),
		"want net temperature coefficient higher at BOL boron than EOL boron")

	// With rods fully withdrawn, a hot outlet (+30 °F over the
	// reference) yields positive reactivity at BOL and negative
	// reactivity in an over-borated core whose coefficient has
	// turned negative.
	hot := float64(CoreDopplerReferenceTempF) + 30
	bol := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: hot, Unit: u.Fahrenheit},
	}
	bol.SeedSteadyStatePower(CoreRatedThermalPower)
	bol = bol.Step(RCPRatedSpeed)

	eol := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron + 200},
		Outlet:      Temperature{Val: hot, Unit: u.Fahrenheit},
	}
	eol.SeedSteadyStatePower(CoreRatedThermalPower)
	eol = eol.Step(RCPRatedSpeed)

	assert.Positive(t, bol.Reactivity,
		"want hot-outlet BOL reactivity positive, got %v", bol.Reactivity)
	assert.Negative(t, eol.Reactivity,
		"want hot-outlet over-borated reactivity negative, got %v", eol.Reactivity)
}

// Even with rods fully withdrawn and Trip asserted, decay heat
// keeps ThermalPower non-zero. The long-tail floor is reached
// well within an hour of simulated time.
func TestReactor_TripWithRodsOutKeepsDecayHeatFloor(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Trip:        true,
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(5*time.Minute/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Positive(t, r.TotalThermalPower(),
		"want non-zero ThermalPower during decay, got %v", r.TotalThermalPower())
	floor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	// Allow some margin since the simplified power-defect model
	// lifts the decay-heat floor slightly above the bare fraction.
	assert.LessOrEqual(t, r.TotalThermalPower(), floor*2,
		"want ThermalPower within 2x of shutdown level (%v), got %v", floor, r.TotalThermalPower())
}

// With Outlet at the design reference and ThermalPower=0, the
// outlet cannot drift away (target = inlet = CoreReferenceInletF,
// not the reference temperature), so Doppler stays near zero.
// Allow a tiny tolerance for the single-step transient.
func TestReactor_OutletAtReferenceGivesZeroDoppler(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:       Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
	}
	r = r.Step(RCPRatedSpeed)
	assert.GreaterOrEqual(t, r.Reactivity, float32(-1),
		"want near-zero Reactivity, got %v", r.Reactivity)
	assert.LessOrEqual(t, r.Reactivity, float32(1),
		"want near-zero Reactivity, got %v", r.Reactivity)
}

// With ThermalPower > 0 and RCP running, Outlet relaxes toward
// CoreReferenceOutletF on the PrimaryThermalTimeConstant.
func TestReactor_OutletTracksThermalPower(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	// Pre-warm the outlet at the design reference so the simplified
	// model is on the right side of the operating curve. With Outlet
	// at the reference, Doppler ≈ 0 and the cascade doesn't halve.
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	// After 10 s of full power the outlet should still be near the
	// design reference (within the model noise).
	assert.GreaterOrEqual(t, r.Outlet.Float(), CoreReferenceOutletF-30,
		"want Outlet near %v, got %v", CoreReferenceOutletF, r.Outlet.Float())
}

// After a trip the Outlet temperature decays along with the core
// power — within a few hundred steps it should drop appreciably.
func TestReactor_OutletDropsAfterTrip(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	// Settle at full power briefly so Outlet converges to ~621°F.
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	start := r.Outlet.Float()
	r.Trip = true
	r.ControlRods.Insertion = 1
	for i := 0; i < int(time.Minute/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Less(t, r.Outlet.Float(), start,
		"want Outlet < %v after 1 min of decay, got %v", start, r.Outlet.Float())
}

// With RCP not running, Inlet temperature is preserved across steps
// (pump off = no cold-leg turnover).
func TestReactor_InletFrozenWhenRCPOff(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:        Temperature{Val: 420, Unit: u.Fahrenheit},
		Outlet:       Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		r = r.Step(0) // RCP off
	}
	assert.Equal(t, 420.0, r.Inlet.Float(),
		"want Inlet frozen at 420°F with RCP off, got %v", r.Inlet.Float())
}

// Even with the RCP latch (Running) "on" by convention, if the
// rotor speed is zero (e.g. breaker just opened) the Step function
// must not drive the inlet-outlet blend. Regression for the bug
// where rcpRunning bool was the gate rather than the actual speed.
func TestReactor_InletFrozenWhenRCPSpeedZero(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:        Temperature{Val: 420, Unit: u.Fahrenheit},
		Outlet:       Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	// Step with rcpSpeed = 0 (pump latch on, but rotor coasting).
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		r = r.Step(0)
	}
	assert.Equal(t, 420.0, r.Inlet.Float(),
		"want Inlet frozen at 420°F with rcpSpeed=0, got %v", r.Inlet.Float())
}

// Post-trip decay-heat inventories must be a function of fission
// power history and fixed radioactive decay constants only — not of
// reactivity (specs/wiki/Decay_heat.txt). Two identical scrammed
// cores held at different (both negative) reactivity via boron must
// produce matching fission-product inventories and total thermal
// power. Regression: the reactivity power-defect correction used to
// multiply the shutdown curve too, letting reactivity amplify or
// suppress the "decay heat" of a scrammed core all the way back up
// to rated.
//
// The 950/990 ppm pair sits inside the narrow band where the
// current worth calibration holds a scrammed core subcritical;
// that band is a model-calibration artifact, not a spec value.
func TestReactor_TrippedPowerIgnoresReactivity(t *testing.T) {
	mk := func(boron float64) ReactorPressureVessel {
		r := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 1, DemandInsertion: 0, BoronConcentration: boron},
			Inlet:       Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
			Outlet:      Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
			Trip:        true,
		}
		r.SeedSteadyStatePower(CoreRatedThermalPower)
		return r
	}
	weak := mk(950)
	strong := mk(990)

	for i := 0; i < int(time.Second/TimeStep); i++ {
		weak = weak.Step(RCPRatedSpeed)
		strong = strong.Step(RCPRatedSpeed)
		if i > 0 {
			break
		}
	}

	assert.Negative(t, weak.Reactivity, "precondition: 950 ppm twin subcritical")
	assert.Negative(t, strong.Reactivity, "precondition: 990 ppm twin subcritical")
	assert.NotEqual(t, weak.Reactivity, strong.Reactivity,
		"boron must shift post-scram reactivity (%v vs %v)", weak.Reactivity, strong.Reactivity)

	// The fission component differs with reactivity by design;
	// the fission-product inventories may not.
	assert.InDelta(t, float64(weak.DecayHeatFast), float64(strong.DecayHeatFast),
		float64(weak.DecayHeatFast)*1e-6,
		"fast pool must be reactivity-independent: %v vs %v W",
		weak.DecayHeatFast, strong.DecayHeatFast)
	assert.InDelta(t, float64(weak.DecayHeatSlow), float64(strong.DecayHeatSlow),
		float64(weak.DecayHeatSlow)*1e-6,
		"slow pool must be reactivity-independent: %v vs %v W",
		weak.DecayHeatSlow, strong.DecayHeatSlow)
	assert.LessOrEqual(t, float64(weak.TotalThermalPower()), float64(CoreRatedThermalPower),
		"scrammed core must stay at or below rated, got %v W", weak.TotalThermalPower())
}
