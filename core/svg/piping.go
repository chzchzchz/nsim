package svg

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/chzchzchz/nsim/core"
)

// Pipe-rendering primitives used by the composition pass to draw
// edges from PlantGraph. Helpers accept the same colour string
// returned by ColourForEdge.
//
// Pipe colour convention (Figure 1-2, Figure 15.3-1):
//
//	red    - primary coolant (RCS)
//	brown  - steam / condensate (secondary)
//	blue   - feedwater / cooling water (circulating water shares
//	         the feedwater hue: both are liquid water supplies,
//	         while the condenser steam/condensate sides stay brown)
//	green  - CCWS / service water (barrier)
//	sis    - safety injection / RHRS (purple)
//	cvcs   - CVCS letdown/charging (light blue)
//	black  - blocked by a shut valve (positive zero flow)
//
// Figure 1-2 line discipline also applies: steam is drawn thick
// (pipeWeightByColour brown 3.0 > feed 2.0, solid), drain legs
// cascade back to the condenser dashed, and idle-but-connected
// legs keep their system colour dimmed (pipeOpacityIdle)
// instead of dropping to black. Black is reserved for
// pipeBlocked (classifyPipe: shut valve), so the legend's
// "Blocked / other" only covers legs a shut valve actually
// blocks.
//   - Figure 1-2: specs/the_westinghouse_pressurized_water_react/imgs/001.md
//   - Figure 3-1: specs/the_westinghouse_pressurized_water_react/imgs/016.md
//
// Spec justification:
//   - "Reactor coolant piping and fittings are made of stainless
//     steel or are carbon steel clad with stainless steel."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_071.txt:93-96
//   - "The main steam lines are provided with a stop valve and a
//     check valve in each line just outside the containment
//     [...] The swing check valves protect against backflow of
//     steam in the event of a steam line break inside the
//     containment."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:70-81

// DrawPipe returns an SVG fragment for a straight process pipe
// from (fromX, fromY) to (toX, toY), with a flow arrowhead at the
// destination. When elbowed is true the segment is broken into
// horizontal + vertical legs with a small Q-curve elbow at the
// corner.
func DrawPipe(fromX, fromY, toX, toY float64, colour string, elbowed bool) string {
	arrow := fmt.Sprintf(` marker-end="url(#arrow-%s)"`, colour)
	if !elbowed {
		return fmt.Sprintf(
			`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" `+
				`stroke="%s" stroke-width="2"%s/>`,
			fromX, fromY, toX, toY, colour, arrow)
	}
	return fmt.Sprintf(
		`<path d="M %.1f,%.1f L %.1f,%.1f Q %.1f,%.1f %.1f,%.1f `+
			`L %.1f,%.1f" fill="none" stroke="%s" stroke-width="2"%s/>`,
		fromX, fromY, fromX, toY,
		fromX, toY, fromX+1, toY,
		toX, toY, colour, arrow)
}

// pipeColours lists every colour ColourForEdge can return so the
// document defines one flow-arrow marker per colour. Keys are the
// names used in stroke attributes; values are the hex colours.
// Direction is a single marker-end per polyline. marker-mid
// fires per vertex, so it would pepper dot splines with arrow
// blobs (170 instances on the real diagram); interior arrows
// were tried and removed - end arrows carry direction.
var pipeColours = map[string]string{
	"red":   "red",
	"blue":  "blue",
	"brown": "brown",
	"ccws": "#2e7d32",
	"sis":   "#7030a0",
	"cvcs":  "#9fc5e8",
	"black": "black",
}

// PipeArrowMarkers emits one arrowhead <marker> per pipe colour.
// markerWidth/Height 3.2 on the shared 10-unit viewBox draws each
// head about 6 px long at the standard 2 px stroke - small enough
// to read as direction cues rather than landmarks.
func PipeArrowMarkers() string {
	var b strings.Builder
	names := make([]string, 0, len(pipeColours))
	for name := range pipeColours {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fill := pipeColours[name]
		fmt.Fprintf(&b,
			`<marker id="arrow-%s" viewBox="0 0 10 10" refX="9" refY="5" `+
				`markerWidth="3.2" markerHeight="3.2" orient="auto-start-reverse">`+
				`<path d="M 0 0 L 10 5 L 0 10 z" fill="%s"/></marker>`+"\n",
			name, fill)
	}
	return b.String()
}

