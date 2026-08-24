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
