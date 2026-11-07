package svg

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each dot invocation on the full P&ID costs seconds, so the
// read-only layout consumers share one computed Layout;
// determinism across calls has its own compare-pair in
// TestComputeLayout_Deterministic.
var (
	layoutOnce   sync.Once
	sharedPlant  *core.Plant
	sharedLayout *Layout
	sharedErr    error
)

func mustSharedLayout(t *testing.T) (*core.Plant, *Layout) {
	t.Helper()
	layoutOnce.Do(func() {
		p := core.MakePlant()
		l, err := ComputeLayout(&p)
		sharedPlant, sharedLayout, sharedErr = &p, l, err
	})
	require.NoError(t, sharedErr)
	require.NotNil(t, sharedLayout)
	require.NotNil(t, sharedLayout.Pos)
	return sharedPlant, sharedLayout
}

// TestComputeLayout_PositionsFilled runs ComputeLayout on MakePlant
// and asserts every graph node ID maps to a positive coordinate
// within the 1200×900 viewport.
func TestComputeLayout_PositionsFilled(t *testing.T) {
	t.Parallel()
	p, layout := mustSharedLayout(t)

	g := p.BuildGraphFor(p)
	for id := range g.Nodes {
		a, ok := layout.Pos[id]
		require.True(t, ok, "want position for node %q", id)
		assert.True(t, a.X >= 0 && a.X <= DocumentWidth,
			"node %q X out of range: %f", id, a.X)
		assert.True(t, a.Y >= 0 && a.Y <= DocumentHeight,
			"node %q Y out of range: %f", id, a.Y)
	}
}

// The dot drawing is larger than the fixed viewport, so
// ComputeLayout must uniformly scale it instead of clamping
// individual nodes. Clamping piles every overlarge coordinate on
// the viewport edges; scaling preserves relative geometry.
func TestComputeLayout_ScaledNotClamped(t *testing.T) {
	t.Parallel()
	const minSpanFraction = 0.4 // fraction of the viewport that must be used

	_, layout := mustSharedLayout(t)

	ys := []float64{}
	xs := []float64{}
	for _, a := range layout.Pos {
		ys = append(ys, a.Y)
		xs = append(xs, a.X)
	}
	minY, maxY := minMax(ys)
	minX, maxX := minMax(xs)
	assert.Greater(t, maxY-minY, minSpanFraction*DocumentHeight,
		"positions collapsed vertically; layout was clamped, not scaled")
	assert.Greater(t, maxX-minX, minSpanFraction*DocumentWidth,
		"positions collapsed horizontally; layout was clamped, not scaled")

	// The plant graph contains cycles, so exact rank order depends
	// on where dot breaks them; just require the anchors to differ.
	for _, pair := range [][2]string{
		{"RPV", "Condenser"},
		{"SG", "CST"},
	} {
		from, okF := layout.Pos[pair[0]]
		to, okT := layout.Pos[pair[1]]
		require.True(t, okF && okT, "missing anchors for %v", pair)
		assert.NotEqual(t, from.Y, to.Y,
			"want distinct rows for %s and %s", pair[0], pair[1])
	}
}

