package svg

import (
	"bytes"
	_ "embed"
	"strings"
)

// Asset library: each sibling .svg file holds one or more
// <symbol> definitions. This file embeds the asset files at
// compile time and exposes them as constants so the package
// callers don't need to read files at runtime. The wrapper
// <svg> element is stripped — only the inner <symbol> blocks
// are returned, ready to be dropped into the parent document's
// <defs>.
//
// Source-of-truth citations for each family live in plans/svg/.

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
)
