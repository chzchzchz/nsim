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

// anchor describes the document-space coordinate of a part's
// primary anchor (used for pipe routing). Defined in layout.go.
type Anchor struct {
	X, Y float64
}

// DebugLayout writes an SVG overlay showing the graphviz-computed
// bounding box and anchor point for every plant-graph node.
// Use this with -svg-dot + dot -Tplain-ext to compare against
// the hand-composed SVG placement.
func DebugLayout(w io.Writer, g *core.PlantGraph, pos map[string]Anchor) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0fpx" height="%.0fpx" viewBox="0 0 %.0f %.0f">`+"\n",
		DocumentWidth, DocumentHeight, DocumentWidth, DocumentHeight)
	b.WriteString(`<g id="debug-layout">` + "\n")
	b.WriteString(`<g id="clusters">` + "\n")
	clusters := []struct {
		name string
		x, y, w, h float64
	}{
		{"Containment", ContainmentX - ContainmentW/2, ContainmentY - ContainmentH/2, ContainmentW, ContainmentH},
		{"Auxiliary", AuxBuildingX, AuxBuildingY, AuxBuildingW, AuxBuildingH},
		{"Turbine", TurbineBuildingX, TurbineBuildingY, TurbineBuildingW, TurbineBuildingH},
	}
	for _, c := range clusters {
		fmt.Fprintf(&b, `  <rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="blue" stroke-width="2" stroke-dasharray="4 2"/>`+"\n",
			c.x, c.y, c.w, c.h)
		fmt.Fprintf(&b, `  <text x="%.1f" y="%.1f" font-size="10" fill="blue">%s</text>`+"\n", c.x+4, c.y+12, c.name)
	}
	b.WriteString("</g>\n")
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a, ok := pos[id]
		if !ok {
			continue
		}
		w, h := nodeSize(g.Nodes[id].Kind)
		x := a.X - w/2
		y := a.Y - h/2
		fmt.Fprintf(&b, `  <rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="rgba(255,0,0,0.15)" stroke="red" stroke-width="1"/>`+"\n", x, y, w*96, h*96)
		fmt.Fprintf(&b, `  <circle cx="%.1f" cy="%.1f" r="3" fill="red"/>`+"\n", a.X, a.Y)
		fmt.Fprintf(&b, `  <text x="%.1f" y="%.1f" font-size="8" fill="red">%s</text>`+"\n", a.X, a.Y-4, id)
	}
	b.WriteString("</g>\n</svg>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

var loadedLayout *Layout = EmptyLayout()

// SetLayout replaces the current layout used by getAnchor.
// If the build includes layout_default_generated.go, the
// default is pre-loaded from embedded graphviz positions so
// no runtime dot invocation is needed. Call SetLayout to override.
func SetLayout(l *Layout) {
	loadedLayout = l
}

// anchorFor returns the layout coordinate of the named graph
// node. Unknown nodes fall back to (0, 0). Values are clamped
// to the document viewport (+10 px margin) so no pipe line
// starts or ends off-screen.
func anchorFor(id string) Anchor {
	const margin = 10.0
	x, y := getAnchor(id, 0, 0)
	if x < margin {
		x = margin
	}
	if y < margin {
		y = margin
	}
	if x > DocumentWidth-margin {
		x = DocumentWidth - margin
	}
	if y > DocumentHeight-margin {
		y = DocumentHeight - margin
	}
	return Anchor{X: x, Y: y}
}

// getAnchor returns the layout coordinate for the given graph node
// ID, falling back to the provided default when the layout does not
// contain that ID.
func getAnchor(id string, defX, defY float64) (float64, float64) {
	if a, ok := loadedLayout.Pos[id]; ok {
		return a.X, a.Y
	}
	return defX, defY
}

// WritePlantSVG writes a hand-composed P&ID-style SVG of the
// given plant to w. The diagram is built from the asset library
// in this package.
func WritePlantSVG(w io.Writer, p *core.Plant) error {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%.0fpx" height="%.0fpx" viewBox="0 0 %.0f %.0f">`+"\n",
		DocumentWidth, DocumentHeight, DocumentWidth, DocumentHeight)
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
	useAt := func(sym string, cx, cy, w, h float64) string {
		return fmt.Sprintf(
			`<use href="#%s" x="%.0f" y="%.0f" `+
				`width="%.0f" height="%.0f"/>`,
			sym, cx-w/2, cy-h/2, w, h)
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
	rpvX, rpvY := getAnchor("RPV", coX+RPVX, coY+RPVY)
	writePart("ReactorBuilding/ReactorPressureVessel",
		useAt("reactor_vessel", rpvX, rpvY, 100, 240))
	writeLabel(rpvX+50, rpvY+130, "RPV")

	// Pressurizer to the upper-right of the RPV, fully inside
	// the dome arch (top at domeTop+30, bottom aligns with RPV centre).
	pzrX, pzrY := getAnchor("Pressurizer", coX+PressurizerX, coY+30)
	writePart("ReactorBuilding/Pressurizer",
		useAt("pressurizer", pzrX, pzrY, 60, 200))
	writeLabel(pzrX, pzrY+220, "PZR")

	// Pressurizer relief tank to the right of the pressurizer,
	// inside the dome.
	prtX, prtY := getAnchor("PressurizedReliefTank", coX+PRTX, coY+30)
	writePart("ReactorBuilding/PressurizedReliefTank",
		useAt("tank-vertical", prtX, prtY, 60, 80))
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">PRT</text>`+"\n",
		prtX+30, prtY+50)

	// RCP between RPV and SG.
	rcpX, rcpY := getAnchor("ReactorCoolantPump", coX+RCPX, coY+RCPY-80)
	writePart("ReactorBuilding/ReactorCoolantPump",
		useAt("pump-rcp", rcpX, rcpY, 50, 160))
	writeLabel(rcpX+25, rcpY-20, "RCP")

	// SG to the right of the RCP.
	sgX, sgY := getAnchor("SG", coX+SGX, coY+SGY-215)
	writePart("ReactorBuilding/SteamGenerator",
		useAt("steam_generator", sgX, sgY, 190, 430))
	writeLabel(sgX+95, sgY+240, "SG (x4)")
	fmt.Fprintf(&b,
		`<text x="%.0f" y="%.0f" font-size="10" `+
			`text-anchor="middle">(x4)</text>`+"\n",
		sgX+95, sgY+220)

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
	turbX, turbY := getAnchor("Turbine", TurbineX, TurbineY)
	writePart("TurbineBuilding/Turbine",
		useAt("turbine-main", turbX, turbY, 360, 200))
	writeLabel(turbX, turbY-110, "Turbine")

	genX, genY := getAnchor("Generator", GeneratorX, GeneratorY)
	writePart("TurbineBuilding/Generator",
		useAt("generator-main", genX, genY, 220, 140))
	writeLabel(genX, genY-85, "Generator")

	condX, condY := getAnchor("Condenser", CondenserX, CondenserY)
	writePart("TurbineBuilding/Condenser",
		useAt("condenser", condX, condY, 400, 100))
	writeLabel(condX, condY+70, "Condenser")

	// Feedwater chain: CST -> condensate pump -> LP heater ->
	// HDT (deaerator) -> feed pump -> HP heater -> SG.
	cstX, cstY := getAnchor("CST", turbX+CSTX, turbY+CSTY)
	writePart("TurbineBuilding/CondensateStorageTank",
		useAt("tank-cst", cstX, cstY, 100, 130))
	writeLabel(cstX, cstY-75, "CST")

	cpuX, cpuY := getAnchor("CondensatePump", turbX+CondPumpX, turbY+CondPumpY)
	writePart("TurbineBuilding/CondensatePump",
		useAt("pump-horizontal", cpuX, cpuY, 60, 40))

	lphX, lphY := getAnchor("LPHeaterDrain", turbX+LPHeaterX, turbY+LPHeaterY)
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("heat-exchanger-vertical", lphX, lphY, 50, 100))
	writeLabel(lphX, lphY-60, "LP Heater")

	hdtX, hdtY := getAnchor("HDT", turbX+HDTX, turbY+HDTY)
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("tank-horizontal", hdtX, hdtY, 100, 50))
	writeLabel(hdtX, hdtY-35, "HDT")

	hppX, hppY := getAnchor("HeaterDrainPump", turbX+HPPumpX, turbY+HPPumpY)
	writePart("TurbineBuilding/HeaterDrainPump",
		useAt("pump-horizontal", hppX, hppY, 60, 40))
	writeLabel(hppX, hppY-30, "Feed Pump")

	hphX, hphY := getAnchor("HPHeaterDrain", turbX+HPHeaterX, turbY+HPHeaterY)
	writePart("TurbineBuilding/HeaterDrainTank",
		useAt("heat-exchanger-vertical", hphX, hphY, 50, 100))
	writeLabel(hphX, hphY-60, "HP Heater")

	// Main feedwater control valve (modulating) on the
	// SG-inlet pipe (offsets from turbine center).
	fwcX, fwcY := getAnchor("MainFeedValve", turbX+FWCtrlX, turbY+FWCtrlY)
	writePart("TurbineBuilding/MainFeedwaterControlValve",
		useAt("valve-modulating", fwcX, fwcY, 24, 20))
	writeLabel(fwcX, fwcY-15, "FW Ctrl")

	// AFW pumps — near the SG inlets (offsets from containment origin).
	daX, daY := getAnchor("DieselAFWPump", coX+DieselAFWX, coY+DieselAFWY)
	writePart("Outside/TurbineAFWPump",
		useAt("pump-turbine", daX, daY, 60, 40))
	writeLabel(daX, daY-28, "AFW TD")

	maX, maY := getAnchor("MotorAFWPump", coX+MotorAFWX, coY+MotorAFWY)
	writePart("Outside/MotorAFWPump",
		useAt("pump-horizontal", maX, maY, 60, 40))
	writeLabel(maX, maY-28, "AFW MD")

	taX, taY := getAnchor("TurbineAFWPump", coX+TurbineAFWX, coY+TurbineAFWY)
	writePart("Outside/DieselAFWPump",
		useAt("pump-horizontal", taX, taY, 60, 40))
	writeLabel(taX, taY-28, "AFW Diesel")

	// SIS accumulators (one cluster of 4).
	sisX, sisY := getAnchor("SISAccumulators", coX+SISX, coY+SISY)
	writePart("ReactorBuilding/SafetyInjectionSystem",
		useAt("sis-accumulators", sisX, sisY-50, 240, 100))
	writeLabel(sisX+120, sisY-60, "SIS Acc")

	// EFWS tank.
	efX, efY := getAnchor("EFWSTank", AuxBuildingX+EFWSTankX, AuxBuildingY+EFWSTankY)
	writePart("AuxiliaryBuilding/EmergencyFeedwaterSystem",
		useAt("efws-tank", efX, efY-50, 80, 100))
	writeLabel(efX+40, efY-60, "EFWS")

	// CCWS heat exchanger.
	ccX, ccY := getAnchor("CCWSHX", AuxBuildingX+CCWSHXX, AuxBuildingY+CCWSHXY)
	writePart("AuxiliaryBuilding/ComponentCoolingWaterSystem",
		useAt("ccws-hx", ccX, ccY-25, 100, 50))
	writeLabel(ccX+50, ccY-35, "CCWS HX")

	// RHRS trains.
	raX, raY := getAnchor("RHRS_A", AuxBuildingX+RHRSTrainAX, AuxBuildingY+RHRSTrainAY)
	writePart("AuxiliaryBuilding/ResidualHeatRemovalSystem",
		useAt("rhrs-train", raX, raY-40, 200, 80))
	writeLabel(raX+100, raY-50, "RHRS A")

	rbX, rbY := getAnchor("RHRS_B", AuxBuildingX+RHRSTrainBX, AuxBuildingY+RHRSTrainBY)
	writePart("AuxiliaryBuilding/ResidualHeatRemovalSystem",
		useAt("rhrs-train", rbX, rbY-40, 200, 80))
	writeLabel(rbX+100, rbY-50, "RHRS B")

	// SFP — offset from containment origin.
	sfpX, sfpY := getAnchor("SFP", coX+SFPX, coY+SFPY)
	writePart("ReactorBuilding/SpentFuelPool",
		useAt("tank-sfp", sfpX, sfpY, 200, 100))
	writeLabel(sfpX, sfpY+60, "SFP")

	// Electrical: diesel + fuel tank, station transformer,
	// ESF buses, grid (offsets from auxiliary building origin).
	dieX, dieY := getAnchor("Diesel", AuxBuildingX+DieselX, AuxBuildingY+DieselY)
	writePart("AuxiliaryBuilding/DieselGenerator",
		useAt("diesel-generator", dieX, dieY, 200, 100))
	writeLabel(dieX+100, dieY-60, "Diesel")

	ftX, ftY := getAnchor("FuelTank", AuxBuildingX+FuelTankX, AuxBuildingY+FuelTankY)
	writePart("AuxiliaryBuilding/DieselGenerator",
		useAt("fuel-tank", ftX, ftY, 30, 40))

	stX, stY := getAnchor("Transformer", AuxBuildingX+StationTransformerX, AuxBuildingY+StationTransformerY)
	writePart("AuxiliaryBuilding/Transformer",
		useAt("transformer-2w", stX, stY, 50, 50))
	writeLabel(stX+25, stY-35, "Xformer")

	esfAX, esfAY := getAnchor("ESF_A", AuxBuildingX+ESFBusAX, AuxBuildingY+ESFBusAY)
	writePart("Outside/ESFBus",
		useAt("esf-bus", esfAX, esfAY, 120, 20))
	writeLabel(esfAX+60, esfAY-15, "ESF Bus A")

	esfBX, esfBY := getAnchor("ESF_B", AuxBuildingX+ESFBusBX, AuxBuildingY+ESFBusBY)
	writePart("Outside/ESFBus",
		useAt("esf-bus", esfBX, esfBY, 120, 20))
	writeLabel(esfBX+60, esfBY-15, "ESF Bus B")

	gridX, gridY := getAnchor("Grid", GridX, GridY)
	writePart("Outside/Grid",
		useAt("grid-bus", gridX, gridY, 30, 50))
	writeLabel(gridX, gridY-35, "Grid")

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