// pipeStrokeHex resolves a ColourForEdge name to the stroke colour
// string used in markup; unknown names pass through.
func pipeStrokeHex(colour string) string {
	if hex, ok := pipeColours[colour]; ok {
		return hex
	}
	return colour
}

// DrawPipePath renders dot's routed polyline for one edge as a
// single SVG polyline so pipes follow graphviz's routing instead
// of cutting straight across the drawing. Direction is a single
// marker-end arrowhead, plus one true mid-route arrow on long
// coarse legs (split at the arc midpoint, one <path> segment
// with marker-mid, not one marker per dot vertex).
func DrawPipePath(pts []Anchor, colour string) string {
	return drawPipePathMid(pts, colour, "", pipeOpacityFull)
}

// midArrowMinLen is the minimum routed-polyline length (document
// units) carrying a mid-route arrow. Short legs get only the
// end arrow: interior arrows there read as blobs.
const midArrowMinLen = 60.0

// midArrowMaxSegs caps vertices before a leg is considered too
// fine-grained for a mid arrow (per-vertex arrows would blob).
const midArrowMaxSegs = 12

// pipeOpacityFull keeps flowing/wet legs at full strength; idle
// legs are dimmed but keep their system colour (Figure 1-2).
const pipeOpacityFull = 1.0

// pipeOpacityIdle dims an idle-but-connected leg; the system
// colour is kept (Figure 1-2) instead of dropping to black.
const pipeOpacityIdle = 0.45

// pipeDashByEdge encodes Figure 1-2 line discipline: extraction/
// drain legs dashed, main steam/feed solid, blocked legs dashed
// black. Keyed by "From>To" graph ids so drain hops
// (LPHeaterDrain/HPHeaterDrain cascade back to the condenser)
// read dashed while main steam stays thick solid.
//   specs/the_westinghouse_pressurized_water_react/imgs/001.md
var pipeDashByEdge = map[string]string{
	"LPHeaterDrain>Condenser": "7 4",
	"HPHeaterDrain>HDT":        "7 4",
	"LPHeaterDrain>HDT":        "7 4",
	"Condenser>LPHeaterDrain":  "7 4",
}

// pipeDashByColour is the colour-level dash: only blocked
// ("black") reads dashed. Steam is thick but solid (Figure 1-2
// main steam legs); drain is edge-aware via pipeDashByEdge.
var pipeDashByColour = map[string]string{
	"black": "7 4", // blocked/other reads dashed
}

// pipeDashFor returns the stroke-dasharray for a system colour,
// or "" for solid lines.
func pipeDashFor(colour string) string {
	return pipeDashByColour[colour]
}

// pipeDashForEdge returns the stroke-dasharray for an edge's
// From>To pair, or "" for solid. Drain/extraction cascade
// legs (Figure 1-2 dashed) read dashed while main steam
// stays thick solid.
func pipeDashForEdge(fromID, toID string) string {
	return pipeDashByEdge[fromID+">"+toID]
}

func drawPipePath(pts []Anchor, colour, edgeKey string, opacity float64) string {
	return drawPipePathWithArrows(pts, colour, edgeKey, opacity, true)
}

