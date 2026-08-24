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
// user-space units (dots per inch = 96). Routes carries dot's
// routed polyline per "From>To" edge pair. Scale is the uniform
// factor that fit dot's drawing inside the viewport; symbol
// dimensions multiply by it so the rendered diagram matches the
// spacing graphviz computed.
type Layout struct {
	Pos    map[string]Anchor
	Routes map[string][]Anchor
	Scale  float64
	DPI    float64
}

// EmptyLayout returns a Layout whose Pos map is unset; callers
// fall back to straight-line pipes when ComputeLayout is not
// available.
func EmptyLayout() *Layout {
	return &Layout{
		Pos:    make(map[string]Anchor),
		Routes: make(map[string][]Anchor),
		Scale:  1.0,
		DPI:    layoutDPI,
	}
}

// extraNodes are renderer-only parts that carry no fluid edge but
// must appear in the P&ID. Adding them to the DOT lets graphviz
// position them next to their cluster peers.
var extraNodes = []struct{ id, cluster, kind string }{
	{"Turbine", "cluster_turbine", "Turbine"},
	{"Generator", "cluster_turbine", "Generator"},
	{"PRT", "cluster_containment", "PressurizerReliefTank"},
	{"MainFeedValve", "cluster_turbine", "modulating"},
	{"RCFC_A", "cluster_containment", "RCFCUnit"},
	{"RCFC_B", "cluster_containment", "RCFCUnit"},
	{"RCFC_C", "cluster_containment", "RCFCUnit"},
	{"RCFC_D", "cluster_containment", "RCFCUnit"},

	// Pumps outside the fluid graph.
	{"CWP", "cluster_turbine", "CirculatingWaterPump"},
	{"CCWSPump", "cluster_auxiliary", "ComponentCoolingWaterPump"},
	{"SFPPA", "cluster_containment", "SpentFuelPoolPump"},
	{"SFPPB", "cluster_containment", "SpentFuelPoolPump"},

	// Heat-rejection boundary: the shared intake house and the
	// essential service water header it feeds
	// (plans/svg/circulating_water.md).
	{"Intake", "cluster_turbine", "RiverIntake"},
	{"SWSHeader", "cluster_auxiliary", "ServiceWaterHeader"},

	// Safety injection completion: the refueling water storage
	// tank and high-head injection pump beside the accumulators
	// (specs/the_westinghouse_pressurized_water_react/
	//  chapter_5/page_097.txt:24-25 - HHSI and RHR take suction
	//  directly from the RWST).
	{"RWST", "cluster_containment", "RefuelingWaterStorageTank"},
	{"HHSI", "cluster_containment", "HHSIPump"},

	// Containment spray train: borated-water and NaOH additive
	// tanks plus the spray pumps (chapter_6/page_114.txt:55-63).
	{"CSSTank", "cluster_auxiliary", "ContainmentSprayStorageTank"},
	{"SATTank", "cluster_auxiliary", "SprayAdditiveTank"},
	{"CSSPump", "cluster_auxiliary", "ContainmentSprayPump"},

	// CVCS skid: volume control tank and charging pump; the aux
	// spray and letdown valves render via scalarValves.
	{"VCT", "cluster_auxiliary", "VolumeControlTank"},
	{"ChgPump", "cluster_auxiliary", "ChargingPump"},
	{"AUXSPRAY", "cluster_containment", "modulating"},
	{"LTDVN", "cluster_auxiliary", "stop"},

	// PRT quench spray: meters cold water into the relief tank
	// (specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt).
	{"PRTSPRAY", "cluster_containment", "modulating"},

	// EFWS subsystem pumps: one motor- and one turbine-driven
	// pump per subsystem, on diverse power sources
	// (specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:61-69).
	{"EAMD", "cluster_auxiliary", "EmergencyFeedwaterPumpMD"},
	{"EATD", "cluster_auxiliary", "EmergencyFeedwaterPumpTD"},
	{"EBMD", "cluster_auxiliary", "EmergencyFeedwaterPumpMD"},
	{"EBTD", "cluster_auxiliary", "EmergencyFeedwaterPumpTD"},

	// Containment isolation penetrations and the post-LOCA
	// hydrogen recombiner
	// (specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:19-26).
	{"CIS", "cluster_containment", "ContainmentIsolation"},
	{"HDS", "cluster_containment", "HydrogenRecombiner"},

	// Brushless exciter on the generator shaft
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24).
	{"EXCITER", "cluster_turbine", "BrushlessExciter"},

	// Second ESF-train diesel and its day tank; the model gives
	// each train its own EDG (core/alarm.go annunciator loop).
	{"DieselB", "cluster_auxiliary", "DieselGenerator"},
	{"FuelTankB", "cluster_auxiliary", "FuelTank"},

	// Spent-fuel pool heat exchanger between the pool pumps.
	{"SFPHX", "cluster_containment", "ComponentCoolingWaterSystem"},

	// Stator coil cooling water pump for the generator
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-61)
	// and the inverter-fed instrument buses that keep RPS power
	// through a station blackout
	// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-27).
	{"SCCWPump", "cluster_turbine", "StatorCoolingPump"},
	{"INVERTER", "cluster_electrical", "InverterBus"},

	// BTRS recirculation: pump routes VCT flow through the
	// demineralizer beds during boration and dilution
	// (specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39).
	{"BTRS", "cluster_auxiliary", "BTRSDemineralizer"},
	{"BTRSPUMP", "cluster_auxiliary", "BTRSPump"},

	// Moisture separator reheater between the HP/LP turbine
	// sections (chapter_15 figure 15.3-1 steam path).
	{"MSR", "cluster_turbine", "MoistureSeparatorReheater"},

	// Live valves that carry no fluid edge; positions come from
	// standaloneValves rendering.
	{"SGMSIV", "cluster_containment", "modulating"},
	{"SGADV", "cluster_containment", "relief"},
	{"SGSSV", "cluster_containment", "safety"},
	{"PZRBLK", "cluster_containment", "stop"},
	{"PZRPORV", "cluster_containment", "porv"},
	{"TSTOP", "cluster_turbine", "stop"},
	{"TGOV", "cluster_turbine", "modulating"},
	{"TDUMP", "cluster_turbine", "relief"},
	{"CREJ", "cluster_turbine", "gate"},
	{"LPDV", "cluster_turbine", "gate"},
	{"HPDV", "cluster_turbine", "gate"},
	{"EAMFV", "cluster_auxiliary", "modulating"},
	{"EATSA", "cluster_auxiliary", "stop"},
	{"EAREC", "cluster_auxiliary", "gate"},
	{"EBMFV", "cluster_auxiliary", "modulating"},
	{"EBTSA", "cluster_auxiliary", "stop"},
	{"EBREC", "cluster_auxiliary", "gate"},
	{"RABYP", "cluster_auxiliary", "modulating"},
	{"RBBYP", "cluster_auxiliary", "modulating"},
	{"RAS1", "cluster_auxiliary", "gate"},
	{"RAS2", "cluster_auxiliary", "gate"},
	{"RBS1", "cluster_auxiliary", "gate"},
	{"RBS2", "cluster_auxiliary", "gate"},
	{"SOLIDPP", "cluster_auxiliary", "modulating"},
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
	b.WriteString(`  graph [splines=true, overlap=false, sep="+8,+8", nodesep=0.18, ranksep=0.44, dpi=96];` + "\n")
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
		emitEdgeNode(clusters, id, clusterFor[id], n.Kind, pos)
	}

	// Dot parks disconnected nodes in the first rank, which after
	// the Y flip floats the auxiliary and electrical buildings
	// above the plant with no physical meaning. Invisible edges
	// from a low turbine-building node sink them into the drawing's
	// bottom band; clusters keep each building contiguous.
	connected := map[string]bool{}
	for _, e := range g.Edges {
		connected[e.From] = true
		connected[e.To] = true
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
	// The same assignment drives ComputeLayout's route stitching.
	hops := assignEdgeHops(g)
	for i := range hops {
		for _, h := range hops[i] {
			if !mainNodes[h.id] {
				emitEdgeNode(clusters, h.id, h.cluster, h.kind, pos)
			}
		}
	}

	subgraphLabel := map[string]string{
		"cluster_containment": "Containment / Nuclear Island",
		"cluster_turbine":     "Turbine Building",
		"cluster_auxiliary":   "Auxiliary Building",
		"cluster_electrical":  "Electrical / ESF",
	}

	// Deterministic cluster order: map iteration would shuffle the
	// emitted DOT between runs, and dot's layout jitters with it.
	names := make([]string, 0, len(clusters))
	for name := range clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := clusters[name]
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

	// Sink the isolated auxiliary/electrical buildings to the bottom
	// rank via invisible edges (see note above).
	for _, id := range ids {
		switch clusterAssignments[id] {
		case "cluster_auxiliary", "cluster_electrical":
			if !connected[id] {
				fmt.Fprintf(&b, `  "Condenser" -> "%s" [style=invis];`+"\n", id)
			}
		}
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
	w, h := nodeSize(kind)
	cl := clusters[cluster]
	img := nodeImage(kind)
	if p, ok := pos[id]; ok {
		fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", image="%s", pos="%.1f,%.1f", pin=true];`+"\n",
			id, w, h, img, p.X, p.Y)
	} else {
		fmt.Fprintf(cl, `    "%s" [width=%.3f, height=%.3f, label="", image="%s"];`+"\n",
			id, w, h, img)
	}
}

