package svg

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	_ "embed"
	"strings"
)

//go:embed assets/valve.svg
var valveAsset string

//go:embed assets/pump.svg
var pumpAsset string

//go:embed assets/tank.svg
var tankAsset string

//go:embed assets/heatexchanger.svg
var heatexchangerAsset string

//go:embed assets/turbine.svg
var turbineAsset string

//go:embed assets/generator.svg
var generatorAsset string

//go:embed assets/transformer.svg
var transformerAsset string

//go:embed assets/diesel.svg
var dieselAsset string

//go:embed assets/containment.svg
var containmentAsset string

//go:embed assets/safety_systems.svg
var safetySystemsAsset string

//go:embed assets/cooling_tower.svg
var coolingTowerAsset string

//go:embed assets/circulating_water.svg
var circulatingWaterAsset string

// extractSymbols returns the inner <symbol>...</symbol> blocks
// of an embedded asset file, separated by newlines.
func extractSymbols(asset string) string {
	var b bytes.Buffer
	for {
		start := strings.Index(asset, "<symbol ")
		if start < 0 {
			break
		}
		end := strings.Index(asset[start:], "</symbol>")
		if end < 0 {
			break
		}
		end += start + len("</symbol>")
		b.WriteString(asset[start:end])
		b.WriteByte('\n')
		asset = asset[end:]
	}
	return b.String()
}

// symbolBlockForID extracts the full <symbol id="...">...</symbol>
// block for the given targetID from asset. Returns empty string if
// the ID is not found.
func symbolBlockForID(asset, targetID string) string {
	openTag := `<symbol id="` + targetID + `"`
	idx := strings.Index(asset, openTag)
	if idx < 0 {
		return ""
	}
	openEnd := strings.Index(asset[idx:], ">")
	if openEnd < 0 {
		return ""
	}
	contentStart := idx + openEnd + 1
	closeTag := `</symbol>`
	end := strings.Index(asset[contentStart:], closeTag)
	if end < 0 {
		return ""
	}
	return asset[idx : contentStart+end+len(closeTag)]
}

// useDeps maps an href target to the asset source that contains
// its <symbol> definition, used to resolve cross-symbol <use>
// references when writing standalone files.
var useDeps = map[string]string{
	"tank-accumulator":  tankAsset,
	"tank-vertical":     tankAsset,
	"heat-exchanger":    heatexchangerAsset,
	"pump-horizontal":   pumpAsset,
}

// resolveUseRefs scans content for <use href="#ID" …/> and, for any
// ID found in useDeps, prepends a <defs> block containing the
// referenced <symbol> so the standalone file is self-contained.
func resolveUseRefs(content string) string {
	type dep struct {
		id  string
		sym string
	}
	var deps []dep
	seen := map[string]bool{}
	start := 0
	for {
		idx := strings.Index(content[start:], `href="#`)
		if idx < 0 {
			break
		}
		idx += start
		valStart := idx + len(`href="#`)
		end := strings.Index(content[valStart:], `"`)
		if end < 0 {
			break
		}
		ref := content[valStart : valStart+end]
		src, ok := useDeps[ref]
		if ok && !seen[ref] {
			sym := symbolBlockForID(src, ref)
			if sym != "" {
				deps = append(deps, dep{ref, sym})
				seen[ref] = true
			}
		}
		start = valStart + end
	}
	if len(deps) == 0 {
		return content
	}
	var b strings.Builder
	b.WriteString("<defs>\n")
	for _, d := range deps {
		b.WriteString(d.sym)
		b.WriteByte('\n')
	}
	b.WriteString("</defs>\n")
	b.WriteString(content)
	return b.String()
}

// stripSymbolWrapper returns the inner content and viewBox attribute
// from a complete <symbol>...</symbol> block.
func stripSymbolWrapper(sym string) (inner, viewBox string) {
	if i := strings.Index(sym, `viewBox="`); i >= 0 {
		i += len(`viewBox="`)
		if j := strings.Index(sym[i:], `"`); j >= 0 {
			viewBox = sym[i : i+j]
		}
	}
	if i := strings.Index(sym, ">"); i >= 0 {
		sym = sym[i+1:]
	}
	if i := strings.LastIndex(sym, `</symbol>`); i >= 0 {
		sym = sym[:i]
	}
	return sym, viewBox
}

// Asset constants: <symbol> blocks ready for the parent <defs>.
// Declared as vars (not consts) because extractSymbols is a
// regular function.
var (
	ValveAsset         = extractSymbols(valveAsset)
	PumpAsset          = extractSymbols(pumpAsset)
	TankAsset          = extractSymbols(tankAsset)
	HeatExchangerAsset = extractSymbols(heatexchangerAsset)
	TurbineAsset       = extractSymbols(turbineAsset)
	GeneratorAsset     = extractSymbols(generatorAsset)
	TransformerAsset   = extractSymbols(transformerAsset)
	DieselAsset        = extractSymbols(dieselAsset)
	ContainmentAsset   = extractSymbols(containmentAsset)
	SafetySystemsAsset = extractSymbols(safetySystemsAsset)
	CoolingTowerAsset  = extractSymbols(coolingTowerAsset)
	CircWaterAsset     = extractSymbols(circulatingWaterAsset)
)

// SymbolFile describes a standalone SVG file to write.
type SymbolFile struct {
	Name    string
	Content string
	W       int
	H       int
	ViewBox string
}

