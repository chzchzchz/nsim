package svg

//go:generate go run ./genlayout -o layout_default_generated.go

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/chzchzchz/nsim/core"
)

// Layout maps plant-graph node IDs to anchor coordinates in SVG
// user-space units (dots per inch = 96). Scale is the uniform
// factor that fit dot's drawing inside the viewport; symbol
// dimensions multiply by it so the rendered diagram matches the
// spacing graphviz computed.
type Layout struct {
	Pos   map[string]Anchor
	Scale float64
	DPI   float64
}

// EmptyLayout returns a Layout whose Pos map is unset; callers
// fall back to the hardcoded constants when ComputeLayout is
// not available.
func EmptyLayout() *Layout {
	return &Layout{Pos: make(map[string]Anchor), Scale: 1.0, DPI: layoutDPI}
}

// nodeSizes maps PlantGraph node Kind strings to a width/height
// pair in inches (Graphviz default DPI is 96). Values are taken
// from the current <use> width/height values in svg.go so the
// layout proportions match the hand-composed symbols.
var nodeSizes = map[string][2]float64{
	"ReactorPressureVessel":     {100.0 / 96.0, 240.0 / 96.0},
	"Pressurizer":           {60.0 / 96.0, 200.0 / 96.0},
	"SteamGenerator":        {190.0 / 96.0, 430.0 / 96.0},
	"Condenser":             {400.0 / 96.0, 100.0 / 96.0},
	"ReactorCoolantPump":    {50.0 / 96.0, 160.0 / 96.0},
	"CondensatePump":        {60.0 / 96.0, 40.0 / 96.0},
	"MainFeedwaterPump":     {60.0 / 96.0, 40.0 / 96.0},
	"HeaterDrainPump":       {60.0 / 96.0, 40.0 / 96.0},
	"DieselAFWPump":         {60.0 / 96.0, 40.0 / 96.0},
	"MotorAFWPump":          {60.0 / 96.0, 40.0 / 96.0},
	"TurbineAFWPump":        {60.0 / 96.0, 40.0 / 96.0},
	"Turbine":               {360.0 / 96.0, 200.0 / 96.0},
	"Generator":             {220.0 / 96.0, 140.0 / 96.0},
	"CondensateStorageTank": {100.0 / 96.0, 130.0 / 96.0},
	"HeaterDrainTank":       {100.0 / 96.0, 50.0 / 96.0},
	"LowPressureHeaterDrain":  {50.0 / 96.0, 100.0 / 96.0},
	"HighPressureHeaterDrain": {50.0 / 96.0, 100.0 / 96.0},
	"SafetyInjectionSystem":     {60.0 / 96.0, 80.0 / 96.0},
	"EmergencyFeedwaterSystem":  {80.0 / 96.0, 100.0 / 96.0},
	"ComponentCoolingWaterSystem": {100.0 / 96.0, 50.0 / 96.0},
	"ResidualHeatRemovalSystem": {200.0 / 96.0, 80.0 / 96.0},
	"SpentFuelPool":              {200.0 / 96.0, 100.0 / 96.0},
	"DieselGenerator":           {200.0 / 96.0, 100.0 / 96.0},
	"FuelTank":                  {30.0 / 96.0, 40.0 / 96.0},
	"StationTransformer":        {50.0 / 96.0, 50.0 / 96.0},
	"ESFBus":                    {120.0 / 96.0, 20.0 / 96.0},
	"OffsiteGrid":               {30.0 / 96.0, 50.0 / 96.0},
	"safety":                    {20.0 / 96.0, 20.0 / 96.0},
	"PressurizerReliefTank":     {60.0 / 96.0, 80.0 / 96.0},
	"modulating":                {24.0 / 96.0, 20.0 / 96.0},
	"stop":                      {24.0 / 96.0, 20.0 / 96.0},
	"gate":                      {24.0 / 96.0, 20.0 / 96.0},
}

