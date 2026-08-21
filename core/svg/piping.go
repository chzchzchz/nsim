package svg

import (
	"fmt"

	"github.com/chzchzchz/nsim/core"
)

// Pipe-rendering primitives used by the composition pass to
// draw edges from PlantGraph. Helpers accept the same colour
// string returned by ColourForEdge.
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

// DrawPipe returns an SVG fragment for a process pipe from
// (fromX, fromY) to (toX, toY). When elbowed is true the segment
// is broken into horizontal + vertical legs with a small Q-curve
// elbow at the corner.
func DrawPipe(fromX, fromY, toX, toY float64, colour string, elbowed bool) string {
	if !elbowed {
		return fmt.Sprintf(
			`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" `+
				`stroke="%s" stroke-width="2"/>`,
			fromX, fromY, toX, toY, colour)
	}
	return fmt.Sprintf(
		`<path d="M %.1f,%.1f L %.1f,%.1f Q %.1f,%.1f %.1f,%.1f `+
			`L %.1f,%.1f" fill="none" stroke="%s" stroke-width="2"/>`,
		fromX, fromY, fromX, toY,
		fromX, toY, fromX+1, toY,
		toX, toY, colour)
}

// DrawPipeWithValve draws a pipe plus a valve symbol at the
// midpoint. valveKind selects the symbol via SymbolForKind.
func DrawPipeWithValve(fromX, fromY, toX, toY float64, colour, valveKind string) string {
	mx := (fromX + toX) / 2
	my := (fromY + toY) / 2
	pipe := DrawPipe(fromX, fromY, mx, my, colour, false) +
		DrawPipe(mx, my, toX, toY, colour, false)
	return pipe + fmt.Sprintf(
		`<use href="#%s" x="%.1f" y="%.1f" width="24" height="16"/>`,
		SymbolForKind(valveKind), mx-12, my-8)
}

// DrawPipeWithPump draws a pipe plus a pump symbol at the
// midpoint. pumpSymbolId selects the symbol (e.g. "pump-rcp",
// "pump-horizontal", "pump-turbine").
func DrawPipeWithPump(fromX, fromY, toX, toY float64, colour, pumpSymbolId string) string {
	mx := (fromX + toX) / 2
	my := (fromY + toY) / 2
	pipe := DrawPipe(fromX, fromY, mx, my, colour, false) +
		DrawPipe(mx, my, toX, toY, colour, false)
	return pipe + fmt.Sprintf(
		`<use href="#%s" x="%.1f" y="%.1f" width="50" height="40"/>`,
		pumpSymbolId, mx-25, my-20)
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
