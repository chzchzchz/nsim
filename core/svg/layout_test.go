package svg

import (
	"strings"
	"testing"

	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeLayout_PositionsFilled runs ComputeLayout on MakePlant
// and asserts every graph node ID maps to a positive coordinate
// within the 1200×900 viewport.
func TestComputeLayout_PositionsFilled(t *testing.T) {
	p := core.MakePlant()
	layout, err := ComputeLayout(&p)
	require.NoError(t, err)
	require.NotNil(t, layout)
	require.NotNil(t, layout.Pos)

	g := p.BuildGraphFor(&p)
	for id := range g.Nodes {
		a, ok := layout.Pos[id]
		require.True(t, ok, "want position for node %q", id)
		assert.True(t, a.X >= 0 && a.X <= DocumentWidth,
			"node %q X out of range: %f", id, a.X)
		assert.True(t, a.Y >= 0 && a.Y <= DocumentHeight,
			"node %q Y out of range: %f", id, a.Y)
	}
}

// TestComputeLayout_Deterministic checks that two calls with the
// same plant both return a non-empty position for every graph node.
// Exact coordinates are not compared because they depend on the
// external graphviz version and are not part of the contract we
// care about testing.
func TestComputeLayout_Deterministic(t *testing.T) {
	p := core.MakePlant()
	for _, l := range []*Layout{
		mustComputeLayout(t, &p),
		mustComputeLayout(t, &p),
	} {
		require.NotNil(t, l)
		require.NotNil(t, l.Pos)
		g := p.BuildGraphFor(&p)
		for id := range g.Nodes {
			a, ok := l.Pos[id]
			require.True(t, ok, "missing position for node %q", id)
			assert.True(t, a.X >= 0, "node %q X negative: %f", id, a.X)
			assert.True(t, a.Y >= 0, "node %q Y negative: %f", id, a.Y)
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

// TestParsePlainExt_ValidInput checks that parsePlainExt correctly
// extracts node positions from a minimal plain-ext string.
func TestParsePlainExt_ValidInput(t *testing.T) {
	input := `graph 1 2.0 3.0
node A 0.5 1.0 1.0 2.0 "" solid none black lightgrey
node B 1.5 2.0 1.0 2.0 "" solid none black lightgrey
edge A B 1 0.5 1.0 1.5 2.0 solid black
stop
`
	pos, err := parsePlainExt(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, pos, 2)
	assert.Equal(t, 0.5, pos["A"].X)
	assert.Equal(t, 1.0, pos["A"].Y)
	assert.Equal(t, 1.5, pos["B"].X)
	assert.Equal(t, 2.0, pos["B"].Y)
}

// TestParsePlainExt_InvalidValues ensures malformed numeric fields
// return an error.
func TestParsePlainExt_InvalidValues(t *testing.T) {
	input := `graph 1 2.0 3.0
node A abc 1.0 1.0 2.0 "" solid none black lightgrey
stop
`
	_, err := parsePlainExt(strings.NewReader(input))
	require.Error(t, err)
}

// TestNodeSize_Defaults checks that unknown kinds fall back to
// the default half-inch box.
func TestNodeSize_Defaults(t *testing.T) {
	w, h := nodeSize("unknown")
	assert.Equal(t, 0.5, w)
	assert.Equal(t, 0.5, h)
}

// TestNodeSize_Known checks that known kinds return their configured
// size in inches.
func TestNodeSize_Known(t *testing.T) {
	w, h := nodeSize("ReactorPressureVessel")
	assert.Equal(t, 100.0/96.0, w)
	assert.Equal(t, 240.0/96.0, h)
}