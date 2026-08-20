package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
)

// With the pressurizer under-filled and the bubble-driven pressure
// model sitting below the heater target, energising the heaters drives
// pressure toward PressurizerHeaterTarget.
func TestPressurizer_HeatersDrivePressureUp(t *testing.T) {
	p := MakePlant()
	// Size the bubble so the bubble-derived pressure sits a few
	// hundred psi below the heater target; equalise primary-loop
	// pressures so the surge-line edges sit at zero deltaP and the
	// graph does not drain the pressurizer over the 1 s test window.
	p.ReactorBuilding.ReactorPressureVessel.Pressure = 2000
	p.ReactorBuilding.SteamGenerator.Pressure = 2000
	p.TurbineBuilding.HeaterDrainTank.Pressure = 2000
	p.ReactorBuilding.Pressurizer.Volume = u.NewValue(1000, u.Gallon)
	p.ReactorBuilding.Pressurizer.Water = u.NewValue(800, u.Gallon)
	p.ReactorBuilding.Pressurizer.Pressure = 2000
	p.ReactorBuilding.Pressurizer.HeatersOn = true

	before := p.ReactorBuilding.Pressurizer.Pressure
	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	if !(after > before) {
		t.Fatalf("Pressure should rise while HeatersOn: before %v after %v", before, after)
	}
	if after > PressurizerHeaterTarget {
		t.Fatalf("Pressure overshot target: after %v target %v", after, PressurizerHeaterTarget)
	}
}

// With the pressurizer over-pressurised and the spray valve open, the
// step bleeds pressure down toward PressurizerSprayMin.
func TestPressurizer_SprayDrivesPressureDown(t *testing.T) {
	p := MakePlant()
	// Size the bubble so the bubble-derived pressure is well above
	// the spray minimum; equalise primary-loop pressures so the
	// surge-line edges are at zero deltaP.
	p.ReactorBuilding.ReactorPressureVessel.Pressure = 2500
	p.ReactorBuilding.SteamGenerator.Pressure = 2500
	p.TurbineBuilding.HeaterDrainTank.Pressure = 2500
	p.ReactorBuilding.Pressurizer.Volume = u.NewValue(1000, u.Gallon)
	p.ReactorBuilding.Pressurizer.Water = u.NewValue(960, u.Gallon)
	p.ReactorBuilding.Pressurizer.Pressure = 2500
	p.ReactorBuilding.Pressurizer.SprayOpen = 1.0

	before := p.ReactorBuilding.Pressurizer.Pressure
	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	if !(after < before) {
		t.Fatalf("Pressure should drop while SprayOpen: before %v after %v", before, after)
	}
	if after < PressurizerSprayMin {
		t.Fatalf("Pressure dropped below spray min: after %v min %v", after, PressurizerSprayMin)
	}
}

// A surge-line inflow (graph-driven, water level rises) shrinks the
// steam bubble, which in this model pulls pressure down.
func TestPressurizer_SurgeLineInflowDropsPressure(t *testing.T) {
	p := MakePlant()
	// Open the surge-line safety valve and pressurise the RPV above
	// the pressurizer so the graph pushes water from RPV into the
	// pressurizer. Equalise the SG so the SG<->RPV edge is at zero
	// deltaP and only the surge-line drives the pressurizer.
	p.ReactorBuilding.SafetyValve.Open = 1
	p.ReactorBuilding.SteamGenerator.Pressure = 3000
	p.ReactorBuilding.ReactorPressureVessel.Pressure = 3000
	p.ReactorBuilding.Pressurizer.Volume = u.NewValue(1000, u.Gallon)
	p.ReactorBuilding.Pressurizer.Water = u.NewValue(800, u.Gallon) // 200 gal bubble -> 2000 psi
	p.ReactorBuilding.Pressurizer.Pressure = 2000
	before := p.ReactorBuilding.Pressurizer.Pressure

	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	if !(after < before) {
		t.Fatalf("Pressure should drop as bubble shrinks: before %v after %v", before, after)
	}
	if !(p.ReactorBuilding.Pressurizer.Water.Float() > 800) {
		t.Fatalf("Water should have risen via surge-line inflow, got %v",
			p.ReactorBuilding.Pressurizer.Water.Float())
	}
}

// When pressure exceeds the PORV trip setpoint the valve opens.
func TestPressurizer_PORVOpensOverTrip(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Volume = u.NewValue(1000, u.Gallon)
	p.ReactorBuilding.Pressurizer.Water = u.NewValue(500, u.Gallon) // big bubble
	p.ReactorBuilding.Pressurizer.Pressure = PressurizerPORVTrip + 100

	stepN(&p, 1)

	if p.ReactorBuilding.PORV.Open == 0 {
		t.Fatalf("PORV should open over trip threshold, Open=%v",
			p.ReactorBuilding.PORV.Open)
	}
}
