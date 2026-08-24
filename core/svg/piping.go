package svg

import (
	"fmt"
	"strings"

	"github.com/chzchzchz/nsim/core"
)

// Pipe-rendering primitives used by the composition pass to draw
// edges from PlantGraph. Helpers accept the same colour string
// returned by ColourForEdge.
//
// Pipe colour convention (Westinghouse P&ID):
//
//	red    — primary coolant (RCS)
//	brown  — steam / condensate (secondary)
//	blue   — feedwater / auxiliary feedwater
//	green  — service water
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
// document defines one flow-arrow marker per colour.
var pipeColours = []string{"red", "blue", "brown", "black"}

// PipeArrowMarkers emits one arrowhead <marker> per pipe colour.
// Polylines attach them with marker-mid so arrowheads appear on
// every interior vertex, oriented along the local segment.
func PipeArrowMarkers() string {
	var b strings.Builder
	for _, c := range pipeColours {
		fmt.Fprintf(&b,
			`<marker id="arrow-%s" viewBox="0 0 10 10" refX="9" refY="5" `+
				`markerWidth="5" markerHeight="5" orient="auto-start-reverse">`+
				`<path d="M 0 0 L 10 5 L 0 10 z" fill="%s"/></marker>`+"\n",
			c, c)
	}
	return b.String()
}

// DrawPipePath renders dot's routed polyline for one edge as a
// single SVG polyline so pipes follow graphviz's routing instead
// of cutting straight across the drawing. marker-mid drops a flow
// arrowhead on every interior vertex, oriented along the local
// segment direction.
func DrawPipePath(pts []Anchor, colour string) string {
	var b strings.Builder
	b.WriteString(`<polyline points="`)
	for i, p := range pts {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%.1f,%.1f", p.X, p.Y)
	}
	fmt.Fprintf(&b, `" fill="none" stroke="%s" stroke-width="2" `, colour)
	fmt.Fprintf(&b, `marker-mid="url(#arrow-%s)"/>`, colour)
	return b.String()
}

// DrawPipePathWithValve renders a routed pipe plus a valve symbol
// anchored at `at`: the hop node's laid-out position when known,
// otherwise the path midpoint. vw/vh give the drawn valve size so
// it scales with the layout.
func DrawPipePathWithValve(pts []Anchor, colour, valveKind string, at Anchor, vw, vh float64) string {
	pipe := DrawPipePath(pts, colour)
	return pipe + fmt.Sprintf(
		`<use href="#%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
		SymbolForKind(valveKind), at.X-vw/2, at.Y-vh/2, vw, vh)
}

// ColourForEdge returns the pipe colour for an edge between the
// given source and destination graph node IDs.
func ColourForEdge(fromID, toID string) string {
	switch {
	case fromID == "RPV" || toID == "RPV" ||
		fromID == "Pressurizer" || toID == "Pressurizer" ||
		fromID == "RCP" || toID == "RCP":
		return "red"
	case fromID == "SG" || toID == "SG":
		return "blue"
	case fromID == "Condenser" || toID == "Condenser":
		return "brown"
	}
	return "black"
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
	case e.From == "SG" && e.To == "Condenser":
		return "modulating"
	case e.From == "Condenser" && e.To == "CST":
		return "stop"
	case e.To == "SG" || e.To == "DieselAFWPump" ||
		e.To == "MotorAFWPump" || e.To == "TurbineAFWPump":
		return "gate"
	}
	return "gate"
}
