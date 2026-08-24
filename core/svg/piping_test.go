package svg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestClipToward covers boundary clipping: axis-aligned pushes land
// on the nearest face, diagonal pushes hit the corner whose axis
// runs out first, and a zero direction is a no-op.
func TestClipToward(t *testing.T) {
	c := Anchor{X: 0, Y: 0}
	w, h := 100.0, 50.0

	t.Run("right face", func(t *testing.T) {
		got := clipToward(c, Anchor{X: 1000, Y: 0}, w, h)
		assert.InDelta(t, w/2, got.X, 1e-9)
		assert.InDelta(t, 0.0, got.Y, 1e-9)
	})

	t.Run("top face", func(t *testing.T) {
		got := clipToward(c, Anchor{X: 0, Y: -1000}, w, h)
		assert.InDelta(t, 0.0, got.X, 1e-9)
		assert.InDelta(t, -h/2, got.Y, 1e-9)
	})

	t.Run("diagonal exits the shallower half-axis first", func(t *testing.T) {
		got := clipToward(c, Anchor{X: 1000, Y: 1000}, w, h)
		assert.InDelta(t, h/2, got.Y, 1e-9)
		assert.InDelta(t, h/2, got.X, 1e-9)
		assert.Less(t, got.X, w/2)
	})

	t.Run("zero direction stays at center", func(t *testing.T) {
		got := clipToward(c, c, w, h)
		assert.Equal(t, c, got)
	})
}

// TestDrawPipePath verifies polyline formatting follows the point
// order and carries the colour and flow-arrow marker.
func TestDrawPipePath(t *testing.T) {
	pts := []Anchor{{X: 1, Y: 2}, {X: 3, Y: 4}, {X: 5, Y: 6}}
	out := DrawPipePath(pts, "red")

	assert.Contains(t, out, `points="1.0,2.0 3.0,4.0 5.0,6.0"`)
	assert.Contains(t, out, `stroke="red"`)
	assert.Contains(t, out, `marker-mid="url(#arrow-red)"`)
	assert.True(t, strings.HasPrefix(out, `<polyline `))
}

// TestDrawPipePathWithValve checks the valve <use> rides on the
// path fragment, centred on the given anchor with the kind's
// symbol id.
func TestDrawPipePathWithValve(t *testing.T) {
	pts := []Anchor{{X: 0, Y: 0}, {X: 10, Y: 10}}
	at := Anchor{X: 5, Y: 5}
	vw, vh := 24.0, 20.0

	out := DrawPipePathWithValve(pts, "blue", "modulating", at, vw, vh)

	assert.Contains(t, out, `<use href="#valve-modulating"`)
	assert.Contains(t, out, `x="-7.0" y="-5.0"`) // at minus half size
	assert.Contains(t, out, `width="24.0" height="20.0"`)
}

// TestPipeArrowMarkers defines one marker per pipe colour.
func TestPipeArrowMarkers(t *testing.T) {
	out := PipeArrowMarkers()
	for _, c := range pipeColours {
		assert.Contains(t, out, `id="arrow-`+c+`"`)
	}
}

// TestColourForEdge_CirculatingWaterIsBlue pins the condenser-tube
// side: cooling water shares feedwater blue while steam and
// condensate keep brown.
func TestColourForEdge_CirculatingWaterIsBlue(t *testing.T) {
	assert.Equal(t, "blue", ColourForEdge("Condenser", "CoolingTower"))
	assert.Equal(t, "blue", ColourForEdge("CoolingTower", "CoolingTower"))
	assert.Equal(t, "brown", ColourForEdge("Condenser", "CST"))
	assert.Equal(t, "red", ColourForEdge("RPV", "SG"))
}

// TestDrawPipePath_NonScalingStroke keeps the 2px stroke crisp once
// the host page scales g#viewport.
func TestDrawPipePath_NonScalingStroke(t *testing.T) {
	out := DrawPipePath([]Anchor{{X: 0, Y: 0}, {X: 5, Y: 5}}, "red")
	assert.Contains(t, out, `vector-effect="non-scaling-stroke"`)
}

// TestFlowDurationSec verifies dash periods shrink with effective
// flow (speed times throttle) and clamp to the documented range.
func TestFlowDurationSec(t *testing.T) {
	full := flowDurationSec(1000, 1)
	throttled := flowDurationSec(1000, 0.25)
	faster := flowDurationSec(1500, 1)

	assert.Greater(t, throttled, full,
		"a throttled regulator must read as slower flow")
	assert.Less(t, faster, full,
		"higher pump speed must stream faster")

	for _, tc := range []struct{ velocity, open float64 }{
		{0, 0}, {1000, 0}, {1e9, 1}, {1, 1e-9},
	} {
		dur := flowDurationSec(tc.velocity, tc.open)
		assert.GreaterOrEqual(t, dur, minFlowDurSec)
		assert.LessOrEqual(t, dur, maxFlowDurSec)
	}
}
