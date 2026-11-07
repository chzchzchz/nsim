package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 21 durable physics: the return to full power after a load
// follow descent is paced by xenon burnout. The reduction leaves a
// surplus inventory above the reduced-power equilibrium; that surplus
// is a negative reactivity contribution, and it clears faster at the
// higher flux of the ascending core because destruction scales with
// power through the burnup term.
//
//   - "During the last 15 percent of the fuel cycle, the time it takes
//     to achieve full power from the 90-percent level is determined by
//     the xenon 'burnout' rate and typically requires 2 to 4 hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:12-15
//   - "following a power reduction, xenon initially builds up for about
//     seven hours, and then slowly decays."
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:83-85
func TestXenonBurnoutPacesReturnToPower(t *testing.T) {
	rated := float64(CoreFissionPowerAtRated)

	// Post-descent state: iodine still near its 90-percent equilibrium,
	// xenon surplus a few percent above it (the seg00 hold end state).
	newDescent := func() ReactorPressureVessel {
		return ReactorPressureVessel{
			Iodine135: 0.99,
			Xenon135:  xenonEquilibriumPhysical(0.9) * 1.033,
		}
	}

	high := newDescent()
	high.FissionPower = Watts(rated)
	low := newDescent()
	low.FissionPower = Watts(rated * 0.9)
	require.Greater(t, xenonNormalized(high.Xenon135), 1.0,
		"fixture: descent must leave a surplus above rated equilibrium")

	hour := time.Hour
	high.advanceXenon(hour)
	low.advanceXenon(hour)

	highNorm := xenonNormalized(high.Xenon135)
	lowNorm := xenonNormalized(low.Xenon135)

	// Physics-forced signs: burning at the ascending core's full flux
	// clears the surplus, while holding at the 90-percent pre-ascent
	// level clears it slower (or keeps building it - below the descent
	// level the inventory would rise toward its pit). Clearing faster
	// at the higher flux is what paces the ascent, not a constant.
	assert.Less(t, highNorm, 1.033,
		"surplus must burn off at rated flux")
	assert.Less(t, highNorm, lowNorm,
		"holding at 90 percent clears the surplus slower than ascending")
}

// The low-Tavg-deviation loading stop: while measured Tavg sits more
// than TurbineLowTavgDeviationF below the load-referenced program, the
// DEHM holds the load reference instead of raising it, and the hold
// releases itself once the deviation recovers. Descent toward a lower
// demand stays permitted. This is the automatic branch of the rods-only
// return-to-power caveat.
//
//	"Tavg will continue to drop until turbine loading is stopped
//	 either by manual operation or automatically upon reaching the
//	 low Tavg deviation setpoint signal."
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_232.txt:62-66
func TestTurbineLowTavgDeviationStopsLoading(t *testing.T) {
	newAscending := func() Turbine {
		return Turbine{
			State:              TurbineRunning,
			Synchronized:       true,
			Setpoint:           0.9,
			RampRate:           TurbineLoadRunbackRate,
			LoadDemandRef:      1.0,
			StopValve:          Valve{Open: 1},
			LubeOilPumpRunning: true,
		}
	}
	minute := int(time.Minute / DefaultTimeStep)
	open := 1.0

	// Loading with Tavg tracking the program ascends freely and never
	// latches the hold (the controlled rate needs two minutes to walk
	// 0.9 -> 1.0).
	tracking := newAscending()
	for i := 0; i < 2*minute; i++ {
		tracking = tracking.Step(DefaultTimeStep, 1000, 0,
			Temperature{Val: programmedRodTavg(tracking.Setpoint)},
			false, &open)
	}
	assert.InDelta(t, 1.0, tracking.Setpoint, 1e-9,
		"tracked loading must reach the dispatcher reference")
	assert.False(t, tracking.LowTavgDeviationHold,
		"tracked loading must not latch the deviation hold")

	// Loading with Tavg pinned below its program latches the hold and
	// freezes the reference at the pre-ramp level.
	sagging := newAscending()
	coldTavg := programmedRodTavg(0.9) + TurbineLowTavgDeviationF - 1
	for i := 0; i < minute; i++ {
		sagging = sagging.Step(DefaultTimeStep, 1000, 0,
			Temperature{Val: coldTavg}, false, &open)
	}
	assert.InDelta(t, 0.9, sagging.Setpoint, 1e-9,
		"held loading must freeze the load reference")
	assert.True(t, sagging.LowTavgDeviationHold,
		"deep deviation must latch the hold")

	// Recovery releases the hold and loading resumes toward the
	// reference.
	for i := 0; i < minute; i++ {
		sagging = sagging.Step(DefaultTimeStep, 1000, 0,
			Temperature{Val: programmedRodTavg(sagging.Setpoint)},
			false, &open)
	}
	assert.False(t, sagging.LowTavgDeviationHold,
		"recovered deviation must release the hold")
	assert.Greater(t, sagging.Setpoint, 0.9,
		"loading must resume after recovery")

	// Lowering the demand under an active hold stays permitted: the
	// stop gates the ascent only.
	held := newAscending()
	held.Step(DefaultTimeStep, 1000, 0, Temperature{Val: coldTavg}, false, &open)
	held.LoadDemandRef = 0.85
	for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
		held = held.Step(DefaultTimeStep, 1000, 0, Temperature{Val: coldTavg}, false, &open)
	}
	assert.Less(t, held.Setpoint, 0.9,
		"descent toward a lowered demand must stay permitted under hold")
}