func drawPipePathWithArrows(pts []Anchor, colour, edgeKey string, opacity float64, arrows bool) string {
	var b strings.Builder
	b.WriteString(`<polyline points="`)
	for i, p := range pts {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%.1f,%.1f", p.X, p.Y)
	}
	// vector-effect keeps the stroke crisp when the host page
	// scales g#viewport for pan/zoom. Width encodes system class
	// per pipeWeightByColour (svg.go): primary > secondary > aux.
	fmt.Fprintf(&b, `" fill="none" stroke="%s" stroke-width="%.1f" `, pipeStrokeHex(colour), pipeWeightByColourFor(colour))
	// Drain legs: edge-aware dash wins (Figure 1-2 extraction/
	// drain), then blocked colour dash.
	if from, to, ok := strings.Cut(edgeKey, ">"); ok {
		if dash := pipeDashForEdge(from, to); dash != "" {
			fmt.Fprintf(&b, `stroke-dasharray="%s" `, dash)
		} else if dash := pipeDashFor(colour); dash != "" {
			fmt.Fprintf(&b, `stroke-dasharray="%s" `, dash)
		}
	} else if dash := pipeDashFor(colour); dash != "" {
		fmt.Fprintf(&b, `stroke-dasharray="%s" `, dash)
	}
	fmt.Fprintf(&b, `stroke-opacity="%.2f" `, opacity)
	fmt.Fprintf(&b, `vector-effect="non-scaling-stroke" `)
	if !arrows {
		fmt.Fprintf(&b, `/>`)
		return b.String()
	}
	fmt.Fprintf(&b, `marker-end="url(#arrow-%s)"/>`, colour)
	return b.String()
}

// DrawPipePathEdge renders one graph edge's polyline: edge-aware
// dash (Figure 1-2 drain) plus the same end arrow as
// DrawPipePath. Used by the composition pass so drain legs
// read dashed without affecting main steam.
func DrawPipePathEdge(pts []Anchor, colour, fromID, toID string) string {
	return drawPipePathMid(pts, colour, fromID+">"+toID, pipeOpacityFull)
}

// Flow-dash tuning: effective speed (rpm times valve open
// fraction) divides flowDashScale for the raw period, clamped
// between minFlowDurSec and maxFlowDurSec.
const (
	flowDashScale = 900.0
	minFlowDurSec = 0.6
	maxFlowDurSec = 10.0
)

// nominalDrivenVelocity stands in for pump speed when a pipe
// streams on pressure differential alone (e.g. main steam into the
// condenser): it tunes the dash period only and carries no model
// meaning.
const nominalDrivenVelocity = 1200.0

// flowDurationSec maps pump speed times throttle position to the
// dash-streaming period: faster flow streams faster, and a
// throttled regulator reads as reduced flow rather than full-speed
// dashes.
func flowDurationSec(velocity, open float64) float64 {
	return clampRange(flowDashScale/(velocity*clampRange(open, 0, 1)),
		minFlowDurSec, maxFlowDurSec)
}

// DrawPipePathAnimated renders a routed polyline whose dash pattern
// streams along the flow direction. durSec sets the animation
// period; the document root supplies --phase so remounts continue
// mid-cycle instead of restarting (see RenderOptions.PhaseMs).
func DrawPipePathAnimated(pts []Anchor, colour string, durSec float64) string {
	base := DrawPipePath(pts, colour)
	return strings.Replace(base,
		`/>`,
		fmt.Sprintf(` class="flow" style="--dur:%.2fs"/>`, durSec),
		1)
}

// DrawPipePathIdle renders a routed polyline for a passive leg the
// model cannot observe (no pump, no valve): same geometry and
// arrows as a flowing pipe, dimmed via stroke-opacity so live
// lines pop without losing direction or fluid-system information.
func DrawPipePathIdle(pts []Anchor, colour string) string {
	return DrawPipePathIdleEdge(pts, colour, "", "")
}

// DrawPipePathIdleEdge renders an idle-but-connected leg edge-
// aware: drain edges keep their dash at reduced opacity.
func DrawPipePathIdleEdge(pts []Anchor, colour, fromID, toID string) string {
	return drawPipePathMid(pts, colour, fromID+">"+toID, pipeOpacityIdle)
}

