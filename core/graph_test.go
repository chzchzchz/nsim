package core

import (
	"bytes"
	"strings"
	"testing"

	u "github.com/bcicen/go-units"
)

func TestGraph_BuildsExpectedNodes(t *testing.T) {
	p := MakePlant()
	g := p.BuildGraph()

	for _, id := range []string{"RPV", "Pressurizer", "SG", "RCP", "Condenser", "CST", "DieselAFWPump"} {
		if _, ok := g.Nodes[id]; !ok {
			t.Errorf("missing node %q", id)
		}
	}
	if _, ok := g.Nodes["RPV"]; ok {
		if g.Nodes["RPV"].Kind != "ReactorPressureVessel" {
			t.Errorf("RPV kind = %q, want ReactorPressureVessel", g.Nodes["RPV"].Kind)
		}
	}
}

func TestGraph_BuildsExpectedEdges(t *testing.T) {
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
		t.Errorf("missing edges: %s", strings.Join(missing, ", "))
	}
}

func TestGraph_ExportDotRoundTrip(t *testing.T) {
	p := MakePlant()
	var buf bytes.Buffer
	if err := p.ExportDot(&buf); err != nil {
		t.Fatalf("ExportDot: %v", err)
	}
	out := buf.String()
	for _, expect := range []string{
		"digraph plant",
		`"RPV"`,
		`"SG"`,
		`"RPV" -> "SG"`,
		`"SG" -> "Condenser"`,
	} {
		if !strings.Contains(out, expect) {
			t.Errorf("dot output missing %q\n--- output ---\n%s", expect, out)
		}
	}
}

func TestFluidFlow_ValveOpen_MovesWater(t *testing.T) {
	p := MakePlant()
	// Open the MakeupValve between SG and Condenser; pressurise the SG.
	p.TurbineBuilding.MakeupValve.Open = 1.0
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.TurbineBuilding.Condenser.Pressure = 0
	p.ReactorBuilding.SteamGenerator.Water = u.NewValue(1000, u.Gallon)

	beforeSG := p.ReactorBuilding.SteamGenerator.Water.Float()
	p.Step()
	p.Update()
	afterSG := p.ReactorBuilding.SteamGenerator.Water.Float()

	if !(afterSG < beforeSG) {
		t.Fatalf("SG water did not drop: before %v after %v", beforeSG, afterSG)
	}
}

func TestFluidFlow_ValveClosed_NoMovement(t *testing.T) {
	p := MakePlant()
	// Open == 0 must block flow across the valve edge. We also need to
	// equalise pressures on every other edge so they don't drain the SG.
	p.TurbineBuilding.MakeupValve.Open = 0
	p.ReactorBuilding.SteamGenerator.Pressure = 100
	p.TurbineBuilding.Condenser.Pressure = 100
	p.ReactorBuilding.ReactorPressureVessel.Pressure = 100
	p.TurbineBuilding.HeaterDrainTank.Pressure = 100
	p.ReactorBuilding.Pressurizer.Pressure = 100
	p.ReactorBuilding.SteamGenerator.Water = u.NewValue(1000, u.Gallon)

	before := p.ReactorBuilding.SteamGenerator.Water.Float()
	p.Step()
	p.Update()
	after := p.ReactorBuilding.SteamGenerator.Water.Float()

	if after != before {
		t.Fatalf("SG water moved with valve closed and pressures equalised: before %v after %v", before, after)
	}
}

func TestFluidFlow_PumpRunning_DrivesFlowAlongEdge(t *testing.T) {
	// The CondensatePump sits on the CST -> CondPump edge; equal
	// pressures and pump off means no flow; pump on means flow out of
	// CST (level drops) even with zero pressure differential.
	p := MakePlant()
	p.Outside.CondensateStorageTank.Pressure = 0
	// Make any flow out of CST observable by routing through a chain
	// whose other nodes have pressure equal to CST.
	p.TurbineBuilding.CondensatePump.Running = false
	before := p.Outside.CondensateStorageTank.Level.Float()
	for i := 0; i < 1000; i++ {
		p.Step()
		p.Update()
	}
	mid := p.Outside.CondensateStorageTank.Level.Float()
	if mid != before {
		t.Fatalf("pump off but CST level moved: %v -> %v", before, mid)
	}

	p.TurbineBuilding.CondensatePump.Running = true
	for i := 0; i < 1000; i++ {
		p.Step()
		p.Update()
	}
	after := p.Outside.CondensateStorageTank.Level.Float()
	if !(after < mid) {
		t.Fatalf("pump on but CST level did not drop: %v -> %v", mid, after)
	}
}

func TestFluidFlow_ValveHalfOpen_HalvesFlow(t *testing.T) {
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
		p.ReactorBuilding.SteamGenerator.Water = u.NewValue(100000, u.Gallon)
		before := p.ReactorBuilding.SteamGenerator.Water.Float()
		for i := 0; i < 1000; i++ {
			p.Step()
			p.Update()
		}
		return before - p.ReactorBuilding.SteamGenerator.Water.Float()
	}
	full := run(1.0)
	half := run(0.5)
	if !(half > 0 && full > 0) {
		t.Fatalf("want positive flow for both, got full=%v half=%v", full, half)
	}
	ratio := half / full
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("half-open / full-open ratio = %v, want ~0.5", ratio)
	}
}
