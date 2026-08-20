package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
)

// With no Trip, ThermalPower should equal CoreRatedThermalPower.
func TestReactor_RunningProducesRatedPower(t *testing.T) {
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Position: 1},
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step()
	}
	if r.ThermalPower != CoreRatedThermalPower {
		t.Fatalf("want ThermalPower = CoreRatedThermalPower (%v), got %v",
			CoreRatedThermalPower, r.ThermalPower)
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
		ControlRods: ControlRods{Position: 1},
	}
	for i := 0; i < int(time.Second/TimeStep); i++ {
		r = r.Step()
	}
	r.Trip = true
	for i := 0; i < int(2*time.Minute/TimeStep); i++ {
		r = r.Step()
	}
	if !(r.ThermalPower < CoreRatedThermalPower) {
		t.Fatalf("want ThermalPower < rated after trip, got %v",
			r.ThermalPower)
	}
	// Should be at or below the shutdown fraction.
	shutdownFloor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	if !(r.ThermalPower <= shutdownFloor) {
		t.Fatalf("want ThermalPower <= %v (shutdown level), got %v",
			shutdownFloor, r.ThermalPower)
	}
}

// A coolant outlet temperature above the design reference produces
// negative Doppler feedback. Raising the outlet temperature must
// shift Reactivity negative relative to the baseline.
func TestReactor_DopplerShiftsReactivityNegative(t *testing.T) {
	ref := ReactorPressureVessel{
		ControlRods: ControlRods{Position: 1},
		Outlet:      Nozzle{Part: Part{Temperature: u.NewValue(float64(CoreDopplerReferenceTempF), u.Fahrenheit)}},
	}
	ref = ref.Step()
	hot := ReactorPressureVessel{
		ControlRods: ControlRods{Position: 1},
		Outlet:      Nozzle{Part: Part{Temperature: u.NewValue(float64(CoreDopplerReferenceTempF)+30, u.Fahrenheit)}},
	}
	hot = hot.Step()
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
		ControlRods: ControlRods{Position: 1},
		Trip:        true,
	}
	// Step past the fast branch (50 s) and well into the slow
	// branch; power is in the shutdown-decay region.
	for i := 0; i < int(5*time.Minute/TimeStep); i++ {
		r = r.Step()
	}
	if !(r.ThermalPower > 0) {
		t.Fatalf("want non-zero ThermalPower during decay, got %v",
			r.ThermalPower)
	}
	floor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	if !(r.ThermalPower <= floor) {
		t.Fatalf("want ThermalPower at or below shutdown level (%v), got %v",
			floor, r.ThermalPower)
	}
}
