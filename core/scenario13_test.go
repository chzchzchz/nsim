package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 13 (plant load maneuvering, reactor lead). The RPC layer's
// descent leg, rate clamp, bumpless engagement, and deadband direction
// are covered by core/rodcontrol_test.go; this file pins the evolution's
// closing steps through production paths:
//
//   - "(7)-(9) Return reactor power to 100% FP at 0.08 %/sec by using
//     the 'reactor power setpoint' pop-up [...] compare against full
//     power for differences."
//     specs/pwr_sim/chapter_3/page_036.txt:57-70
//   - In reactor lead the reactor power setpoint moves first and the
//     turbine follows.
//     specs/pwr_sim/chapter_3/page_036.txt:72-88

func TestScenario13_ReactorLeadReturnToFullPower(t *testing.T) {
	// Same width argument as the descent test in core/rodcontrol_test.go:
	// the integrator is linear in dt, so a 10 ms tick lands on identical
	// Runtime exceeds 5 s because the return leg itself is the subject
	// and is duration-bound: ten points at the modeled envelope pace of
	// 0.02 %/s takes 500 s of simulated coolant time that seeding
	// cannot shortcut.
	testDt := 10 * time.Millisecond

	// Start from the post-descent plateau instead of re-walking the
	// descent (its coverage lives in the RPC tests): inventories seeded
	// at their 90 percent equilibria, integrator parked at the plateau
	// demand exactly as the bumpless handoff would leave it.
	p := rpcReactorLeadPlant()
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		Watts(0.9 * float64(CoreRatedThermalPower)))
	p.PlantOperation.DemandedPowerSetpoint = 0.90
	// Park the coolant on the plateau's program line too: the outlet
	// paces at 50 F/hr, so leaving it at the full-load anchor would
	// spend the whole hold walking six points of program.
	p.ReactorBuilding.ReactorPressureVessel.Outlet = Temperature{
		Val: p.PlantOperation.referenceTempF(), Unit: u.Fahrenheit}

	setControl(t, &p, "PlantOperation/LeadLagMode", "1")
	setControl(t, &p, "PlantOperation/RodControlAuto", "true")
	setControl(t, &p, "PlantOperation/PowerSetpointTarget", "90")
	setControl(t, &p, "PlantOperation/PowerRateDemand", "0.02")

	rpv := &p.ReactorBuilding.ReactorPressureVessel

	const (
		settleSeconds = 180.0
		ascentSeconds = 500.0 // 0.10 / 0.0002 covers the ten-point return
		returnSeconds = 90.0
	)
	for i := 0; i < int(settleSeconds/testDt.Seconds()); i++ {
		p.Step(testDt)
		p.Update()
	}
	require.False(t, rpv.Trip, "the plateau hold must not trip the reactor")
	require.InDelta(t, 0.90, fissionFraction(rpv.FissionPower), 0.05,
		"seeded plateau holds near 90 percent fission power")

	// Return to full power at the same selectable rate and let the
	// loop stabilize.
	setControl(t, &p, "PlantOperation/PowerSetpointTarget", "100")
	tripped := false
	for i := 0; i < int((ascentSeconds+returnSeconds)/testDt.Seconds()); i++ {
		p.Step(testDt)
		p.Update()
		if rpv.Trip {
			tripped = true
		}
	}

	po := &p.PlantOperation
	pf := fissionFraction(rpv.FissionPower)
	assert.False(t, tripped, "the return maneuver must not trip the reactor")
	assert.InDelta(t, 1.0, po.DemandedPowerSetpoint, 1e-9,
		"the integrator returned to the full-power target")
	assert.GreaterOrEqual(t, pf, 0.95,
		"fission power settled back at full power, got %v", pf)
	assert.LessOrEqual(t, absF(rpv.Outlet.Float()-po.referenceTempF()),
		RodControlDeadbandF,
		"outlet temperature held inside the Tref deadband through the return")
	turbine := &p.TurbineBuilding.Turbine
	assert.InDelta(t, pf, turbine.Setpoint, 1e-9,
		"turbine load demand followed the reactor back up with no manual write")
}
