// Package svg renders the simulator's plant state as a hand-composed
// P&ID-style SVG. The asset library is split across sibling files
// keyed by part family (vessel, pump, valve, tank, heatexchanger,
// turbine, generator, transformer, diesel, containment,
// safety_systems, piping); this file holds the package-level
// layout grid, the document viewBox, and the entry point that
// the simulator calls from (*Plant).ExportPlantSVG.
//
// Spec justification for the P&ID layout (chapter 3 figure 3-1 +
// chapter 15 figure 15.3-1):
//   - "Figure 3-1 is a schematic flow diagram (one-loop) of the
//     RCS. Each loop contains a reactor coolant pump and a steam
//     generator. The RCS also includes a pressurizer, safety and
//     relief valves, a pressurizer relief tank [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_043.txt:7-19
//   - "A flow diagram of a typical steam, condensate, and
//     feedwater system is shown in Figure 15.3-1."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:28-29
package svg

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/chzchzchz/nsim/core"
)

// Document dimensions in user-space units.
const (
	DocumentWidth  = 1200.0
	DocumentHeight = 900.0
)

// Layout grid coordinates for each major component. The grid
// follows the chapter-3 / chapter-15 figures: nuclear island on
// the left, turbine island on the right, containment dome on the
// far left, electrical / diesel at the bottom.
//
// The four-loop plant's four SGs and four RCPs are collapsed into
// a single SG and single RCP, with a "(×4)" label added by the
// live overlay.
const (
	// Reactor building bounds: RPV at the centre, with one
	// lumped SG (×4) to its right and the RCP in between.
	RPVX, RPVY             float64 = 350, 450
	PressurizerX, PressurizerY float64 = 540, 280
	PRTX, PRTY             float64 = 680, 200
	RCPX, RCPY             float64 = 540, 530
	SGX, SGY               float64 = 720, 450

	// Turbine island.
	TurbineX, TurbineY         float64 = 900, 450
	GeneratorX, GeneratorY     float64 = 1050, 450
	CondenserX, CondenserY     float64 = 950, 700

	// Feedwater side: CST far right top, condensate pump +
	// LP heater + deaerator (HDT) cascade back to the SG.
	CSTX, CSTY           float64 = 1100, 250
	CondPumpX, CondPumpY float64 = 1050, 250
	LPHeaterX, LPHeaterY float64 = 950, 250
	HDTX, HDTY           float64 = 850, 250
	HPPumpX, HPPumpY     float64 = 800, 320
	HPHeaterX, HPHeaterY float64 = 800, 380
	FWCtrlX, FWCtrlY     float64 = 760, 450

	// Safety systems: SIS accumulators and EFWS tank lower-left.
	SISX, SISY               float64 = 100, 200
	EFWSTankX, EFWSTankY     float64 = 100, 350
	RHRSTrainAX, RHRSTrainAY float64 = 400, 700
	RHRSTrainBX, RHRSTrainBY float64 = 500, 700

	// Containment dome on the far left.
	ContainmentX, ContainmentY float64 = 150, 500

	// Electrical: diesel generator, station transformer, ESF
	// buses, grid.
	DieselX, DieselY                     float64 = 100, 800
	FuelTankX, FuelTankY                 float64 = 100, 850
	StationTransformerX, StationTransformerY float64 = 250, 800
	ESFBusAX, ESFBusAY                   float64 = 400, 850
	ESFBusBX, ESFBusBY                   float64 = 450, 850
	GridX, GridY                         float64 = 1100, 100

	// AFWPumps inside the EFWS pump building (under the
	// SG outlets).
	DieselAFWX, DieselAFWY   float64 = 600, 380
	MotorAFWX, MotorAFWY     float64 = 620, 360
	TurbineAFWX, TurbineAFWY float64 = 640, 340

	// CCWS heat exchanger (one for the safeguards train).
	CCWSHXX, CCWSHXY float64 = 250, 720

	// SFP at the lower-centre.
	SFPX, SFPY float64 = 700, 780
)

