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
	{"PRT", "cluster_containment", "PressurizerReliefTank"},
	{"MainFeedValve", "cluster_turbine", "modulating"},
	{"RCFC_A", "cluster_containment", "RCFCUnit"},
	{"RCFC_B", "cluster_containment", "RCFCUnit"},
	{"RCFC_C", "cluster_containment", "RCFCUnit"},
	{"RCFC_D", "cluster_containment", "RCFCUnit"},

	// Pumps outside the fluid graph.
	{"CWP", "", "CirculatingWaterPump"},
	{"CCWSPump", "cluster_auxiliary", "ComponentCoolingWaterPump"},
	// Spent-fuel pool train lives in the fuel building adjacent
	// to containment (Figure 1-5), not inside the primary loop.
	{"SFPPA", "cluster_fuel", "SpentFuelPoolPump"},
	{"SFPPB", "cluster_fuel", "SpentFuelPoolPump"},

	// Heat-rejection boundary: the shared intake house and the
	// essential service water header it feeds
	// (plans/svg/circulating_water.md). Unclustered (the outside
	// / heat-sink column is unclustered and emits leftmost, and
	// Intake/CWP/SWSHeader anchor the CW tunnel there).
	{"Intake", "", "RiverIntake"},
	{"SWSHeader", "", "ServiceWaterHeader"},

	// Safety injection completion: the refueling water storage
	// tank and high-head injection pump beside the accumulators
	// (specs/the_westinghouse_pressurized_water_react/
	//  chapter_5/page_097.txt:24-25 - HHSI and RHR take suction
	//  directly from the RWST).
	{"RWST", "cluster_containment", "RefuelingWaterStorageTank"},
	{"HHSI", "cluster_containment", "HHSIPump"},

	// Containment spray train: borated-water and NaOH additive
	// tanks plus the spray pumps (chapter_6/page_114.txt:55-63,
	// Figure 6-4 spray ring headers at the containment top).
	{"CSSTank", "cluster_containment", "ContainmentSprayStorageTank"},
	{"SATTank", "cluster_containment", "SprayAdditiveTank"},
	{"CSSPump", "cluster_containment", "ContainmentSprayPump"},

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
	{"Sump", "cluster_containment", "ContainmentSump"},

	// Brushless exciter on the generator shaft
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24).
	{"EXCITER", "cluster_turbine", "BrushlessExciter"},

	// Second ESF-train diesel and its day tank; the model gives
	// each train its own EDG (core/alarm.go annunciator loop).
	// Diesels live with the ESF buses (Figures 11-1/11-2).
	{"DieselB", "cluster_electrical", "DieselGenerator"},
	{"FuelTankB", "cluster_electrical", "FuelTank"},

	// p63_1: SFPHX, SCCWPump, BTRS are now graph nodes;
	// removed from extraNodes to avoid double-draw. BTRSPUMP
	// stays as the hop id on VCT->BTRS (diagramParts kind).
	// Stator coil cooling water pump for the generator
	// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-61)
	// and the inverter-fed instrument buses that keep RPS power
	// through a station blackout
	// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_167.txt:22-27).
	{"INVERTER", "cluster_electrical", "InverterBus"},

	// BTRS recirculation: pump routes VCT flow through the
	// demineralizer beds during boration and dilution
	// (specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39).
	{"BTRSPUMP", "cluster_auxiliary", "BTRSPump"},

	// Moisture separator reheater between the HP/LP turbine
	// sections (chapter_15 figure 15.3-1 steam path).
	{"MSR", "cluster_turbine", "MoistureSeparatorReheater"},

	// Live valves that carry no fluid edge; positions come from
	// standaloneValves rendering.
	{"SGMSIV", "cluster_containment", "modulating"},
	{"SGADV", "cluster_containment", "relief"},
	{"SGSSV", "cluster_containment", "safety"},
	{"SteamlineVent", "cluster_containment", "relief"},
	{"DrainValve", "cluster_containment", "gate"},
	{"WetLayupFillValve", "cluster_containment", "gate"},
	{"FeedwaterRegulator", "cluster_turbine", "modulating"},
	{"ContainmentFiltration", "cluster_containment", "ContainmentFilter"},
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
	"SFP":              "cluster_fuel",

	// Turbine / condensate / feedwater.
	"Turbine":        "cluster_turbine",
	"Generator":      "cluster_turbine",
	"Condenser":      "cluster_turbine",
	"CST":            "cluster_turbine",
	"HDT":            "cluster_turbine",
	"LPHeaterDrain":  "cluster_turbine",
	"HPHeaterDrain":  "cluster_turbine",
	"MainFeedValve":  "cluster_turbine",

	// Auxiliary (EFWS, CCWS, RHRS, BRS).
	"EFWSTank":    "cluster_auxiliary",
	"CCWSHX":      "cluster_auxiliary",
	"RHRS_A":      "cluster_auxiliary",
	"RHRS_B":      "cluster_auxiliary",
	"Transformer": "cluster_electrical",
	"Diesel":      "cluster_electrical",
	"FuelTank":    "cluster_electrical",
	"BRS":         "cluster_auxiliary",

	// Electrical / ESF buses and grid.
	"ESF_A": "cluster_electrical",
	"ESF_B": "cluster_electrical",
	"Grid":  "cluster_electrical",

	// p63_1 loop nodes.
	"BTRS":      "cluster_auxiliary",
	"SFPHX":     "cluster_fuel",
	"SCCWPump":  "cluster_turbine",
	"CCWSSurge": "cluster_auxiliary",
}

