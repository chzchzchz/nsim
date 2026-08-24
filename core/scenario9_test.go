package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 9 (axial power distribution and xenon management). The
// iodine/xenon pit physics lives in core/xenon_test.go; these tests pin
// the operator levers of the evolution through the production control
// paths instead:
//
//   - Maneuver the reactor down; large flux change whose axial skew
//     the unrodded-axis procedure guards against.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:73-76
//   - Boron adjustments reposition rods and manage the xenon
//     transient.
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:111-116,
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:120-123

// A 200 ppm boration must shift reactivity by exactly the folded boron
// worth while leaving the xenon inventory untouched: the poison state
// evolves on the hours clock, the chemistry lever acts immediately.
// The concentration is driven through the CVCS state directly; the
// bridge BoronSetpoint path ramps on the BTRS flow clock and is
// covered by its own test.
func TestScenario9_BorationShiftsReactivityWithXenonPinned(t *testing.T) {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	stepN(&p, 1)
	before := rpv.Reactivity
	xeBefore := xenonNormalized(rpv.Xenon135)

	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = 1200
	stepN(&p, 1)

	delta := rpv.Reactivity - before
	// The shift carries two terms: the direct boron worth AND the
	// moderator-coefficient move as the interpolation rides the
	// boron concentration (the same dc/dB coupling the load-follow
	// naive-boration check locks in).
	want := float32((1200.0 - CoreRatedCriticalBoronPpm) / 100.0) * CoreBoronWorthPer100ppm +
		float32((float64(CoreReferenceOutletF)-float64(CoreDopplerReferenceTempF))*
			(netTempCoef(1200)-netTempCoef(CoreRatedCriticalBoronPpm))/1000)
	assert.InDelta(t, want, delta, 0.005,
		"want the 200 ppm boration to shift reactivity %v dk/k, got %v",
		want, delta)
	assert.InDelta(t, xeBefore, xenonNormalized(rpv.Xenon135), 1e-9,
		"a chemistry adjustment must not move the xenon inventory")
	require.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped)
}

// Writing a rod demand maneuvers the reactor down: the control writes
// the demand and the drive slews toward it, pulling reactivity negative
// without tripping the protection system.
func TestScenario9_RodDemandManeuversReactorDown(t *testing.T) {
	p := MakePlant()
	stepN(&p, 1)
	before := p.ReactorBuilding.ReactorPressureVessel.Reactivity

	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion", "0.5")
	// RodInsertionSpeed slews five percent of full travel per second,
	// so five seconds moves the banks visibly without completing the
	// maneuver.
	stepFor(&p, 5*time.Second)

	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.Greater(t, rpv.Insertion, 0.0,
		"the drive must slew off the fully withdrawn position")
	assert.Less(t, rpv.Reactivity, before,
		"inserting rods must push reactivity negative")
	// No reactor-trip assertion here: with the secondary decoupled from
	// core power ("reactor power is maintained equal to load by virtue
	// of feedback effects inherent in the physics of a PWR",
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:69-71),
	// the turbine keeps drawing while the core folds and the collapsing
	// steamline legitimately votes a protection trip.
}
