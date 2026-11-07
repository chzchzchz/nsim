package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setPressurizerPsi holds the pressurizer at a target pressure. Under
// the decoupled two-loop model pressure is an independent saturation
// state, so the helper asserts both the state itself and the matching
// liquid inventory (the step's boiloff/relief bookkeeping consumes
// bubble volume from that inventory).
func setPressurizerPsi(p *Plant, psia float64) {
	bubble := (psia - float64(PressurizerHeaterTarget)) /
		PressurizerBubbleStiffness
	p.ReactorBuilding.Pressurizer.Water =
		vol(p.ReactorBuilding.Pressurizer.Water,
			p.ReactorBuilding.Pressurizer.Volume.Float()-bubble)
	p.ReactorBuilding.Pressurizer.Pressure = Pressure(psia)
}

// holdPressure re-asserts a pressurizer pressure before each step so a
// test can drive sustained relief without heaters or the surge line
// interfering.
func holdPressure(p *Plant, psia float64, steps int) {
	for range steps {
		setPressurizerPsi(p, psia)
		p.Step(DefaultTimeStep)
		p.Update()
	}
}

// Acceptance 2: the code safety valves self-actuate at
// PressurizerSafetySetpoint, above the PORV setpoint, with no operator
// action. "If system pressure continues to rise, self-actuating
// ASME-code safety valves will open."
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-32
func TestPressurizer_SafetyValveAutoActuates(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	setPressurizerPsi(&p, float64(PressurizerSafetySetpoint)+50)
	p.Step(DefaultTimeStep)
	p.Update()
	assert.NotZero(t, p.ReactorBuilding.SafetyValve.Open,
		"safety valve must auto-actuate above its setpoint")
	assert.NotZero(t, p.ReactorBuilding.PORV.Open,
		"PORV must also be open at this pressure")
}

// Acceptance 1 (wiring check on the discharge path): sustained relief
// depressurizes the pressurizer toward base while the bubble lasts.
// The rate constant is derived; this pins that opening relief moves
// pressure down, not up or nowhere.
func TestPressurizer_ReliefLowersPressure(t *testing.T) {
	t.Parallel()
	hotLeg := Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	prz := Pressurizer{
		Volume: Volume{Val: 1200, Unit: u.Gallon},
		Water:  Volume{Val: 780, Unit: u.Gallon},
	}
	prz.Pressure = PressurizerPORVTrip + 50
	start := float64(prz.Pressure)
	porv := Valve{Open: 1}
	block := Valve{Open: 1}
	var safety Valve
	for range int(5 * time.Second / DefaultTimeStep) {
		prz, _ = prz.Step(DefaultTimeStep, hotLeg, &porv, &safety, &block, 1, 0)
	}
	assert.Less(t, float64(prz.Pressure), start,
		"relief must depressurize the pressurizer")
	assert.GreaterOrEqual(t, float64(prz.Pressure),
		float64(PressurizerSatPressure(hotLeg.Float())),
		"discharge clamps at the saturation floor for the hot-leg coolant")
}

// Acceptance 3: relief fills the PRT and containment pressure does not
// move from relief alone while the rupture disc holds.
func TestRelief_RoutesToPRTNotContainment(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	startLevel := p.ReactorBuilding.PressurizedReliefTank.Level.Float()
	holdPressure(&p, float64(PressurizerPORVTrip)+50,
		int(time.Second/DefaultTimeStep))
	assert.Greater(t, p.ReactorBuilding.PressurizedReliefTank.Level.Float(),
		startLevel, "PRT level must rise under sustained relief")
	assert.False(t, p.ReactorBuilding.PRTRuptureDiscBurst,
		"disc must stay intact while the tank has headspace")
	assert.InDelta(t, float64(AtmosphericPressure),
		float64(p.ReactorBuilding.ContainmentPressure), 0.01,
		"containment must not pressurize while the disc holds")
}

// Acceptance 4 end-to-end: an overfilled PRT bursts the disc, continued
// relief raises containment pressure, and the CSS auto-start pulls it
// back down once actuated.
//
// "A rupture disc vents the tank to the containment if design pressure
// is exceeded." specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:37-38
func TestRelief_RuptureDiscBurstVentsAndCSSCatches(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection

	// Nearly fill the tank so a few seconds of relief bursts the disc.
	tank := &p.ReactorBuilding.PressurizedReliefTank
	tank.Level = vol(tank.Level, PressurizedReliefTankCapacityGal-5)
	holdPressure(&p, float64(PressurizerPORVTrip)+50,
		int(10*time.Second/DefaultTimeStep))
	require.True(t, p.ReactorBuilding.PRTRuptureDiscBurst,
		"disc must burst once the tank reaches capacity")

	peak := float64(p.ReactorBuilding.ContainmentPressure)
	assert.Greater(t, peak, float64(AtmosphericPressure),
		"containment must pressurize after the disc bursts")

	// Keep relieving; once the CSS starts it must turn the rise around.
	for range int(60 * time.Second / DefaultTimeStep) {
		setPressurizerPsi(&p, float64(PressurizerPORVTrip)+50)
		before := float64(p.ReactorBuilding.ContainmentPressure)
		p.Step(DefaultTimeStep)
		p.Update()
		if p.AuxiliaryBuilding.ContainmentSpraySystem.Running &&
			float64(p.ReactorBuilding.ContainmentPressure) < before {
			break // spray caught the vent
		}
	}
	assert.True(t, p.AuxiliaryBuilding.ContainmentSpraySystem.Running,
		"CSS must auto-start on high containment pressure")
	assert.Less(t, float64(p.ReactorBuilding.ContainmentPressure),
		CSSContainmentPressureSetpoint,
		"spray must drag containment back below the setpoint")
}

// Acceptance 5: with the block valve shut, no discharge reaches the PRT
// even with the PORV commanded open — "Remotely operated stop valves
// are provided to isolate these relief valves should excessive leakage
// occur." specs/the_westinghouse_pressurized_water_react/chapter_3/page_072.txt:44-47
func TestBlockValve_GatesReliefDischarge(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.BlockValve = Valve{}
	startLevel := p.ReactorBuilding.PressurizedReliefTank.Level.Float()
	holdPressure(&p, float64(PressurizerPORVTrip)+50,
		int(time.Second/DefaultTimeStep))
	assert.Equal(t, startLevel,
		p.ReactorBuilding.PressurizedReliefTank.Level.Float(),
		"shut block valve must isolate the PRT from discharge")
	assert.Greater(t, float64(p.ReactorBuilding.Pressurizer.Pressure),
		float64(PressurizerPORVTrip)-60,
		"pressure must stay near setpoint when relief is isolated")
}

// Acceptance 6: the PRT spray adds inventory only while open.
func TestPRTSpray_AddsInventoryWhileOpen(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.PRTSprayValve = Valve{Open: 1}
	startLevel := p.ReactorBuilding.PressurizedReliefTank.Level.Float()
	holdPressure(&p, float64(PressurizerSatPressure(CoreReferenceOutletF)),
		int(time.Second/DefaultTimeStep))
	assert.Greater(t, p.ReactorBuilding.PressurizedReliefTank.Level.Float(),
		startLevel, "open spray must add PRT inventory")

	p2 := MakePlant()
	holdPressure(&p2, float64(PressurizerSatPressure(CoreReferenceOutletF)),
		int(time.Second/DefaultTimeStep))
	assert.Equal(t, startLevel,
		p2.ReactorBuilding.PressurizedReliefTank.Level.Float(),
		"closed spray must leave PRT level untouched")
}
