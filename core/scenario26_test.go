package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 26 stuck-open pressurizer relief valve: a PORV latched
// mechanically open must keep venting steam to the pressurizer relief
// tank even though pressure sits far below the auto-open setpoint,
// converting steam-bubble volume into liquid level as it goes, until
// the operator isolates the path with the block valve.
//
//   - Catalog: plans/x/scenarios.txt item 26.
//   - Malfunction class (secondary training source): "This malfunction
//     transient causes depressurization of the pressurizer; with steam
//     vapor going to the letdown condenser through the failed opened
//     pressure relief valve CV22."
//     specs/pwr_sim/chapter_4/page_049.txt:28-31 - this model routes
//     the same failure into the PRT per the primary spec ("Steam from
//     the safety and/or relief valves is piped to the pressurizer
//     relief tank").
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-34
func TestScenario26_StuckPORVVentsUntilBlockValveIsolates(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/Faults/PORVStuckOpen"].Set("1"))

	stepN(&p, int(1*time.Second/DefaultTimeStep), DefaultTimeStep)

	// The stuck latch defeats the auto-reclose that would otherwise
	// shut a manually opened PORV the moment pressure falls below
	// PressurizerPORVTrip (2500 psia).
	assert.Equal(t, float64(1), p.ReactorBuilding.PORV.Open,
		"stuck-open valve must stand open below the setpoint")

	// Venting converts bubble to liquid and fills the PRT. Floor is a
	// wiring check recomputed from production constants: half of
	// PressurizerReliefDischargeGalPerSec over the one-second window.
	// The window stays short because the failure signature is the
	// valve's refusal to reclose - instantaneous mechanics, not a
	// long depressurization ramp.
	prt := p.ReactorBuilding.PressurizedReliefTank.Level.Float()
	assert.GreaterOrEqual(t, prt, 6350.0+
		PressurizerReliefDischargeGalPerSec*0.5,
		"PRT gains at least half the design discharge rate")
	assert.Greater(t, p.ReactorBuilding.Pressurizer.Water.Float(), 900.0,
		"vented bubble volume lands as pressurizer liquid")

	// Operator response: isolate through the block valve, then clear
	// the failed-component latch. With the failure cleared the
	// auto-reclose returns and shuts the valve below its setpoint.
	require.NoError(t, cm["ReactorBuilding/BlockValve/Open"].Set("0"))
	require.NoError(t, cm["ReactorBuilding/Faults/PORVStuckOpen"].Set("0"))
	stepN(&p, int(1*time.Second/DefaultTimeStep), DefaultTimeStep)

	assert.Equal(t, float64(0), p.ReactorBuilding.PORV.Open,
		"isolated valve recloses below its setpoint")
	assert.Equal(t, prt,
		p.ReactorBuilding.PressurizedReliefTank.Level.Float(),
		"no further loss after the block valve shuts")
}
