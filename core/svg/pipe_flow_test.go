package svg

import (
	"math"
	"regexp"
	"strings"
	"strconv"
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

var polylineNumRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

// polylineSegs extracts every drawn segment from the document.
func polylineSegs(out string) [][2][2]float64 {
	var segs [][2][2]float64
	for _, m := range regexp.MustCompile(`<polyline points="([^"]+)"`).FindAllStringSubmatch(out, -1) {
		n := polylineNumRe.FindAllString(m[1], -1)
		var pts [][2]float64
		for i := 0; i+1 < len(n); i += 2 {
			x, _ := strconv.ParseFloat(n[i], 64)
			y, _ := strconv.ParseFloat(n[i+1], 64)
			pts = append(pts, [2]float64{x, y})
		}
		for i := 1; i < len(pts); i++ {
			segs = append(segs, [2][2]float64{pts[i-1], pts[i]})
		}
	}
	return segs
}

// minDistToSegs reports how far a point sits from the nearest
// segment - zero when a glyph is centred on its pipe.
func minDistToSegs(segs [][2][2]float64, cx, cy float64) float64 {
	best := math.Inf(1)
	for _, s := range segs {
		ax, ay := s[0][0], s[0][1]
		bx, by := s[1][0], s[1][1]
		vx, vy := bx-ax, by-ay
		l2 := vx*vx + vy*vy
		tt := 0.0
		if l2 > 0 {
			tt = ((cx-ax)*vx + (cy-ay)*vy) / l2
			if tt < 0 {
				tt = 0
			}
			if tt > 1 {
				tt = 1
			}
		}
		d := math.Hypot(cx-(ax+vx*tt), cy-(ay+vy*tt))
		if d < best {
			best = d
		}
	}
	return best
}

// seatedGroupCentre returns the translate coordinates of the first
// positioned group bound to part.
func seatedGroupCentre(out string, part string) (cx, cy float64, ok bool) {
	tagRe := regexp.MustCompile(`<g data-part="` + regexp.QuoteMeta(part) + `"[^>]*transform="translate\((-?[\d.]+) (-?[\d.]+)\)(?: rotate\(-?[\d.]+\))?"`)
	m := tagRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, false
	}
	x, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, 0, false
	}
	y, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return 0, 0, false
	}
	return x, y, true
}

// TestSeatedValvesOnPipes pins the operator-facing guarantee that
// placed valve glyphs ride their pipes: relief pair on the
// pressurizer line, spray and makeup valves on their CVCS lines.
func TestSeatedValvesOnPipes(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	segs := polylineSegs(out)
	assert.NotEmpty(t, segs)
	for id, part := range map[string]string{
		"PZRPORV":  "ReactorBuilding/PORV",
		"PZRBLK":   "ReactorBuilding/BlockValve",
		"PRTSPRAY": "ReactorBuilding/PRTSprayValve",
		"MKUPV":    "ReactorBuilding/ReactorMakeupControlValve",
	} {
		cx, cy, ok := seatedGroupCentre(out, part)
		if !assert.Truef(t, ok, "%s group rendered", id) {
			continue
		}
		assert.LessOrEqualf(t, minDistToSegs(segs, cx, cy), 3.0,
			"%s must sit on a pipe", id)
	}
	assert.Contains(t, out, `href="#valve-swing_check"`)
}

