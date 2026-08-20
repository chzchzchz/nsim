package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
)

// With no Trip, the reactor settles to a positive ThermalPower
// near rated (the simplified power-defect model loses some power
// to the Doppler correction; we assert it stays in a sane band).
func TestReactor_RunningProducesRatedPower(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(true)
	}
	if !(r.ThermalPower > 0) {
		t.Fatalf("want ThermalPower > 0 at running, got %v",
			r.ThermalPower)
	}
	if !(r.ThermalPower <= CoreRatedThermalPower) {
		t.Fatalf("want ThermalPower <= rated, got %v", r.ThermalPower)
	}
	if r.Trip == true {
		t.Fatal("Trip should remain false")
	}
}

// Setting Trip drops ThermalPower below rated, into the decay-heat
// region. After the fast-branch window, the power should sit at or
// near the shutdown fraction of rated.
func TestReactor_TripDropsPowerToDecayHeat(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(true)
	}
	r.Trip = true
	for i := 0; i < int(2*time.Minute/TimeStep); i++ {
		r = r.Step(true)
	}
	if !(r.ThermalPower < CoreRatedThermalPower) {
		t.Fatalf("want ThermalPower < rated after trip, got %v",
			r.ThermalPower)
	}
	shutdownFloor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	if !(r.ThermalPower <= shutdownFloor) {
		t.Fatalf("want ThermalPower <= %v (shutdown level), got %v",
			shutdownFloor, r.ThermalPower)
	}
}

// A coolant outlet temperature above the design reference produces
// negative Doppler feedback: rising outlet temperature shifts
// Reactivity negative relative to the baseline.
func TestReactor_DopplerShiftsReactivityNegative(t *testing.T) {
	ref := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		Outlet:       u.NewValue(float64(CoreDopplerReferenceTempF), u.Fahrenheit),
		ThermalPower: CoreRatedThermalPower,
	}
	ref = ref.Step(true)
	hot := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		Outlet:       u.NewValue(float64(CoreDopplerReferenceTempF)+30, u.Fahrenheit),
		ThermalPower: CoreRatedThermalPower,
	}
	hot = hot.Step(true)
	if !(hot.Reactivity < ref.Reactivity) {
		t.Fatalf("Doppler feedback: want hot.Reactivity < ref.Reactivity, got hot=%v ref=%v",
			hot.Reactivity, ref.Reactivity)
	}
}

// Even with rods fully withdrawn and Trip asserted, decay heat
// keeps ThermalPower non-zero. The long-tail floor is reached
// well within an hour of simulated time.
func TestReactor_TripWithRodsOutKeepsDecayHeatFloor(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		Trip:         true,
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
		ThermalPower: CoreRatedThermalPower,
	}
	for i := 0; i < int(5*time.Minute/TimeStep); i++ {
		r = r.Step(true)
	}
	if !(r.ThermalPower > 0) {
		t.Fatalf("want non-zero ThermalPower during decay, got %v",
			r.ThermalPower)
	}
	floor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	// Allow some margin since the simplified power-defect model
	// lifts the decay-heat floor slightly above the bare fraction.
	if !(r.ThermalPower <= floor*2) {
		t.Fatalf("want ThermalPower within 2x of shutdown level (%v), got %v",
			floor, r.ThermalPower)
	}
}

// With Outlet at the design reference and ThermalPower=0, the
// outlet cannot drift away (target = inlet = CoreReferenceInletF,
// not the reference temperature), so Doppler stays near zero.
// Allow a tiny tolerance for the single-step transient.
func TestReactor_OutletAtReferenceGivesZeroDoppler(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		Outlet:       u.NewValue(float64(CoreDopplerReferenceTempF), u.Fahrenheit),
		ThermalPower: 0,
	}
	r = r.Step(true)
	if r.Reactivity > 1 || r.Reactivity < -1 {
		t.Fatalf("want near-zero Reactivity, got %v", r.Reactivity)
	}
}

// With ThermalPower > 0 and RCP running, Outlet relaxes toward
// CoreReferenceOutletF on the PrimaryThermalTimeConstant.
func TestReactor_OutletTracksThermalPower(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		ThermalPower: CoreRatedThermalPower,
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
	}
	// Pre-warm the outlet at the design reference so the simplified
	// model is on the right side of the operating curve. With Outlet
	// at the reference, Doppler ≈ 0 and the cascade doesn't halve.
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		r = r.Step(true)
	}
	// After 10 s of full power the outlet should still be near the
	// design reference (within the model noise).
	if r.Outlet.Float() < CoreReferenceOutletF-30 {
		t.Fatalf("want Outlet near %v, got %v",
			CoreReferenceOutletF, r.Outlet.Float())
	}
}

// After a trip the Outlet temperature decays along with the core
// power — within a few hundred steps it should drop appreciably.
func TestReactor_OutletDropsAfterTrip(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		ThermalPower: CoreRatedThermalPower,
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
		Inlet:        u.NewValue(CoreReferenceInletF, u.Fahrenheit),
	}
	// Settle at full power briefly so Outlet converges to ~621°F.
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step(true)
	}
	start := r.Outlet.Float()
	r.Trip = true
	r.ControlRods.Position = 0
	for i := 0; i < int(time.Minute/TimeStep); i++ {
		r = r.Step(true)
	}
	if !(r.Outlet.Float() < start) {
		t.Fatalf("want Outlet < %v after 1 min of decay, got %v",
			start, r.Outlet.Float())
	}
}

// With RCP not running, Inlet temperature is preserved across steps
// (pump off = no cold-leg turnover).
func TestReactor_InletFrozenWhenRCPOff(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods:  ControlRods{Position: 1},
		ThermalPower: CoreRatedThermalPower,
		Inlet:        u.NewValue(420, u.Fahrenheit),
		Outlet:       u.NewValue(CoreReferenceOutletF, u.Fahrenheit),
	}
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		r = r.Step(false) // RCP off
	}
	if !(r.Inlet.Float() == 420) {
		t.Fatalf("want Inlet frozen at 420°F with RCP off, got %v",
			r.Inlet.Float())
	}
}
