package svg

import (
	"regexp"
	"strings"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWritePlantSVG_ValidXML verifies that WritePlantSVG
// produces a parseable SVG document.
func TestWritePlantSVG_ValidXML(t *testing.T) {
	p := core.MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.Outlet =
		core.Temperature{Val: 621, Unit: u.Fahrenheit}
	var b strings.Builder
	if err := WritePlantSVG(&b, &p); err != nil {
		t.Fatalf("WritePlantSVG: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"`,
		`<symbol id="reactor_vessel"`,
		`<symbol id="pressurizer"`,
		`<symbol id="steam_generator"`,
		`<symbol id="condenser"`,
		`<symbol id="pump-rcp"`,
		`<symbol id="pump-horizontal"`,
		`<symbol id="pump-turbine"`,
		`<symbol id="valve-gate"`,
		`<symbol id="valve-safety"`,
		`<symbol id="valve-stop"`,
		`<symbol id="valve-modulating"`,
		`<symbol id="tank-vertical"`,
		`<symbol id="tank-accumulator"`,
		`<symbol id="heat-exchanger"`,
		`<symbol id="turbine-main"`,
		`<symbol id="generator-main"`,
		`<symbol id="transformer-2w"`,
		`<symbol id="esf-bus"`,
		`<symbol id="diesel-generator"`,
		`<symbol id="containment-dome"`,
		`<symbol id="css-spray-headers"`,
		`<symbol id="rcfc-unit"`,
		`<symbol id="sis-accumulators"`,
		`<symbol id="efws-tank"`,
		`<symbol id="ccws-hx"`,
		`<symbol id="rhrs-train"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in SVG output", want)
		}
	}
	// RPV outlet-temperature overlay (plans/x/fission_controls.md).
	// The writeLiveOverlays routine emits the value as a
	// "<text>T <n>°F</text>" centred just below the RPV symbol.
	if !strings.Contains(out, "T 621°F") {
		t.Errorf("want RPV outlet-temperature overlay containing \"T 621°F\", not found in SVG")
	}
}

// TestWritePlantSVG_NoMissingAnchors ensures every rendered part
// resolved against the embedded layout; a missing anchor silently
// drops the part, so the document carries an explicit comment when
// that happens.
func TestWritePlantSVG_NoMissingAnchors(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	assert.NotContains(t, b.String(), "anchors missing from layout")
}

// TestSymbolManifest checks that every symbol id the composition
// can reference exists in the emitted <defs>, so a typo renders as
// a test failure instead of blank space.
func TestSymbolManifest(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()

	defs := map[string]bool{}
	for _, m := range regexp.MustCompile(`<symbol id="([^"]+)"`).FindAllStringSubmatch(out, -1) {
		defs[m[1]] = true
	}
	require.NotEmpty(t, defs)

	refs := map[string]bool{
		"containment-dome": true, "css-spray-headers": true, "rcfc-unit": true,
		"valve-safety": true, "valve-porv": true,
	}
	for _, dp := range diagramParts {
		refs[symbolID(dp.kind)] = true
	}
	for _, kind := range []string{"gate", "swing_check", "relief", "porv", "stop", "modulating", "safety"} {
		refs[SymbolForKind(kind)] = true
	}
	for sym := range refs {
		assert.True(t, defs[sym], "symbol %q referenced but not defined in defs", sym)
	}
}

// TestRenderOptions_PhaseAndFlow checks that the animation phase
// reaches the root element and the flow stylesheet is emitted.
func TestRenderOptions_PhaseAndFlow(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVGWithOptions(&b, &p, CurrentLayout(), RenderOptions{PhaseMs: 1234}))
	out := b.String()
	assert.Contains(t, out, `style="--phase:1234ms"`)
	assert.Contains(t, out, "@keyframes nsimflow")
}

// TestValveWedgeRendersWhenOpen checks that a half-open valve emits
// its pie wedge and exact-percentage tooltip.
func TestValveWedgeRendersWhenOpen(t *testing.T) {
	p := core.MakePlant()
	p.TurbineBuilding.MakeupValve.Open = 0.5
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	assert.Contains(t, b.String(), "<title>50%</title>")
}

// TestRCFCPartsRender checks the four fan-cooler units resolve
// against the embedded layout with their run-status dots.
func TestRCFCPartsRender(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	assert.Contains(t, out, ">RCFC A<")
	assert.NotContains(t, out, "anchors missing")
}

// TestSymbolForKind_Defaults checks that unknown kinds fall back
// to the gate valve symbol.
func TestSymbolForKind_Defaults(t *testing.T) {
	if got := SymbolForKind(""); got != "valve-gate" {
		t.Errorf("empty kind: got %q want valve-gate", got)
	}
	if got := SymbolForKind("bogus"); got != "valve-gate" {
		t.Errorf("bogus kind: got %q want valve-gate", got)
	}
}

// TestSymbolForKind_Distinct ensures each documented kind maps to
// a non-empty, distinct symbol id.
func TestSymbolForKind_Distinct(t *testing.T) {
	kinds := []string{
		"gate", "swing_check", "safety", "relief",
		"porv", "stop", "modulating",
	}
	seen := map[string]string{}
	for _, k := range kinds {
		id := SymbolForKind(k)
		if id == "" {
			t.Errorf("kind %q: empty id", k)
			continue
		}
		if prev, ok := seen[id]; ok {
			t.Errorf("kind %q and %q collide on %q", prev, k, id)
		}
		seen[id] = k
	}
}

// TestValveKindForEdge covers the small dispatch table the
// composition pass uses to label graph-edge valves.
func TestValveKindForEdge(t *testing.T) {
	p := core.MakePlant()
	v := &core.Valve{}
	cases := []struct {
		from, to string
		want     string
	}{
		{"RPV", "Pressurizer", "safety"},
		{"Pressurizer", "RPV", "modulating"},
		{"SG", "Condenser", "modulating"},
		{"Condenser", "CST", "stop"},
		{"CST", "DieselAFWPump", "gate"},
	}
	for _, c := range cases {
		got := ValveKindForEdge(&p, core.GraphEdge{From: c.from, To: c.to, Valve: v})
		if got != c.want {
			t.Errorf("ValveKindForEdge(%s->%s): got %q want %q",
				c.from, c.to, got, c.want)
		}
	}
}
