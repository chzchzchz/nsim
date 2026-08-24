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
