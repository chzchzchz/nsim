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

// FluidState exposes a node's water inventory and pressure to
// presentation consumers such as the SVG pipe classifier, letting
// the picture show what the per-edge integrator actually moves.
// ok is false for parts carrying no fluid, e.g. the reactor
// pressure vessel (see the RPV note above).
func (n GraphNode) FluidState() (level Volume, pressure Pressure, ok bool) {
	fn, ok := n.Ref.(fluidNode)
	if !ok {
		return Volume{}, 0, false
	}
	return fn.level(), fn.pressure(), true
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
	// Flow, when set, is the Step's rated transfer rate (gal/s)
	// for a Step-owned edge (see plans/p59_1.txt §1, option B):
	// simulateFluidFlow runs last, after those Steps already
	// moved the inventory, so a Flow edge is audit-only and
	// records the post-Step volume on the sink (see
	// flowRecorder) without moving it again. simulateFluidFlow
	// prefers Flow over deltaP on such edges. Wiring captures
	// the ledger's pointers, so Plant is not needed here.
	Flow func(time.Duration) float64
}

// PlantGraph is a directed graph of fluid-bearing parts.
type PlantGraph struct {
	Nodes map[string]GraphNode
	Edges []GraphEdge
	// fromIdx/toIdx parallel g.Edges, resolved from the node map
	// once per graph build so simulateFluidFlow does not hash
	// string IDs on every tick. -1 means the edge endpoint is
	// absent from Nodes (skipped by the integrator).
	fromIdx []int
	toIdx   []int
	// nodeList orders Nodes for reset iteration without a map
	// walk on the cached hot path.
	nodeList []GraphNode
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

// ReactorPressureVessel deliberately does NOT implement fluidNode.
// The primary and secondary sides are separate fluids that exchange
// only heat across the SG tubes, so the primary-loop edges (hot leg
// RPV->SG, cold leg SG->RPV, surge line RPV<->Pressurizer) carry no
// net volume transfer:
//   "Pressurized water reactors utilize a closed-cycle system design
//    principle. This means that radioactive coolant in the reactor
//    cycle is completely separated from the turbine-generator cycle.
//    [...] The link connecting the two cycles is the steam generator."
//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_064.txt:20-27
// SG inventory is instead governed by the feedwater regulator and
// the steam outlets on the secondary side:
//   "Feedwater flow to each steam generator is controlled by a
//    feedwater regulator in each feedwater line. The regulator is
//    controlled by steam generator level, steam flow and feedwater
//    flow."
//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73

// The condenser's transferable inventory is the hotwell: the
// condensate-pump edge takes suction from it, so level/setLevel read
// and write HotwellLevel. Shell vacuum is not a meaningful static
// head for these liquid edges (the shell is at vacuum, not pressure),
// so pressure() reads zero: with the pump secured the edge sees no
// differential and moves nothing, and a running pump lifts the full
// pump head. capacity() reads zero so the simulator's room clamps
// stay disabled: the graph has no hotwell ceiling — it is a deltaP
// audit-only edge, and the simulator must not invent one (see below).
// The hotwell capacity ceiling lives in Condenser.creditHotwell
// (plans/p64_1.txt), which every Step credit routes through. The
// only graph-side ceiling is the per-edge room clamp applied to
// finite-capacity nodes (Tank, SteamGenerator, Pressurizer —
// capacity() > 0, graph.go:313-348); Condenser is zero-capacity so
// that clamp never fires on it, by design. The only graph write to
// the hotwell is the Condenser -> HDT edge's setLevel on its
// from-node (the condensate-pump suction deltaP debit, graph.go:3489
// edge definition); it only subtracts what the pump head lifts,
// which the Step's own delivery debit (plant.go:2204) does not
// double-move because simulateFluidFlow runs last (graph.go:59-62).
// Because the from-node is zero-capacity, dropping capacity() to
// zero disables the simulator's low-level suction clamp, so setLevel
// clamps at zero itself: an empty hotwell stops delivering.
// "The condensate pumps take suction from the condenser hotwell
//
//	[...]."
//
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-42
func (c *Condenser) pressure() Pressure { return 0 }
func (c *Condenser) level() Volume      { return c.HotwellLevel }
func (c *Condenser) setLevel(v Volume) {
	if v.Val < 0 {
		v = Volume{Val: 0, Unit: v.UnitType()}
	}
	c.HotwellLevel = v
}
func (c *Condenser) capacity() Volume { return zeroVolume }

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

var zeroVolume = Volume{Val: 0, Unit: u.Gallon}

// flowCoefficient is the per-second, per-psi volume transfer
// rate: an edge moves deltaP × flowCoefficient gallons per
// second. Derived sizing assumption (the spec states no value);
// chosen so valve flows respond on observable test timescales.
const flowCoefficient = 1.0 // gal per second per psi

// pumpHead is the head a running pump adds to an edge in psi. Sized to
// match the AFWP shutoff head cited in
// specs/westinghouse_tech_manual/chapter_5/page_0532.txt:9-12 (and the equivalent
// comment block in cmd/nsim/main.go:34-39).
const pumpHead = 1472.0

// flowRecorder optionally records credited volume on a sink node
// (InjectionSink, CIS, ...), which cannot store inventory itself.
// The recorded value is adapter-local (plans/p59_1.txt §5) and
// never feeds back into any Step ledger.
type flowRecorder interface {
	recordFlow(gal float64)
	resetFlow()
}

func (g *PlantGraph) resolveEdgeIndices() {
	g.nodeList = g.nodeList[:0]
	index := make(map[string]int, len(g.Nodes))
	for id, n := range g.Nodes {
		index[id] = len(g.nodeList)
		g.nodeList = append(g.nodeList, n)
	}
	g.fromIdx = g.fromIdx[:0]
	g.toIdx = g.toIdx[:0]
	for _, e := range g.Edges {
		fi, ok := index[e.From]
		if !ok {
			fi = -1
		}
		ti, ok := index[e.To]
		if !ok {
			ti = -1
		}
		g.fromIdx = append(g.fromIdx, fi)
		g.toIdx = append(g.toIdx, ti)
	}
}

func recordGraphFlow(g *PlantGraph, e GraphEdge, dt time.Duration) {
	tn, ok := g.Nodes[e.To]
	if !ok {
		return
	}
	rec, ok := tn.Ref.(flowRecorder)
	if !ok {
		return
	}
	// Audit-only (plans/p59_1.txt §1, option B): the Step
	// already moved the inventory. The Flow closure is invoked
	// with the post-Step plant and the dt the Step just ran
	// on; the sink records that tick's volume.
	rate := 0.0
	if e.Flow != nil {
		rate = e.Flow(dt)
		if rate < 0 {
			rate = 0
		}
	}
	// A closed valve or secured pump delivers nothing this tick.
	if e.Valve != nil && e.Valve.Open <= 0 {
		rate = 0
	}
	if e.Pump != nil && !e.Pump.IsRunning() {
		rate = 0
	}
	rec.recordFlow(rate * dt.Seconds())
}

// simulateFluidFlow advances every edge in g by dt. It mutates the
// parts pointed at by g.Nodes in place.
//
// An optional efwsBlocked predicate, when set, receives the edge's
// tick-visible state and returns true when a Step-owned EFWS delivery
// is active; when EFWS is actively crediting SG water in the same
// tick, the AFW deltaP edges are skipped so the two systems cannot
// both credit SG.Water in one tick.
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
// Edges with a Flow field are Step-owned and audit-only: they
// move nothing here (the Step already did) and only record the
// Step's volume on the sink node.
//
// Spec justification for the per-edge rule: the spec frames flow in
// terms of pressure differential, pumps, and valves
// (specs/the_westinghouse_pressurized_water_react/glossary.md
// hot standby,
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:70,
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:81-82).
func simulateFluidFlow(g *PlantGraph, dt time.Duration, efwsBlocked ...bool) {
	if g == nil {
		return
	}
	efwsActive := false
	if len(efwsBlocked) > 0 && efwsBlocked[0] {
		efwsActive = true
	}
	dtSec := dt.Seconds()
	if g.fromIdx == nil {
		g.resolveEdgeIndices()
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		if g.fromIdx[i] < 0 || g.toIdx[i] < 0 {
			continue
		}
		fn := g.nodeList[g.fromIdx[i]]
		tn := g.nodeList[g.toIdx[i]]
		// Step-owned flow edges are audit-only (plans/p59_1.txt
		// §1, option B): the owning Step already moved the volume
		// and simulateFluidFlow runs last (plant.go:2362), so moving
		// it here would count the same gallons twice. Record the
		// tick's volume on the sink instead and skip deltaP.
		if e.Flow != nil {
			recordGraphFlow(g, *e, dt)
			continue
		}
		// Interlock: when EFWS is actively crediting SG water this
		// tick, the Outside AFW deltaP edges must not also credit
		// SG.Water. The tie-break is EFWS-wins; the AFW graph edges
		// are skipped whenever EFWS delivered flow above zero.
		if efwsActive && (e.From == "CST" && (e.To == "DieselAFWPump" || e.To == "MotorAFWPump" || e.To == "TurbineAFWPump") || (e.To == "SG" && (e.From == "DieselAFWPump" || e.From == "MotorAFWPump" || e.From == "TurbineAFWPump"))) {
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
		// (Table 3.4-1, specs/the_westinghouse_pressurized_water_react//chapter_3/page_065.txt:24-61).
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
	unit := v.UnitType()
	if unit.Name == "" {
		unit = u.Gallon
	}
	return Volume{Val: g, Unit: unit}
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