// SymbolFiles returns the list of symbols exported by WriteAssetFiles.
// Each entry contains one symbol's inner content and its viewBox,
// ready to wrap in a standalone graphviz-compatible SVG document.
func SymbolFiles() []SymbolFile {
	add := func(name, sym string, w, h int) SymbolFile {
		inner, vb := stripSymbolWrapper(sym)
		inner = resolveUseRefs(inner)
		return SymbolFile{Name: name, Content: inner, W: w, H: h, ViewBox: vb}
	}
	return []SymbolFile{
		add("reactor_vessel.svg", ReactorVesselSymbol, 100, 240),
		add("pressurizer.svg", PressurizerSymbol, 60, 200),
		add("steam_generator.svg", SteamGeneratorSymbol, 190, 430),
		add("condenser.svg", CondenserSymbol, 400, 100),
		add("pump-rcp.svg", symbolBlockForID(pumpAsset, "pump-rcp"), 50, 160),
		add("pump-horizontal.svg", symbolBlockForID(pumpAsset, "pump-horizontal"), 60, 40),
		add("pump-turbine.svg", symbolBlockForID(pumpAsset, "pump-turbine"), 60, 40),
		add("valve-safety.svg", symbolBlockForID(valveAsset, "valve-safety"), 20, 20),
			add("valve-porv.svg", symbolBlockForID(valveAsset, "valve-porv"), 24, 20),
			add("valve-modulating.svg", symbolBlockForID(valveAsset, "valve-modulating"), 24, 20),
				add("valve-gate.svg", symbolBlockForID(valveAsset, "valve-gate"), 24, 20),
				add("valve-stop.svg", symbolBlockForID(valveAsset, "valve-stop"), 24, 20),
		add("tank-vertical.svg", symbolBlockForID(tankAsset, "tank-vertical"), 60, 80),
		add("tank-horizontal.svg", symbolBlockForID(tankAsset, "tank-horizontal"), 100, 50),
		add("tank-cst.svg", symbolBlockForID(tankAsset, "tank-cst"), 100, 130),
		add("tank-sfp.svg", symbolBlockForID(tankAsset, "tank-sfp"), 200, 100),
		add("heat-exchanger-horizontal.svg", symbolBlockForID(heatexchangerAsset, "heat-exchanger"), 100, 50),
		add("heat-exchanger-vertical.svg", symbolBlockForID(heatexchangerAsset, "heat-exchanger-vertical"), 50, 100),
		add("turbine-main.svg", symbolBlockForID(turbineAsset, "turbine-main"), 360, 200),
		add("generator-main.svg", symbolBlockForID(generatorAsset, "generator-main"), 220, 140),
		add("transformer-2w.svg", symbolBlockForID(transformerAsset, "transformer-2w"), 50, 50),
		add("diesel-generator.svg", symbolBlockForID(dieselAsset, "diesel-generator"), 200, 100),
		add("fuel-tank.svg", symbolBlockForID(dieselAsset, "fuel-tank"), 30, 40),
		add("esf-bus.svg", symbolBlockForID(transformerAsset, "esf-bus"), 120, 20),
		add("grid-bus.svg", symbolBlockForID(transformerAsset, "grid-bus"), 30, 50),
		add("breaker.svg", symbolBlockForID(transformerAsset, "breaker"), 24, 24),
		add("inverter-bus.svg", symbolBlockForID(transformerAsset, "inverter-bus"), 40, 30),
		add("sis-accumulators.svg", symbolBlockForID(safetySystemsAsset, "sis-accumulators"), 60, 80),
		add("efws-tank.svg", symbolBlockForID(safetySystemsAsset, "efws-tank"), 80, 100),
		add("ccws-hx.svg", symbolBlockForID(safetySystemsAsset, "ccws-hx"), 100, 50),
		add("rhrs-train.svg", symbolBlockForID(safetySystemsAsset, "rhrs-train"), 200, 80),
		add("cooling-tower.svg", symbolBlockForID(coolingTowerAsset, "cooling-tower"), 140, 190),
		add("river-intake.svg", symbolBlockForID(circulatingWaterAsset, "river-intake"), 180, 80),
		add("pump-cw.svg", symbolBlockForID(circulatingWaterAsset, "pump-cw"), 50, 90),
		add("sws-train.svg", symbolBlockForID(circulatingWaterAsset, "sws-train"), 120, 24),
	}
}

// WriteAssetFiles exports every symbol as a standalone SVG
// file in dir so Graphviz's image attribute can reference them.
// Each file contains exactly one symbol's inner content (no
// <symbol> wrapper), with the symbol's own viewBox preserved
// and width/height set in absolute inches.
func WriteAssetFiles(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	for _, f := range SymbolFiles() {
		if f.Content == "" {
			continue
		}
		path := filepath.Join(dir, f.Name)
		svg := fmt.Sprintf(
			`<?xml version="1.0" encoding="UTF-8" standalone="no"?>`+"\n"+
				`<svg xmlns="http://www.w3.org/2000/svg" width="%.4fin" height="%.4fin" viewBox="%s">`+"\n"+
				"%s\n"+
				`</svg>`+"\n",
			float64(f.W)/96.0, float64(f.H)/96.0, f.ViewBox, f.Content)
		if err := os.WriteFile(path, []byte(svg), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
