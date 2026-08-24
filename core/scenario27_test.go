package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// Scenario 27 dropped rod bank: once one bank has dropped (large
// sustained negative reactivity), the automatic power control system
// responds by withdrawing rod demand at its capped slew rate - an
// attempt that cannot recover the worth before the protective trip.
//
//   - Catalog: plans/x/scenarios.txt item 27.
//   - Malfunction class (secondary training source): "One bank of dark
//     rods drops [...], imparting large negative reactivity into the
//     core", and the control system withdraws rods in response.
//     specs/pwr_sim/chapter_4/page_049.txt:61-67
//     specs/pwr_sim/chapter_4/page_050.txt:1-9
//   - Withdrawal authority is the demand slew cap
//     (RodCtrlMaxDemandSlewPerSec), a named production constant.
func TestScenario27_RPCWithdrawsAtCappedRateAfterDrop(t *testing.T) {
	t.Parallel()

	// Post-drop state: actual and demanded insertion parked deep, core
	// power collapsed to 30 percent under roughly -3.9 percent delta-k,
	// turbine demand still at rated so the program point stays 100
	// percent.
	c := ControlRods{
		Insertion:       0.85,
		DemandInsertion: 0.85,
		AutoControl:     true,
	}
	steps := int(1 * time.Second / TimeStep)
	for i := 0; i < steps; i++ {
		c.applyAutoControl(
			Temperature{Val: 621, Unit: u.Fahrenheit}, 1.0, 0.30, -3.9)
	}

	withdrawn := 0.85 - c.DemandInsertion
	assert.Greater(t, withdrawn, 0.0,
		"the controller must answer the drop with withdrawal")
	assert.LessOrEqual(t, withdrawn,
		RodCtrlMaxDemandSlewPerSec*float64(steps)*TimeStep.Seconds()*1.01,
		"withdrawal runs at the demand slew cap")
	assert.GreaterOrEqual(t, withdrawn,
		RodCtrlMaxDemandSlewPerSec*float64(steps)*TimeStep.Seconds()*0.99,
		"sustained large error keeps the cap pinned")

	// Balance hold: at the program point with balanced reactivity the
	// deadband returns before any integration, so a compensated plant
	// does not hunt.
	var held ControlRods
	held.AutoControl = true
	held.DemandInsertion = 0.30
	for i := 0; i < steps; i++ {
		held.applyAutoControl(
			Temperature{Val: 621, Unit: u.Fahrenheit}, 0.30, 0.30, 0)
	}
	assert.Equal(t, 0.30, held.DemandInsertion,
		"deadband must leave a balanced lineup untouched")
}
