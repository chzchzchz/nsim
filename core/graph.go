package core

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	u "github.com/bcicen/go-units"
)

// GraphNode identifies one fluid-bearing part in the plant graph.
//
// Spec justification:
//   - "The reactor coolant cold leg is the reactor coolant return line
//     from the steam generator to the reactor vessel." (glossary.md
//     cold leg, p. 51)
//   - "The reactor coolant hot leg is the reactor coolant exit line from
//     the reactor vessel to the steam generator." (glossary.md hot leg,
//     p. 87)
//     These two lines are the primary physical edges in a PWR; the graph
//     below represents them explicitly.
type GraphNode struct {
	ID   string
	Kind string
	Ref  interface{}
}

// GraphEdge is one directed fluid connection between two nodes. The
// optional Valve modulates the driving pressure delta; the optional Pump
// adds head when running.
//
// Spec justification:
//   - "piping configurations and valving"
//     (specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:70)
//   - "pumps directed into common pipe header"
//     (specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:81-82)
type GraphEdge struct {
	From  string
	To    string
	Valve *Valve
	Pump  *Pump
}

// PlantGraph is a directed graph of fluid-bearing parts.
type PlantGraph struct {
	Nodes map[string]GraphNode
	Edges []GraphEdge
}

// fluidNode is the minimal interface a graph node must implement for the
// per-edge flow integrator to read its pressure and transfer volume in
// and out. Most fluid-bearing parts in core/parts.go already satisfy it.
type fluidNode interface {
	pressure() Pressure
	level() Volume
	setLevel(Volume)
	capacity() Volume
}

func (t *Tank) pressure() Pressure { return t.Pressure }
func (t *Tank) level() Volume      { return t.Level }
func (t *Tank) setLevel(v Volume)  { t.Level = v }
func (t *Tank) capacity() Volume   { return t.Capacity }

func (s *SteamGenerator) pressure() Pressure { return s.Pressure }
func (s *SteamGenerator) level() Volume      { return s.Water }
func (s *SteamGenerator) setLevel(v Volume)  { s.Water = v }
func (s *SteamGenerator) capacity() Volume   { return s.Volume }

func (p *Pressurizer) pressure() Pressure { return p.Pressure }
func (p *Pressurizer) level() Volume      { return p.Water }
func (p *Pressurizer) setLevel(v Volume)  { p.Water = v }
func (p *Pressurizer) capacity() Volume   { return p.Volume }

func (r *ReactorPressureVessel) pressure() Pressure { return r.Pressure }
func (r *ReactorPressureVessel) level() Volume      { return zeroVolume }
func (r *ReactorPressureVessel) setLevel(Volume)    {}
func (r *ReactorPressureVessel) capacity() Volume   { return zeroVolume }

func (c *Condenser) pressure() Pressure { return c.Pressure }
func (c *Condenser) level() Volume      { return zeroVolume }
func (c *Condenser) setLevel(Volume)    {}
func (c *Condenser) capacity() Volume   { return zeroVolume }

// LP and HP heater drains are bookkeeping edges in the plant
// graph. The heater Steps accumulate drain volume directly into
// their destination tanks (condenser for LP, deaerator tank for
// HP) per plans/p10_1.txt; the fluid-graph simulator never
// transfers volume across these edges, so the implementations
// return zero everywhere and satisfy the fluidNode interface.
func (l *LowPressureHeater) pressure() Pressure { return 0 }
func (l *LowPressureHeater) level() Volume      { return zeroVolume }
func (l *LowPressureHeater) setLevel(Volume)    {}
func (l *LowPressureHeater) capacity() Volume   { return zeroVolume }

func (h *HighPressureHeater) pressure() Pressure { return 0 }
func (h *HighPressureHeater) level() Volume      { return zeroVolume }
func (h *HighPressureHeater) setLevel(Volume)    {}
func (h *HighPressureHeater) capacity() Volume   { return zeroVolume }

var zeroVolume = u.NewValue(0, u.Gallon)

// flowCoefficient is the per-second, per-psi volume transfer rate.
// Sized so a 100 psid drop across an open valve moves ~1 gpm — fast
// enough to be observable on test timescales.
const flowCoefficient = 60.0 / 60.0 // gal per second per psi

// pumpHead is the head a running pump adds to an edge in psi. Sized to
// match the AFWP shutoff head cited in
// specs/westinghouse_tech_manual/chapter_5/page_0532.txt:9-12 (and the equivalent
// comment block in cmd/nsim/main.go:34-39).
const pumpHead = 1472.0