// edgeHopRef names one inline device node on a subdivided DOT
// edge, plus the cluster and symbol-table kind it emits under.
type edgeHopRef struct{ id, cluster, kind string }

// pumpHopFor maps a pumped edge to its inline pump device.
func pumpHopFor(e core.GraphEdge) (edgeHopRef, bool) {
	switch {
	case e.From == "RPV" && e.To == "SG":
		return edgeHopRef{"RCP", clusterFor["RPV"], "ReactorCoolantPump"}, true
	case e.From == "Condenser" && e.To == "HDT":
		// Suction is from the condenser hotwell
		// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-46).
		return edgeHopRef{"CondensatePump", clusterFor["Condenser"], "CondensatePump"}, true
	case e.From == "HDT" && e.To == "SG":
		return edgeHopRef{"MainFeedwaterPump", clusterFor["HDT"], "MainFeedwaterPump"}, true
	case e.From == "HPHeaterDrain" && e.To == "HDT":
		return edgeHopRef{"HeaterDrainPump", clusterFor["HPHeaterDrain"], "HeaterDrainPump"}, true
	}
	return edgeHopRef{}, false
}

// valveHopFor maps a valved edge to its inline valve device.
func valveHopFor(e core.GraphEdge) (edgeHopRef, bool) {
	var r edgeHopRef
	switch {
	case e.From == "RPV" && e.To == "Pressurizer":
		r = edgeHopRef{"SafetyValve", clusterFor["RPV"], "safety"}
	case e.From == "Pressurizer" && e.To == "RPV":
		r = edgeHopRef{"ModulatingValve", clusterFor["Pressurizer"], "modulating"}
	case e.From == "SG" && e.To == "Condenser":
		r = edgeHopRef{"MakeupValve", clusterFor["SG"], "modulating"}
	case e.From == "HDT" && e.To == "SG":
		r = edgeHopRef{"MainFeedValve", clusterFor["HDT"], "modulating"}
	case e.From == "Condenser" && e.To == "CST":
		r = edgeHopRef{"RecirculateValve", clusterFor["Condenser"], "stop"}
	case e.To == "DieselAFWPump" || e.To == "MotorAFWPump" || e.To == "TurbineAFWPump":
		r = edgeHopRef{"CSTValve", clusterFor["CST"], "gate"}
	case e.From == "DieselAFWPump" || e.From == "MotorAFWPump" || e.From == "TurbineAFWPump":
		r = edgeHopRef{"ServiceWaterValve", clusterFor[e.From], "gate"}
	default:
		r = edgeHopRef{"Valve", clusterFor[e.From], "gate"}
	}
	return r, true
}

