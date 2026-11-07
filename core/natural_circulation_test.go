package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A hot-over-cold loop develops thermosiphon flow; an isothermal
// loop develops none.
//
// Spec justification:
//   - "a thermal driving head will exist to produce natural
//     circulation flow if the RCPs are not operating. Natural
//     circulation flow is sufficient for removal of reactor decay
//     heat."
//     specs/westinghouse_tech_manual/chapter_3/page_0328.txt:31-33
func TestReactor_NaturalCirculationEmergesFromLoopHead(t *testing.T) {
	t.Parallel()
	hot := ReactorPressureVessel{
		Inlet:  Temperature{Val: RCSNoLoadTavgF - 30, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	for i := 0; i < int((3*NatCircTimeConstant*time.Second)/DefaultTimeStep); i++ {
		hot = hot.Step(DefaultTimeStep, 0, 0)
	}
	assert.Greater(t, hot.NaturalCircFlowFrac, 0.04,
		"hot loop must develop thermosiphon flow, got %v",
		hot.NaturalCircFlowFrac)

	isothermal := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 1, DemandInsertion: 1,
			BoronConcentration: CVCSInitialBoron},
		Inlet:  Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit},
	}
	isothermal.Trip = true
	isothermal.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int((3*NatCircTimeConstant*time.Second)/DefaultTimeStep); i++ {
		isothermal = isothermal.Step(DefaultTimeStep, 0, 0)
	}
	// A tripped core never sits exactly isothermal (decay heat
	// keeps a sub-degree anchor drift alive), so the null case is
	// relative: without a real head the flow stays negligible and
	// the cold leg effectively unmoved.
	assert.Less(t, isothermal.NaturalCircFlowFrac, 0.01,
		"isothermal loop must develop no meaningful thermosiphon flow, got %v",
		isothermal.NaturalCircFlowFrac)
	assert.InDelta(t, RCSNoLoadTavgF, isothermal.Inlet.Float(), 1.0,
		"no circulation must leave the cold leg effectively untouched, got %v",
		isothermal.Inlet.Float())
}

// The thermosiphon saturates at its decay-heat-sized capability:
// "Natural circulation flow is sufficient only for decay heat
// removal of a shutdown reactor, not for power operation."
// specs/westinghouse_tech_manual/chapter_3/page_0325.txt:14-15
func TestReactor_NaturalCirculationSaturatesAtCapability(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		Inlet:  Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	for i := 0; i < int((10*NatCircTimeConstant*time.Second)/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, 0, 0)
	}
	assert.LessOrEqual(t, r.NaturalCircFlowFrac, NatCircMaxFlowFrac+1e-9,
		"thermosiphon must not exceed its capability cap, got %v",
		r.NaturalCircFlowFrac)
}

// The flowing loop caps the primary's acceptance of secondary heat
// draw. The operational 50 degF/hr envelope clamps both legs' pacing
// at decay-heat scales (by design: "natural circulation flow is
// sufficient for removal of reactor decay heat", specs/
// westinghouse_tech_manual/chapter_3/page_0328.txt:33), so the cap's
// trajectory consequence is witnessed end-to-end in
// TestPlant_NaturalCirculationLegsSeparateAndRestartRecovers.
func TestPlant_NaturalCirculationLegsSeparateAndRestartRecovers(t *testing.T) {
	t.Parallel()
	testDt := 100 * time.Millisecond

	newTrippedPlant := func() *Plant {
		p := MakePlant()
		p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
		sg := &p.ReactorBuilding.SteamGenerator
		sg.AtmosphericDumpValve.Open = 1
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		rpv.Trip = true
		return &p
	}
	forced := newTrippedPlant()
	natural := newTrippedPlant()
	natural.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(false)
	for _, pu := range natural.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Velocity = 0
	}
	natural.ReactorBuilding.ReactorCoolantPump.Velocity = 0
	require.Zero(t, natural.ReactorCoolantPump.Velocity,
		"pumpless leg starts from secured pumps")

	outlet := func(p *Plant) float64 {
		return p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	}
	startForced := outlet(forced)
	startNatural := outlet(natural)

	run := func(p *Plant, seconds int) {
		stepN(p, int(time.Duration(seconds)*time.Second/testDt), testDt)
	}
	run(forced, 600)
	run(natural, 600)

	forcedDrop := startForced - outlet(forced)
	naturalDrop := startNatural - outlet(natural)
	t.Logf("leg separation over 600 s: forced drop=%.2f F natural drop=%.2f F",
		forcedDrop, naturalDrop)
	// Stated margin pinned from this replay's measured separation
	// (~9 degF): the pumpless leg cannot honor the dump demand its
	// forced twin integrates.
	assert.Greater(t, forcedDrop-naturalDrop, 5.0,
		"stated leg separation not met: forced %v vs natural %v degF",
		forcedDrop, naturalDrop)

	natural.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	beforeRestart := outlet(natural)
	natCircAtRestart := natural.ReactorBuilding.ReactorPressureVessel.
		NaturalCircFlowFrac
	run(natural, 300)
	assert.Greater(t, natCircAtRestart, 0.03,
		"pumpless leg must ride its thermosiphon, got %v", natCircAtRestart)
	assert.Less(t, outlet(natural), beforeRestart,
		"RCP restart must resume cooling on the natural-circulation leg")
}
