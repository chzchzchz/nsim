package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
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
	p.ReactorBuilding.Pressurizer.Volume = Volume{Val: 1000, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Water = Volume{Val: 800, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Pressure = 2000
	p.ReactorBuilding.Pressurizer.HeatersOn = true

	before := p.ReactorBuilding.Pressurizer.Pressure
	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	assert.Greater(t, after, before,
		"Pressure should rise while HeatersOn: before %v after %v", before, after)
	assert.LessOrEqual(t, after, PressurizerHeaterTarget,
		"Pressure overshot target: after %v target %v", after, PressurizerHeaterTarget)
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
	p.ReactorBuilding.Pressurizer.Volume = Volume{Val: 1000, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Water = Volume{Val: 960, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Pressure = 2500
	p.ReactorBuilding.Pressurizer.SprayOpen = 1.0

	before := p.ReactorBuilding.Pressurizer.Pressure
	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	assert.Less(t, after, before,
		"Pressure should drop while SprayOpen: before %v after %v", before, after)
	assert.GreaterOrEqual(t, after, PressurizerSprayMin,
		"Pressure dropped below spray min: after %v min %v", after, PressurizerSprayMin)
}

// Inflow into the pressurizer raises the water level, shrinking the
// steam bubble and pulling the bubble-derived pressure down. Since
// ReactorPressureVessel left the fluid graph (the primary and
// secondary loops are closed cycles that exchange only heat across
// the SG tubes:
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_064.txt:20-27),
// tracked inflow arrives through the CVCS charging line:
//   "pressurizer level represents a balance of injection water flow
//    into the RCS and water letdown flow into the Chemical and
//    Volume Control System."
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
func TestPressurizer_InflowShrinksBubbleDropsPressure(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Volume = Volume{Val: 1000, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Water = Volume{Val: 800, Unit: u.Gallon} // 200 gal bubble -> 2000 psi
	p.ReactorBuilding.Pressurizer.Pressure = 2000
	// Charge-only window: letdown stays shut so the net RCS inflow
	// is the full design charging flow.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false

	waterBefore := p.ReactorBuilding.Pressurizer.Water.Float()
	before := p.ReactorBuilding.Pressurizer.Pressure

	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	assert.Less(t, after, before,
		"Pressure should drop as bubble shrinks: before %v after %v", before, after)
	assert.Greater(t, p.ReactorBuilding.Pressurizer.Water.Float(), waterBefore,
		"Water should have risen via charging inflow")
}

// When pressure exceeds the PORV trip setpoint the valve opens.
func TestPressurizer_PORVOpensOverTrip(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Volume = Volume{Val: 1000, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Water = Volume{Val: 500, Unit: u.Gallon} // big bubble
	p.ReactorBuilding.Pressurizer.Pressure = PressurizerPORVTrip + 100

	stepN(&p, 1)

	assert.NotZero(t, p.ReactorBuilding.PORV.Open,
		"PORV should open over trip threshold, Open=%v", p.ReactorBuilding.PORV.Open)
}