// drawPipePathMid renders with a true mid-route arrow on long
// coarse legs: the polyline is emitted as one <polyline> for
// the ends, plus one short <path> segment at the arc midpoint
// carrying marker-mid. One arrow, not one per vertex. Blocked
// legs (colour "black") never take a mid arrow - they carry no
// arrows at all.
func drawPipePathMid(pts []Anchor, colour, edgeKey string, opacity float64) string {
	if colour == "black" || !midArrowWanted(pts) {
		return drawPipePath(pts, colour, edgeKey, opacity)
	}
	base := drawPipePath(pts, colour, edgeKey, opacity)
	mid, deg := routePointAt(pts, routeLen(pts)/2)
	_ = deg
	// One short segment along the local direction.
	at, _ := routePointAt(pts, routeLen(pts)/2)
	ang := deg * math.Pi / 180
	dx, dy := math.Cos(ang), math.Sin(ang)
	segLen := 8.0
	seg := []Anchor{{X: at.X - dx*segLen, Y: at.Y - dy*segLen},
		{X: at.X + dx*segLen, Y: at.Y + dy*segLen}}
	_ = mid
	// Insert a short mid path before the polyline close.
	base = strings.TrimSuffix(base, "/>")
	return base + `/>` + drawMidArrow(seg, colour, opacity)
}

// drawMidArrow is one short oriented segment with marker-mid.
func drawMidArrow(pts []Anchor, colour string, opacity float64) string {
	var b strings.Builder
	b.WriteString(`<polyline points="`)
	for i, p := range pts {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%.1f,%.1f", p.X, p.Y)
	}
	fmt.Fprintf(&b, `" fill="none" stroke="%s" stroke-width="%.1f" `, pipeStrokeHex(colour), pipeWeightByColourFor(colour))
	fmt.Fprintf(&b, `stroke-opacity="%.2f" `, opacity)
	fmt.Fprintf(&b, `vector-effect="non-scaling-stroke" `)
	fmt.Fprintf(&b, `marker-mid="url(#arrow-%s)" marker-end="url(#arrow-%s)"/>`, colour, colour)
	return b.String()
}

// midArrowWanted reports whether pts is long and coarse enough
// to carry one mid-route arrow.
func midArrowWanted(pts []Anchor) bool {
	if routeLen(pts) <= midArrowMinLen {
		return false
	}
	if len(pts) > midArrowMaxSegs {
		return false
	}
	return true
}

// pipeOpacityDead dims a blocked leg past idle: closed-valve legs
// are the only dashed-black lines, and at opacity 0.45 a fan of a
// dozen parallel lines reads as a ruled texture. Near-miss but
// present-versus-green only, so the dash plus low opacity says
// "dead" without touching the pipes under them.
const pipeOpacityDead = 0.30

// DrawPipePathDead renders a routed polyline whose flow state is
// positively known to be zero - a shut valve blocking the path.
// Black is the unassigned-pipe colour, so the leg reads as a dead
// line rather than a dimmed hint of its former self. Blocked legs
// carry no arrowheads (no flow to direct) and thin slowly so a
// shut auxiliary leg does not scream over live systems.
func DrawPipePathDead(pts []Anchor) string {
	return drawPipePathWithArrows(pts, "black", "", pipeOpacityDead, false)
}



// routeLen sums the segment lengths of a polyline.
func routeLen(pts []Anchor) float64 {
	total := 0.0
	for i := 1; i < len(pts); i++ {
		dx, dy := pts[i].X-pts[i-1].X, pts[i].Y-pts[i-1].Y
		total += math.Hypot(dx, dy)
	}
	return total
}

// routePointAt returns the point at arc distance d along the
// polyline plus the local segment angle in degrees (screen
// coordinates, folded into (-90, 90] so glyphs never render
// upside-down).
func routePointAt(pts []Anchor, d float64) (Anchor, float64) {
	walked := 0.0
	for i := 1; i < len(pts); i++ {
		ax, ay := pts[i-1].X, pts[i-1].Y
		dx, dy := pts[i].X-ax, pts[i].Y-ay
		seg := math.Hypot(dx, dy)
		if walked+seg >= d || i == len(pts)-1 {
			t := 0.0
			if seg > 0 {
				t = clampRange((d-walked)/seg, 0, 1)
			}
			deg := math.Atan2(dy, dx)*180/math.Pi
			for deg > 90 {
				deg -= 180
			}
			for deg <= -90 {
				deg += 180
			}
			return Anchor{X: ax + dx*t, Y: ay + dy*t}, deg
		}
		walked += seg
	}
	return pts[len(pts)-1], 0
}

