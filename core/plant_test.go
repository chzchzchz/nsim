package core

import (
	"testing"
	"time"
)

// When the reactor trips, Plant.Step must cascade the trip to the
// turbine: StopValve.Open drops to 0 and the turbine transitions
// to TurbineTripped within a few steps.
//
// Spec justification:
//   - "Turbine trip following a reactor trip." (item 2 of the
//     protective features list)
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:64
//   - "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
func TestPlant_ReactorTripCascadesToTurbine(t *testing.T) {
	p := MakePlant()
	// Bring the reactor to rated power; let the SG build pressure.
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = 1
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	stepN(&p, int(2*time.Second/TimeStep))
	// Manually trip the reactor.
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	// Step past StopValveCloseDelay.
	stepN(&p, int(100*time.Millisecond/TimeStep))
	if !(p.TurbineBuilding.Turbine.StopValve.Open == 0) {
		t.Fatalf("want StopValve.Open == 0 after reactor trip, got %v",
			p.TurbineBuilding.Turbine.StopValve.Open)
	}
	if p.TurbineBuilding.Turbine.State != TurbineTripped {
		t.Fatalf("want TurbineTripped after reactor trip, got %v",
			p.TurbineBuilding.Turbine.State)
	}
}

// With the reactor tripped, the condenser no longer receives full
// steam load from the turbine and Pressure relaxes toward the
// design vacuum.
//
// Spec justification:
//   - "Under normal operation the air is removed by steam jet air
//     ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
func TestPlant_CondenserVacuumRecoversAfterTrip(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = true
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	// Vacuum recovery only — no reactor or turbine dynamics needed.
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		p.TurbineBuilding.Condenser = p.TurbineBuilding.Condenser.Step(
			p.TurbineBuilding.Condenser.CirculatingWaterIn,
			true,
			true,
		)
	}
	if !(p.TurbineBuilding.Condenser.Pressure < AtmosphericPressure) {
		t.Fatalf("want vacuum below atmospheric, got %v",
			p.TurbineBuilding.Condenser.Pressure)
	}
}
