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

	for _, id := range []string{"RPV", "Pressurizer", "SG", "RCP", "Condenser", "CST", "DieselAFWPump"} {
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
		{"SG", "Condenser"}:      true, // steam line
		{"Condenser", "CST"}:     true, // condensate
		{"HDT", "SG"}:            true, // feedwater
		{"CST", "DieselAFWPump"}: true, // AFW suction
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
		`"SG" -> "Condenser"`,
	} {
		assert.Contains(t, out, expect, "dot output missing %q", expect)
	}
}

func TestFluidFlow_ValveOpen_MovesWater(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	// Open the MakeupValve between SG and Condenser; pressurise the SG.
	// Equalise the RPV and HDT pressures so the cold-leg and
	// heater-drain edges sit at zero deltaP and don't drain the SG
	// across the 1-step assertion window. Trip the reactor and pin
	// the rods at full insertion so HeatInput collapses to the
	// decay-heat floor within one step.
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 1
	p.ReactorBuilding.ReactorPressureVessel.Pressure = 100
	p.TurbineBuilding.MakeupValve.Open = 1.0
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.TurbineBuilding.Condenser.Pressure = 0
	p.TurbineBuilding.HeaterDrainTank.Pressure = 100
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 1000, Unit: u.Gallon}

	beforeSG := p.ReactorBuilding.SteamGenerator.Water.Float()
	p.Step(DefaultTimeStep)
	p.Update()
	afterSG := p.ReactorBuilding.SteamGenerator.Water.Float()

	assert.Less(t, afterSG, beforeSG, "SG water did not drop: before %v after %v", beforeSG, afterSG)
}

func TestFluidFlow_ValveClosed_NoMovement(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	// Open == 0 must block flow across the valve edge. We also need to
	// equalise pressures on every other edge so they don't drain the SG.
	p.TurbineBuilding.MakeupValve.Open = 0
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.TurbineBuilding.Condenser.Pressure = 100
	p.ReactorBuilding.ReactorPressureVessel.Pressure = 100
	p.TurbineBuilding.HeaterDrainTank.Pressure = 100
	p.ReactorBuilding.Pressurizer.Pressure = 100
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
	run := func(open float64) float64 {
		p := MakePlant()
		// Equalise every other pressure so the only driving pressure is
		// SG -> Condenser through the MakeupValve.
		p.TurbineBuilding.MakeupValve.Open = open
		p.ReactorBuilding.SteamGenerator.Pressure = 1000
		p.ReactorBuilding.ReactorPressureVessel.Pressure = 1000
		p.TurbineBuilding.Condenser.Pressure = 0
		p.ReactorBuilding.Pressurizer.Pressure = 1000
		p.TurbineBuilding.HeaterDrainTank.Pressure = 1000
		p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 100000, Unit: u.Gallon}
		before := p.ReactorBuilding.SteamGenerator.Water.Float()
		for i := 0; i < 1000; i++ {
			p.Step(DefaultTimeStep)
			p.Update()
		}
		return before - p.ReactorBuilding.SteamGenerator.Water.Float()
	}
	full := run(1.0)
	half := run(0.5)
	require.Positive(t, half, "want positive flow for half-open")
	require.Positive(t, full, "want positive flow for full-open")
	ratio := half / full
	assert.InDelta(t, 0.5, ratio, 0.05, "half-open / full-open ratio = %v, want ~0.5", ratio)
}
