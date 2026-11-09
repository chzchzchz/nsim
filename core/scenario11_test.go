package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// Scenario 11 (boration to cold shutdown margin): with the BTRS
// recirculation pump lined up and the CVCS boron mode selector on
// Boration, RCS boron concentration rises at the design rate.
//
// "If the plant is to be taken to the cold shutdown condition, boration
// is initiated to bring the boron concentration to the cold shutdown
// value."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:50-52
func TestScenario11_BorationModeRaisesBoronAtRatedRate(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	start := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration

	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Boration")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")
	stepFor(&p, 10*time.Second, DefaultTimeStep)

	got := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	want := start + CVCSBorationRate*10.0
	assert.InDelta(t, want, got, 0.5,
		"boron must rise at CVCSBorationRate while the BTRS pump runs")
}

// At the derived cold shutdown boron concentration the core is at least
// 1 percent subcritical at the 140 F cold reference WITH NO ROD WORTH
// CREDITED (all-rods-out conservatism). The -1 pct margin and no-rod
// credit are modeling conventions; the chapter states the action but
// not a numeric margin (page_227.txt:54-55).
//
// Cold condition reference (<140 F):
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:95
func TestScenario11_ColdShutdownMarginHoldsWithoutRodCredit(t *testing.T) {
	t.Parallel()
	const (
		coldF        = 140.0
		targetBoron  = 2140.0 // solves rho <= -1.0 pct with rods out
		marginTarget = -1.0
	)

	// Sample on a coherent just-tripped state: coolant at the cold
	// reference, fission and decay heat at zero, xenon still at its
	// rated equilibrium (worth 0 - the margin must hold before the
	// poison decays away), rods fully withdrawn. Stepping the RPV
	// directly isolates the reactivity evaluation from plant-side
	// thermal transients.
	rpv := MakePlant().ReactorBuilding.ReactorPressureVessel
	rpv.Insertion = 0
	rpv.Inlet = Temperature{Val: coldF, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: coldF, Unit: u.Fahrenheit}
	rpv.FissionPower = 0
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0
	rpv.ControlRods.BoronConcentration = targetBoron

	rpv = rpv.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)

	assert.LessOrEqual(t, float64(rpv.Reactivity), marginTarget,
		"cold shutdown margin must hold with no rod credit, rho=%v", rpv.Reactivity)
	assert.Equal(t, targetBoron,
		rpv.ControlRods.BoronConcentration,
		"reactivity must be evaluated at the sampled boron value")
}