// anchor describes the document-space coordinate of a part's
// primary anchor (used for pipe routing) plus a flag indicating
// whether the anchor should be the centre of the part or one of
// the named SVG anchors exposed by the part's <symbol>.
type anchor struct {
	X, Y float64
}

// anchors maps node IDs from BuildGraphFor to the layout
// coordinates computed by WritePlantSVG. New part placements
// should add entries here so the pipe layer can find the source
// and destination of each edge.
var anchors = map[string]anchor{
	// Reactor primary
	"RPV":           {RPVX, RPVY},
	"Pressurizer":   {PressurizerX, PressurizerY},
	"ReactorCoolantPump": {RCPX, RCPY},
	"SG":            {SGX, SGY},

	// Secondary
	"Turbine":       {TurbineX, TurbineY},
	"Condenser":     {CondenserX, CondenserY},
	"CST":           {CSTX, CSTY},
	"HDT":           {HDTX, HDTY},
	"LPHeaterDrain": {LPHeaterX, LPHeaterY},
	"HPHeaterDrain": {HPHeaterX, HPHeaterY},

	// Pressurizer relief
	"PressurizedReliefTank": {PRTX, PRTY},

	// AFW pumps
	"DieselAFWPump":  {DieselAFWX, DieselAFWY},
	"MotorAFWPump":   {MotorAFWX, MotorAFWY},
	"TurbineAFWPump": {TurbineAFWX, TurbineAFWY},
}

// anchorFor returns the layout coordinate of the named graph
// node. Unknown nodes fall back to (0, 0).
func anchorFor(id string) anchor {
	if a, ok := anchors[id]; ok {
		return a
	}
	return anchor{0, 0}
}

