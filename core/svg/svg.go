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
	ContainmentX, ContainmentY float64 = 260, 380
	ContainmentW                         float64 = 480 // cylinder width
	ContainmentH                         float64 = 440 // cylinder + arch height

	// Reactor pressure vessel — centred inside the containment.
	// Relative to containment origin (ContainmentX - ContainmentW/2, ContainmentY - ContainmentH/2).
	RPVX, RPVY float64 = 190, 110

	// Pressurizer — upper-right quadrant inside the containment,
	// connected to the RPV hot leg via a short surge line.
	PressurizerX, PressurizerY float64 = 300, 10

	// Pressurizer relief tank — above the pressurizer, inside dome.
	PRTX, PRTY                 float64 = 340, -120

	// Steam generator — right of the RPV, inside containment.
	// Only one shown; "(×4)" label added by live overlay.
	SGX, SGY float64 = 380, 130

	// RCP — between the RPV and SG on the hot leg.
	RCPX, RCPY float64 = 275, 150

	// Turbine island — right of containment with separation gap
	// for main steam stop valves ("just outside the containment").
	// Turbine island — right of containment with separation gap.
	// These constants hold the turbine center (doc-space), used
	// as the reference for all turbine/feedwater components.
	TurbineX, TurbineY     float64 = 900, 430
	GeneratorX, GeneratorY float64 = 1070, 370
	CondenserX, CondenserY float64 = 1000, 610

	// Feedwater chain — offsets from the turbine center (TurbineX, TurbineY).
	CSTX, CSTY           float64 = 170, -200
	CondPumpX, CondPumpY float64 = 100, -200
	LPHeaterX, LPHeaterY float64 = 20, -200
	HDTX, HDTY           float64 = -70, -170
	HPPumpX, HPPumpY     float64 = -150, -100
	HPHeaterX, HPHeaterY float64 = -220, -30
	FWCtrlX, FWCtrlY     float64 = -340, 30

	// Safety systems — offsets relative to containment origin.
	SISX, SISY           float64 = 0, 40
	EFWSTankX, EFWSTankY float64 = 80, 90

	// AFW pumps — offsets relative to containment origin.
	DieselAFWX, DieselAFWY   float64 = 510, 210
	MotorAFWX, MotorAFWY     float64 = 570, 210
	TurbineAFWX, TurbineAFWY float64 = 630, 210

	// CCWS heat exchanger — offset relative to containment origin.
	CCWSHXX, CCWSHXY float64 = 130, 200

	// RHRS trains — offsets relative to containment origin.
	RHRSTrainAX, RHRSTrainAY float64 = 240, 220
	RHRSTrainBX, RHRSTrainBY float64 = 460, 310

	// Spent fuel pool — offset relative to containment origin.
	SFPX, SFPY float64 = 600, 330

	// Electrical — offsets relative to auxiliary building origin.
	DieselX, DieselY                     float64 = 0, 100
	FuelTankX, FuelTankY                 float64 = 65, 150
	StationTransformerX, StationTransformerY float64 = 205, 100
	ESFBusAX, ESFBusAY                   float64 = 320, 180
	ESFBusBX, ESFBusBY                   float64 = 370, 180
	GridX, GridY                         float64 = 1100, 80

	// Building footprints — simple rectangles behind the symbols.
	// Spec references:
	//   - "auxiliary building around the containment"
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:76-78
	//   - "turbine building houses the turbine-generator"
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_027.txt:18
	AuxBuildingX, AuxBuildingY float64 = 20, 600
	AuxBuildingW             float64 = 480
	AuxBuildingH             float64 = 770

	TurbineBuildingX, TurbineBuildingY float64 = 560, 40
	TurbineBuildingW                   float64 = 640
	TurbineBuildingH                   float64 = 810
)

// containmentTL returns the containment dome top-left in doc-space.
func containmentTL() (float64, float64) {
	return ContainmentX - ContainmentW/2, ContainmentY - ContainmentH/2
}

// auxTL returns the auxiliary building top-left in doc-space.
func auxTL() (float64, float64) {
	return AuxBuildingX, AuxBuildingY
}

// turbineCenter returns the turbine center in doc-space.
func turbineCenter() (float64, float64) {
	return TurbineX, TurbineY
}

