package svg

import (
	"math"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"

	"github.com/chzchzchz/nsim/core"
)

// wetNode returns a graph node whose part holds the given water
// level (gallons) and pressure (psia).
func wetNode(level float64, press float64) core.GraphNode {
	return core.GraphNode{ID: "n", Ref: &core.Tank{
		Level:    core.Volume{Val: level, Unit: u.Gallon},
		Pressure: core.Pressure(press),
	}}
}

func dryNode() core.GraphNode {
	return core.GraphNode{ID: "n", Ref: &core.Tank{}}
}

func graphFor(from, to core.GraphNode) *core.PlantGraph {
	return &core.PlantGraph{Nodes: map[string]core.GraphNode{
		"F": from, "T": to,
	}}
}

func edgeWith(valveOpen float64, hasValve bool, pumpRunning bool, velocity float64) core.GraphEdge {
	e := core.GraphEdge{From: "F", To: "T"}
	if hasValve {
		e.Valve = &core.Valve{Open: valveOpen}
	}
	if pumpRunning || velocity > 0 {
		e.Pump = &core.Pump{Running: pumpRunning, Velocity: core.Velocity(velocity)}
	}
	return e
}

// TestClassifyPipe pins the operator-facing rule: water shows
// unless a valve blocks the path, and only a running pump or a
// positive pressure differential animates it.
func TestClassifyPipe(t *testing.T) {
	wetHigh := wetNode(500, 900)
	wetLow := wetNode(500, 15)
	dry := dryNode()

	cases := []struct {
		name string
		from core.GraphNode
		to   core.GraphNode
		edge core.GraphEdge
		want pipeWater
	}{
		{"shut valve blocks even with water", wetHigh, wetLow,
			edgeWith(0, true, false, 0), pipeBlocked},
		{"running pump streams", wetHigh, wetLow,
			edgeWith(1, true, true, 1000), pipeFlowing},
		{"positive deltaP streams without a pump", wetHigh, wetLow,
			edgeWith(1, true, false, 0), pipeFlowing},
		{"equal pressures stay static", wetLow, wetLow,
			edgeWith(1, true, false, 0), pipeWet},
		{"dry endpoints dim the line", dry, dry,
			edgeWith(1, true, false, 0), pipeEmpty},
		{"readable water beside unreadable node stays wet", wetLow, core.GraphNode{ID: "x"},
			edgeWith(0, false, false, 0), pipeWet},
		{"open valve on unreadable sides shows intent", core.GraphNode{ID: "x"}, core.GraphNode{ID: "y"},
			edgeWith(1, true, false, 0), pipeFlowing},
	}
	for _, tc := range cases {
		g := graphFor(tc.from, tc.to)
		assert.Equal(t, tc.want, classifyPipe(g, tc.edge), tc.name)
	}
}

func gal(v float64) core.Volume {
	return core.Volume{Val: v, Unit: u.Gallon}
}

// TestRouteProjection covers the arc-length helpers the visual
// valve placement uses: midpoints, clamping at the ends, nearest-
// point projection, and angle folding into (-90, 90].
func TestRouteProjection(t *testing.T) {
	horizontal := []Anchor{{X: 0, Y: 0}, {X: 100, Y: 0}}
	assert.InDelta(t, 100, routeLen(horizontal), 1e-9)

	at, deg := routePointAt(horizontal, 50)
	assert.InDelta(t, 50, at.X, 1e-9)
	assert.InDelta(t, 0, at.Y, 1e-9)
	assert.InDelta(t, 0, deg, 1e-9)

	vertical := []Anchor{{X: 5, Y: 0}, {X: 5, Y: 40}}
	at, deg = routePointAt(vertical, 10)
	assert.InDelta(t, 5, at.X, 1e-9)
	assert.InDelta(t, 10, at.Y, 1e-9)
	assert.InDelta(t, 90, math.Abs(deg), 1e-9)
	assert.LessOrEqual(t, deg, 90.0)
	assert.Greater(t, deg, -90.0)

	// Projection lands on the nearest segment position.
	elbow := []Anchor{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}}
	assert.InDelta(t, 120, projectAnchorDist(elbow, Anchor{X: 100, Y: 20}), 1e-6)
	assert.InDelta(t, 0, projectAnchorDist(elbow, Anchor{X: -30, Y: 0}), 1e-6)
}