var nodeImages = map[string]string{
	"ReactorPressureVessel":     "reactor_vessel.svg",
	"Pressurizer":           "pressurizer.svg",
	"SteamGenerator":        "steam_generator.svg",
	"Condenser":             "condenser.svg",
	"ReactorCoolantPump":    "pump-rcp.svg",
	"CondensateStorageTank": "tank-cst.svg",
	"HeaterDrainTank":       "tank-horizontal.svg",
	"LowPressureHeaterDrain":  "heat-exchanger-horizontal.svg",
	"HighPressureHeaterDrain": "heat-exchanger-vertical.svg",
	"DieselAFWPump":         "pump-horizontal.svg",
	"MotorAFWPump":          "pump-horizontal.svg",
	"TurbineAFWPump":        "pump-turbine.svg",
	"CondensatePump":        "pump-horizontal.svg",
	"MainFeedwaterPump":     "pump-horizontal.svg",
	"HeaterDrainPump":       "pump-horizontal.svg",
	"SpentFuelPool":     "tank-sfp.svg",
	"SafetyInjectionSystem":     "sis-accumulators.svg",
	"EmergencyFeedwaterSystem":  "efws-tank.svg",
	"ComponentCoolingWaterSystem": "ccws-hx.svg",
	"ResidualHeatRemovalSystem": "rhrs-train.svg",
	"DieselGenerator":           "diesel-generator.svg",
	"FuelTank":                  "fuel-tank.svg",
	"StationTransformer":        "transformer-2w.svg",
	"ESFBus":                    "esf-bus.svg",
	"OffsiteGrid":               "grid-bus.svg",
	"safety":                    "valve-safety.svg",
	"modulating":                "valve-modulating.svg",
	"stop":                      "valve-stop.svg",
	"gate":                      "valve-gate.svg",
	"PressurizerReliefTank":     "tank-vertical.svg",
}


// extraNodes are renderer-only parts that carry no fluid edge but
// must appear in the P&ID. Adding them to the DOT lets graphviz
// position them next to their cluster peers.
var extraNodes = []struct{ id, cluster, kind string }{
	{"Turbine", "cluster_turbine", "Turbine"},
	{"Generator", "cluster_turbine", "Generator"},
	{"PRT", "cluster_containment", "PressurizerReliefTank"},
	{"MainFeedValve", "cluster_turbine", "modulating"},
}

// clusterFor groups PlantGraph node IDs into DOT subgraphs that
// match the spec's building layouts. Nodes not in the map fall
// into the catch-all "other" cluster.
var clusterFor = map[string]string{
	// Nuclear island (containment).
	"RPV":              "cluster_containment",
	"Pressurizer":      "cluster_containment",
	"SG":               "cluster_containment",
	"RCP":              "cluster_containment",
	"SISAccumulators":  "cluster_containment",
	"SFP":              "cluster_containment",

	// Turbine / condensate / feedwater.
	"Turbine":        "cluster_turbine",
	"Generator":      "cluster_turbine",
	"Condenser":      "cluster_turbine",
	"CST":            "cluster_turbine",
	"HDT":            "cluster_turbine",
	"LPHeaterDrain":  "cluster_turbine",
	"HPHeaterDrain":  "cluster_turbine",
	"MainFeedValve":  "cluster_turbine",

	// Auxiliary (EFWS, CCWS, RHRS, diesel, transformer).
	"EFWSTank":    "cluster_auxiliary",
	"CCWSHX":      "cluster_auxiliary",
	"RHRS_A":      "cluster_auxiliary",
	"RHRS_B":      "cluster_auxiliary",
	"Diesel":      "cluster_auxiliary",
	"Transformer": "cluster_auxiliary",
	"FuelTank":    "cluster_auxiliary",

	// Electrical / ESF buses and grid.
	"ESF_A": "cluster_electrical",
	"ESF_B": "cluster_electrical",
	"Grid":  "cluster_electrical",
}