// projectAnchorDist returns the arc distance along the polyline of
// the point nearest to a - where a glyph bound near that anchor
// should sit.
func projectAnchorDist(pts []Anchor, a Anchor) float64 {
	best, bestD := 0.0, math.Inf(1)
	walked := 0.0
	for i := 1; i < len(pts); i++ {
		ax, ay := pts[i-1].X, pts[i-1].Y
		bx, by := pts[i].X, pts[i].Y
		vx, vy := bx-ax, by-ay
		l2 := vx*vx + vy*vy
		t := 0.0
		if l2 > 0 {
			t = clampRange(((a.X-ax)*vx+(a.Y-ay)*vy)/l2, 0, 1)
		}
		px, py := ax+vx*t, ay+vy*t
		if d := math.Hypot(a.X-px, a.Y-py); d < bestD {
			bestD = d
			best = walked + t*math.Sqrt(l2)
		}
		walked += math.Sqrt(l2)
	}
	return best
}

// ColourForEdge returns the pipe colour for an edge between the
// given source and destination graph node IDs.
//
// Legend mapping: red = RCS primary (RPV, pressurizer, RCP),
// brown = secondary/steam, blue = feedwater/cooling water (SG
// feed legs and circulating water), black = blocked/other
// (DrawPipePathDead reroutes shut-valve legs to black).
// Spec: primary coolant steel in chapter_3/page_071.txt:93-96;
// main steam stop/check valves in chapter_15/page_214.txt:70-81.
func ColourForEdge(fromID, toID string) string {
	// Per-system colours so CVCS/SIS/RHRS/CCWS/EFWS flows read as
	// systems, not "Blocked/other" (plan p66). Order matters: more
	// specific systems win over generic buckets (e.g. CCWSHX->PRT
	// is green, not black; EFWSTankA->SG is blue feedwater, not
	// generic SG blue).
	switch {
	case fromID == "CoolingTower" || toID == "CoolingTower":
		// Condenser-tube circulating water is cooling water, not
		// steam/condensate:
		//   "The Circulating Water System serves as the major heat
		//    sink for the power plant by providing large amounts of
		//    cooling water for the main condensers."
		//   specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
		return "blue"
	case isCVCS(fromID) || isCVCS(toID):
		return "cvcs"
	case isCCWS(fromID) || isCCWS(toID):
		return "ccws"
	case isEFWS(fromID) || isEFWS(toID):
		// EFWS pumps and tanks deliver feedwater to the SGs
		// (checked before SIS so EFWS tanks read blue feedwater,
		// not purple safety injection):
		//   "The primary function of the Emergency Feedwater
		//    System (EFWS) ... is to supply feedwater to the
		//    steam generators following accident or transient
		//    conditions when the main feedwater system is not
		//    available."
		//   specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:109-113
		return "blue"
	case isSIS(fromID) || isSIS(toID):
		return "sis"
	case fromID == "RPV" || toID == "RPV" ||
		fromID == "Pressurizer" || toID == "Pressurizer" ||
		fromID == "RCP" || toID == "RCP":
		return "red"
	case fromID == "Turbine" || toID == "Turbine":
		// Main-steam legs SG->Turbine and Turbine->Condenser
		// share the secondary steam colour. The SG->Turbine
		// leg is the saturated steam delivery line; the
		// exhaust leg is the expanded steam to the condenser.
		//   "During operation, the four steam generators deliver
		//    saturated steam through four steam lines to the main
		//    turbine. [...] The greater portion of the steam is
		//    fully expanded through the turbine before being
		//    exhausted to the condenser."
		//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:20-42
		return "brown"
	case fromID == "Condenser" || toID == "Condenser":
		return "brown"
	case fromID == "SG" || toID == "SG":
		return "blue"
	}
	return "black"
}

