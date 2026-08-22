// Package svg renders the simulator's plant state as a hand-composed
// P&ID-style SVG. The asset library is split across sibling files
// keyed by part family (vessel, pump, valve, tank, heatexchanger,
// turbine, generator, transformer, diesel, containment,
// safety_systems, piping); this file holds the package-level
// layout grid, the document viewBox, and the entry point that
// the simulator calls from (*Plant).ExportPlantSVG.
//
// Spec justification for the P&ID layout (chapter 3 figure 3-1 + chapter 15 figure 15.3-1):
//   - "Figure 3-1 is a schematic flow diagram (one-loop) of the RCS. Each loop
//     contains a reactor coolant pump and a steam generator. The RCS also
//     includes a pressurizer, safety and relief valves, a pressurizer relief
//     tank [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_043.txt:12-18
//   - "A flow diagram of a typical steam, condensate, and feedwater system is
//     shown in Figure 15.3-1."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:27-28
//   - "The containment building [...] is a cylindrical, steel lined, concrete
//     pressure vessel with a hemispherical top closure and stepped flat
//     bottom. The building encloses the reactor coolant system [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:28-42
//   - "The main steam lines are provided with a stop valve and a check valve in
//     each line just outside the containment."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:70-73
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
// follows the chapter-3 / chapter-15 figures: containment on the
// left enclosing the nuclear island, turbine building radially to
// the right, feedwater chain as a ladder above the turbine,
// safety systems inside the containment base, AFW pumps below
// the containment, and electrical (diesel, transformer, ESF buses)
// at the bottom-left.
//
// The containment dimensions and separation from the turbine are
// justified by:
//   - "The containment building and contiguous buildings or compartments
//     around it house the systems and equipment of the nuclear island."
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:28-34
//   - "Of particular note to this view is the radial orientation of the
//     turbine building to the containment building."
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:74-78
//   - Stop valves "just outside the containment" leave a separation gap.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:70-73
//
// The four-loop plant's four SGs and four RCPs are collapsed into
// a single SG and single RCP, with a "(×4)" label added by the
// live overlay.
const (
	// Containment dome dimensions: large enough to enclose the RPV,
	// pressurizer, SG, and RCP (the nuclear island).
	ContainmentX, ContainmentY float64 = 260, 500
	ContainmentW                         float64 = 480 // cylinder width
	ContainmentH                         float64 = 440 // cylinder + arch height

	// Reactor pressure vessel — centred inside the containment.
	RPVX, RPVY float64 = 260, 510

	// Pressurizer — upper-right quadrant inside the containment,
	// connected to the RPV hot leg via a short surge line.
	PressurizerX, PressurizerY float64 = 380, 330

	// Pressurizer relief tank — above the pressurizer, inside dome.
	PRTX, PRTY                 float64 = 420, 200

	// Steam generator — right of the RPV, inside containment.
	// Only one shown; "(×4)" label added by live overlay.
	SGX, SGY float64 = 430, 500

	// RCP — between the RPV and SG on the hot leg.
	RCPX, RCPY float64 = 350, 540

	// Turbine island — right of containment with separation gap
	// for main steam stop valves ("just outside the containment").
	TurbineX, TurbineY     float64 = 900, 470
	GeneratorX, GeneratorY float64 = 1150, 470
	CondenserX, CondenserY float64 = 1010, 710

	// Feedwater chain — ladder above the turbine.
	CSTX, CSTY           float64 = 1090, 170
	CondPumpX, CondPumpY float64 = 1020, 170
	LPHeaterX, LPHeaterY float64 = 940, 170
	HDTX, HDTY           float64 = 850, 200
	HPPumpX, HPPumpY     float64 = 770, 290
	HPHeaterX, HPHeaterY float64 = 700, 360
	FWCtrlX, FWCtrlY     float64 = 580, 420

	// Safety systems — inside the containment dome.
	SISX, SISY           float64 = 140, 180
	EFWSTankX, EFWSTankY float64 = 120, 370

	// AFW pumps — below the containment base, near the SG inlets.
	DieselAFWX, DieselAFWY   float64 = 530, 690
	MotorAFWX, MotorAFWY     float64 = 590, 690
	TurbineAFWX, TurbineAFWY float64 = 650, 690

	// CCWS heat exchanger — bottom-left, just inside the base.
	CCWSHXX, CCWSHXY float64 = 200, 700

	// RHRS trains — bottom-centre, just inside the base.
	RHRSTrainAX, RHRSTrainAY float64 = 360, 700
	RHRSTrainBX, RHRSTrainBY float64 = 480, 700

	// Spent fuel pool — below the containment, right side.
	SFPX, SFPY float64 = 620, 770

	// Electrical — bottom-left.
	DieselX, DieselY                     float64 = 100, 820
	FuelTankX, FuelTankY                 float64 = 100, 870
	StationTransformerX, StationTransformerY float64 = 250, 820
	ESFBusAX, ESFBusAY                   float64 = 400, 860
	ESFBusBX, ESFBusBY                   float64 = 450, 860
	GridX, GridY                         float64 = 1100, 80

	// Building footprints — simple rectangles behind the symbols.
	// Spec references:
	//   - "auxiliary building around the containment"
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:76-78
	//   - "turbine building houses the turbine-generator"
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_027.txt:18
	//   - "several pump houses"
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:77-78
	//   - "spent fuel into the fuel handling building"
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_173.txt:28-29
	AuxBuildingX, AuxBuildingY float64 = 20, 120
	AuxBuildingW             float64 = 660
	AuxBuildingH             float64 = 770

	TurbineBuildingX, TurbineBuildingY float64 = 580, 40
	TurbineBuildingW                   float64 = 620
	TurbineBuildingH                   float64 = 810

	PumpHouseX, PumpHouseY float64 = 480, 660
	PumpHouseW             float64 = 240
	PumpHouseH             float64 = 70

	FuelHandlingX, FuelHandlingY float64 = 520, 710
	FuelHandlingW               float64 = 200
	FuelHandlingH               float64 = 120
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

	// Each <use> below is wrapped in a <g data-part="..."> so the
	// web UI can bind SVG elements to controls / sensors by
	// part-name. The data-part value matches the bridge.Part
	// string for that plant subsystem.
	writePart := func(part, use string) {
		fmt.Fprintf(&b, `<g data-part="%s">%s</g>`+"\n", part, use)
	}
	useAt := func(sym string, x, y, w, h float64) string {
		return fmt.Sprintf(
			`<use href="#%s" x="%.0f" y="%.0f" `+
				`width="%.0f" height="%.0f"/>`,
			sym, x, y, w, h)
	}
	writeLabel := func(x, y float64, text string) {
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">%s</text>`+"\n", x, y, text)
	}
	writeBuilding := func(x, y, w, h float64, label string) {
		fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" `+
			`fill="#f5f5f5" stroke="#999" stroke-width="1.5"/>`+"\n",
			x, y, w, h)
		if label != "" {
			fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-size="8" `+
				`fill="#666" text-anchor="middle">%s</text>`+"\n",
				x+w/2, y+h/2+3, label)
		}
	}

	// Building footprints.
	// specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:75-78
	writeBuilding(AuxBuildingX, AuxBuildingY, AuxBuildingW, AuxBuildingH, "")
	writeBuilding(TurbineBuildingX, TurbineBuildingY, TurbineBuildingW, TurbineBuildingH, "")
	writeBuilding(PumpHouseX, PumpHouseY, PumpHouseW, PumpHouseH, "")
	writeBuilding(FuelHandlingX, FuelHandlingY, FuelHandlingW, FuelHandlingH, "")

	// Background: containment dome on the left, large enough to
	// enclose the nuclear island (RPV, pressurizer, SG, RCP).
	// specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:28-34
	writePart("ReactorBuilding/ContainmentBuilding",
		useAt("containment-dome",
			ContainmentX-ContainmentW/2, ContainmentY-ContainmentH/2,
			ContainmentW, ContainmentH))
	writeLabel(ContainmentX, ContainmentY-ContainmentH/2-10, "Containment")

	// CSS spray headers overlay inside the dome.
	writePart("ReactorBuilding/ContainmentBuilding",
		useAt("css-spray-headers", ContainmentX-120, ContainmentY-110, 240, 120))

	// Four RCFC units in a 2x2 grid inside the dome.
	for dy := 0; dy < 2; dy++ {
		for dx := 0; dx < 2; dx++ {
			writePart("ReactorBuilding/ContainmentBuilding",
				useAt("rcfc-unit",
					ContainmentX-90+float64(dx)*70,
					ContainmentY-60+float64(dy)*70,
					40, 40))
		}
	}

	// RPV at the centre of the reactor building.
	writePart("ReactorBuilding/ReactorPressureVessel",
		useAt("reactor_vessel", RPVX-50, RPVY-120, 100, 240))
	writeLabel(RPVX, RPVY+130, "RPV")

	// Pressurizer to the upper-right of the RPV, fully inside
	// the dome arch (top at domeTop+30, bottom aligns with RPV centre).
	writePart("ReactorBuilding/Pressurizer",
		useAt("pressurizer", PressurizerX-30,
			ContainmentY-ContainmentH/2+30, 60, 200))
	writeLabel(PressurizerX, ContainmentY-ContainmentH/2+250, "PZR")

	// Pressurizer relief tank to the right of the pressurizer,
	// inside the dome.
	writePart("ReactorBuilding/PressurizedReliefTank",
		useAt("tank-vertical", PRTX-30,
			ContainmentY-ContainmentH/2+30, 60, 80))
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">PRT</text>`+"\n",
		PRTX, ContainmentY-ContainmentH/2+30+50)

	// RCP between RPV and SG.
	writePart("ReactorBuilding/ReactorCoolantPump",
		useAt("pump-rcp", RCPX-25, RCPY-80, 50, 160))
	writeLabel(RCPX, RCPY-100, "RCP")

	// SG to the right of the RCP.
	writePart("ReactorBuilding/SteamGenerator",
		useAt("steam_generator", SGX-70, SGY-200, 140, 400))
	writeLabel(SGX, SGY+240, "SG (x4)")
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">(x4)</text>`+"\n",
		SGX, SGY+220)

	// Pressurizer safety + relief valves.
	for i := 0; i < 3; i++ {
		writePart("ReactorBuilding/Pressurizer",
			useAt("valve-safety",
				PressurizerX-22+float64(i)*14,
				ContainmentY-ContainmentH/2+20, 20, 20))
	}
	for i := 0; i < 2; i++ {
		writePart("ReactorBuilding/Pressurizer",
			useAt("valve-porv",
				PressurizerX-12+float64(i)*14,
				ContainmentY-ContainmentH/2+14, 24, 20))
	}

	// Turbine, generator, condenser.
	writePart("TurbineBuilding/Turbine",
		useAt("turbine-main", TurbineX-180, TurbineY-100, 360, 200))
	writeLabel(TurbineX, TurbineY-110, "Turbine")
	writePart("TurbineBuilding/Generator",
		useAt("generator-main", GeneratorX-110, GeneratorY-70, 220, 140))
	writeLabel(GeneratorX, GeneratorY-85, "Generator")
	writePart("TurbineBuilding/Condenser",
		useAt("condenser", CondenserX-200, CondenserY-50, 400, 100))
	writeLabel(CondenserX, CondenserY+70, "Condenser")

	// Feedwater chain: CST -> condensate pump -> LP heater ->
	// HDT (deaerator) -> feed pump -> HP heater -> SG.
	writePart("TurbineBuilding/CondensateStorageTank",
		useAt("tank-cst", CSTX-50, CSTY-65, 100, 130))
	writeLabel(CSTX, CSTY-75, "CST")
	writePart("TurbineBuilding/CondensatePump",
		useAt("pump-horizontal", CondPumpX-30, CondPumpY-20, 60, 40))
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("heat-exchanger-vertical", LPHeaterX-25, LPHeaterY-50, 50, 100))
	writeLabel(LPHeaterX, LPHeaterY-60, "LP Heater")
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("tank-horizontal", HDTX-50, HDTY-25, 100, 50))
	writeLabel(HDTX, HDTY-35, "HDT")
	writePart("TurbineBuilding/HeaterDrainPump",
		useAt("pump-horizontal", HPPumpX-30, HPPumpY-20, 60, 40))
	writeLabel(HPPumpX, HPPumpY-30, "Feed Pump")
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("heat-exchanger-vertical", HPHeaterX-25, HPHeaterY-50, 50, 100))
	writeLabel(HPHeaterX, HPHeaterY-60, "HP Heater")

	// Main feedwater control valve (modulating) on the
	// SG-inlet pipe.
	writePart("TurbineBuilding/MainFeedwaterControlValve",
		useAt("valve-modulating", FWCtrlX-12, FWCtrlY-10, 24, 20))
	writeLabel(FWCtrlX, FWCtrlY-15, "FW Ctrl")

	// AFW pumps near the SG inlet.
	writePart("Outside/TurbineAFWPump",
		useAt("pump-turbine", TurbineAFWX-30, TurbineAFWY-20, 60, 40))
	writeLabel(TurbineAFWX, TurbineAFWY-28, "AFW TD")
	writePart("Outside/MotorAFWPump",
		useAt("pump-horizontal", MotorAFWX-30, MotorAFWY-20, 60, 40))
	writeLabel(MotorAFWX, MotorAFWY-28, "AFW MD")
	writePart("Outside/DieselAFWPump",
		useAt("pump-horizontal", DieselAFWX-30, DieselAFWY-20, 60, 40))
	writeLabel(DieselAFWX, DieselAFWY-28, "AFW Diesel")

	// SIS accumulators (one cluster of 4).
	writePart("ReactorBuilding/SafetyInjectionSystem",
		useAt("sis-accumulators", SISX-100, SISY-40, 200, 80))
	writeLabel(SISX, SISY-50, "SIS Acc")

	// EFWS tank.
	writePart("AuxiliaryBuilding/EmergencyFeedwaterSystem",
		useAt("efws-tank", EFWSTankX-40, EFWSTankY-50, 80, 100))
	writeLabel(EFWSTankX, EFWSTankY-60, "EFWS")

	// CCWS heat exchanger.
	writePart("AuxiliaryBuilding/ComponentCoolingWaterSystem",
		useAt("ccws-hx", CCWSHXX-50, CCWSHXY-25, 100, 50))
	writeLabel(CCWSHXX, CCWSHXY-35, "CCWS HX")

	// RHRS trains.
	writePart("AuxiliaryBuilding/ResidualHeatRemovalSystem",
		useAt("rhrs-train", RHRSTrainAX-100, RHRSTrainAY-40, 200, 80))
	writeLabel(RHRSTrainAX, RHRSTrainAY-50, "RHRS A")
	writePart("AuxiliaryBuilding/ResidualHeatRemovalSystem",
		useAt("rhrs-train", RHRSTrainBX-100, RHRSTrainBY-40, 200, 80))
	writeLabel(RHRSTrainBX, RHRSTrainBY-50, "RHRS B")

	// SFP.
	writePart("ReactorBuilding/SpentFuelPool",
		useAt("tank-sfp", SFPX-100, SFPY-50, 200, 100))
	writeLabel(SFPX, SFPY+60, "SFP")

	// Electrical: diesel + fuel tank, station transformer,
	// ESF buses, grid.
	writePart("AuxiliaryBuilding/DieselGenerator",
		useAt("diesel-generator", DieselX-100, DieselY-50, 200, 100))
	writeLabel(DieselX, DieselY-60, "Diesel")
	writePart("AuxiliaryBuilding/DieselGenerator",
		useAt("fuel-tank", FuelTankX-15, FuelTankY-20, 30, 40))
	writePart("Outside/StationTransformer",
		useAt("transformer-2w", StationTransformerX-25, StationTransformerY-25, 50, 50))
	writeLabel(StationTransformerX, StationTransformerY-35, "Xformer")
	writePart("Outside/ESFBus",
		useAt("esf-bus", ESFBusAX-60, ESFBusAY-10, 120, 20))
	writeLabel(ESFBusAX, ESFBusAY-15, "ESF Bus A")
	writePart("Outside/ESFBus",
		useAt("esf-bus", ESFBusBX-60, ESFBusBY-10, 120, 20))
	writeLabel(ESFBusBX, ESFBusBY-15, "ESF Bus B")
	writePart("Outside/Grid",
		useAt("grid-bus", GridX-15, GridY-25, 30, 50))
	writeLabel(GridX, GridY-35, "Grid")

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
	if p.AuxiliaryBuilding.DieselGenerator.State == core.DieselRunning {
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
