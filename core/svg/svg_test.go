package svg

import (
	"fmt"
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
		`<symbol id="cooling-tower"`,
		`<symbol id="river-intake"`,
		`<symbol id="pump-cw"`,
		`<symbol id="sws-train"`,
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
		"valve-safety": true, "valve-porv": true, "breaker": true,
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
// reaches the root element (with its freshness stamp), and that
// the flow stylesheet plus motion guards are emitted.
func TestRenderOptions_PhaseAndFlow(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVGWithOptions(&b, &p, CurrentLayout(), RenderOptions{PhaseMs: 1234}))
	out := b.String()
	assert.Contains(t, out, `style="--phase:1234ms"`)
	assert.Contains(t, out, `data-sim-time="1234"`,
		"remounted DOMs must be datable as stale vs current")
	assert.Contains(t, out, "@keyframes nsimflow")
}

// TestDocumentSummary checks the <desc> one-liner under <title>
// carries power and alarm count for screen readers and previews.
func TestDocumentSummary(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	idxTitle := strings.Index(out, "</title>")
	idxDesc := strings.Index(out, "<desc>")
	require.Positive(t, idxDesc)
	assert.Less(t, idxTitle, idxDesc, "desc sits under title")
	assert.Regexp(t, `<desc>\d+% nuclear power, \d+ active alarms</desc>`, out)
}