func minMax(vs []float64) (float64, float64) {
	lo, hi := vs[0], vs[0]
	for _, v := range vs[1:] {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// TestComputeLayout_Deterministic checks that two calls with the
// same plant return identical positions, routed edge polylines, and
// fit scale: within a single process the graphviz output must
// reproduce exactly, since the generated default file depends on it
// for stable diffs. Cross-version drift is absorbed by regenerating
// the committed default file rather than loosened assertions here.
func TestComputeLayout_Deterministic(t *testing.T) {
	p := core.MakePlant()
	a := mustComputeLayout(t, &p)
	b := mustComputeLayout(t, &p)
	assert.Equal(t, a.Pos, b.Pos)
	assert.Equal(t, a.Scale, b.Scale)

	require.Equal(t, len(a.Routes), len(b.Routes))
	for key, ra := range a.Routes {
		rb, ok := b.Routes[key]
		require.True(t, ok, "route %q missing in second layout", key)
		require.Len(t, rb, len(ra))
		for i := range ra {
			assert.Equal(t, ra[i], rb[i], "route %q point %d", key, i)
		}
	}
}

func mustComputeLayout(t *testing.T, p *core.Plant) *Layout {
	t.Helper()
	l, err := ComputeLayout(p)
	require.NoError(t, err)
	return l
}

// TestEmptyLayout returns a Layout whose Pos map is unset; callers
// fall back to the hardcoded constants when ComputeLayout is
// not available.
func TestEmptyLayout(t *testing.T) {
	l := EmptyLayout()
	require.NotNil(t, l)
	require.NotNil(t, l.Pos)
	assert.Equal(t, 0, len(l.Pos))
}

// TestParsePlainExt_ValidInput checks that parsePlainExt extracts
// node positions and the drawing dimensions from a minimal
// plain-ext document. The fixture is built from the expected
// values so the test states a round trip rather than duplicating
// magic numbers.
func TestParsePlainExt_ValidInput(t *testing.T) {
	const (
		wantW    = 2.5
		wantH    = 3.5
		wantAX   = 0.75
		wantAY   = 1.25
		wantBX   = 1.25
		wantBY   = 2.25
		nodeSize = 1.5
	)
	input := fmt.Sprintf(
		"graph 1 %g %g\n"+
			"node A %g %g %g %g \"\" solid none black lightgrey\n"+
			"node B %g %g %g %g \"\" solid none black lightgrey\n"+
			"edge A B 2 %g %g %g %g solid black\n"+
			"stop\n",
		wantW, wantH,
		wantAX, wantAY, nodeSize, nodeSize,
		wantBX, wantBY, nodeSize, nodeSize,
		wantAX, wantAY, wantBX, wantBY)

	pos, routes, wIn, hIn, err := parsePlainExt(strings.NewReader(input))
	require.NoError(t, err)
	assert.InDelta(t, wantW, wIn, 1e-9)
	assert.InDelta(t, wantH, hIn, 1e-9)
	require.Len(t, pos, 2)
	assert.InDelta(t, wantAX, pos["A"].X, 1e-9)
	assert.InDelta(t, wantAY, pos["A"].Y, 1e-9)
	assert.InDelta(t, wantBX, pos["B"].X, 1e-9)
	assert.InDelta(t, wantBY, pos["B"].Y, 1e-9)
	require.Len(t, routes["A>B"], 2)
}

// TestParsePlainExt_InvalidValues ensures malformed numeric fields
// return an error.
func TestParsePlainExt_InvalidValues(t *testing.T) {
	input := `graph 1 2.0 3.0
node A abc 1.0 1.0 2.0 "" solid none black lightgrey
stop
`
	_, _, _, _, err := parsePlainExt(strings.NewReader(input))
	require.Error(t, err)
}

// TestAssignEdgeHops checks the hop chains that subdivide DOT
// edges through inline devices, including suffix dedup for repeated
// valve names and reuse of an existing plant-graph node.
func TestAssignEdgeHops(t *testing.T) {
	p := core.MakePlant()
	g := p.BuildGraphFor(&p)
	hops := assignEdgeHops(g)

	idsFor := func(from, to string) []string {
		for i, e := range g.Edges {
			if e.From == from && e.To == to {
				ids := make([]string, len(hops[i]))
				for j, h := range hops[i] {
					ids[j] = h.id
				}
				return ids
			}
		}
		return nil
	}

	assert.Equal(t, []string{"RCP"}, idsFor("RPV", "SG"))
	// The condensate pump takes suction from the condenser hotwell
	// (chapter_15/page_220.txt:41-46), so its inline hop sits on the
	// Condenser -> HDT edge.
	assert.Equal(t, []string{"CondensatePump"}, idsFor("Condenser", "HDT"))
	assert.Equal(t, []string{"MainFeedwaterPump", "MainFeedValve"}, idsFor("HDT", "SG"))
}

// TestComputeFit verifies the behavioral contract of the fit
// transform: oversized drawings shrink to fit inside the viewport
// with margins, keep their aspect ratio (uniform scaling), flip Y
// so rank order reads top-down, and stay centered. All dimensions
// are derived from the document constants instead of hard-coded
// pixel values.
func TestComputeFit(t *testing.T) {
	availW := DocumentWidth - 2*layoutMargin
	availH := DocumentHeight - 2*layoutMargin

	t.Run("oversized scales down", func(t *testing.T) {
		// A drawing wider than the viewport in both directions.
		wIn := (availW * 2) / layoutDPI
		hIn := (availH * 1.5) / layoutDPI
		tr := computeFit(wIn, hIn)

		assert.Less(t, tr.scale, 1.0, "oversized drawing must scale down")

		bottomLeft := tr.point(Anchor{X: 0, Y: 0})
		topRight := tr.point(Anchor{X: wIn, Y: hIn})
		assert.Greater(t, bottomLeft.Y, topRight.Y, "Y must flip so ranks read top-down")

		for _, a := range []Anchor{bottomLeft, topRight} {
			assert.GreaterOrEqual(t, a.X, 0.0)
			assert.LessOrEqual(t, a.X, DocumentWidth)
			assert.GreaterOrEqual(t, a.Y, 0.0)
			assert.LessOrEqual(t, a.Y, DocumentHeight)
		}

		// Uniform scale preserves the drawing's aspect ratio.
		gotAspect := (topRight.X - bottomLeft.X) / (bottomLeft.Y - topRight.Y)
		assert.InDelta(t, wIn/hIn, gotAspect, 1e-6)

		// The drawing is centered inside the margins.
		leftGap := bottomLeft.X
		rightGap := DocumentWidth - topRight.X
		assert.InDelta(t, leftGap, rightGap, 1e-6)
	})

	t.Run("smaller than viewport stays full scale", func(t *testing.T) {
		tr := computeFit((availW/4)/layoutDPI, (availH/4)/layoutDPI)
		assert.Equal(t, 1.0, tr.scale)
	})
}

// TestComputeLayout_ESFAndFuelColumns asserts the step-3
// cluster-block acceptance: the electrical building reads as two
// ESF train columns (Diesel/ESF A and Diesel B/ESF B side by
// side, grid alongside) and the fuel building gets its own rank
// block (SFP with its pump train), per Figures 11-1/11-2 and
// 1-5. Uses the shared computed layout; no exact coordinates.
func TestComputeLayout_ESFAndFuelColumns(t *testing.T) {
	_, layout := mustSharedLayout(t)
	pos := func(id string) Anchor {
		a, ok := layout.Pos[id]
		require.True(t, ok, "want position for node %q", id)
		return a
	}
	// Train columns: A left of B for both the diesels and the buses.
	assert.Less(t, pos("Diesel").X, pos("DieselB").X)
	assert.Less(t, pos("ESF_A").X, pos("ESF_B").X)
	// Same rank, so the diesels share a row (Y within node height).
	assert.InDelta(t, pos("Diesel").Y, pos("DieselB").Y, 40)
	// The day tank rides with its diesel on the same rank.
	assert.InDelta(t, pos("Diesel").Y, pos("FuelTank").Y, 40)
	assert.InDelta(t, pos("DieselB").Y, pos("FuelTankB").Y, 40)
	// Each diesel feeds its own bus on the next rank down.
	assert.Greater(t, pos("ESF_A").Y, pos("Diesel").Y)
	assert.Greater(t, pos("ESF_B").Y, pos("DieselB").Y)
	// Fuel block: SFP with its pump train, SFP above the pumps.
	assert.Less(t, pos("SFP").Y, pos("SFPPA").Y)
	assert.InDelta(t, pos("SFPPA").Y, pos("SFPPB").Y, 40)
}

// TestComputeLayout_CVCSBlock asserts the step-5 CVCS clustering
// (Figure 4.1-1): letdown valve, VCT and charging pump share one
// rank on the cold-leg side, not scattered among EFWS/RHR rows.
// Uses the shared computed layout; no exact coordinates.
func TestComputeLayout_CVCSBlock(t *testing.T) {
	_, layout := mustSharedLayout(t)
	pos := func(id string) Anchor {
		a, ok := layout.Pos[id]
		require.True(t, ok, "want position for node %q", id)
		return a
	}
	// One rank: same anchor row within a node height.
	assert.InDelta(t, pos("VCT").Y, pos("ChgPump").Y, 40)
	assert.InDelta(t, pos("VCT").Y, pos("LTDVN").Y, 40)
	// The block sits at the RPV inlet side, next to the loop
	// rather than down among the auxiliary safety systems.
	assert.InDelta(t, pos("VCT").Y, pos("RPV").Y, 80)
}

// TestNodeSize_Defaults checks that unknown kinds fall back to
// the default half-inch box.
func TestNodeSize_Defaults(t *testing.T) {
	w, h := nodeSize("unknown")
	assert.Equal(t, defaultNodeInches, w)
	assert.Equal(t, defaultNodeInches, h)
}

// TestNodeSize_Known checks that known kinds resolve through the
// shared symbol table.
func TestNodeSize_Known(t *testing.T) {
	want := symbolTable["ReactorPressureVessel"]
	w, h := nodeSize("ReactorPressureVessel")
	assert.Equal(t, want.wIn, w)
	assert.Equal(t, want.hIn, h)
}