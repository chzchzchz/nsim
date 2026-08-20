package core

import (
	"io"

	u "github.com/bcicen/go-units"
)

func (r *ReactorBuilding) Step() ReactorBuilding {
	return *r
}

func (t *TurbineBuilding) Step() TurbineBuilding {
	ret := *t
	// SteamGenerator lives on ReactorBuilding, not TurbineBuilding;
	// the caller (Plant.Step) passes the upstream pressure in via
	// StepWithSteam. This signature stays for the legacy call path
	// where no upstream is wired; production uses StepWithSteam.
	return ret
}

// StepWithSteam advances the TurbineBuilding with the upstream steam
// generator's pressure supplied by the caller.
func (t *TurbineBuilding) StepWithSteam(sgPressure Pressure) TurbineBuilding {
	ret := *t
	ret.Turbine = ret.Turbine.Step(
		sgPressure,
		ret.Condenser.Pressure,
		ret.MakeupValve.Open,
		false,
	)
	ret.Generator.RealPower = ret.Turbine.RealPower
	return ret
}

// Plant represents all state relevant to the facility.
type Plant struct {
	ReactorBuilding
	TurbineBuilding
	Outside
	Electrical
	nextPlant *Plant
}

func (p *Plant) Step() {
	// Run Outside.Step first so the diesel generator state is in
	// nextPlant by the time Electrical.Step reads it. The cross-building
	// source pointers must point at nextPlant (where the fresh state is
	// being written), not at p (the current state being read).
	p.nextPlant.Outside = p.Outside.Step()
	p.Electrical.DieselGenA = &p.nextPlant.Outside.DieselGenerator
	p.Electrical.DieselGenB = &p.nextPlant.Outside.DieselGenerator

	p.nextPlant.ReactorBuilding = p.ReactorBuilding.Step()
	p.nextPlant.TurbineBuilding = p.TurbineBuilding.StepWithSteam(p.ReactorBuilding.SteamGenerator.Pressure)
	p.nextPlant.Electrical = p.Electrical.Step()

	// Pressure-driven fluid flow integrates the freshly-stepped parts.
	// Graph nodes hold pointers into nextPlant; the mutations land in
	// nextPlant directly so Update() copies them into p.
	simulateFluidFlow(p.BuildGraphFor(p.nextPlant), TimeStep)
}

func (p *Plant) Update() {
	p.ReactorBuilding = p.nextPlant.ReactorBuilding
	p.TurbineBuilding = p.nextPlant.TurbineBuilding
	p.Outside = p.nextPlant.Outside
	p.Electrical = p.nextPlant.Electrical
}

func MakePlant() Plant {
	p := Plant{nextPlant: &Plant{}}
	p.TurbineBuilding.HeaterDrainTank = Tank{
		Level:    u.NewValue(0, u.Gallon),
		Capacity: u.NewValue(13120, u.Gallon),
		Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
	}
	p.Outside = Outside{
		CondensateStorageTank: Tank{
			Level:    u.NewValue(300000, u.Gallon),
			Capacity: u.NewValue(450000, u.Gallon),
			Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
		},
		DieselGenerator: DieselGenerator{
			Speed: 0,
			Fuel: Tank{
				Level: u.NewValue(0, u.Gallon),
				// Can run feed pump for 24 hours
				// Can run generators for 4 days
				Capacity: u.NewValue(500, u.Gallon),
				Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
			},
			CoolantTemp: u.NewValue(80, u.Fahrenheit),
		},
		RiverTemp: u.NewValue(75, u.Fahrenheit),
	}
	return p
}

// BuildGraph returns a PlantGraph whose node refs point into p. Used
// outside the Step loop (ExportDot, tests) where mutating p is fine.
func (p *Plant) BuildGraph() *PlantGraph { return p.BuildGraphFor(p) }

// BuildGraphFor is the same as BuildGraph but lets the caller target an
// arbitrary *Plant (typically nextPlant during Step). Internal helpers
// use this to avoid mutating the current plant directly.
//
// The connections modelled are the primary loop, the secondary loop,
// and the AFWS suction line, drawn from
// specs/the_westinghouse_pressurized_water_react/glossary.md
// (cold leg, hot leg, hot standby) and chapter_5/page_099.txt (AFWS
// pumps, common pipe header).
func (p *Plant) BuildGraphFor(target *Plant) *PlantGraph {
	g := &PlantGraph{Nodes: map[string]GraphNode{}}

	add := func(id, kind string, ref interface{}) {
		g.Nodes[id] = GraphNode{ID: id, Kind: kind, Ref: ref}
	}
	edge := func(from, to string, valve *Valve, pump *Pump) {
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Valve: valve, Pump: pump})
	}

	add("RPV", "ReactorPressureVessel", &target.ReactorBuilding.ReactorPressureVessel)
	add("Pressurizer", "Pressurizer", &target.ReactorBuilding.Pressurizer)
	add("SG", "SteamGenerator", &target.ReactorBuilding.SteamGenerator)
	add("RCP", "ReactorCoolantPump", &target.ReactorBuilding.ReactorCoolantPump)
	edge("RPV", "SG", nil, &target.ReactorBuilding.ReactorCoolantPump) // hot leg through RCP
	edge("SG", "RPV", nil, nil)                                        // cold leg

	edge("RPV", "Pressurizer", &target.ReactorBuilding.SafetyValve, nil)
	edge("Pressurizer", "RPV", nil, nil)

	add("Condenser", "Condenser", &target.TurbineBuilding.Condenser)
	add("CST", "CondensateStorageTank", &target.Outside.CondensateStorageTank)
	add("HDT", "HeaterDrainTank", &target.TurbineBuilding.HeaterDrainTank)
	edge("SG", "Condenser", &target.TurbineBuilding.MakeupValve, nil)
	edge("Condenser", "CST", &target.TurbineBuilding.RecirculateValve, nil)
	edge("CST", "HDT", nil, &target.TurbineBuilding.CondensatePump)
	edge("HDT", "SG", nil, &target.TurbineBuilding.MainFeedwaterPump)

	add("DieselAFWPump", "DieselAFWPump", &target.Outside.DieselAFWPump.Pump)
	add("MotorAFWPump", "MotorAFWPump", &target.Outside.MotorAFWPump.Pump)
	add("TurbineAFWPump", "TurbineAFWPump", &target.Outside.TurbineAFWPump.Pump)
	edge("CST", "DieselAFWPump", &target.Outside.DieselAFWPump.CSTValve, nil)
	edge("DieselAFWPump", "SG", &target.Outside.DieselAFWPump.ServiceWaterValve, nil)
	edge("CST", "MotorAFWPump", &target.Outside.MotorAFWPump.CSTValve, nil)
	edge("MotorAFWPump", "SG", &target.Outside.MotorAFWPump.ServiceWaterValve, nil)
	edge("CST", "TurbineAFWPump", &target.Outside.TurbineAFWPump.CSTValve, nil)
	edge("TurbineAFWPump", "SG", &target.Outside.TurbineAFWPump.ServiceWaterValve, nil)

	return g
}

// ExportDot writes the plant's graph in Graphviz DOT format.
func (p *Plant) ExportDot(w io.Writer) error {
	return p.BuildGraph().ExportDot(w)
}