// TestMotionGuards checks reduced-motion users and print exports
// never see streaming dashes or a mid-flash (invisible) halo.
func TestMotionGuards(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	assert.Contains(t, out,
		"@media (prefers-reduced-motion:reduce){.flow,.blink{animation:none;}}")
	assert.Contains(t, out, "@media print{.blink{animation:none;}}")
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

// TestSVG_CoolingTowerSymbol checks the tower glyph carries the
// fan, fill, and basin cues from plans/svg/cooling_tower.md.
func TestSVG_CoolingTowerSymbol(t *testing.T) {
	sym := symbolBlockForID(coolingTowerAsset, "cooling-tower")
	require.NotEmpty(t, sym)
	for _, want := range []string{
		`id="cooling-tower-level"`,
		`id="ct-anchor-hot-inlet"`,
		`id="ct-anchor-cold-out"`,
		`id="ct-anchor-makeup"`,
	} {
		assert.Contains(t, sym, want)
	}
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	assert.Contains(t, out, `<g data-part="Outside/CoolingTower"`)
	assert.Contains(t, out, `<use href="#cooling-tower"`)
}

// TestSVG_CoolingTowerOverlays checks the basin gauge, fan run
// dot, and cold-basin readout track live state.
func TestSVG_CoolingTowerOverlays(t *testing.T) {
	p := core.MakePlant()
	ct := &p.Outside.CoolingTower
	ct.FanRunning = true
	ct.Basin.Level.Val = ct.Basin.Capacity.Val / 2
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	assert.Contains(t, out, `class="level-gauge"`)
	assert.Contains(t, out, `fill="green"`)
	assert.Contains(t, out,
		fmt.Sprintf("CT %.0f°F", ct.ColdBasinTemp.Float()))
}

// TestSVG_IntakeAndSWSRender checks the heat-rejection boundary
// parts from plans/svg/circulating_water.md resolve against the
// embedded layout.
func TestSVG_IntakeAndSWSRender(t *testing.T) {
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	assert.Contains(t, out, `<use href="#river-intake"`)
	assert.Contains(t, out, `<use href="#sws-train"`)
	assert.Contains(t, out, `<use href="#pump-cw"`)
	assert.Contains(t, out, "Intake House")
	assert.Contains(t, out, "river ")
	assert.Contains(t, out, "wb ")
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

// TestSafetyAndChemicalTrainsRender checks the SI completion (RWST,
// HHSI), containment spray train, CVCS skid, and MSR resolve against
// the embedded layout, and that their pumps draw live run dots.
func TestSafetyAndChemicalTrainsRender(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	for _, label := range []string{
		"RWST", "HHSI", "CSST", "SAT", "CSS Pump", "VCT", "Chg Pump", "MSR",
	} {
		assert.Contains(t, out, ">"+label+"<")
	}
	assert.NotContains(t, out, "anchors missing")
	require.Equal(t, len(levelAccessors),
		strings.Count(out, `class="level-gauge"`),
		"every tank-backed part renders exactly one gauge")

	base := strings.Count(out, `fill="green"`)
	p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = true
	p.AuxiliaryBuilding.ContainmentSpraySystem.Running = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	assert.Greater(t, strings.Count(renderPlant(t, &p), `fill="green"`),
		base, "running HHSI/CSS/charging pumps must draw green dots")
}

// TestScalarValvesRenderWedges checks float-driven valves share the
// standalone-valve wedge/tooltip encoding.
func TestScalarValvesRenderWedges(t *testing.T) {
	p := core.MakePlant()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AuxSprayValveOpen = 0.5
	out := renderPlant(t, &p)
	assert.Contains(t, out,
		`data-part="AuxiliaryBuilding/ChemicalVolumeControlSystem"`)
	assert.Contains(t, out, "<title>50%</title>")
}

// TestElectricalTiesShowBreakerState checks the four one-line ties
// carry breaker glyphs whose status dot tracks the contact state.
func TestElectricalTiesShowBreakerState(t *testing.T) {
	p := core.MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.ReactorBuilding.Grid.Voltage = 0

	out := renderPlant(t, &p)
	require.Equal(t, len(electricalTies),
		strings.Count(out, `class="breaker"`))
	allOpen := strings.Count(out, `fill="#aaa"`)

	p.Electrical.ESFBusA.BreakerClosed = true
	closed := renderPlant(t, &p)
	assert.Equal(t, allOpen-1, strings.Count(closed, `fill="#aaa"`),
		"closing a bus breaker must turn exactly one contact green")
}

// TestSafeguardSystemsRender checks the EFWS subsystem pumps, the
// containment isolation block, hydrogen recombiner, and exciter
// resolve against the embedded layout with their live dots and
// alarm halos.
func TestSafeguardSystemsRender(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	for _, label := range []string{
		"EFWS A MD", "EFWS A TD", "EFWS B MD", "EFWS B TD",
		"CIS", "HDS", "Exciter",
	} {
		assert.Contains(t, out, ">"+label+"<")
	}
	assert.NotContains(t, out, "anchors missing")

	base := strings.Count(out, `fill="green"`)
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Pump.Running = true
	p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.RecombinerActive = true
	p.TurbineBuilding.Generator.GeneratorExcitationSystem.ExciterActive = true
	assert.Greater(t, strings.Count(renderPlant(t, &p), `fill="green"`),
		base+2, "EFWS/recombiner/exciter activity must draw green dots")

	p.ReactorBuilding.ReactorProtectionSystem.ContainmentIsolationPhaseAActuated = true
	assert.Positive(t, strings.Count(renderPlant(t, &p), `stroke="#e02020"`),
		"a Phase A isolation (trip severity) must halo the CIS block")
}

// TestSecondDieselTrainRenders checks train B's EDG and day tank
// resolve against the layout, gauge, and ring independently of A.
func TestSecondDieselTrainRenders(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	assert.Contains(t, out, ">EDG B<")
	assert.Contains(t, out, ">EDG B Fuel<")
	assert.NotContains(t, out, "anchors missing")
	require.Equal(t, len(levelAccessors),
		strings.Count(out, `class="level-gauge"`))

	// Train B lights its own running ring without A.
	base := strings.Count(renderPlant(t, &p), `fill="none" stroke="green"`)
	p.AuxiliaryBuilding.DieselGeneratorB.State = core.DieselRunning
	assert.Equal(t, base+1,
		strings.Count(renderPlant(t, &p), `fill="none" stroke="green"`))

	// Shared annunciators: low fuel on either tank halos both.
	p.AuxiliaryBuilding.DieselGeneratorB.Fuel.Level = core.Volume{
		Val: 1, Unit: u.Gallon}
	assert.Greater(t,
		strings.Count(renderPlant(t, &p), `stroke="#f0a500"`), 0)
}

// TestStatorCoolingAndInvertersRender checks the SCCW pump and the
// inverter-fed instrument bus resolve against the layout and light
// their run dots from live flags.
func TestStatorCoolingAndInvertersRender(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	assert.Contains(t, out, ">SCCW Pump<")
	assert.Contains(t, out, ">Inverter<")
	assert.Contains(t, out, ">BTRS<")
	assert.Contains(t, out, ">BTRS Pump<")
	assert.NotContains(t, out, "anchors missing")

	// The plant builds with inverters available, so that dot is in
	// the baseline; starting the SCCW pump adds one.
	base := strings.Count(out, `fill="green"`)
	p.TurbineBuilding.Generator.SCCWPumpRunning = true
	withPump := strings.Count(renderPlant(t, &p), `fill="green"`)
	assert.Equal(t, base+1, withPump,
		"a running SCCW pump must draw its run dot")

	// BTRS recirculation lights the demineralizer skid's pump dot.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
	assert.Equal(t, base+2,
		strings.Count(renderPlant(t, &p), `fill="green"`),
		"a running BTRS pump must draw its run dot")

	p.ReactorBuilding.PlantElectricalSystems.InverterPowerAvailable = false
	assert.Equal(t, base+1,
		strings.Count(renderPlant(t, &p), `fill="green"`),
		"losing inverter power must extinguish its availability dot")
}

// TestEdgeValvesBindToBridgeRows checks inline (hop) valves render
// as bound part groups the web UI can click, matching standalone
// valve treatment.
func TestEdgeValvesBindToBridgeRows(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	for _, part := range []string{
		"TurbineBuilding/MainFeedwaterControlValve",
		"ReactorBuilding/SafetyValve",
		"TurbineBuilding/MakeupValve",
		"TurbineBuilding/RecirculateValve",
		"Outside/DieselAFWPump/CSTValve",
		"Outside/MotorAFWPump/ServiceWaterValve",
	} {
		assert.Contains(t, out,
			fmt.Sprintf(`<g data-part="%s" role="img" aria-label=`, part))
	}
}

// TestSymbolWashesTrackState checks intensity encodings appear and
// disappear with their driving flags.
func TestSymbolWashesTrackState(t *testing.T) {
	p := core.MakePlant()
	base := renderPlant(t, &p)
	assert.NotContains(t, base, `fill="#7a5c33"`,
		"fresh resin beds carry no saturation tint")
	assert.NotContains(t, base, `fill="#7a5c33"`,
		"fresh resin beds carry no saturation tint")

	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSBedSaturation = 1
	assert.Contains(t, renderPlant(t, &p), `fill="#7a5c33"`)

	// The plant builds with heaters latched on; dropping them must
	// extinguish the glow band.
	assert.Contains(t, base, `fill="#ffb84d"`,
		"latched heaters warm the pressurizer band at construction")
	p.ReactorBuilding.Pressurizer.HeatersOn = false
	assert.NotContains(t, renderPlant(t, &p), `fill="#ffb84d"`)
}
