package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With the pressurizer pressure sitting below the heater target,
// energising the heaters drives pressure toward PressurizerHeaterTarget
// without overshooting it.
func TestPressurizer_HeatersDrivePressureUp(t *testing.T) {
	p := MakePlant()
	// Equalise primary-loop pressures so the surge-line edges sit at
	// zero deltaP and the graph does not drain the pressurizer over
	// the 1 s test window.
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
	// Equalise primary-loop pressures so the surge-line edges are at
	// zero deltaP.
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

// Charging inflow raises the liquid inventory while pressure stays on
// its own loop: level control (charging versus letdown) is independent
// of pressure control (heaters and spray), so inflow alone must not
// move pressure beyond the idle-cooldown drift. Since
// ReactorPressureVessel left the fluid graph (the primary and
// secondary loops are closed cycles that exchange only heat across
// the SG tubes:
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_064.txt:20-27),
// tracked inflow arrives through the CVCS charging line:
//   "pressurizer level represents a balance of injection water flow
//    into the RCS and water letdown flow into the Chemical and
//    Volume Control System."
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
func TestPressurizer_InflowRaisesLevelWithoutMovingPressure(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Volume = Volume{Val: 1000, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Water = Volume{Val: 800, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Pressure = 2000
	// Charge-only window: letdown stays shut so the net RCS inflow
	// is the full design charging flow.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false

	waterBefore := p.ReactorBuilding.Pressurizer.Water.Float()
	before := p.ReactorBuilding.Pressurizer.Pressure

	stepN(&p, int(time.Second/TimeStep))

	after := p.ReactorBuilding.Pressurizer.Pressure
	wantDrift := float64(PressurizerAmbientDecay)
	assert.InDelta(t, float64(before), float64(after), wantDrift+1,
		"charging inflow must not drive pressure; only idle drift allowed: " +
			"before %v after %v", before, after)
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

// Heaters take a water-solid pressurizer to the saturation target and
// boil liquid into the newly formed steam space.
//
// "All pressurizer heaters are energized to start the heatup of the
// pressurizer necessary to form the steam bubble."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-17
// The ~300 gal displacement expectation is a wiring check recomputed
// from PressurizerHeaterRate and PressurizerBubbleStiffness, not an
// independently sourced figure.
func TestPressurizer_BubbleFormationFromWaterSolid(t *testing.T) {
	pz := Pressurizer{
		Volume:    Volume{Val: 1500, Unit: u.Gallon},
		Water:     Volume{Val: 1500, Unit: u.Gallon},
		Pressure:  PressurizerBasePressure,
		HeatersOn: true,
	}
	steps := int(12 * time.Second / TimeStep)
	for i := 0; i < steps; i++ {
		pz, _ = pz.Step(nil, nil, nil)
	}

	assert.InDelta(t, float64(PressurizerHeaterTarget), float64(pz.Pressure), 10,
		"heaters should bring pressure to the saturation target")
	assert.InDelta(t, 1200.0, pz.Water.Float(), 2,
		"boiloff should convert ~300 gal of liquid into steam")
	assert.Greater(t, pz.SteamBubble.Float(), 0.0,
		"a steam bubble should have formed")
}

// Letdown drains liquid inventory while heaters hold pressure: level
// control (charging versus letdown) is independent of pressure control
// (heaters and spray).
//
// "Flow in the letdown line is increased by the operator to lower
// pressurizer level to the normal range of 20 to 80 percent"
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:20-23
// "RCS pressure is maintained by adjustment of the pressurizer
// heaters and/or pressurizer spray."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:28-29
func TestPressurizer_LetdownDrainHoldsPressure(t *testing.T) {
	pz := Pressurizer{
		Volume:    Volume{Val: 1500, Unit: u.Gallon},
		Water:     Volume{Val: 1200, Unit: u.Gallon},
		Pressure:  PressurizerHeaterTarget,
		HeatersOn: true,
	}
	netLetdown := (CVCSDesignLetdownFlow.Float() - CVCSDesignChargingFlow.Float()) *
		TimeStep.Seconds()

	for pz.Water.Float() > 375 {
		pz, _ = pz.Step(nil, nil, nil)
		pz.Water = vol(pz.Water, pz.Water.Float()-netLetdown)

		require.Less(t, float64(pz.Pressure), float64(PressurizerPORVTrip),
			"controlled drain must never lift the PORV")
		require.InDelta(t, float64(PressurizerHeaterTarget), float64(pz.Pressure), 50,
			"heaters must hold pressure throughout the drain")
		require.GreaterOrEqual(t, pz.Water.Float(), 0.0)
	}

	level := 100.0 * pz.Water.Float() / pz.Volume.Float()
	assert.InDelta(t, 25.0, level, 5,
		"drain should end near the no-load level")
}

// With heaters off and spray shut the steam space condenses back into
// the liquid inventory and pressure settles downward. The decay rate
// is a modeling convention (PressurizerAmbientDecay), so only the
// direction and bounds are asserted, not an exact figure. (The source
// page's cooldown narrative is OCR-truncated mid-sentence; see
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-82)
func TestPressurizer_IdleCooldownCondensesBubble(t *testing.T) {
	pz := Pressurizer{
		Volume:   Volume{Val: 1500, Unit: u.Gallon},
		Water:    Volume{Val: 1200, Unit: u.Gallon},
		Pressure: PressurizerHeaterTarget,
	}
	steps := int(time.Second / TimeStep)
	for i := 0; i < steps; i++ {
		pz, _ = pz.Step(nil, nil, nil)
	}

	assert.Less(t, float64(pz.Pressure), float64(PressurizerHeaterTarget),
		"idle cooldown should bleed pressure below its start")
	assert.GreaterOrEqual(t, float64(pz.Pressure), float64(PressurizerBasePressure),
		"idle cooldown should not fall below base pressure")
	assert.Greater(t, pz.Water.Float(), 1200.0,
		"condensed steam should return to the liquid inventory")
	assert.LessOrEqual(t, pz.Water.Float(), 1202.0,
		"one second of convention-rate condensation should stay small")
}