// docPos converts a parent-relative offset to a document-space point.
// px, py is the parent origin; rx, ry is the child's offset from that origin.
func docPos(px, py, rx, ry float64) (x, y float64) {
	return px + rx, py + ry
}

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
//
// Containment children use offsets relative to containment top-left
// (ContainmentX - ContainmentW/2, ContainmentY - ContainmentH/2).
// Turbine/feedwater children use doc-space coordinates referenced
// from TurbineX/Y (turbine center).
var anchors = map[string]anchor{
	// Containment children — document-space origins (useAt positions).
	"RPV":           {ContainmentX - ContainmentW/2 + RPVX, ContainmentY - ContainmentH/2 + RPVY},
	"Pressurizer":   {ContainmentX - ContainmentW/2 + PressurizerX, ContainmentY - ContainmentH/2 + PressurizerY},
	"ReactorCoolantPump": {ContainmentX - ContainmentW/2 + RCPX, ContainmentY - ContainmentH/2 + RCPY},
	"SG":            {ContainmentX - ContainmentW/2 + SGX, ContainmentY - ContainmentH/2 + SGY},

	// Secondary — turbine center.
	"Turbine":       {TurbineX, TurbineY},
	"Condenser":     {CondenserX, CondenserY},
	"CST":           {CSTX, CSTY},
	"HDT":           {HDTX, HDTY},
	"LPHeaterDrain": {LPHeaterX, LPHeaterY},
	"HPHeaterDrain": {HPHeaterX, HPHeaterY},

	// Pressurizer relief — document-space position.
	// useAt: coX+PRTX-30, coY+30 → center at coX+PRTX-30+30 = coX+PRTX
	"PressurizedReliefTank": {ContainmentX - ContainmentW/2 + PRTX - 30 + 30, ContainmentY - ContainmentH/2 + 30},

	// AFW pumps — document-space positions (offsets from containment origin).
	"DieselAFWPump":  {ContainmentX - ContainmentW/2 + DieselAFWX, ContainmentY - ContainmentH/2 + DieselAFWY},
	"MotorAFWPump":   {ContainmentX - ContainmentW/2 + MotorAFWX, ContainmentY - ContainmentH/2 + MotorAFWY},
	"TurbineAFWPump": {ContainmentX - ContainmentW/2 + TurbineAFWX, ContainmentY - ContainmentH/2 + TurbineAFWY},
}

