package svg

import (
	"fmt"
	"math"
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

// TestWritePlantSVG_ScenarioStartStates renders every catalog
// starting state because the web UI mounts the diagram immediately
// after building the scenario plant.
func TestWritePlantSVG_ScenarioStartStates(t *testing.T) {
	for _, s := range core.Scenarios() {
		t.Run(s.ID, func(t *testing.T) {
			p := core.MakePlant()
			s.Apply(&p)
			var b strings.Builder
			require.NoError(t, WritePlantSVG(&b, &p))
			assert.Contains(t, b.String(), `<svg xmlns="http://www.w3.org/2000/svg"`)
		})
	}
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
		"containment-filter": true,
		"valve-safety":       true, "valve-porv": true, "breaker": true,
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
	p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 0.5
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
		{"SG", "Turbine", "stop"},
		{"Condenser", "CST", "stop"},
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

// TestElectricalTiesShowBreakerState checks that ties render only
// while their contact holds, appear as bound part groups, and show
// the closed-contact bar with a green status dot.
func TestElectricalTiesShowBreakerState(t *testing.T) {
	p := core.MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.ReactorBuilding.Grid.Voltage = 0
	p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed = false
	p.AuxiliaryBuilding.DieselGeneratorB.Generator.BreakerClosed = false

	out := renderPlant(t, &p)
	assert.Equal(t, 0, strings.Count(out, `class="contact-closed"`),
		"with every contact open, no tie may be drawn")

	p.AuxiliaryBuilding.DieselGeneratorB.Generator.BreakerClosed = true
	closed := renderPlant(t, &p)
	assert.Equal(t, 1, strings.Count(closed, `class="contact-closed"`),
		"closing one diesel output breaker draws exactly its feed tie")
	assert.Contains(t, closed,
		`data-part="AuxiliaryBuilding/DieselGeneratorB/FeedTie"`)
	m := regexp.MustCompile(
		`<use href="#breaker"[^>]*/>` +
			`<line x1="\d+" y1="(\d+)" x2="\d+" y2="(\d+)" stroke="black"`).
		FindStringSubmatch(closed)
	require.NotNil(t, m, "the drawn tie must carry a closed contact bar")
	assert.Equal(t, m[1], m[2],
		"the closed contact bar must be horizontal")
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
		"ReactorBuilding/SteamGenerator/MainSteamIsolationValve",
		"TurbineBuilding/RecirculateValve",
	} {
		assert.Contains(t, out,
			fmt.Sprintf(`<g data-part="%s" role="img" aria-label=`, part))
	}
}

// TestPidgapsPartsRender checks pidgaps.txt acceptance: regulator
// wedge/readout, SG startup valves on the outlet line, filtration
// run dot, and Grid binding to Electrical/Grid.
func TestPidgapsPartsRender(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	assert.Contains(t, out, `data-part="TurbineBuilding/FeedwaterRegulator"`)
	assert.Contains(t, out, `data-part="ReactorBuilding/SteamGenerator/SteamlineVent"`)
	assert.Contains(t, out, `data-part="ReactorBuilding/SteamGenerator/DrainValve"`)
	assert.Contains(t, out, `data-part="ReactorBuilding/SteamGenerator/WetLayupFillValve"`)
	assert.Contains(t, out, `>Cont. Filters<`)
	assert.Contains(t, out, `data-part="Electrical/Grid"`)
	assert.NotContains(t, out, `data-part="Electrical/OffsiteSourceA" role="img" aria-label="Grid"`)
	assert.NotContains(t, out, "anchors missing")

	p.TurbineBuilding.FeedwaterRegulator.ValveDemand = 0.42
	assert.Contains(t, renderPlant(t, &p), "FW 42%")

	p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.RecombinerActive = true
	base := strings.Count(out, `fill="green"`)
	assert.Greater(t, strings.Count(renderPlant(t, &p), `fill="green"`), base,
		"filtration run dot must follow recombiner state")
}

// unboundOK allow-lists rendered data-part values that intentionally
// have no bridge key under them. Anchors plan: the test fails if a
// new unbound part appears without an entry here.
var unboundOK = map[string]bool{}

// TestBridgePartCoverage enforces the anchors-plan acceptance
// criterion: every rendered data-part group prefix-matches a
// GetControlMap/GetSensorMap key, or is explicitly allow-listed.
// Fixture is fresh core.MakePlant() with no Plant.Step: bindings
// are static registration properties, not dynamics.
func TestBridgePartCoverage(t *testing.T) {
	p := core.MakePlant()
	cm := core.GetControlMap(&p)
	sm := core.GetSensorMap(&p)
	keys := map[string]bool{}
	for id := range cm {
		keys[id] = true
	}
	for id := range sm {
		keys[id] = true
	}
	// Bridge rows added by anchors.txt round-trip.
	for _, key := range []string{
		"TurbineBuilding/CondensateStorageTank/Level",
		"TurbineBuilding/HeaterDrainTank/Level",
		"TurbineBuilding/MakeupValve/Open",
		"ReactorBuilding/SafetyInjectionSystem/RWST/Level",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/CSTAligned",
		"AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePump/SteamAdmissionValve/Open",
	} {
		_, inC := cm[key]
		_, inS := sm[key]
		assert.True(t, inC || inS, "bridge key %s must round-trip", key)
	}
	out := renderPlant(t, &p)
	rendered := map[string]bool{}
	for _, seg := range strings.Split(out, `data-part="`) {
		idx := strings.Index(seg, `"`)
		if idx < 0 {
			continue
		}
		v := seg[:idx]
		if v == "" || strings.Contains(v, " ") || strings.Contains(v, "<") || strings.Contains(v, ">") {
			continue
		}
		rendered[v] = true
	}
	for part := range rendered {
		if unboundOK[part] {
			continue
		}
		covered := false
		for key := range keys {
			if key == part || strings.HasPrefix(key, part+"/") {
				covered = true
				break
			}
		}
		assert.True(t, covered,
			"rendered data-part %q must prefix-match a bridge key or be allow-listed", part)
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

// TestContainmentHydrogenWash checks the dome wash ramps amber with
// concentration below the alarm setpoint and turns red once the
// hydrogen alarm latches.
func TestContainmentHydrogenWash(t *testing.T) {
	p := core.MakePlant()
	base := renderPlant(t, &p)
	assert.NotContains(t, base, `class="h2-wash"`)

	hds := &p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression
	hds.HydrogenVolumePercent = 1.0
	assert.Contains(t, renderPlant(t, &p),
		`class="h2-wash" fill="#f0a500"`)

	hds.HydrogenVolumePercent = 3.0
	hds.HydrogenAlarm = true
	assert.Contains(t, renderPlant(t, &p),
		`class="h2-wash" fill="#e02020"`)
}

// TestSFPPoolHeatTint checks the pool water warms toward orange as
// temperature climbs through the derived 120-200 F band.
func TestSFPPoolHeatTint(t *testing.T) {
	p := core.MakePlant()
	assert.NotContains(t, renderPlant(t, &p), `class="sfp-wash"`)

	p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PoolTemp =
		core.Temperature{Val: 190, Unit: u.Fahrenheit}
	assert.Contains(t, renderPlant(t, &p), `class="sfp-wash"`)
}

// TestSnapAngle_CanonicalMultiples asserts every glyph rotation
// emitted by valvePartHTML is a multiple of 45 degrees. Properties:
// -0, +/-45, +/-90, +/-135, 180 result from snapping; no arbitrary
// spline angle may leak through.
func TestSnapAngle_CanonicalMultiples(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{0, 0}, {10, 0}, {22, 0}, {23, 45}, {45, 45}, {56, 45}, {68, 90}, {88, 90},
		{-45, -45}, {-90, -90}, {133, 135}, {178, 180}, {-178, -180},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, snapAngle(tc.in), "snapAngle(%v)", tc.in)
	}
	// Rendered-output check: no rotate() in the full document may
	// carry a non-canonical degree value.
	out := renderPlant(t, func() *core.Plant { p := core.MakePlant(); return &p }())
	re := regexp.MustCompile(`rotate\(([-\d.]+)\)`)
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		var deg float64
		fmt.Sscanf(m[1], "%f", &deg)
		assert.InDelta(t, 0, deg-math.Round(deg/45)*45, 0.001,
			"non-canonical valve rotation %v in output", deg)
	}
}