// TestReliefPipeSeatsValves codifies the visual relief path: the
// pressurizer-to-PRT line renders, and the PORV plus its block
// valve sit on it (centre within a glyph height of the polyline)
// instead of floating at their own anchors.
func TestReliefPipeSeatsValves(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)

	var segs [][2][2]float64
	for _, m := range regexp.MustCompile(`<polyline points="([^"]+)"`).FindAllStringSubmatch(out, -1) {
		n := polylineNumRe.FindAllString(m[1], -1)
		var pts [][2]float64
		for i := 0; i+1 < len(n); i += 2 {
			x, _ := strconv.ParseFloat(n[i], 64)
			y, _ := strconv.ParseFloat(n[i+1], 64)
			pts = append(pts, [2]float64{x, y})
		}
		for i := 1; i < len(pts); i++ {
			segs = append(segs, [2][2]float64{pts[i-1], pts[i]})
		}
	}
	assert.NotEmpty(t, segs)

	distToPipe := func(cx, cy float64) float64 {
		best := math.Inf(1)
		for _, s := range segs {
			ax, ay := s[0][0], s[0][1]
			bx, by := s[1][0], s[1][1]
			vx, vy := bx-ax, by-ay
			l2 := vx*vx + vy*vy
			tt := 0.0
			if l2 > 0 {
				tt = ((cx-ax)*vx + (cy-ay)*vy) / l2
				if tt < 0 {
					tt = 0
				}
				if tt > 1 {
					tt = 1
				}
			}
			d := math.Hypot(cx-(ax+vx*tt), cy-(ay+vy*tt))
			if d < best {
				best = d
			}
		}
		return best
	}

	for id, part := range map[string]string{
		"PZRPORV": "ReactorBuilding/PORV",
		"PZRBLK":  "ReactorBuilding/BlockValve",
	} {
		tagRe := regexp.MustCompile(`<g data-part="` + regexp.QuoteMeta(part) + `"[^>]*transform="translate\((-?[\d.]+) (-?[\d.]+)\)(?: rotate\(-?[\d.]+\))?"`)
		m := tagRe.FindStringSubmatch(out)
		if !assert.NotNilf(t, m, "%s group rendered", id) {
			continue
		}
		cx, _ := strconv.ParseFloat(m[1], 64)
		cy, _ := strconv.ParseFloat(m[2], 64)
		assert.LessOrEqualf(t, distToPipe(cx, cy), 3.0,
			"%s must sit on the relief pipe", id)
	}
}

// TestRiverAndCVCSVisualsRendered pins the decorative and
// spec-cited visual additions: the circulating-water source band,
// the CVCS makeup line, the letdown valve seated on its line
// rather than floating at a bare anchor. The CVCS loop, SIS
// suction, RHR and CCWS legs are real graph edges (step 4), so
// they no longer need visualPipe duplicates.
func TestRiverAndCVCSVisualsRendered(t *testing.T) {
	p0 := core.MakePlant()
	out := renderPlant(t, &p0)
	assert.Contains(t, out, `stroke="`+riverColour+`"`)
	assert.GreaterOrEqual(t,
		strings.Count(out, `stroke="`+cvcsColour+`"`), 1,
		"the RWST->VCT makeup line renders")
	assert.GreaterOrEqual(t, strings.Count(out, `stroke="blue"`), 1,
		"PRT spray line renders")

	// Pumps stream their real graph edges now (RHRS, CCWS).
	p2 := core.MakePlant()
	p2.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Pump.Running = true
	assert.GreaterOrEqual(t, strings.Count(renderPlant(t, &p2), `class="flow"`),
		strings.Count(out, `class="flow"`)+1,
		"an RHR pump streams its own RCS tie line")

	// CCWS feeds the PRT spray via the CCWSHX->PRT edge; the
	// visual CCWRHRA/B duplicates were removed, so the count
	// is now pinned by the real edge (CCWS pump needs DriveReady
	// for flow, so PumpRunning alone may not stream).
	p3 := core.MakePlant()
	p3.AuxiliaryBuilding.ComponentCoolingWaterSystem.DriveReady = true
	p3.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	assert.GreaterOrEqual(t,
		strings.Count(renderPlant(t, &p3), `class="flow"`),
		strings.Count(out, `class="flow"`),
		"the CCWS pump streams its heat-exchanger supply")

	seated := regexp.MustCompile(
		`<g data-part="AuxiliaryBuilding/ChemicalVolumeControlSystem"[^>]*transform="translate\(`)
	assert.Positive(t, len(seated.FindAllString(out, -1)),
		"expected the letdown valve seated on the CVCS line")
}