// assignEdgeHops returns, per graph edge, the ordered hop nodes its
// DOT routing passes through. Ids colliding with earlier hops get a
// numeric suffix; an id matching an existing plant-graph node (RCP)
// is reused verbatim so the standalone node becomes connected.
func assignEdgeHops(g *core.PlantGraph) [][]edgeHopRef {
	mainNodes := make(map[string]bool, len(g.Nodes))
	for id := range g.Nodes {
		mainNodes[id] = true
	}
	seen := map[string]bool{}
	out := make([][]edgeHopRef, len(g.Edges))
	for i, e := range g.Edges {
		add := func(ref edgeHopRef) {
			if !mainNodes[ref.id] {
				if seen[ref.id] {
					for n := 2; ; n++ {
						cand := fmt.Sprintf("%s_%d", ref.id, n)
						if !seen[cand] {
							ref.id = cand
							break
						}
					}
				}
				seen[ref.id] = true
			}
			out[i] = append(out[i], ref)
		}
		if e.Pump != nil {
			if h, ok := pumpHopFor(e); ok {
				add(h)
			}
		}
		if e.Valve != nil {
			if h, ok := valveHopFor(e); ok {
				add(h)
			}
		}
	}
	return out
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
// node centers, routed edge polylines keyed "from>to", and the
// overall drawing size in inches. Format:
//	graph <name> <width> <height>
//	node <name> <x> <y> <w> <h> <label> <style> <shape> ...
//	edge <from> <to> <n> <x1> <y1> ... <xn> <yn> <style> ...
//	stop
// Coordinates are Y-up from the bottom-left corner.
func parsePlainExt(r io.Reader) (map[string]Anchor, map[string][]Anchor, float64, float64, error) {
	pos := map[string]Anchor{}
	routes := map[string][]Anchor{}
	var wIn, hIn float64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "graph "):
			fields := strings.Fields(line)
			if len(fields) < 4 {
				return nil, nil, 0, 0, fmt.Errorf("parse graph header %q", line)
			}
			var err error
			if wIn, err = strconv.ParseFloat(fields[2], 64); err != nil {
				return nil, nil, 0, 0, fmt.Errorf("parse graph width: %w", err)
			}
			if hIn, err = strconv.ParseFloat(fields[3], 64); err != nil {
				return nil, nil, 0, 0, fmt.Errorf("parse graph height: %w", err)
			}
		case strings.HasPrefix(line, "node "):
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			name := fields[1]
			x, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				return nil, nil, 0, 0, fmt.Errorf("parse node %q x: %w", name, err)
			}
			y, err := strconv.ParseFloat(fields[3], 64)
			if err != nil {
				return nil, nil, 0, 0, fmt.Errorf("parse node %q y: %w", name, err)
			}
			pos[name] = Anchor{X: x, Y: y}
		case strings.HasPrefix(line, "edge "):
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			nSeg, err := strconv.Atoi(fields[3])
			if err != nil || nSeg < 2 || len(fields) < 4+2*nSeg {
				continue
			}
			pts := make([]Anchor, 0, nSeg)
			for k := 0; k < nSeg; k++ {
				x, err := strconv.ParseFloat(fields[4+2*k], 64)
				if err != nil {
					return nil, nil, 0, 0, fmt.Errorf("parse edge %q point x: %w", fields[1], err)
				}
				y, err := strconv.ParseFloat(fields[5+2*k], 64)
				if err != nil {
					return nil, nil, 0, 0, fmt.Errorf("parse edge %q point y: %w", fields[1], err)
				}
				pts = append(pts, Anchor{X: x, Y: y})
			}
			routes[fields[1]+">"+fields[2]] = pts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, 0, 0, fmt.Errorf("scan plain-ext: %w", err)
	}
	return pos, routes, wIn, hIn, nil
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

