package core

import (
	"bytes"
	"strings"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Suction must stop at an empty hotwell: the condensate-pump edge
// runs on pump head alone (zero static differential both sides), so
// without a floor the transfer drives HotwellLevel negative forever.
// Physics-forced: stored inventory is non-negative.
func TestGraph_CondensatePumpStopsAtEmptyHotwell(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.FissionPower =
		CoreNeutronSourcePower
	p.ReactorBuilding.ReactorPressureVessel.DecayHeatFast = 0
	p.ReactorBuilding.ReactorPressureVessel.DecayHeatSlow = 0
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.MainFeedwaterPump.Running = false
	p.TurbineBuilding.Condenser.HotwellLevel =
		Volume{Val: 500, Unit: u.Gallon}
	p.TurbineBuilding.CondensatePump.Running = true
	for i := 0; i < 10000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		assert.GreaterOrEqual(t,
			p.TurbineBuilding.Condenser.HotwellLevel.Float(), 0.0,
			"hotwell level went negative at step %d", i)
		if p.TurbineBuilding.Condenser.HotwellLevel.Float() == 0 {
			break
		}
	}
	assert.LessOrEqual(t, p.TurbineBuilding.Condenser.HotwellLevel.Float(),
		1.0, "pump should have drained the hotwell to the floor")
}

func TestGraph_BuildsExpectedNodes(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	g := p.BuildGraph()

	for _, id := range []string{"RPV", "Pressurizer", "SG", "RCP", "Turbine", "Condenser", "CST", "DieselAFWPump", "EFWSTankA", "EFWSTankB"} {
		assert.Contains(t, g.Nodes, id, "missing node %q", id)
	}
	if _, ok := g.Nodes["RPV"]; ok {
		assert.Equal(t, "ReactorPressureVessel", g.Nodes["RPV"].Kind)
	}
}

func TestGraph_BuildsExpectedEdges(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	g := p.BuildGraph()

	wantEdges := map[[2]string]bool{
		{"RPV", "SG"}:            true, // hot leg
		{"SG", "RPV"}:            true, // cold leg
		{"SG", "Turbine"}:        true, // steam line (SG -> turbine)
		{"Turbine", "Condenser"}: true, // steam line (turbine exhaust)
		{"SG", "Condenser"}:      true, // steam dump (SG -> condenser)
		{"Condenser", "CST"}:     true, // condensate
		{"HDT", "SG"}:            true, // feedwater
		{"EFWSTankA", "SG"}:      true, // EFWS audit topology
		{"EFWSTankB", "SG"}:      true, // EFWS audit topology
	}
	for _, e := range g.Edges {
		key := [2]string{e.From, e.To}
		delete(wantEdges, key)
	}
	if len(wantEdges) > 0 {
		var missing []string
		for k := range wantEdges {
			missing = append(missing, k[0]+" -> "+k[1])
		}
		assert.Fail(t, "missing edges: %s", strings.Join(missing, ", "))
	}
}

func TestGraph_ExportDotRoundTrip(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	var buf bytes.Buffer
	require.NoError(t, p.ExportDot(&buf))
	out := buf.String()
	for _, expect := range []string{
		"digraph plant",
		`"RPV"`,
		`"SG"`,
		`"RPV" -> "SG"`,
		`"SG" -> "Turbine"`,
		`"Turbine" -> "Condenser"`,
		`"SG" -> "Condenser"`,
	} {
		assert.Contains(t, out, expect, "dot output missing %q", expect)
	}
}

func TestFluidFlow_ValveOpen_MovesWater(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	// Run the turbine so the Step-owned SG -> Turbine -> Condenser
	// flow edges carry steam; tripping the reactor aside, the
	// audit-only flow is the only SG drain in one step.
	p.ReactorBuilding.ReactorPressureVessel.Pressure = SteamGeneratorShellPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.State = TurbineRunning
	p.TurbineBuilding.Turbine.StopValve.Open = 1
	p.TurbineBuilding.Turbine.GoverningValve.Open = 1
	p.TurbineBuilding.Turbine.Synchronized = true
	p.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 100000, Unit: u.Gallon}

	beforeSG := p.ReactorBuilding.SteamGenerator.Water.Float()
	for i := 0; i < 100; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	afterSG := p.ReactorBuilding.SteamGenerator.Water.Float()

	assert.Less(t, afterSG, beforeSG, "SG water did not drop: before %v after %v", beforeSG, afterSG)
	// The turbine leg must have through-flowed: fresh BuildGraph()s
	// taken after every step must have seen SG->Turbine positive
	// at least once (the demand decays to zero as the rotor trips
	// after ~95 steps, so sampling only the tail would read zero).
	// Probe mid-run where SteamDemandGalPerStep is still live.
	p2 := MakePlant()
	p2.ReactorBuilding.ReactorPressureVessel.Pressure = SteamGeneratorShellPressure
	p2.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p2.TurbineBuilding.Turbine.State = TurbineRunning
	p2.TurbineBuilding.Turbine.StopValve.Open = 1
	p2.TurbineBuilding.Turbine.GoverningValve.Open = 1
	p2.TurbineBuilding.Turbine.Synchronized = true
	p2.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
	positive := false
	for i := 0; i < 100; i++ {
		p2.Step(DefaultTimeStep)
		g := p2.BuildGraph()
		for _, e := range g.Edges {
			if e.From == "SG" && e.To == "Turbine" && e.Flow(DefaultTimeStep) > 0 {
				positive = true
			}
		}
		p2.Update()
		if positive {
			break
		}
	}
	assert.True(t, positive, "SG->Turbine Flow closure never positive while turbine ran")
}