// isCVCS reports whether a node id belongs to the chemical and
// volume control letdown/charging loop. Spec:
//   "Letdown water leaves the RCS and flows through the shell side
//    of the regenerative heat exchanger where it gives up its heat
//    to makeup water being returned to the RCS."
//   specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:6-10
func isCVCS(id string) bool {
	switch id {
	case "VCT", "ChgPump", "BTRS", "BTRSPUMP", "BRS", "LDC":
		return true
	}
	return false
}

// isCCWS reports whether a node id belongs to component cooling.
// Spec:
//   "The Component Cooling Water System (CCWS) provides a
//    continuous supply of cooling water to plant components which
//    handle potentially radioactive fluids. In doing so it forms
//    an intermediate barrier between these potentially
//    radioactive systems and the Service Water System"
//   specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:46-55
// The service-water header that feeds CCWS (chapter_12) and its
// intake house share the green service-water barrier reading.
func isCCWS(id string) bool {
	switch id {
	case "CCWSSurge", "CCWSPump", "CCWSHX",
		"SWSHeader", "Intake":
		return true
	}
	return false
}

// isSIS reports whether a node id belongs to safety injection or
// residual heat removal. Spec:
//   "followed by the HHSI and RHR pumps, all discharging into the
//    cold legs of the RCS. The HHSI and RHR pumps are aligned to
//    take suction directly from the RWST."
//   specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:22-26
// RHRS train auxiliaries (recirculation, SOLIDPP, catch-tank VCT
// interface) and LHRS train letter variants ride the same loop.
func isSIS(id string) bool {
	switch id {
	case "RWST", "HHSI", "RHRS_A", "RHRS_B", "SISAccumulators",
		"Sump", "InjectionSink", "CSSTank", "SATTank", "CSSPump",
		"CIS", "RABYP", "RBBYP", "RAS1", "RAS2", "RBS1", "RBS2",
		"EAMFV", "EATSA", "EAREC", "EBMFV", "EBTSA", "EBREC",
		"SOLIDPP", "EFWSTankA", "EFWSTankB", "LDC", "PRT":
		return true
	}
	return false
}

// isEFWS reports whether a node id belongs to emergency feedwater.
// CST/HDT stay out on purpose: Condenser->CST is condensate recirculation
// (brown, secondary side) and HDT->SG is already feedwater-blue via
// the SG endpoint; the AFW pump endpoints carry the affinity.
func isEFWS(id string) bool {
	switch id {
	case "EFWSTank", "EFWSTankA", "EFWSTankB", "EAMD", "EATD", "EBMD", "EBTD",
		"DieselAFWPump", "MotorAFWPump", "TurbineAFWPump",
		"CSTValve", "CSTValve_2", "CSTValve_3",
		"ServiceWaterValve", "ServiceWaterValve_2", "ServiceWaterValve_3",
		"Valve_2", "Valve_3", "Valve_4", "Valve_5":
		return true
	}
	return false
}

// ValveKindForEdge maps a graph edge's valve pointer to a Valve
// kind string for the SVG symbol. The simulator's Valve type
// does not yet carry a Kind field, so this is computed from the
// graph node IDs: the RPV→Pressurizer edge carries the safety
// valve, the SG→Condenser edge carries the makeup / turbine
// bypass, the Condenser→CST edge is the recirculate / stop
// valve, and the AFWPump paths carry CST isolation valves.
func ValveKindForEdge(p *core.Plant, e core.GraphEdge) string {
	if e.Valve == nil {
		return "gate"
	}
	switch {
	case e.From == "RPV" && e.To == "Pressurizer":
		return "safety"
	case e.From == "Pressurizer" && e.To == "RPV":
		return "modulating"
	case e.From == "SG" && e.To == "Turbine":
		return "stop"
	case e.From == "Condenser" && e.To == "CST":
		return "stop"
	case e.To == "SG" || e.To == "DieselAFWPump" ||
		e.To == "MotorAFWPump" || e.To == "TurbineAFWPump":
		return "gate"
	}
	return "gate"
}