// ComputeLayout generates a DOT description of the plant graph, runs
// Graphviz dot to obtain a layout, and converts node positions and
// routed edge polylines into SVG user-space coordinates. Fallback
// to EmptyLayout when dot is unavailable.
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
	posIn, routesIn, wIn, hIn, err := parsePlainExt(strings.NewReader(out))
	if err != nil {
		return EmptyLayout(), err
	}
	t := computeFit(wIn, hIn)
	layout := &Layout{
		Pos:    make(map[string]Anchor, len(posIn)),
		Routes: make(map[string][]Anchor, len(g.Edges)),
		Scale:  t.scale,
		DPI:    layoutDPI,
	}
	for id, aIn := range posIn {
		a := t.point(aIn)
		a.X = clampRange(a.X, layoutMargin, DocumentWidth-layoutMargin)
		a.Y = clampRange(a.Y, layoutMargin, DocumentHeight-layoutMargin)
		layout.Pos[id] = a
	}
	// Rebuild each ORIGINAL edge's polyline by concatenating the
	// routed segments through its hop chain.
	transformed := make(map[string][]Anchor, len(routesIn))
	for key, pts := range routesIn {
		out := make([]Anchor, len(pts))
		for i, pt := range pts {
			out[i] = t.point(pt)
		}
		transformed[key] = out
	}
	hops := assignEdgeHops(g)
	for i, e := range g.Edges {
		key := e.From + ">" + e.To
		chain := append([]string{e.From}, hopIDs(hops[i])...)
		chain = append(chain, e.To)
		var route []Anchor
		complete := true
		for j := 0; j+1 < len(chain); j++ {
			seg, ok := transformed[chain[j]+">"+chain[j+1]]
			if !ok {
				complete = false
				break
			}
			if len(route) > 0 {
				route = route[:len(route)-1] // drop duplicated junction
			}
			route = append(route, seg...)
		}
		if complete && len(route) >= 2 {
			layout.Routes[key] = route
		}
	}
	return layout, nil
}

func hopIDs(hops []edgeHopRef) []string {
	ids := make([]string, len(hops))
	for i, h := range hops {
		ids[i] = h.id
	}
	return ids
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