// BuildDot generates a DOT description of the plant graph with
// layout clusters, node sizes, and optional pinned positions.
// When pos is non-nil, each known node gets pos="X,Y" and pin=true
// so graphviz respects the supplied coordinates.
func BuildDot(g *core.PlantGraph, pos map[string]Anchor) (string, error) {
	var b strings.Builder
	b.WriteString("digraph plant {\n")
	b.WriteString(`  graph [splines=true, overlap=false, sep="+12,+12", ranksep=0.5, dpi=96];` + "\n")
	b.WriteString(`  node [shape=none];` + "\n")

	// Cluster definitions, keyed by cluster name.
	clusters := map[string]*strings.Builder{
		"cluster_containment": &strings.Builder{},
		"cluster_turbine":     &strings.Builder{},
		"cluster_auxiliary":   &strings.Builder{},
		"cluster_electrical":  &strings.Builder{},
		"":                    &strings.Builder{}, // unclustered
	}

	// Emit nodes into the appropriate cluster subgraph.
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	mainNodes := map[string]bool{}
	for _, id := range ids {
		n := g.Nodes[id]
		mainNodes[id] = true
		size, ok := nodeSizes[n.Kind]
		w, h := defaultNodeInches, defaultNodeInches
		if ok {
			w, h = size[0], size[1]
		}
		c := clusterFor[id]
		cl := clusters[c]
		if img, ok := nodeImages[n.Kind]; ok {
			if p, ok := pos[id]; ok {
				fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", image="%s", pos="%.1f,%.1f", pin=true];`+"\n",
					id, w, h, img, p.X, p.Y)
			} else {
				fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", image="%s"];`+"\n",
					id, w, h, img)
			}
		} else {
			if p, ok := pos[id]; ok {
				fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", pos="%.1f,%.1f", pin=true];`+"\n",
					id, w, h, p.X, p.Y)
			} else {
				fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label=""];`+"\n",
					id, w, h)
			}
		}
	}

	// Renderer-only parts join the layout so graphviz positions
	// them beside their cluster peers.
	for _, en := range extraNodes {
		if mainNodes[en.id] {
			continue
		}
		mainNodes[en.id] = true
		emitEdgeNode(clusters, en.id, en.cluster, en.kind, pos)
	}

	// Inline pumps and valves ride on graph edges but are rendered
	// as standalone symbols, so the DOT routes each carrying edge
	// through a hop node. Wiring them into the flow gives their
	// graphviz coordinates meaning instead of stranding them in an
	// isolated row detached from the components they serve. An id
	// matching an existing plant-graph node (RCP) reuses that node.
	type edgeHop struct{ id, cluster, kind string }
	hops := make([][]edgeHop, len(g.Edges))
	seen := map[string]bool{}
	for i, e := range g.Edges {
		addHop := func(id, cluster, kind string) {
			h := edgeHop{id: id, cluster: cluster, kind: kind}
			if !mainNodes[id] {
				if seen[id] {
					for n := 2; ; n++ {
						cand := fmt.Sprintf("%s_%d", id, n)
						if !seen[cand] {
							id = cand
							break
						}
					}
				}
				seen[id] = true
				emitEdgeNode(clusters, id, cluster, kind, pos)
			}
			h.id = id
			hops[i] = append(hops[i], h)
		}
		if e.Pump != nil {
			switch {
			case e.From == "RPV" && e.To == "SG":
				addHop("RCP", clusterFor["RPV"], "ReactorCoolantPump")
			case e.From == "CST" && e.To == "HDT":
				addHop("CondensatePump", clusterFor["CST"], "CondensatePump")
			case e.From == "HDT" && e.To == "SG":
				addHop("MainFeedwaterPump", clusterFor["HDT"], "MainFeedwaterPump")
			case e.From == "HPHeaterDrain" && e.To == "HDT":
				addHop("HeaterDrainPump", clusterFor["HPHeaterDrain"], "HeaterDrainPump")
			}
		}
		if e.Valve != nil {
			var id, cluster, kind string
			switch {
			case e.From == "RPV" && e.To == "Pressurizer":
				id, cluster, kind = "SafetyValve", clusterFor["RPV"], "safety"
			case e.From == "Pressurizer" && e.To == "RPV":
				id, cluster, kind = "ModulatingValve", clusterFor["Pressurizer"], "modulating"
			case e.From == "SG" && e.To == "Condenser":
				id, cluster, kind = "MakeupValve", clusterFor["SG"], "modulating"
			case e.From == "HDT" && e.To == "SG":
				id, cluster, kind = "MainFeedValve", clusterFor["HDT"], "modulating"
			case e.From == "Condenser" && e.To == "CST":
				id, cluster, kind = "RecirculateValve", clusterFor["Condenser"], "stop"
			case e.To == "DieselAFWPump" || e.To == "MotorAFWPump" || e.To == "TurbineAFWPump":
				id, cluster, kind = "CSTValve", clusterFor["CST"], "gate"
			case e.From == "DieselAFWPump" || e.From == "MotorAFWPump" || e.From == "TurbineAFWPump":
				id, cluster, kind = "ServiceWaterValve", clusterFor[e.From], "gate"
			default:
				id, cluster, kind = "Valve", clusterFor[e.From], "gate"
			}
			addHop(id, cluster, kind)
		}
	}

	subgraphLabel := map[string]string{
		"cluster_containment": "Containment / Nuclear Island",
		"cluster_turbine":     "Turbine Building",
		"cluster_auxiliary":   "Auxiliary Building",
		"cluster_electrical":  "Electrical / ESF",
	}

	for name, body := range clusters {
		if body.Len() == 0 {
			continue
		}
		if name == "" {
			b.WriteString(body.String())
			continue
		}
		fmt.Fprintf(&b, "  subgraph %s {\n", name)
		if lbl, ok := subgraphLabel[name]; ok {
			fmt.Fprintf(&b, `    label="%s";`+"\n", lbl)
			fmt.Fprintf(&b, "    style=filled;\n")
			fmt.Fprintf(&b, "    color=lightgrey;\n")
			fmt.Fprintf(&b, `    fillcolor="#f8f8f8";`+"\n")
		}
		b.WriteString(body.String())
		b.WriteString("  }\n")
	}

	// Emit edges routed through any inline pump/valve hops so the
	// pipe path passes through each device symbol.
	for i, e := range g.Edges {
		attr := ""
		if e.Valve != nil {
			attr = fmt.Sprintf(" [label=%q]", valveLabel(e.Valve))
		} else if e.Pump != nil {
			attr = fmt.Sprintf(" [label=%q]", pumpLabel(e.Pump))
		}
		path := make([]string, 0, len(hops[i])+2)
		path = append(path, e.From)
		for _, h := range hops[i] {
			path = append(path, h.id)
		}
		path = append(path, e.To)
		for j := 0; j < len(path)-1; j++ {
			segAttr := ""
			if j == 0 {
				segAttr = attr
			}
			fmt.Fprintf(&b, "  %q -> %q%s;\n", path[j], path[j+1], segAttr)
		}
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func emitEdgeNode(clusters map[string]*strings.Builder, id, cluster, kind string, pos map[string]Anchor) {
	size, ok := nodeSizes[kind]
	w, h := defaultNodeInches, defaultNodeInches
	if ok {
		w, h = size[0], size[1]
	}
	cl := clusters[cluster]
	if img, ok := nodeImages[kind]; ok {
		if p, ok := pos[id]; ok {
			fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", image="%s", pos="%.1f,%.1f", pin=true];`+"\n",
				id, w, h, img, p.X, p.Y)
		} else {
			fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", image="%s"];`+"\n",
				id, w, h, img)
		}
	} else {
		if p, ok := pos[id]; ok {
			fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", pos="%.1f,%.1f", pin=true];`+"\n",
				id, w, h, p.X, p.Y)
		} else {
			fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label=""];`+"\n",
				id, w, h)
		}
	}
}

// valveLabel returns the edge label text for a valve.
func valveLabel(v *core.Valve) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("valve %.0f%%", v.Open*100)
}

// pumpLabel returns the edge label text for a pump.
func pumpLabel(p *core.Pump) string {
	if p == nil {
		return ""
	}
	state := "stopped"
	switch {
	case p.Velocity > 0:
		state = fmt.Sprintf("running %.0f rpm", p.Velocity)
	case p.Running:
		state = "starting"
	}
	return "pump " + state
}

func runDot(dot string) (string, error) {
	cmd := exec.Command("dot", "-Tplain-ext")
	cmd.Stdin = strings.NewReader(dot)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			return "", fmt.Errorf("dot failed: %w: %s", err, stderr)
		}
		return "", fmt.Errorf("dot failed: %w", err)
	}
	return string(out), nil
}

// layoutDPI is the SVG user-space resolution used to convert
// graphviz inch coordinates.
const layoutDPI = 96.0

// parsePlainExt parses the "plain-ext" output from dot and returns
// node centers plus the overall drawing size in inches. Format:
//	graph <name> <width> <height>
//	node <name> <x> <y> <w> <h> <label> <style> <shape> ...
//	edge <from> <to> <n> <x1> <y1> ... <xn> <yn> <style> ...
//	stop
// Coordinates are Y-up from the bottom-left corner.
func parsePlainExt(r io.Reader) (map[string]Anchor, float64, float64, error) {
	pos := map[string]Anchor{}
	var wIn, hIn float64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "graph "):
			fields := strings.Fields(line)
			if len(fields) < 4 {
				return nil, 0, 0, fmt.Errorf("parse graph header %q", line)
			}
			var err error
			if wIn, err = strconv.ParseFloat(fields[2], 64); err != nil {
				return nil, 0, 0, fmt.Errorf("parse graph width: %w", err)
			}
			if hIn, err = strconv.ParseFloat(fields[3], 64); err != nil {
				return nil, 0, 0, fmt.Errorf("parse graph height: %w", err)
			}
		case strings.HasPrefix(line, "node "):
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			name := fields[1]
			x, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("parse node %q x: %w", name, err)
			}
			y, err := strconv.ParseFloat(fields[3], 64)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("parse node %q y: %w", name, err)
			}
			pos[name] = Anchor{X: x, Y: y}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("scan plain-ext: %w", err)
	}
	return pos, wIn, hIn, nil
}

// fitTransform converts inch-space, Y-up layout coordinates into
// document-space SVG coordinates: uniform scale so the drawing
// fits the viewport inside a margin, then flip Y so graphviz rank
// order reads top-down like the rendered diagram.
type fitTransform struct {
	scale float64
	offX  float64
	offY  float64
	hIn   float64
}

const layoutMargin = 10.0

func computeFit(wIn, hIn float64) fitTransform {
	availW := DocumentWidth - 2*layoutMargin
	availH := DocumentHeight - 2*layoutMargin
	scale := 1.0
	if wIn > 0 && hIn > 0 {
		scale = math.Min(1.0, math.Min(availW/(wIn*layoutDPI), availH/(hIn*layoutDPI)))
	}
	wPx := wIn * layoutDPI * scale
	hPx := hIn * layoutDPI * scale
	return fitTransform{
		scale: scale,
		offX:  layoutMargin + (availW-wPx)/2,
		offY:  layoutMargin + (availH-hPx)/2,
		hIn:   hIn,
	}
}

func (t fitTransform) point(a Anchor) Anchor {
	return Anchor{
		X: t.offX + a.X*layoutDPI*t.scale,
		Y: t.offY + (t.hIn-a.Y)*layoutDPI*t.scale,
	}
}

// defaultNodeInches is the fallback footprint for kinds missing
// from nodeSizes: Graphviz's own half-inch default box.
const defaultNodeInches = 0.5

// clusterAssignments maps every DOT-emitted node id to its
// building cluster, combining fluid-graph clusters with the
// renderer-only extra nodes.
var clusterAssignments = func() map[string]string {
	m := make(map[string]string, len(clusterFor)+len(extraNodes))
	for id, c := range clusterFor {
		m[id] = c
	}
	for _, en := range extraNodes {
		m[en.id] = en.cluster
	}
	return m
}()

const buildingPad = 20.0

// clusterKind returns the diagramPart footprint key for a layout
// node id.
func clusterKind(id string) string {
	for i := range diagramParts {
		if diagramParts[i].id == id {
			return diagramParts[i].kind
		}
	}
	return ""
}

// clusterBox returns the bounding rectangle in document space of
// all laid-out nodes assigned to a cluster, padded so building
// outlines enclose their symbols.
func clusterBox(name string) (x, y, w, h float64, ok bool) {
	first := true
	for id, a := range loadedLayout.Pos {
		if clusterAssignments[id] != name {
			continue
		}
		sw, sh := symbolSizePx(clusterKind(id))
		minX, minY := a.X-sw/2-buildingPad, a.Y-sh/2-buildingPad
		maxX, maxY := a.X+sw/2+buildingPad, a.Y+sh/2+buildingPad
		if first {
			x, y, w, h = minX, minY, maxX-minX, maxY-minY
			first = false
			continue
		}
		if minX < x {
			w += x - minX
			x = minX
		}
		if minY < y {
			h += y - minY
			y = minY
		}
		if maxX > x+w {
			w = maxX - x
		}
		if maxY > y+h {
			h = maxY - y
		}
	}
	return x, y, w, h, !first
}

// nodeSize returns the width/height in inches for a graph node kind.
func nodeSize(kind string) (w, h float64) {
	if s, ok := nodeSizes[kind]; ok {
		return s[0], s[1]
	}
	return defaultNodeInches, defaultNodeInches
}

// ComputeLayout generates a DOT description of the plant graph, runs
// Graphviz dot to obtain a layout, and converts node positions into
// SVG user-space coordinates. Fallback to EmptyLayout when dot is
// unavailable.
func ComputeLayout(p *core.Plant) (*Layout, error) {
	g := p.BuildGraphFor(p)
	dot, err := BuildDot(g, nil)
	if err != nil {
		return EmptyLayout(), err
	}
	out, err := runDot(dot)
	if err != nil {
		return EmptyLayout(), err
	}
	posIn, wIn, hIn, err := parsePlainExt(strings.NewReader(out))
	if err != nil {
		return EmptyLayout(), err
	}
	t := computeFit(wIn, hIn)
	layout := &Layout{
		Pos:   make(map[string]Anchor, len(posIn)),
		Scale: t.scale,
		DPI:   layoutDPI,
	}
	for id, aIn := range posIn {
		a := t.point(aIn)
		a.X = clampRange(a.X, layoutMargin, DocumentWidth-layoutMargin)
		a.Y = clampRange(a.Y, layoutMargin, DocumentHeight-layoutMargin)
		layout.Pos[id] = a
	}
	return layout, nil
}

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}