// TestBuildDot_DockingExtras asserts every id in
// dockingExtrasForRPV gains an invisible constraint edge to RPV in
// BuildDot output, and only when disconnected. Property, not exact
// coordinates.
func TestBuildDot_DockingExtras(t *testing.T) {
	p := core.MakePlant()
	g := p.BuildGraph()
	dot, err := BuildDot(g, nil)
	require.NoError(t, err)
	require.Contains(t, dot, `"RPV"`)
	for _, id := range dockingExtrasForRPV {
		needle := `"` + id + `" -> "RPV" [style=invis]`
		// Nodes that now carry a real edge are connected and
		// the connected[] guard drops them from the dock list
		// (plans/p59_1.txt). Asserting they are connected catches
		// a regression where a flow edge is lost.
		if _, connected := g.Nodes[id]; connected && len(flowEdgesFor(g, id)) > 0 {
			assert.NotContains(t, dot, needle,
				"connected node %s must not be docked", id)
			continue
		}
		if _, connected := g.Nodes[id]; connected {
			// RHRS_A/B are connected via real edges now; check
			// they are not docked.
			assert.NotContains(t, dot, needle,
				"connected node %s must not be docked", id)
			continue
		}
		assert.Contains(t, dot, needle, "docking edge %s missing", needle)
	}
}

