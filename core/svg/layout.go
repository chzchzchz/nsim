package svg

//go:generate go run ./genlayout -o layout_default_generated.go

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/chzchzchz/nsim/core"
)

// Layout maps plant-graph node IDs to anchor coordinates in SVG
// user-space units (dots per inch = 96).
type Layout struct {
	Pos map[string]Anchor
	DPI  float64
}

// EmptyLayout returns a Layout whose Pos map is unset; callers
// fall back to the hardcoded constants when ComputeLayout is
// not available.
func EmptyLayout() *Layout {
	return &Layout{Pos: make(map[string]Anchor), DPI: 96.0}
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
	"safety":                "valve-safety.svg",
	"modulating":            "valve-modulating.svg",
	"stop":                  "valve-stop.svg",
	"gate":                  "valve-gate.svg",
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
	b.WriteString(`  graph [splines=true, overlap=false, sep="+30,+30", dpi=96];` + "\n")
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
	for _, id := range ids {
		n := g.Nodes[id]
		size, ok := nodeSizes[n.Kind]
		w, h := 0.5, 0.5 // default half-inch box
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

	// Emit edge-attached pumps and valves as standalone nodes so
	// graphviz assigns them layout coordinates and the resulting
	// DOT exposes them as image nodes.
	seen := map[string]bool{}
	for _, e := range g.Edges {
		if e.Pump != nil {
			switch {
			case e.From == "CST" && e.To == "HDT":
				if !seen["CondensatePump"] {
					seen["CondensatePump"] = true
					emitEdgeNode(&b, clusters, "CondensatePump", clusterFor["CST"], "CondensatePump", pos)
				}
			case e.From == "HDT" && e.To == "SG":
				if !seen["MainFeedwaterPump"] {
					seen["MainFeedwaterPump"] = true
					emitEdgeNode(&b, clusters, "MainFeedwaterPump", clusterFor["HDT"], "MainFeedwaterPump", pos)
				}
			case e.From == "HPHeaterDrain" && e.To == "HDT":
				if !seen["HeaterDrainPump"] {
					seen["HeaterDrainPump"] = true
					emitEdgeNode(&b, clusters, "HeaterDrainPump", clusterFor["HPHeaterDrain"], "HeaterDrainPump", pos)
				}
			}
		}
	}
	for _, e := range g.Edges {
		if e.Valve == nil {
			continue
		}
		var id, cluster, kind string
		switch {
		case e.From == "RPV" && e.To == "Pressurizer":
			id, cluster, kind = "SafetyValve", clusterFor["RPV"], "safety"
		case e.From == "Pressurizer" && e.To == "RPV":
			id, cluster, kind = "ModulatingValve", clusterFor["Pressurizer"], "modulating"
		case e.From == "SG" && e.To == "Condenser":
			id, cluster, kind = "MakeupValve", clusterFor["SG"], "modulating"
		case e.From == "Condenser" && e.To == "CST":
			id, cluster, kind = "RecirculateValve", clusterFor["Condenser"], "stop"
		case e.To == "DieselAFWPump" || e.To == "MotorAFWPump" || e.To == "TurbineAFWPump":
			id, cluster, kind = "CSTValve", clusterFor["CST"], "gate"
		case e.From == "DieselAFWPump" || e.From == "MotorAFWPump" || e.From == "TurbineAFWPump":
			id, cluster, kind = "ServiceWaterValve", clusterFor[e.From], "gate"
		default:
			id, cluster, kind = "Valve", clusterFor[e.From], "gate"
		}
		if !seen[id] {
			seen[id] = true
			emitEdgeNode(&b, clusters, id, cluster, kind, pos)
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

	// Emit edges.
	for _, e := range g.Edges {
		attr := ""
		if e.Valve != nil {
			attr = fmt.Sprintf(" [label=%q]", valveLabel(e.Valve))
		} else if e.Pump != nil {
			attr = fmt.Sprintf(" [label=%q]", pumpLabel(e.Pump))
		}
		fmt.Fprintf(&b, "  %q -> %q%s;\n", e.From, e.To, attr)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func emitEdgeNode(b *strings.Builder, clusters map[string]*strings.Builder, id, cluster, kind string, pos map[string]Anchor) {
	size, ok := nodeSizes[kind]
	w, h := 0.5, 0.5
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

// parsePlainExt parses the "plain-ext" output from dot.
// Format:
//	node <name> <x> <y> <w> <h> <label> <style> <shape> ...
//	edge <from> <to> <n> <x1> <y1> ... <xn> <yn> <style> ...
//	stop
func parsePlainExt(r io.Reader) (map[string]Anchor, error) {
	pos := map[string]Anchor{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "node ") {
			fields := strings.Fields(line)
			if len(fields) < 6 {
				continue
			}
			name := fields[1]
			x, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				return nil, fmt.Errorf("parse node %q x: %w", name, err)
			}
			y, err := strconv.ParseFloat(fields[3], 64)
			if err != nil {
				return nil, fmt.Errorf("parse node %q y: %w", name, err)
			}
			pos[name] = Anchor{X: x, Y: y}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan plain-ext: %w", err)
	}
	return pos, nil
}

// nodeSize returns the width/height in inches for a graph node kind.
func nodeSize(kind string) (w, h float64) {
	if s, ok := nodeSizes[kind]; ok {
		return s[0], s[1]
	}
	return 0.5, 0.5
}

// ComputeLayout generates a DOT description of the plant graph, runs
// Graphviz dot to obtain a layout, parses the node positions, and
// returns them mapped to SVG user-space coordinates. Fallback to
// EmptyLayout when dot is unavailable.
func ComputeLayout(p *core.Plant) (*Layout, error) {
	g := p.BuildGraphFor(p)
	dot, err := BuildDot(g, nil)
	if err != nil {
		return EmptyLayout(), err
	}
	plain, err := runDot(dot)
	if err != nil {
		return EmptyLayout(), err
	}
	posIn, err := parsePlainExt(strings.NewReader(plain))
	if err != nil {
		return EmptyLayout(), err
	}
	layout := &Layout{
		Pos: make(map[string]Anchor, len(posIn)),
		DPI: 96.0,
	}
	// Store center positions in pixels. useAt() converts center
	// -> top-left when placing SVG symbols.
	const margin = 10.0
	for id, aIn := range posIn {
		w, h := nodeSize(g.Nodes[id].Kind)
		x := aIn.X * layout.DPI
		y := aIn.Y * layout.DPI
		if x-w/2 < margin {
			x = margin + w/2
		}
		if y-h/2 < margin {
			y = margin + h/2
		}
		if x+w/2 > DocumentWidth-margin {
			x = DocumentWidth - margin - w/2
		}
		if y+h/2 > DocumentHeight-margin {
			y = DocumentHeight - margin - h/2
		}
		layout.Pos[id] = Anchor{X: x, Y: y}
	}
	return layout, nil
}