// WritePlantSVG writes a hand-composed P&ID-style SVG of the
// given plant to w. The diagram is built from the asset library
// in this package.
func WritePlantSVG(w io.Writer, p *core.Plant) error {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" `+
			`viewBox="0 0 %.0f %.0f">`+"\n",
		DocumentWidth, DocumentHeight)
	b.WriteString(`<title>PWR plant P\u0026ID</title>` + "\n")
	b.WriteString(`<desc>Hand-composed P\u0026ID-style SVG of the ` +
		`simulator plant, built from the asset library under ` +
		`core/svg. Source-of-truth citations live in plans/svg/.</desc>` + "\n")

	// defs: every symbol referenced by the diagram. Symbols
	// are loaded from the embedded asset library in
	// core/svg/assets.
	b.WriteString("<defs>\n")
	b.WriteString(ContainmentAsset)
	b.WriteString(SafetySystemsAsset)
	b.WriteString(ReactorVesselSymbol + "\n")
	b.WriteString(PressurizerSymbol + "\n")
	b.WriteString(SteamGeneratorSymbol + "\n")
	b.WriteString(CondenserSymbol + "\n")
	b.WriteString(PumpAsset)
	b.WriteString(ValveAsset)
	b.WriteString(TankAsset)
	b.WriteString(HeatExchangerAsset)
	b.WriteString(TurbineAsset)
	b.WriteString(GeneratorAsset)
	b.WriteString(TransformerAsset)
	b.WriteString(DieselAsset)
	b.WriteString("</defs>\n")

	// Background: containment dome on the far left.
	fmt.Fprintf(&b,
		`<use href="#containment-dome" x="%.0f" y="%.0f" `+
			`width="280" height="280"/>`+"\n",
		ContainmentX-140, ContainmentY-140)

	// CSS spray headers overlay inside the dome.
	fmt.Fprintf(&b,
		`<use href="#css-spray-headers" x="%.0f" y="%.0f" `+
			`width="240" height="160"/>`+"\n",
		ContainmentX-120, ContainmentY-130)

	// Four RCFC units in a 2x2 grid inside the dome.
	for dy := 0; dy < 2; dy++ {
		for dx := 0; dx < 2; dx++ {
			fmt.Fprintf(&b,
				`<use href="#rcfc-unit" x="%.0f" y="%.0f" `+
					`width="40" height="40"/>`+"\n",
				ContainmentX-90+float64(dx)*70,
				ContainmentY-40+float64(dy)*70)
		}
	}

	// RPV at the centre of the reactor building.
	fmt.Fprintf(&b,
		`<use href="#reactor_vessel" x="%.0f" y="%.0f" `+
			`width="100" height="240"/>`+"\n",
		RPVX-50, RPVY-120)

	// Pressurizer to the upper-right of the RPV.
	fmt.Fprintf(&b,
		`<use href="#pressurizer" x="%.0f" y="%.0f" `+
			`width="60" height="320"/>`+"\n",
		PressurizerX-30, PressurizerY-160)

	// Pressurizer relief tank to the right of the pressurizer.
	fmt.Fprintf(&b,
		`<use href="#tank-vertical" x="%.0f" y="%.0f" `+
			`width="60" height="100"/>`+"\n",
		PRTX-30, PRTY-50)
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">PRT</text>`+"\n",
		PRTX, PRTY)

	// RCP between RPV and SG.
	fmt.Fprintf(&b,
		`<use href="#pump-rcp" x="%.0f" y="%.0f" `+
			`width="50" height="160"/>`+"\n",
		RCPX-25, RCPY-80)

	// SG to the right of the RCP.
	fmt.Fprintf(&b,
		`<use href="#steam_generator" x="%.0f" y="%.0f" `+
			`width="140" height="400"/>`+"\n",
		SGX-70, SGY-200)
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">(x4)</text>`+"\n",
		SGX, SGY+220)

	// Pressurizer safety + relief valves (3 + 2).
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b,
			`<use href="#valve-safety" x="%.0f" y="%.0f" `+
				`width="20" height="20"/>`+"\n",
			PressurizerX-22+float64(i)*14,
			PressurizerY-176)
	}
	for i := 0; i < 2; i++ {
		fmt.Fprintf(&b,
			`<use href="#valve-porv" x="%.0f" y="%.0f" `+
				`width="24" height="20"/>`+"\n",
			PressurizerX-12+float64(i)*14,
			PressurizerY-186)
	}

	// Turbine, generator, condenser.
	fmt.Fprintf(&b,
		`<use href="#turbine-main" x="%.0f" y="%.0f" `+
			`width="360" height="200"/>`+"\n",
		TurbineX-180, TurbineY-100)
	fmt.Fprintf(&b,
		`<use href="#generator-main" x="%.0f" y="%.0f" `+
			`width="220" height="140"/>`+"\n",
		GeneratorX-110, GeneratorY-70)
	fmt.Fprintf(&b,
		`<use href="#condenser" x="%.0f" y="%.0f" `+
			`width="400" height="100"/>`+"\n",
		CondenserX-200, CondenserY-50)

	// Feedwater chain: CST -> condensate pump -> LP heater ->
	// HDT (deaerator) -> feed pump -> HP heater -> SG.
	fmt.Fprintf(&b,
		`<use href="#tank-cst" x="%.0f" y="%.0f" `+
			`width="100" height="130"/>`+"\n",
		CSTX-50, CSTY-65)
	fmt.Fprintf(&b,
		`<use href="#pump-horizontal" x="%.0f" y="%.0f" `+
			`width="60" height="40"/>`+"\n",
		CondPumpX-30, CondPumpY-20)
	fmt.Fprintf(&b,
		`<use href="#heat-exchanger-vertical" x="%.0f" y="%.0f" `+
			`width="50" height="100"/>`+"\n",
		LPHeaterX-25, LPHeaterY-50)
	fmt.Fprintf(&b,
		`<use href="#tank-horizontal" x="%.0f" y="%.0f" `+
			`width="100" height="50"/>`+"\n",
		HDTX-50, HDTY-25)
	fmt.Fprintf(&b,
		`<use href="#pump-horizontal" x="%.0f" y="%.0f" `+
			`width="60" height="40"/>`+"\n",
		HPPumpX-30, HPPumpY-20)
	fmt.Fprintf(&b,
		`<use href="#heat-exchanger-vertical" x="%.0f" y="%.0f" `+
			`width="50" height="100"/>`+"\n",
		HPHeaterX-25, HPHeaterY-50)

	// Main feedwater control valve (modulating) on the
	// SG-inlet pipe.
	fmt.Fprintf(&b,
		`<use href="#valve-modulating" x="%.0f" y="%.0f" `+
			`width="24" height="20"/>`+"\n",
		FWCtrlX-12, FWCtrlY-10)

	// AFW pumps near the SG inlet.
	fmt.Fprintf(&b,
		`<use href="#pump-turbine" x="%.0f" y="%.0f" `+
			`width="60" height="40"/>`+"\n",
		TurbineAFWX-30, TurbineAFWY-20)
	fmt.Fprintf(&b,
		`<use href="#pump-horizontal" x="%.0f" y="%.0f" `+
			`width="60" height="40"/>`+"\n",
		MotorAFWX-30, MotorAFWY-20)
	fmt.Fprintf(&b,
		`<use href="#pump-horizontal" x="%.0f" y="%.0f" `+
			`width="60" height="40"/>`+"\n",
		DieselAFWX-30, DieselAFWY-20)

	// SIS accumulators (one cluster of 4).
	fmt.Fprintf(&b,
		`<use href="#sis-accumulators" x="%.0f" y="%.0f" `+
			`width="200" height="80"/>`+"\n",
		SISX-100, SISY-40)

	// EFWS tank.
	fmt.Fprintf(&b,
		`<use href="#efws-tank" x="%.0f" y="%.0f" `+
			`width="80" height="100"/>`+"\n",
		EFWSTankX-40, EFWSTankY-50)

	// CCWS heat exchanger.
	fmt.Fprintf(&b,
		`<use href="#ccws-hx" x="%.0f" y="%.0f" `+
			`width="100" height="50"/>`+"\n",
		CCWSHXX-50, CCWSHXY-25)

	// RHRS trains.
	fmt.Fprintf(&b,
		`<use href="#rhrs-train" x="%.0f" y="%.0f" `+
			`width="200" height="80"/>`+"\n",
		RHRSTrainAX-100, RHRSTrainAY-40)
	fmt.Fprintf(&b,
		`<use href="#rhrs-train" x="%.0f" y="%.0f" `+
			`width="200" height="80"/>`+"\n",
		RHRSTrainBX-100, RHRSTrainBY-40)

	// SFP.
	fmt.Fprintf(&b,
		`<use href="#tank-sfp" x="%.0f" y="%.0f" `+
			`width="200" height="100"/>`+"\n",
		SFPX-100, SFPY-50)

	// Electrical: diesel + fuel tank, station transformer,
	// ESF buses, grid.
	fmt.Fprintf(&b,
		`<use href="#diesel-generator" x="%.0f" y="%.0f" `+
			`width="200" height="100"/>`+"\n",
		DieselX-100, DieselY-50)
	fmt.Fprintf(&b,
		`<use href="#fuel-tank" x="%.0f" y="%.0f" `+
			`width="30" height="40"/>`+"\n",
		FuelTankX-15, FuelTankY-20)
	fmt.Fprintf(&b,
		`<use href="#transformer-2w" x="%.0f" y="%.0f" `+
			`width="50" height="50"/>`+"\n",
		StationTransformerX-25, StationTransformerY-25)
	fmt.Fprintf(&b,
		`<use href="#esf-bus" x="%.0f" y="%.0f" `+
			`width="120" height="20"/>`+"\n",
		ESFBusAX-60, ESFBusAY-10)
	fmt.Fprintf(&b,
		`<use href="#esf-bus" x="%.0f" y="%.0f" `+
			`width="120" height="20"/>`+"\n",
		ESFBusBX-60, ESFBusBY-10)
	fmt.Fprintf(&b,
		`<use href="#grid-bus" x="%.0f" y="%.0f" `+
			`width="30" height="50"/>`+"\n",
		GridX-15, GridY-25)

	// Pipes + inline valves / pumps. Walk the build graph so
	// the diagram is faithful to the simulator's actual edge
	// set.
	g := p.BuildGraphFor(p)
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, e := range g.Edges {
		from := anchorFor(e.From)
		to := anchorFor(e.To)
		colour := ColourForEdge(e.From, e.To)
		if e.Valve != nil {
			kind := ValveKindForEdge(p, e)
			b.WriteString(DrawPipeWithValve(
				from.X, from.Y, to.X, to.Y,
				colour, kind))
			b.WriteString("\n")
		} else if e.Pump != nil {
			sym := SymbolForPumpByID(p, e.Pump, e.From)
			b.WriteString(DrawPipeWithPump(
				from.X, from.Y, to.X, to.Y, colour, sym))
			b.WriteString("\n")
		} else {
			b.WriteString(DrawPipe(
				from.X, from.Y, to.X, to.Y, colour, false))
			b.WriteString("\n")
		}
	}

	// Live overlays: text labels for valve open %, pump RPM,
	// level percent, and alarm rings.
	writeLiveOverlays(&b, p)

	b.WriteString("</svg>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// writeLiveOverlays emits the per-part live state as <text>
// elements and any alarm-coloured rings.
func writeLiveOverlays(b *strings.Builder, p *core.Plant) {
	// RCP RPM.
	if p.ReactorBuilding.ReactorCoolantPump.Running ||
		p.ReactorBuilding.ReactorCoolantPump.Velocity > 0 {
		fmt.Fprintf(b,
			`<text x="%.0f" y="%.0f" font-size="10" `+
				`text-anchor="middle">%d rpm</text>`+"\n",
			RCPX, RCPY+90,
			int(p.ReactorBuilding.ReactorCoolantPump.Velocity))
	}

	// Generator terminal voltage + frequency.
	g := p.TurbineBuilding.Generator
	fmt.Fprintf(b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">%.1f kV / %.1f Hz</text>`+"\n",
		GeneratorX, GeneratorY+90,
		float64(g.Voltage)/1000.0,
		float64(g.Frequency))

	// SG water level percent.
	sg := p.ReactorBuilding.SteamGenerator
	if sg.Volume.Float() > 0 {
		pct := 100.0 * sg.Water.Float() / sg.Volume.Float()
		fmt.Fprintf(b,
			`<text x="%.0f" y="%.0f" font-size="10" `+
				`text-anchor="middle">SG %.0f%%</text>`+"\n",
			SGX, SGY+230, pct)
	}

	// Condenser pressure.
	cd := p.TurbineBuilding.Condenser
	fmt.Fprintf(b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">%.2f psia</text>`+"\n",
		CondenserX, CondenserY+60,
		float64(cd.Pressure))

	// Containment pressure + dome red ring above design.
	cp := p.ReactorBuilding.ContainmentPressure - core.AtmosphericPressure
	fmt.Fprintf(b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">Cont %.1f psig</text>`+"\n",
		ContainmentX, ContainmentY+170, float64(cp))
	if cp > 30 {
		fmt.Fprintf(b,
			`<circle cx="%.0f" cy="%.0f" r="140" `+
				`fill="none" stroke="red" stroke-width="3"/>`+"\n",
			ContainmentX, ContainmentY)
	}

	// Turbine red ring on trip.
	if p.TurbineBuilding.Turbine.State == core.TurbineTripped {
		fmt.Fprintf(b,
			`<circle cx="%.0f" cy="%.0f" r="110" `+
				`fill="none" stroke="red" stroke-width="3"/>`+"\n",
			TurbineX, TurbineY)
	}

	// Diesel running green ring.
	if p.Outside.DieselGenerator.State == core.DieselRunning {
		fmt.Fprintf(b,
			`<circle cx="%.0f" cy="%.0f" r="55" `+
				`fill="none" stroke="green" stroke-width="2"/>`+"\n",
			DieselX, DieselY)
	}

	// Reactor trip red ring.
	if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
		fmt.Fprintf(b,
			`<circle cx="%.0f" cy="%.0f" r="60" `+
				`fill="none" stroke="red" stroke-width="3"/>`+"\n",
			RPVX, RPVY)
	}
}