// dockingExtrasForRPV are disconnected nodes (graph nodes with no
// fluid edge, or renderer-only extras) that Fig 3-1 shows
// injecting into the RCS at the RPV/cold leg. Invisible constraint
// edges dock them adjacent to RPV instead of leaving them in the
// disconnected rank rows. Connected nodes are skipped by the
// connected[] guard at the emit site, so the list may safely
// include ids that later gain a real edge.
//
// p63_1: RWST, HHSI, ChgPump, BTRS, SFPHX, SCCWPump, CCWSSurge,
// Generator, CWP now carry real graph edges, so they are
// connected and the guard skips them. The list stays nil;
// only truly isolated extras added later would need docking.
var dockingExtrasForRPV = []string(nil)

// BuildDot generates a DOT description of the plant graph with
// layout clusters, node sizes, and optional pinned positions.
// When pos is non-nil, each known node gets pos="X,Y" and pin=true
// so graphviz respects the supplied coordinates.
func BuildDot(g *core.PlantGraph, pos map[string]Anchor) (string, error) {
	var b strings.Builder
	b.WriteString("digraph plant {\n")
	// Spacing tunes how tightly dot packs the row-chunked
	// buildings (layoutRows): the fit scale is width-limited
	// (probed: the five rank blocks stack side by side), so
	// raising nodesep widens the drawing and shrinks every
	// symbol; tighter packing raises the scale. ranksep stays
	// at 0.30 because smaller crowds captions on adjacent rows.
	b.WriteString(`  graph [rankdir=TB, splines=true, overlap=false, sep="+8,+8", nodesep=0.05, ranksep=0.30, dpi=96];` + "\n")
	b.WriteString(`  node [shape=none];` + "\n")

	// Cluster definitions, keyed by cluster name.
	clusters := map[string]*strings.Builder{
		"cluster_containment": &strings.Builder{},
		"cluster_fuel":        &strings.Builder{},
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
	// the Y flip floats them above the plant with no physical
	// meaning. layoutRows folds them into per-building rank rows
	// (the spec's plant section, chapter_1/page_025.txt:75-77,
	// Figure 1-4 cross-section) so the drawing stays near the
	// viewport aspect instead of stretching one row across it.
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
		"cluster_fuel":        "Fuel Building",
		"cluster_turbine":     "Turbine Building",
		"cluster_auxiliary":   "Auxiliary Building",
		"cluster_electrical":  "Electrical / ESF",
	}

	// Emission order: cluster name order is the cluster
	// declaration order in the layoutRows chain below (so dot
	// folds rows in chain order); the unclustered catch-all
	// emits first only when it holds the intake/circulation
	// heat-rejection parts, which sit outside the buildings
	// (chapter_12/page_169.txt:29-34, Figure 1-4).
	emitOrder := []string{"", "cluster_containment", "cluster_fuel",
		"cluster_turbine", "cluster_auxiliary", "cluster_electrical"}
	// Row-chunk the bridge-connected-less nodes per building
	// (layoutRows) and collect flow-relative orderings as dot
	// subgraph ranks (layoutRankRows): the connected edges give
	// each platform's upstream/downstream order, and the
	// constraint edges keep a platform's rank groups in that
	// order without collapsing them into one strip. Pinned
	// layouts (pos != nil) skip the ranks so hand-supplied
	// coordinates survive.
	rows := layoutRows(g, clusters, connected)
	for _, name := range emitOrder {
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
		if name == "cluster_electrical" {
			// The explicit ESF train rows below replace the
			// generic subsystem groups for this building.
		} else {
			for _, r := range layoutRankRows(rows[name]) {
				fmt.Fprintf(&b, "    {rank=same; %s}\n", joinStrings(r, " "))
			}
		}
		if name == "cluster_containment" {
			// RCS loop reads as one horizontal rank (Figure 3-1):
			// RPV, RCP and SG share a rank so their anchors
			// align, with the pressurizer above the hot leg.
			// rankdir is global-TB, so the same-rank group is
			// the horizontal mechanism; the listing order does
			// not set X (probed: dot minimizes edge crossings,
			// and SG's eight feedwater inbound edges from the
			// left columns pin it leftmost regardless).
			fmt.Fprintf(&b, "    {rank=same; %q %q %q}\n", "RPV", "RCP", "SG")
		}
		if name == "cluster_auxiliary" {
			// CVCS reads as one block on the cold-leg side
			// (Figure 4.1-1): letdown valve, VCT and charging
			// pump share a rank, docked together by the real
			// Pressurizer->VCT / RWST->VCT / VCT->InjectionSink
			// edges rather than scattered among EFWS/RHR rows.
			fmt.Fprintf(&b, "    {rank=same; %q %q %q}\n", "LTDVN", "VCT", "ChgPump")
		}
		if name == "cluster_electrical" {
			// ESF trains read as two columns (Figures 11-1/11-2):
			// Diesel/ESF A and Diesel B/ESF B side by side, with
			// grid/transformer/inverter alongside rather than
			// in one row. rankdir is global-TB, so the trains
			// are parallel invisible chains, not one chain.
			for _, r := range [][]string{
				{"Diesel", "FuelTank"},
				{"DieselB", "FuelTankB"},
				{"ESF_A", "ESF_B"},
				{"Grid", "Transformer", "INVERTER"},
			} {
				fmt.Fprintf(&b, "    {rank=same; %s}\n", joinStrings(quotedRankRow(r), " "))
			}
		}
		b.WriteString("  }\n")
	}
	for _, e := range layoutRankEdges(rows) {
		fmt.Fprintf(&b, "  %q -> %q [style=invis];\n", e[0], e[1])
	}
	// Dock safety-system extras to the RPV per Fig 3-1 injection
	// topology; pinned layouts (pos != nil) skip ranks in the
	// layoutRows path above, so skip docking edges there too.
	if pos == nil {
		for _, id := range dockingExtrasForRPV {
			if connected[id] || !mainNodes[id] {
				continue
			}
			fmt.Fprintf(&b, "  %q -> %q [style=invis];\n", id, "RPV")
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

// layoutRowOrder chains the building rows in rankdir=TB flow
// order: containment first (the nuclear island of Figure 3-1,
// chapter_3/page_043.txt:12-15), then the fuel building
// (Figure 1-5, adjacent via the fuel-handling path), turbine,
// auxiliary, and electrical.
var layoutRowOrder = []string{
	"cluster_containment", "cluster_fuel",
	"cluster_turbine", "cluster_auxiliary", "cluster_electrical",
}

// layoutRowWidths caps each building's disconnected-node rows so
// dot stacks several short rows instead of one drawing-width row.
// The values roughly equalize the drawn row widths across
// buildings; electrical holds only the two ESF buses, grid, and
// inverter, so one row suffices. They are also the cluster
// spread: narrower widths keep each building's rank blocks from
// overlapping the neighbouring columns (dot lays the rank
// groups side by side within one rank, verified empirically
// against two clusters).
//
// Sorted ids alone interleave unrelated subsystems (diesels next
// to spray tanks), because alphabetical order ignores the plant's
// ticketing. layoutRowGroups instead groups them by subsystem
// prefix so a row reads as one system (chapter_5/page_097.txt
// safety-injection grouping, Figure 4.1-1 CVCS groups).
var layoutRowWidths = map[string]int{
	"cluster_containment": 3,
	"cluster_fuel":        2,
	"cluster_turbine":     3,
	"cluster_auxiliary":   3,
	"cluster_electrical":  2,
}

// rowGroupRank orders disconnected-node sections so the turbine
// and auxiliary buildings read in steam-path order: power
// conversion first, then feed and cooling, then safety systems.
// Prefixes must match the actual node ids (EFWS pumps are EA*/EB*,
// CVCS parts are VCT/ChgPump/LTDVN, RHR valves RA*/RB*, CCWS is
// CCWS*): a prefix naming the system instead of the ids ("CVCS",
// "EFW") matches nothing and scatters its parts into the tail
// group. "CCWS" precedes "CSS" so CCWS* ids win over the CSS tank
// prefix they superficially resemble.
var rowGroupRank = []string{"Turbine", "Generator", "Condenser", "HDT", "CST", "Exciter", "Diesel", "FuelTank", "EFW", "EA", "EB", "RA", "RB", "RHRS", "VCT", "Chg", "LTD", "BTRS", "CCWS", "CSS", "SAT", "SOLID", "BRS", "SFP", "SIS", "Containment", "ESF", "Grid", "Transformer", "INVERTER", "MSR", "Main", "SCCW", "AUX"}


// layoutRows chunks each cluster's bridge-disconnected nodes into
// rank rows of layoutRowWidths[cluster] nodes, in DOT emission
// order. Connected nodes keep their flow positions and are
// excluded; a cluster holding only connected nodes gets no rows.
func layoutRows(g *core.PlantGraph, clusters map[string]*strings.Builder, connected map[string]bool) map[string][][]string {
	rows := map[string][][]string{}
	for name, body := range clusters {
		if name == "" || body.Len() == 0 {
			continue
		}
		width := layoutRowWidths[name]
		if width <= 0 {
			continue
		}
		var disc []string
		for id := range g.Nodes {
			if clusterAssignments[id] != name || connected[id] {
				continue
			}
			disc = append(disc, id)
		}
		sort.Strings(disc)
		for i := 0; i < len(disc); i += width {
			end := i + width
			if end > len(disc) {
				end = len(disc)
			}
			rows[name] = append(rows[name], disc[i:end])
		}
	}
	// Extra nodes (dispatcher rows such as Turbine extras and
	// standalone valves) are not plant-graph nodes; append them in
	// extraNodes declaration order after the sorted graph ids so
	// the sequence stays deterministic.
	for _, en := range extraNodes {
		if connected[en.id] {
			continue
		}
		width := layoutRowWidths[en.cluster]
		if width <= 0 {
			continue
		}
		cr := rows[en.cluster]
		if len(cr) == 0 || len(cr[len(cr)-1]) >= width {
			cr = append(cr, []string{})
		}
		cr[len(cr)-1] = append(cr[len(cr)-1], en.id)
		rows[en.cluster] = cr
	}
	return rows
}

// layoutRowGroups folds one building's row chunks into the rank
// groups dot should treat as indivisible: singleton rows join the
// next chunk so a two-node platform never merges onto a neighbour
// building's outline. Groups are unquoted node ids; layoutRankRows
// renders them quoted.
func layoutRowGroups(rowCh [][]string) [][]string {
	return layoutRowGroupsSubsystem(rowCh)
}

// layoutRowGroupsSubsystem folds the building's disconnected
// nodes into single-subsystem rank groups: flatten the chunks,
// sort by subsystem prefix (rowGroupRank, then id), then cut
// the sorted sequence at subsystem boundaries. Each group is
// capped so dot stacks several short rows instead of one
// drawing-width row.
func layoutRowGroupsSubsystem(rowCh [][]string) [][]string {
	var all []string
	for _, row := range rowCh {
		all = append(all, row...)
	}
	if len(all) == 0 {
		return nil
	}
	sort.SliceStable(all, func(i, j int) bool {
		ri, rj := rowGroupRankOf(all[i]), rowGroupRankOf(all[j])
		if ri != rj {
			return ri < rj
		}
		return all[i] < all[j]
	})
	// The cap comes from the original chunk width: rows were
	// chunked at layoutRowWidths, so the fullest chunk is that
	// width (and a partial last row never inflates it).
	maxChunk := 0
	for _, row := range rowCh {
		if len(row) > maxChunk {
			maxChunk = len(row)
		}
	}
	width := len(all)
	if maxChunk > 0 {
		width = maxChunk
	}
	var groups [][]string
	for i := 0; i < len(all); {
		j := i + 1
		for j < len(all) && rowGroupRankOf(all[j]) == rowGroupRankOf(all[i]) && (j-i) < width {
			j++
		}
		// Groups of 2+ read as one subsystem (e.g. a diesel with
		// its day tank, SFP with its pumps); singletons drop out
		// so a lone id does not merge onto a neighbour row.
		if j-i >= 2 {
			groups = append(groups, all[i:j])
		}
		i = j
	}
	return groups
}

// rowGroupRankOf maps a node id to its subsystem section rank.
// Prefix matching keeps train-suffix ids (EAMD/EBMD, RAS1/RBS1)
// together.
func rowGroupRankOf(id string) int {
	for i, prefix := range rowGroupRank {
		if strings.HasPrefix(id, prefix) {
			return i
		}
	}
	return len(rowGroupRank)
}

// layoutRankRows renders one building's row groups as quoted
// "{rank=same; ...}" subgraphs.
func layoutRankRows(rowCh [][]string) [][]string {
	groups := layoutRowGroups(rowCh)
	out := make([][]string, 0, len(groups))
	for _, ids := range groups {
		quoted := make([]string, len(ids))
		for i, id := range ids {
			quoted[i] = fmt.Sprintf("%q", id)
		}
		out = append(out, quoted)
	}
	return out
}

// layoutRankEdges orders the rank groups in flow order: each
// group's head is constrained after the previous group's tail so
// dot stacks the groups in chunk order. Cross-building links
// (the steam path RPV -> SG -> Condenser) keep the containment
// and turbine outlines contiguous in rank order.
func layoutRankEdges(rows map[string][][]string) [][2]string {
	var edges [][2]string
	var prevTail string
	for _, name := range layoutRowOrder {
		// Each building's rank block starts fresh: dot then places
		// blocks side by side in emit order, so the buildings read
		// as columns instead of one TB chain (D2: per-cluster
		// rank blocks).
		prevTail = ""
		if name == "cluster_electrical" {
			// Parallel ESF train chains (Figures 11-1/11-2):
			// a single chain would stack A below B in one
			// column, which is exactly the one-row failure
			// this cluster must not repeat.
			edges = append(edges,
				[2]string{"Diesel", "ESF_A"},
				[2]string{"DieselB", "ESF_B"},
			)
			continue
		}
		groups := layoutRowGroups(rows[name])
		for _, ids := range groups {
			if prevTail != "" {
				edges = append(edges, [2]string{prevTail, ids[0]})
			}
			for i := 0; i+1 < len(ids); i++ {
				edges = append(edges, [2]string{ids[i], ids[i+1]})
			}
			prevTail = ids[len(ids)-1]
		}
	}
	// Steam/feed rank order (Figures 1-2/15.3-1): the forward
	// chain runs SG -> Turbine -> MSR -> Condenser -> LP
	// heaters -> HDT -> feed pumps -> HP heaters -> SG per
	// "A flow diagram of a typical steam, condensate, and
	// feedwater system is shown in Figure 15.3-1."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:20-28
	// The real graph carries SG->Turbine, Turbine->Condenser
	// (steam path) and the return legs (Condenser->HDT,
	// HDT->SG, LPHeaterDrain->Condenser, HPHeaterDrain->HDT),
	// so only the unmodeled MSR node needs an invisible
	// constraint, and re-adding a feed-heater chain would fight
	// the real drain-cascade directions (probed: dot breaks the
	// cycle arbitrarily). These constraints keep the rank
	// placement in chain order so dot routes the steam-leg
	// polylines through MSR instead of cutting a diagonal
	// across both buildings.
	edges = append(edges,
		[2]string{"Turbine", "MSR"},
		[2]string{"MSR", "Condenser"},
	)
	return edges
}

// quotedRankRow quotes DOT node ids for a rank=same subgraph.
func quotedRankRow(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprintf("%q", id)
	}
	return out
}

func joinStrings(parts []string, sep string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(p)
	}
	return b.String()
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
// Valves additionally carry the bridge part path their rendered
// group binds to, so the web UI can click inline valves the same
// way it clicks standalone parts.
type edgeHopRef struct{ id, cluster, kind, part string }

// pumpHopFor maps a pumped edge to its inline pump device.
func pumpHopFor(e core.GraphEdge) (edgeHopRef, bool) {
	switch {
	case e.From == "RPV" && e.To == "SG":
		return edgeHopRef{"RCP", clusterFor["RPV"], "ReactorCoolantPump", ""}, true
	case e.From == "Condenser" && e.To == "HDT":
		// Suction is from the condenser hotwell
		// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-46).
		return edgeHopRef{"CondensatePump", clusterFor["Condenser"], "CondensatePump", ""}, true
	case e.From == "HDT" && e.To == "SG":
		return edgeHopRef{"MainFeedwaterPump", clusterFor["HDT"], "MainFeedwaterPump", ""}, true
	case e.From == "HPHeaterDrain" && e.To == "HDT":
		return edgeHopRef{"HeaterDrainPump", clusterFor["HPHeaterDrain"], "HeaterDrainPump", ""}, true
	case e.From == "CST" && e.To == "DieselAFWPump":
		// AFW subsystem pumps draw suction from the CST and discharge
		// to the steam generators through their respective service
		// water valves.
		// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:61-69
		return edgeHopRef{"DieselAFWPump", clusterFor["CST"], "DieselAFWPump", ""}, true
	case e.From == "CST" && e.To == "MotorAFWPump":
		return edgeHopRef{"MotorAFWPump", clusterFor["CST"], "MotorAFWPump", ""}, true
	case e.From == "CST" && e.To == "TurbineAFWPump":
		return edgeHopRef{"TurbineAFWPump", clusterFor["CST"], "TurbineAFWPump", ""}, true
	case e.From == "DieselAFWPump" && e.To == "SG":
		// AFW pumps have their own CST suction and service-water
		// discharge valves already mapped in valveHopFor; the hop
		// here mirrors the pump node so the symbol sits on the pipe.
		return edgeHopRef{"DieselAFWPump", clusterFor["DieselAFWPump"], "DieselAFWPump", ""}, true
	case e.From == "MotorAFWPump" && e.To == "SG":
		return edgeHopRef{"MotorAFWPump", clusterFor["MotorAFWPump"], "MotorAFWPump", ""}, true
	case e.From == "TurbineAFWPump" && e.To == "SG":
		return edgeHopRef{"TurbineAFWPump", clusterFor["TurbineAFWPump"], "TurbineAFWPump", ""}, true
	// p63_1 hop rows: hop-only pumps (never graph nodes).
	// Hop vs node split prevents double-draw: these ids are
	// not in BuildGraphFor as nodes.
	case e.From == "VCT" && e.To == "InjectionSink":
		return edgeHopRef{"ChgPump", clusterFor["VCT"], "ChargingPump", ""}, true
	case e.From == "RWST" && e.To == "InjectionSink":
		return edgeHopRef{"HHSI", clusterFor["RWST"], "HHSIPump", ""}, true
	case e.From == "CSSTank" && e.To == "CIS":
		return edgeHopRef{"CSSPump", clusterFor["CSSTank"], "ContainmentSprayPump", ""}, true
	case e.From == "CCWSSurge" && e.To == "CCWSHX":
		return edgeHopRef{"CCWSPump", clusterFor["CCWSSurge"], "ComponentCoolingWaterPump", ""}, true
	case e.From == "CoolingTower" && e.To == "Condenser":
		return edgeHopRef{"CWP", "", "CirculatingWaterPump", ""}, true
	case e.From == "VCT" && e.To == "BTRS":
		return edgeHopRef{"BTRSPUMP", clusterFor["BTRS"], "BTRSPump", ""}, true
	case e.From == "SFP" && e.To == "SFPHX":
		return edgeHopRef{"SFPPA", clusterFor["SFP"], "SpentFuelPoolPump", ""}, true
	case e.From == "Generator" && e.To == "SCCWPump":
		return edgeHopRef{"SCCWPump", clusterFor["SCCWPump"], "StatorCoolingPump", ""}, true
	}
	return edgeHopRef{}, false
}

// valveHopFor maps a valved edge to its inline valve device. Part
// paths mirror the model field owning each valve so rendered groups
// bind to the same rows the bridge registers (or will register).
func valveHopFor(e core.GraphEdge) (edgeHopRef, bool) {
	var r edgeHopRef
	switch {
	case e.From == "RPV" && e.To == "Pressurizer":
		r = edgeHopRef{"SafetyValve", clusterFor["RPV"], "safety", "ReactorBuilding/SafetyValve"}
	case e.From == "Pressurizer" && e.To == "RPV":
		r = edgeHopRef{"ModulatingValve", clusterFor["Pressurizer"], "modulating", "ReactorBuilding/Pressurizer/ModulatingValve"}
	case e.From == "SG" && e.To == "Turbine":
		r = edgeHopRef{"SGMSIV", clusterFor["SG"], "stop", "ReactorBuilding/SteamGenerator/MainSteamIsolationValve"}
	case e.From == "SG" && e.To == "Condenser":
		// Dump leg: no hop here — TDUMP seats on this route
		// via valvePlacements. A MakeupValve hop here drew a
		// valve glyph that implied the makeup valve gates the
		// dump; it does not.
		return edgeHopRef{}, false
	case e.From == "HDT" && e.To == "SG":
		r = edgeHopRef{"MainFeedValve", clusterFor["HDT"], "modulating", "TurbineBuilding/MainFeedwaterControlValve"}
	case e.From == "Condenser" && e.To == "CST":
		r = edgeHopRef{"RecirculateValve", clusterFor["Condenser"], "stop", "TurbineBuilding/RecirculateValve"}
	case e.To == "DieselAFWPump" || e.To == "MotorAFWPump" || e.To == "TurbineAFWPump":
		r = edgeHopRef{"CSTValve", clusterFor["CST"], "gate", "Outside/" + e.To + "/CSTValve"}
	case e.From == "DieselAFWPump" || e.From == "MotorAFWPump" || e.From == "TurbineAFWPump":
		r = edgeHopRef{"ServiceWaterValve", clusterFor[e.From], "gate", "Outside/" + e.From + "/ServiceWaterValve"}
	default:
		r = edgeHopRef{"Valve", clusterFor[e.From], "gate", ""}
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

// pumpLabel returns the edge label text for a pump. Bool-wrapper
// pumps have no Velocity dynamics (never stepped), so the rpm
// branch never fires for them; gate on IsRunning() so a
// running wrapper reads "running", not "starting" forever.
func pumpLabel(p *core.Pump) string {
	if p == nil {
		return ""
	}
	state := "stopped"
	switch {
	case p.Velocity > 0:
		state = fmt.Sprintf("running %.0f rpm", p.Velocity)
	case p.IsRunning():
		state = "running"
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