// anchorFor returns the layout coordinate of the named graph
// node. Unknown nodes fall back to (0, 0). Values are clamped
// to the document viewport (+10 px margin) so no pipe line
// starts or ends off-screen.
func anchorFor(id string) anchor {
	const margin = 10.0
	a := anchor{0, 0}
	if v, ok := anchors[id]; ok {
		a = v
	}
	if a.X < margin {
		a.X = margin
	}
	if a.Y < margin {
		a.Y = margin
	}
	if a.X > DocumentWidth-margin {
		a.X = DocumentWidth - margin
	}
	if a.Y > DocumentHeight-margin {
		a.Y = DocumentHeight - margin
	}
	return a
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
	drawAuxiliaryBuilding := func(b *strings.Builder) {
		fmt.Fprintf(b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" `+
			`fill="#f5f5f5" stroke="#999" stroke-width="1.5"/>`+"\n",
			AuxBuildingX, AuxBuildingY, AuxBuildingW, AuxBuildingH)
		writeLabel(AuxBuildingX+AuxBuildingW/2, AuxBuildingY+15, "Auxiliary\nBuilding")
	}
	drawTurbineBuilding := func(b *strings.Builder) {
		fmt.Fprintf(b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" `+
			`fill="#f5f5f5" stroke="#999" stroke-width="1.5"/>`+"\n",
			TurbineBuildingX, TurbineBuildingY, TurbineBuildingW, TurbineBuildingH)
		writeLabel(TurbineBuildingX+TurbineBuildingW/2, TurbineBuildingY+15, "Turbine\nBuilding")
	}

	// Building footprints.
	// specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:75-78
	drawAuxiliaryBuilding(&b)
	drawTurbineBuilding(&b)

	// Containment origin (top-left in doc space).
	coX, coY := containmentTL()


	// Background: containment dome.
	writePart("ReactorBuilding/ContainmentBuilding",
		useAt("containment-dome", coX, coY, ContainmentW, ContainmentH))
	writeLabel(ContainmentX, coY-10, "Containment")

	// CSS spray headers overlay inside the dome.
	writePart("ReactorBuilding/ContainmentBuilding",
		useAt("css-spray-headers", coX+ContainmentX-120, coY+ContainmentY-110, 240, 120))

	// Four RCFC units in a 2x2 grid inside the dome.
	for dy := 0; dy < 2; dy++ {
		for dx := 0; dx < 2; dx++ {
			writePart("ReactorBuilding/ContainmentBuilding",
				useAt("rcfc-unit",
					coX+ContainmentX-90+float64(dx)*70,
					coY+ContainmentY-60+float64(dy)*70,
					40, 40))
		}
	}

	// RPV at the centre of the reactor building.
	writePart("ReactorBuilding/ReactorPressureVessel",
		useAt("reactor_vessel", coX+RPVX, coY+RPVY, 100, 240))
	writeLabel(coX+RPVX+50, coY+RPVY+130, "RPV")

	// Pressurizer to the upper-right of the RPV, fully inside
	// the dome arch (top at domeTop+30, bottom aligns with RPV centre).
	writePart("ReactorBuilding/Pressurizer",
		useAt("pressurizer", coX+PressurizerX, coY+30, 60, 200))
	writeLabel(coX+PressurizerX+30, coY+250, "PZR")

	// Pressurizer relief tank to the right of the pressurizer,
	// inside the dome.
	writePart("ReactorBuilding/PressurizedReliefTank",
		useAt("tank-vertical", coX+PRTX, coY+30, 60, 80))
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">PRT</text>`+"\n",
		coX+PRTX+30, coY+30+50)

	// RCP between RPV and SG.
	writePart("ReactorBuilding/ReactorCoolantPump",
		useAt("pump-rcp", coX+RCPX, coY+RCPY-80, 50, 160))
	writeLabel(coX+RCPX+25, coY+RCPY-100, "RCP")

	// SG to the right of the RCP.
	writePart("ReactorBuilding/SteamGenerator",
		useAt("steam_generator", coX+SGX, coY+SGY-215, 190, 430))
	writeLabel(coX+SGX+95, coY+SGY+240, "SG (x4)")
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">(x4)</text>`+"\n",
		coX+SGX+95, coY+SGY+220)

	// Containment origin for valve placement (nozzle y).
	for i := 0; i < 3; i++ {
		writePart("ReactorBuilding/Pressurizer",
			useAt("valve-safety",
				coX+PressurizerX-22+float64(i)*14,
				coY+20, 20, 20))
	}
	for i := 0; i < 2; i++ {
		writePart("ReactorBuilding/Pressurizer",
			useAt("valve-porv",
				coX+PressurizerX-12+float64(i)*14,
				coY+14, 24, 20))
	}

	// Turbine + feedwater chain.
	toX := TurbineX
	toY := TurbineY
	writePart("TurbineBuilding/Turbine",
		useAt("turbine-main", toX-180, toY-100, 360, 200))
	writeLabel(toX, toY-110, "Turbine")
	writePart("TurbineBuilding/Generator",
		useAt("generator-main", GeneratorX-110, GeneratorY-70, 220, 140))
	writeLabel(GeneratorX, GeneratorY-85, "Generator")
	writePart("TurbineBuilding/Condenser",
		useAt("condenser", CondenserX-200, CondenserY-50, 400, 100))
	writeLabel(CondenserX, CondenserY+70, "Condenser")

	// Feedwater chain: CST -> condensate pump -> LP heater ->
	// HDT (deaerator) -> feed pump -> HP heater -> SG.
	writePart("TurbineBuilding/CondensateStorageTank",
		useAt("tank-cst", toX+CSTX-50, toY+CSTY-65, 100, 130))
	writeLabel(toX+CSTX, toY+CSTY-75, "CST")
	writePart("TurbineBuilding/CondensatePump",
		useAt("pump-horizontal", toX+CondPumpX-30, toY+CondPumpY-20, 60, 40))
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("heat-exchanger-vertical", toX+LPHeaterX-25, toY+LPHeaterY-50, 50, 100))
	writeLabel(toX+LPHeaterX, toY+LPHeaterY-60, "LP Heater")
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("tank-horizontal", toX+HDTX-50, toY+HDTY-25, 100, 50))
	writeLabel(toX+HDTX, toY+HDTY-35, "HDT")
	writePart("TurbineBuilding/HeaterDrainPump",
		useAt("pump-horizontal", toX+HPPumpX-30, toY+HPPumpY-20, 60, 40))
	writeLabel(toX+HPPumpX, toY+HPPumpY-30, "Feed Pump")
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("heat-exchanger-vertical", toX+HPHeaterX-25, toY+HPHeaterY-50, 50, 100))
	writeLabel(toX+HPHeaterX, toY+HPHeaterY-60, "HP Heater")

	// Main feedwater control valve (modulating) on the
	// SG-inlet pipe (offsets from turbine center).
	writePart("TurbineBuilding/MainFeedwaterControlValve",
		useAt("valve-modulating", toX+FWCtrlX-12, toY+FWCtrlY-10, 24, 20))
	writeLabel(toX+FWCtrlX, toY+FWCtrlY-15, "FW Ctrl")

	// AFW pumps — near the SG inlets (offsets from containment origin).
	writePart("Outside/TurbineAFWPump",
		useAt("pump-turbine", coX+DieselAFWX-30, coY+DieselAFWY-20, 60, 40))
	writeLabel(coX+DieselAFWX, coY+DieselAFWY-28, "AFW TD")
	writePart("Outside/MotorAFWPump",
		useAt("pump-horizontal", coX+MotorAFWX-30, coY+MotorAFWY-20, 60, 40))
	writeLabel(coX+MotorAFWX, coY+MotorAFWY-28, "AFW MD")
	writePart("Outside/DieselAFWPump",
		useAt("pump-horizontal", coX+TurbineAFWX-30, coY+TurbineAFWY-20, 60, 40))
	writeLabel(coX+TurbineAFWX, coY+TurbineAFWY-28, "AFW Diesel")

	// SIS accumulators (one cluster of 4).
	writePart("ReactorBuilding/SafetyInjectionSystem",
		useAt("sis-accumulators", coX+SISX, coY+SISY-50, 240, 100))
	writeLabel(coX+SISX+120, coY+SISY-60, "SIS Acc")

	// EFWS tank.
	writePart("AuxiliaryBuilding/EmergencyFeedwaterSystem",
		useAt("efws-tank", AuxBuildingX+EFWSTankX, AuxBuildingY+EFWSTankY-50, 80, 100))
	writeLabel(AuxBuildingX+EFWSTankX+40, AuxBuildingY+EFWSTankY-60, "EFWS")

	// CCWS heat exchanger.
	writePart("AuxiliaryBuilding/ComponentCoolingWaterSystem",
		useAt("ccws-hx", AuxBuildingX+CCWSHXX, AuxBuildingY+CCWSHXY-25, 100, 50))
	writeLabel(AuxBuildingX+CCWSHXX+50, AuxBuildingY+CCWSHXY-35, "CCWS HX")

	// RHRS trains.
	writePart("AuxiliaryBuilding/ResidualHeatRemovalSystem",
		useAt("rhrs-train", AuxBuildingX+RHRSTrainAX, AuxBuildingY+RHRSTrainAY-40, 200, 80))
	writeLabel(AuxBuildingX+RHRSTrainAX+100, AuxBuildingY+RHRSTrainAY-50, "RHRS A")
	writePart("AuxiliaryBuilding/ResidualHeatRemovalSystem",
		useAt("rhrs-train", AuxBuildingX+RHRSTrainBX, AuxBuildingY+RHRSTrainBY-40, 200, 80))
	writeLabel(AuxBuildingX+RHRSTrainBX+100, AuxBuildingY+RHRSTrainBY-50, "RHRS B")

	// SFP — offset from containment origin.
	writePart("ReactorBuilding/SpentFuelPool",
		useAt("tank-sfp", coX+SFPX-100, coY+SFPY-50, 200, 100))
	writeLabel(coX+SFPX, coY+SFPY+60, "SFP")

	// Electrical: diesel + fuel tank, station transformer,
	// ESF buses, grid (offsets from auxiliary building origin).
	writePart("AuxiliaryBuilding/DieselGenerator",
		useAt("diesel-generator", AuxBuildingX+DieselX, AuxBuildingY+DieselY-50, 200, 100))
	writeLabel(AuxBuildingX+DieselX+100, AuxBuildingY+DieselY-60, "Diesel")
	writePart("AuxiliaryBuilding/DieselGenerator",
		useAt("fuel-tank", AuxBuildingX+FuelTankX, AuxBuildingY+FuelTankY-20, 30, 40))
	writePart("AuxiliaryBuilding/Transformer",
		useAt("transformer-2w", AuxBuildingX+StationTransformerX,
			AuxBuildingY+StationTransformerY-25, 50, 50))
	writeLabel(AuxBuildingX+StationTransformerX+25, AuxBuildingY+StationTransformerY-35, "Xformer")
	writePart("Outside/ESFBus",
		useAt("esf-bus", AuxBuildingX+ESFBusAX, AuxBuildingY+ESFBusAY-10, 120, 20))
	writeLabel(AuxBuildingX+ESFBusAX+60, AuxBuildingY+ESFBusAY-15, "ESF Bus A")
	writePart("Outside/ESFBus",
		useAt("esf-bus", AuxBuildingX+ESFBusBX, AuxBuildingY+ESFBusBY-10, 120, 20))
	writeLabel(AuxBuildingX+ESFBusBX+60, AuxBuildingY+ESFBusBY-15, "ESF Bus B")
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
	coX, coY := containmentTL()
	auxX, auxY := auxTL()

	// RCP RPM.
	if p.ReactorBuilding.ReactorCoolantPump.Running ||
		p.ReactorBuilding.ReactorCoolantPump.Velocity > 0 {
		fmt.Fprintf(b,
			`<text x="%.0f" y="%.0f" font-size="10" `+
				`text-anchor="middle">%d rpm</text>`+"\n",
			RCPX + coX + 25, RCPY + coY + 90 - 10,
			int(p.ReactorBuilding.ReactorCoolantPump.Velocity))
	}

	// RPV outlet temperature. The primary-side thermal model
	// integrates Outlet toward CoreReferenceOutletF (621°F)
	// at full power; the live readout lets the operator see
	// the result without leaving the diagram.
	//   "Reactor coolant outlet temperature: 621°F (327°C)"
	//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:61
	rpv := p.ReactorBuilding.ReactorPressureVessel
	fmt.Fprintf(b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">T %.0f°F</text>`+"\n",
		coX+RPVX+50, coY+RPVY+150,
		rpv.Outlet.Float())

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
			coX + SGX + 95, coY + SGY + 230, pct)
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

	// Alarm-driven rings (plans/x/alarms.md). Each ring
	// corresponds to a trip-severity alarm aggregated in
	// p.Alarms(); the data-driven loop replaces the
	// earlier hand-written conditionals so adding a new
	// alarm does not require touching this overlay.
	hasAlarm := func(id core.AlarmID) bool {
		for _, a := range p.Alarms() {
			if a.ID == id {
				return true
			}
		}
		return false
	}
	if hasAlarm(core.AlarmIDOPCTrip) {
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
			auxX + DieselX, auxY + DieselY)
	}

	// Reactor trip red ring driven by AlarmSet.
	if hasAlarm(core.AlarmIDReactorTrip) {
		fmt.Fprintf(b,
			`<circle cx="%.0f" cy="%.0f" r="60" `+
				`fill="none" stroke="red" stroke-width="3"/>`+"\n",
			coX + RPVX + 50, coY + RPVY + 130)
	}

	// Diesel trip red ring (covers overspeed, coolant
	// over-temperature, loss of fuel, loss of lube oil;
	// the simplified model collapses them into one
	// latch in core/diesel.go:62-71,121,147).
	if hasAlarm(core.AlarmIDDieselTripped) {
		fmt.Fprintf(b,
			`<circle cx="%.0f" cy="%.0f" r="55" `+
				`fill="none" stroke="red" stroke-width="3"/>`+"\n",
			DieselX, DieselY)
	}
}