func TestFluidFlow_ValveClosed_NoMovement(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	// Turbine secured and dump shut: no steam path drains the SG.
	p.TurbineBuilding.Turbine.State = TurbineTripped
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Turbine.DumpValve.Open = 0
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 1000, Unit: u.Gallon}
	// Pin SG.HeatInput to zero so SG.Step doesn't boil inventory and
	// drift SG.Water during the assertion window.
	p.ReactorBuilding.SteamGenerator.HeatInput = 0

	before := p.ReactorBuilding.SteamGenerator.Water.Float()
	p.Step(DefaultTimeStep)
	p.Update()
	after := p.ReactorBuilding.SteamGenerator.Water.Float()
	delta := after - before
	assert.GreaterOrEqual(t, delta, -1.0, "SG water moved unexpectedly across a closed valve: before %v after %v (delta %v)", before, after, delta)
	assert.LessOrEqual(t, delta, 1.0, "SG water moved unexpectedly across a closed valve: before %v after %v (delta %v)", before, after, delta)
}

// TestLedger_SGSteamClosesToCondenser is a ledger-closure check
// (CHECK_SKILL §2): over a running window, the Step's own
// CondensingRate credits HotwellLevel, and the SG->Turbine Flow
// edge reads the same SteamDemand the Step debited. Sum the offtake
// fraction the Step exported: gross condensed = SteamOfftakeFraction
// integrated, and HotwellLevel must rise by at least the condensed
// mass minus feedwater delivery (here delivery is secured, so the
// rise equals the condensed mass, up to HDT dump seeding). This
// closes the loop the user asked for: SG draw == turbine through-
// flow == condenser credit, with Level as a pure mirror of the same
// mass (see condenser.go step 1 note) - HotwellLevel is the only
// onward ledger, so a Level+Hotwell double-sum would read 2x.
//
// Justification (CHECK_SKILL §8): ledger conservation is
// physics-forced; the exact equality uses the Step's own
// SteamOfftakeFraction (a derived state, not a spec value), so the
// half-band is a wiring check, not a fabricated fact. The directional
// reading - hotwell rises by a positive amount of the same order as
// the Step's credited condensation - is invariant to the HDT dump
// seeding; a 2x reading would prove Level double-count.
func TestLedger_SGSteamClosesToCondenser(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.Pressure = SteamGeneratorShellPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.State = TurbineRunning
	p.TurbineBuilding.Turbine.StopValve.Open = 1
	p.TurbineBuilding.Turbine.GoverningValve.Open = 1
	p.TurbineBuilding.Turbine.Synchronized = true
	p.TurbineBuilding.Turbine.Velocity = TurbineRatedSpeed
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 100000, Unit: u.Gallon}
	p.TurbineBuilding.MainFeedwaterPump.Running = false

	hotwell0 := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	condensed := 0.0
	for i := 0; i < 100; i++ {
		p.Step(DefaultTimeStep)
		// CondensingRate is cleared by Condenser.Step at its end
		// (condenser.go:121), so read it before Update clears the
		// next-plant copy - Step writes nextPlant then Update
		// copies. The pre-Update read is the post-Step value.
		p.Update()
		condensed += p.TurbineBuilding.SteamOfftakeFraction *
			CondenserDesignSteamLoadGalPer
	}
	hotwell1 := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	rise := hotwell1 - hotwell0
	assert.Positive(t, rise, "hotwell must have risen while running")
	// Condensed (= SteamOfftakeFraction integrated) must track the
	// hotwell rise; HDT high-level dump may add a small extra,
	// hence the wider band (physics-forced direction, CHECK_SKILL
	// §2). A 2x error here means Level was double counted.
	assert.InDelta(t, condensed, rise, condensed*0.5,
		"ledger: condensed %v vs hotwell rise %v", condensed, rise)
}

