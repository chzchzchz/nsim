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
// Pipe colour convention (Westinghouse P&ID):
//
//	red    - primary coolant (RCS)
//	brown  - steam / condensate (secondary)
//	blue   - feedwater / cooling water (circulating water shares
//	         the feedwater hue: both are liquid water supplies,
//	         while the condenser steam/condensate sides stay brown)
//	green  - service water (CCWS barrier)
//	sis    - safety injection / residual heat removal (purple)
//	cvcs   - chemical and volume control (light blue)
//	black  — anything else
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
// marker-mid is intentionally absent: direction is a single
// marker-end per polyline (arrow blobs on short legs otherwise).
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
// marker-end arrowhead; interior marker-mid is dropped so short
// legs do not read as solid arrow blobs (see plan p66 rev 2).
func DrawPipePath(pts []Anchor, colour string) string {
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
	fmt.Fprintf(&b, `vector-effect="non-scaling-stroke" `)
	fmt.Fprintf(&b, `marker-end="url(#arrow-%s)"/>`, colour)
	return b.String()
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
// arrows as a flowing pipe, dimmed by the .idle rule so live lines
// pop without losing direction or fluid-system information.
func DrawPipePathIdle(pts []Anchor, colour string) string {
	base := DrawPipePath(pts, colour)
	return strings.Replace(base, `/>`, ` class="idle"/>`, 1)
}

// DrawPipePathDead renders a routed polyline whose flow state is
// positively known to be zero - a shut valve blocking the path.
// Black is the unassigned-pipe colour, so the leg reads as a dead
// line rather than a dimmed hint of its former self; arrows stay
// on for direction.
func DrawPipePathDead(pts []Anchor) string {
	base := DrawPipePath(pts, "black")
	return strings.Replace(base, `/>`, ` class="idle"/>`, 1)
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
	case isSIS(fromID) || isSIS(toID):
		return "sis"
	case isEFWS(fromID) || isEFWS(toID):
		// EFWS pumps and tanks deliver feedwater to the SGs:
		//   "The primary function of the Emergency Feedwater
		//    System (EFWS) ... is to supply feedwater to the
		//    steam generators following accident or transient
		//    conditions when the main feedwater system is not
		//    available."
		//   specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:109-113
		return "blue"
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
	case "VCT", "ChgPump", "BTRS", "BTRSPUMP", "BRS":
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
func isCCWS(id string) bool {
	switch id {
	case "CCWSSurge", "CCWSPump", "CCWSHX":
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
func isSIS(id string) bool {
	switch id {
	case "RWST", "HHSI", "RHRS_A", "RHRS_B", "SISAccumulators",
		"Sump", "InjectionSink", "CSSTank", "SATTank", "CSSPump",
		"CIS", "RABYP", "RBBYP", "RAS1", "RAS2", "RBS1", "RBS2",
		"EAMFV", "EATSA", "EAREC", "EBMFV", "EBTSA", "EBREC":
		return true
	}
	return false
}

// isEFWS reports whether a node id belongs to emergency feedwater.
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