// simulateFluidFlow advances every edge in g by dt. It mutates the
// parts pointed at by g.Nodes in place.
//
// Per-edge rule:
//
//	deltaP = (from.P - to.P)
//	       + (pump head when Pump.Running)
//	       * (Valve.Open if Valve != nil, else 1.0)
//
//	flow = deltaP * flowCoefficient * dt.Seconds()
//
//	transfer `flow` from from.Level to to.Level, clamped so neither
//	tank drops below zero.
//
// Spec justification for the per-edge rule: the spec frames flow in
// terms of pressure differential, pumps, and valves
// (specs/the_westinghouse_pressurized_water_react/glossary.md
// hot standby,
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:70,
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:81-82).
func simulateFluidFlow(g *PlantGraph, dt time.Duration) {
	if g == nil {
		return
	}
	dtSec := dt.Seconds()
	for _, e := range g.Edges {
		fn, ok := g.Nodes[e.From]
		if !ok {
			continue
		}
		tn, ok := g.Nodes[e.To]
		if !ok {
			continue
		}
		from, fok := fn.Ref.(fluidNode)
		to, tok := tn.Ref.(fluidNode)
		if !fok || !tok {
			continue
		}

		deltaP := float64(from.pressure() - to.pressure())
		if e.Pump != nil && e.Pump.Running {
			head := pumpHead
			// Other pumps keep full head because they have
			// no speed dynamics yet; once their Velocity is
			// non-zero the head scales with speed. The RCP
			// is the canonical case: "Design head 280 ft"
			// is achieved at RCPRatedSpeed (1189 rpm), and
			// lower speeds produce proportionally less head.
			//
			// Spec justification:
			//   - Design point: "Design head 280 ft (85.3 m)"
			//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:124-130
			//   - Coastdown: "long coastdown times [...] to
			//     preclude rapid decreases in core cooling
			//     flow during pump trips."
			//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_020.txt:83-84
			if e.Pump.Velocity > 0 {
				head *= float64(e.Pump.Velocity) /
					float64(RCPRatedSpeed)
			}
			deltaP += head
		}
		if e.Valve != nil {
			deltaP *= e.Valve.Open
		}
		if deltaP == 0 {
			continue
		}

		flow := deltaP * flowCoefficient * dtSec

		fromLevel := from.level().Float()
		toLevel := to.level().Float()

		// Clamp the source only if it has a finite capacity (i.e. it
		// is a Tank-like node). A capacity of zero means "infinite
		// source/sink" — common for ReactorPressureVessel and Condenser
		// which model pressure, not inventory.
		fromCap := from.capacity().Float()
		toCap := to.capacity().Float()
		if flow > 0 && fromCap > 0 && fromLevel < flow {
			flow = fromLevel
		} else if flow < 0 && toCap > 0 && toLevel < -flow {
			flow = -toLevel
		}
		// Clamp the destination so it cannot exceed its
		// finite capacity (e.g. SteamGenerator shell volume).
		// Without this the destination monotonically grows
		// past Volume in the SG case because the primary-loop
		// edges push volume in faster than the boil rate pulls
		// it out at low primary-to-secondary ΔT.
		// Spec: the SG shell has a finite design volume
		// (Table 3.4-1, specs/.../chapter_3/page_065.txt:24-61).
		// Note that for negative flow the actual transfer
		// reverses: the "to" node is the source and the "from"
		// node is the destination, so the capacity clamp must
		// apply to whichever node is receiving volume.
		if flow > 0 && toCap > 0 {
			room := toCap - toLevel
			if room <= 0 {
				flow = 0
			} else if room < flow {
				flow = room
			}
		}
		if flow < 0 && fromCap > 0 {
			// flow < 0 means volume moves from `to` to `from`,
			// so `from` is the receiver. Clamp `from` to its
			// capacity.
			room := fromCap - fromLevel
			if room <= 0 {
				flow = 0
			} else if -flow > room {
				flow = -room
			}
		}
		if flow == 0 {
			continue
		}

		from.setLevel(vol(from.level(), fromLevel-flow))
		to.setLevel(vol(to.level(), toLevel+flow))
	}
}

// vol returns a Volume with the same unit as `v` and the given numeric
// value, falling back to gallon for a zero-unit value.
func vol(v Volume, g float64) Volume {
	unit := v.Unit()
	if unit.Name == "" {
		unit = u.Gallon
	}
	return u.NewValue(g, unit)
}

// ExportDot serialises g in Graphviz DOT format.
func (g *PlantGraph) ExportDot(w io.Writer) error {
	if _, err := fmt.Fprintln(w, "digraph plant {"); err != nil {
		return err
	}
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.Nodes[id]
		label := strings.TrimSpace(n.ID + " " + n.Kind)
		if _, err := fmt.Fprintf(w, "  %q [label=%q];\n", n.ID, label); err != nil {
			return err
		}
	}
	for _, e := range g.Edges {
		attr := ""
		switch {
		case e.Valve != nil:
			attr = fmt.Sprintf(" [label=%q]", valveLabel(e.Valve))
		case e.Pump != nil:
			attr = fmt.Sprintf(" [label=%q]", pumpLabel(e.Pump))
		}
		if _, err := fmt.Fprintf(w, "  %q -> %q%s;\n", e.From, e.To, attr); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "}")
	return err
}

func valveLabel(v *Valve) string {
	if v == nil {
		return "valve"
	}
	return fmt.Sprintf("valve %.0f%%", v.Open*100)
}

func pumpLabel(p *Pump) string {
	if p == nil {
		return "pump"
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