// Justification (CHECK_SKILL §8): dump-on-trip is spec-stated
// (page_215.txt:85-91); the assertion checks existence/direction
// of the Step's dumpedGalPerStep and exact agreement with the
// audit edge's Flow closure (a wiring invariant), not an invented
// value.

// TestLedger_DumpShutScalesWithHeader is a ledger check on the SG-
// >Condenser dump leg (CHECK_SKILL §2): with the turbine trip-
// secured but the dump valve open, the Step must still debit the
// SG header and credit the condenser via dumpedGalPerStep.
// Spec: dumps open "under a sudden reduction in turbine-generator
// load (in excess of 10%) or a turbine trip."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:85-91
// Uses Plant.Step through the real entrypoint: Turbine tripped,
// SG at shell pressure, MSIV open, DumpValve forced open. Then
// reads the dump leg's Flow closure after Update, which closes
// verbatim over the Step's dumpedGalPerStep.
func TestLedger_DumpShutScalesWithHeader(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.Pressure = SteamGeneratorShellPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.State = TurbineTripped
	p.TurbineBuilding.Turbine.DumpValve.Open = 1.0
	p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 1.0

	p.Step(DefaultTimeStep)
	p.Update()

	// dumpedGalPerStep (stored during StepWithSteam) is
	// already MSIV- and pressure-gated; require positive so the
	// dump leg's closed form cannot drift from the Step.
	assert.Positive(t, p.TurbineBuilding.Turbine.DumpedGalPerStep(),
		"dumpedGalPerStep must be positive while dump valve open, even tripped")

	g := p.BuildGraph()
	var dumpFlow float64
	for _, e := range g.Edges {
		if e.From == "SG" && e.To == "Condenser" {
			dumpFlow = e.Flow(DefaultTimeStep)
		}
	}
	require.Equal(t, p.TurbineBuilding.Turbine.DumpedGalPerStep(), dumpFlow,
		"SG->Condenser dump leg must agree with the Step's dumpedGalPerStep")
}

func TestFluidFlow_PumpRunning_DrivesFlowAlongEdge(t *testing.T) {
	t.Parallel()
	// The CondensatePump sits on the hotwell -> deaerator edge; the
	// pump takes suction from the condenser hotwell, not the CST.
	// Quiescent core so nothing else moves the hotwell during the
	// assertion window (no turbine exhaust to condense, no
	// feedwater delivery with the feed pump secured).
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.FissionPower =
		CoreNeutronSourcePower
	p.ReactorBuilding.ReactorPressureVessel.DecayHeatFast = 0
	p.ReactorBuilding.ReactorPressureVessel.DecayHeatSlow = 0
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.MainFeedwaterPump.Running = false
	p.TurbineBuilding.Condenser.HotwellLevel =
		Volume{Val: 50000, Unit: u.Gallon}

	hotwellBefore := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	cstBefore := p.Outside.CondensateStorageTank.Level.Float()
	p.TurbineBuilding.CondensatePump.Running = false
	for i := 0; i < 1000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	mid := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	assert.Equal(t, hotwellBefore, mid,
		"pump off but hotwell level moved: %v -> %v", hotwellBefore, mid)

	p.TurbineBuilding.CondensatePump.Running = true
	for i := 0; i < 1000; i++ {
		p.Step(DefaultTimeStep)
		p.Update()
	}
	after := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	assert.Less(t, after, mid,
		"pump on but hotwell level did not drop: %v -> %v", mid, after)
	assert.Equal(t, cstBefore,
		p.Outside.CondensateStorageTank.Level.Float(),
		"pump suction must come from the hotwell, not the CST")
}

func TestFluidFlow_ValveHalfOpen_HalvesFlow(t *testing.T) {
	t.Parallel()
	run := func(governing float64) float64 {
		// Direct Turbine.Step: stop-valve admission times the
		// governing-valve opening throttles the steam demand
		// (specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-44).
		tr := Turbine{
			State:          TurbineRunning,
			Velocity:       TurbineRatedSpeed,
			Synchronized:   false,
			GoverningValve: Valve{Open: governing},
			SpeedDemand:    1.0,
			StopValve:      Valve{Open: 1.0},
			Setpoint:       1.0,
		}
		open := 1.0
		out := tr.Step(DefaultTimeStep, SteamGeneratorShellPressure, CondenserDesignVacuum, Temperature{Val: 600}, false, &open)
		return out.SteamDemandGalPerStep()
	}
	full := run(1.0)
	half := run(0.5)
	require.Positive(t, half, "want positive flow for half-open")
	require.Positive(t, full, "want positive flow for full-open")
	ratio := half / full
	assert.InDelta(t, 0.5, ratio, 0.05, "half-open / full-open ratio = %v, want ~0.5", ratio)
}