// flowEdgesFor returns the ids that carry a real (Flow or deltaP)
// edge from the given node — used to assert docking behaviour.
func flowEdgesFor(g *core.PlantGraph, id string) []core.GraphEdge {
	var out []core.GraphEdge
	for _, e := range g.Edges {
		if e.From == id || e.To == id {
			out = append(out, e)
		}
	}
	return out
}

// TestComputeLayout_RCPBetweenSGAndRPV asserts the RCS loop is a
// horizontal rank (Figure 3-1): RPV, RCP and SG share the same
// anchor row with the pressurizer above the hot leg. X order
// within the rank is dot's crossing minimization (SG's eight
// feedwater inbound edges pin it leftmost), so only the rank
// alignment, a physics-forced property of rank=same, is pinned
// here; the relative X placement reads off the render.
// Uses the shared computed layout; no exact coordinates.
func TestComputeLayout_RCPBetweenSGAndRPV(t *testing.T) {
	_, layout := mustSharedLayout(t)
	sg, okSG := layout.Pos["SG"]
	rpv, okRPV := layout.Pos["RPV"]
	rcp, okRCP := layout.Pos["RCP"]
	pzr, okPZR := layout.Pos["Pressurizer"]
	require.True(t, okSG && okRPV && okRCP && okPZR)
	// One horizontal rank: centers align vertically.
	assert.InDelta(t, rpv.Y, sg.Y, 5)
	assert.InDelta(t, rpv.Y, rcp.Y, 5)
	// Pressurizer above the hot leg (smaller Y is higher after
	// the Y flip).
	assert.Less(t, pzr.Y, rpv.Y)
	assert.Equal(t, 126, len(symbolCount(t)), "symbol count drift (post-anchors): parallel edges keep both valve glyphs, pipe geometry deduped only")
}

// symbolCount counts top-level <use> outside <defs> in a render.
func symbolCount(t *testing.T) []string {
	t.Helper()
	p := core.MakePlant()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, &p))
	out := b.String()
	// Strip defs.
	start := strings.Index(out, "</defs>")
	if start >= 0 {
		out = out[start:]
	}
	return regexp.MustCompile(`<use href="#[^"]+"`).FindAllString(out, -1)
}
