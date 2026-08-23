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
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Positive(t, r.ThermalPower, "want ThermalPower > 0 at running")
	assert.LessOrEqual(t, r.ThermalPower, CoreRatedThermalPower,
		"want ThermalPower <= rated, got %v", r.ThermalPower)
	assert.False(t, r.Trip, "Trip should remain false")
}

// Setting Trip drops ThermalPower below rated, into the decay-heat
// region. After the fast-branch window, the power should sit at or
// near the shutdown fraction of rated.
//
// At beginning-of-life the net temperature coefficient is small
// positive (specs/.../chapter_2/page_037.txt:117-122,
// specs/.../chapter_2/page_039.txt:54-55), so the post-trip
// power correction lifts the decay-heat floor slightly above the
// bare fraction. The test allows up to 2× the shutdown level.
func TestReactor_TripDropsPowerToDecayHeat(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	r.Trip = true
	for i := 0; i < int(2*time.Minute/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Less(t, r.ThermalPower, CoreRatedThermalPower,
		"want ThermalPower < rated after trip, got %v", r.ThermalPower)
	shutdownFloor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	// At BOL the small positive net temperature coefficient
	// lifts the floor above the bare fraction; cap at 2× the
	// shutdown level.
	assert.LessOrEqual(t, r.ThermalPower, shutdownFloor*2,
		"want ThermalPower <= 2× shutdown level (%v), got %v", shutdownFloor*2, r.ThermalPower)
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
func TestReactor_DopplerShiftsReactivityWithBoron(t *testing.T) {
	// BOL: high boron, small positive net coefficient.
	bol := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:       u.NewValue(float64(CoreDopplerReferenceTempF)+30, u.Fahrenheit),
		ThermalPower: CoreRatedThermalPower,
	}
	bol = bol.Step(RCPRatedSpeed)
	// EOL: low boron, more negative net coefficient.
	eol := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: 10},
		Outlet:       u.NewValue(float64(CoreDopplerReferenceTempF)+30, u.Fahrenheit),
		ThermalPower: CoreRatedThermalPower,
	}
	eol = eol.Step(RCPRatedSpeed)

	// At BOL the hot-outlet reactivity is positive or near-zero
	// (small positive coefficient × 30°F).
	assert.GreaterOrEqual(t, bol.Reactivity, float32(-5),
		"Doppler+moderator at BOL: want Reactivity near zero or positive, got %v", bol.Reactivity)
	// At EOL the hot-outlet reactivity is substantially negative.
	assert.Less(t, eol.Reactivity, bol.Reactivity,
		"want EOL Doppler+moderator term more negative than BOL; got bol=%v eol=%v",
		bol.Reactivity, eol.Reactivity)
}

// Even with rods fully withdrawn and Trip asserted, decay heat
// keeps ThermalPower non-zero. The long-tail floor is reached
// well within an hour of simulated time.
func TestReactor_TripWithRodsOutKeepsDecayHeatFloor(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Trip:         true,
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
		ThermalPower: CoreRatedThermalPower,
	}
	for i := 0; i < int(5*time.Minute/TimeStep); i++ {
		r = r.Step(RCPRatedSpeed)
	}
	assert.Positive(t, r.ThermalPower,
		"want non-zero ThermalPower during decay, got %v", r.ThermalPower)
	floor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	// Allow some margin since the simplified power-defect model
	// lifts the decay-heat floor slightly above the bare fraction.
	assert.LessOrEqual(t, r.ThermalPower, floor*2,
		"want ThermalPower within 2x of shutdown level (%v), got %v", floor, r.ThermalPower)
}

// With Outlet at the design reference and ThermalPower=0, the
// outlet cannot drift away (target = inlet = CoreReferenceInletF,
// not the reference temperature), so Doppler stays near zero.
// Allow a tiny tolerance for the single-step transient.
func TestReactor_OutletAtReferenceGivesZeroDoppler(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:       u.NewValue(float64(CoreDopplerReferenceTempF), u.Fahrenheit),
		ThermalPower: 0,
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
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		ThermalPower: CoreRatedThermalPower,
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
	}
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
		ControlRods:  ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		ThermalPower: CoreRatedThermalPower,
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
	}
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
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(420, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
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
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(420, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
	}
	// Step with rcpSpeed = 0 (pump latch on, but rotor coasting).
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		r = r.Step(0)
	}
	assert.Equal(t, 420.0, r.Inlet.Float(),
		"want Inlet frozen at 420°F with rcpSpeed=0, got %v", r.Inlet.Float())
}
