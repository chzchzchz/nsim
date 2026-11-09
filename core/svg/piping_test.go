package svg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOffsetParallel pins the parallel-pipe twin shift (P4):
// dot routed SG->Condenser x3 and RPV->RHRS x2 as one
// polyline, so the renderer drew a single underline and dropped
// the extra legs to lone valve glyphs. The offset shifts the
// whole polyline laterally by the constant, perpendicular to
// its mean direction; degenerate inputs pass through untouched.
func TestOffsetParallel(t *testing.T) {
	// Mostly-horizontal leg: the perpendicular is vertical, so
	// the twin reads as a second parallel pipe above/below.
	base := []Anchor{{X: 10, Y: 20}, {X: 60, Y: 20}, {X: 110, Y: 20}}
	got := offsetParallel(base, parallelOffsetUnits)
	assert.Len(t, got, 3)
	for i, p := range got {
		assert.InDelta(t, base[i].X, p.X, 1e-9)
		assert.InDelta(t, base[i].Y+parallelOffsetUnits, p.Y, 1e-9)
	}
	// Sign flips to the other side (third leg of a triple).
	neg := offsetParallel(base, -parallelOffsetUnits)
	for i, p := range neg {
		assert.InDelta(t, base[i].X, p.X, 1e-9)
		assert.InDelta(t, base[i].Y-parallelOffsetUnits, p.Y, 1e-9)
	}
	// Degenerate inputs never offset: a self-loop would read as
	// a second tower, and a 2-point stub has no leg to parallel.
	loop := []Anchor{{X: 5, Y: 5}, {X: 5, Y: 5}}
	assert.Equal(t, loop, offsetParallel(loop, parallelOffsetUnits))
	short := []Anchor{{X: 0, Y: 0}, {X: 10, Y: 0}}
	assert.Equal(t, short, offsetParallel(short, 0.0))
}

// TestDeadOpacityOutdimsIdle pins the P4 dead-leg sink: closed-
// valve legs render at 0.30 opacity, distinctly below the idle
// 0.45, so the dashed-black fan in the aux building stays
// muted against live systems.
func TestDeadOpacityOutdimsIdle(t *testing.T) {
	pts := []Anchor{{X: 0, Y: 0}, {X: 100, Y: 10}, {X: 200, Y: 0}}
	assert.Contains(t, DrawPipePathDead(pts), `stroke-opacity="0.30"`)
	assert.Contains(t, DrawPipePathIdle(pts, "sis"), `stroke-opacity="0.45"`)
	assert.Less(t, pipeOpacityDead, pipeOpacityIdle)
}

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
	assert.Contains(t, out, `marker-end="url(#arrow-red)"`)
	assert.True(t, strings.HasPrefix(out, `<polyline `))
}

// TestDrawPipePath_LegibilityGuards pins the Figure 1-2 lining
// that keeps direction to a single end arrow (marker-mid fires
// per vertex and would pepper dot splines - 170 instances on
// the real diagram), keeps idle legs in their system colour at
// reduced opacity, dashes blocked legs black, and dashes drain
// edges while main steam stays thick solid.
func TestDrawPipePath_LegibilityGuards(t *testing.T) {
	coarse := []Anchor{{X: 0, Y: 0}, {X: 100, Y: 20}, {X: 200, Y: 0}}
	// Long coarse leg: end arrow on the main polyline plus
	// exactly one mid-route arrow (one short segment
	// polyline), never one arrow per dot vertex.
	out := DrawPipePath(coarse, "red")
	assert.Contains(t, out, `marker-end="url(#arrow-red)"`)
	assert.Equal(t, 1, strings.Count(out, "marker-mid"),
		"exactly one mid arrow, not per-vertex")
	// Fine-grained spline (>12 vertices): no mid arrow,
	// end arrow only.
	fine := make([]Anchor, 20)
	for i := range fine {
		fine[i] = Anchor{X: float64(i * 10), Y: float64(i)}
	}
	assert.NotContains(t, DrawPipePath(fine, "red"), "marker-mid")
	assert.Contains(t, DrawPipePath(fine, "red"), `marker-end="url(#arrow-red)"`)
	// Blocked legs are dashed black with no arrows (no flow
	// to direct), thinly weighted.
	dead := DrawPipePathDead(coarse)
	assert.Contains(t, dead, `stroke="black"`)
	assert.Contains(t, dead, `stroke-dasharray=`)
	assert.NotContains(t, dead, "marker-")
	assert.Contains(t, dead, `stroke-width="0.6"`)
	// Blocked legs sit past idle opacity (P4): at 0.45 a fan of
	// two dozen dashed-black lines in the aux building reads as
	// ruled texture over the live systems underneath.
	assert.Contains(t, dead, `stroke-opacity="0.30"`,
		"dead legs must out-dim idle legs (0.45)")
	assert.Contains(t, DrawPipePathIdle(coarse, "sis"), `stroke="#7030a0"`)
	assert.Contains(t, DrawPipePathIdle(coarse, "sis"), `stroke-opacity="0.45"`)
	// Edge-aware dash (Figure 1-2): drain edges dashed, main
	// steam thick solid.
	assert.Contains(t, DrawPipePathEdge(coarse, "brown", "LPHeaterDrain", "Condenser"),
		`stroke-dasharray="7 4"`)
	assert.NotContains(t, DrawPipePath(coarse, "brown"), "stroke-dasharray",
		"main steam is thick but solid (Figure 1-2)")
	assert.NotContains(t, DrawPipePathEdge(coarse, "brown", "SG", "Turbine"),
		"stroke-dasharray")
}

// TestPipeArrowMarkers defines one marker per pipe colour.
func TestPipeArrowMarkers(t *testing.T) {
	out := PipeArrowMarkers()
	for name := range pipeColours {
		assert.Contains(t, out, `id="arrow-`+name+`"`)
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

// TestColourForEdge_Systems pins shut-valve legs keeping their
// system colour instead of reading "Blocked / other" black:
// SWS is CCWS-green, AFW train legs are feedwater-blue.
// RHRS->VCT reads cvcs (VCT = charging tank dominates;
// the SOLIDPP hop identifies it as RHRS in the graph but
// ColourForEdge only sees the pair).
func TestColourForEdge_Systems(t *testing.T) {
	assert.Equal(t, "ccws", ColourForEdge("Intake", "SWSHeader"))
	assert.Equal(t, "ccws", ColourForEdge("SWSHeader", "CCWSHX"))
	assert.Equal(t, "cvcs", ColourForEdge("RHRS_A", "VCT"))
	assert.Equal(t, "sis", ColourForEdge("RPV", "RHRS_A"))
	assert.Equal(t, "blue", ColourForEdge("CST", "DieselAFWPump"))
	assert.Equal(t, "blue", ColourForEdge("EFWSTankA", "EAMD"))
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
