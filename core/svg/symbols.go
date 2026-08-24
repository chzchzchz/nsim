package svg

import "strings"

// symbolSpec is the single source of truth for how a plant-graph
// node kind maps onto the asset library: nominal footprint in
// inches (matching the <use> dimensions the composition draws),
// plus the asset file whose <symbol> renders the part. The symbol
// id equals the file name without extension, so one table drives
// DOT node sizing, image hints, and SVG symbol references alike.
type symbolSpec struct {
	wIn, hIn float64
	image    string
}

// symbolTable keys are either PlantGraph node Kind values or Valve
// Kind values; every kind the renderer can encounter must have an
// entry.
//
// Footprints match the Westinghouse P&ID figure proportions cited
// in the package comment of svg.go.
var symbolTable = map[string]symbolSpec{
	"ReactorPressureVessel":        {100.0 / 96.0, 240.0 / 96.0, "reactor_vessel.svg"},
	"Pressurizer":                  {60.0 / 96.0, 200.0 / 96.0, "pressurizer.svg"},
	"SteamGenerator":               {190.0 / 96.0, 430.0 / 96.0, "steam_generator.svg"},
	"Condenser":                    {400.0 / 96.0, 100.0 / 96.0, "condenser.svg"},
	"ReactorCoolantPump":           {50.0 / 96.0, 160.0 / 96.0, "pump-rcp.svg"},
	"CondensatePump":               {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"MainFeedwaterPump":            {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"HeaterDrainPump":              {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"DieselAFWPump":                {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"MotorAFWPump":                 {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"TurbineAFWPump":               {60.0 / 96.0, 40.0 / 96.0, "pump-turbine.svg"},
	"Turbine":                      {360.0 / 96.0, 200.0 / 96.0, "turbine-main.svg"},
	"Generator":                    {220.0 / 96.0, 140.0 / 96.0, "generator-main.svg"},
	"CondensateStorageTank":        {100.0 / 96.0, 130.0 / 96.0, "tank-cst.svg"},
	"HeaterDrainTank":              {100.0 / 96.0, 50.0 / 96.0, "tank-horizontal.svg"},
	"LowPressureHeaterDrain":       {50.0 / 96.0, 100.0 / 96.0, "heat-exchanger-vertical.svg"},
	"HighPressureHeaterDrain":      {50.0 / 96.0, 100.0 / 96.0, "heat-exchanger-vertical.svg"},
	"SafetyInjectionSystem":        {60.0 / 96.0, 80.0 / 96.0, "sis-accumulators.svg"},
	"EmergencyFeedwaterSystem":     {80.0 / 96.0, 100.0 / 96.0, "efws-tank.svg"},
	"ComponentCoolingWaterSystem":  {100.0 / 96.0, 50.0 / 96.0, "ccws-hx.svg"},
	"ResidualHeatRemovalSystem":    {200.0 / 96.0, 80.0 / 96.0, "rhrs-train.svg"},
	"SpentFuelPool":                {200.0 / 96.0, 100.0 / 96.0, "tank-sfp.svg"},
	"DieselGenerator":              {200.0 / 96.0, 100.0 / 96.0, "diesel-generator.svg"},
	"FuelTank":                     {30.0 / 96.0, 40.0 / 96.0, "fuel-tank.svg"},
	"StationTransformer":           {50.0 / 96.0, 50.0 / 96.0, "transformer-2w.svg"},
	"ESFBus":                       {120.0 / 96.0, 20.0 / 96.0, "esf-bus.svg"},
	"OffsiteGrid":                  {30.0 / 96.0, 50.0 / 96.0, "grid-bus.svg"},
	"PressurizerReliefTank":        {60.0 / 96.0, 80.0 / 96.0, "tank-vertical.svg"},
	"RCFCUnit":                     {40.0 / 96.0, 40.0 / 96.0, "rcfc-unit.svg"},
	"CirculatingWaterPump":         {50.0 / 96.0, 90.0 / 96.0, "pump-cw.svg"},
	"ComponentCoolingWaterPump":    {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"SpentFuelPoolPump":            {60.0 / 96.0, 40.0 / 96.0, "pump-horizontal.svg"},
	"CoolingTower":                 {140.0 / 96.0, 190.0 / 96.0, "cooling-tower.svg"},
	"RiverIntake":                  {180.0 / 96.0, 80.0 / 96.0, "river-intake.svg"},
	"ServiceWaterHeader":           {120.0 / 96.0, 24.0 / 96.0, "sws-train.svg"},
	"safety":                       {20.0 / 96.0, 20.0 / 96.0, "valve-safety.svg"},
	"relief":                       {20.0 / 96.0, 20.0 / 96.0, "valve-relief.svg"},
	"porv":                         {24.0 / 96.0, 20.0 / 96.0, "valve-porv.svg"},
	"stop":                         {24.0 / 96.0, 20.0 / 96.0, "valve-stop.svg"},
	"modulating":                   {24.0 / 96.0, 20.0 / 96.0, "valve-modulating.svg"},
	"swing_check":                  {24.0 / 96.0, 20.0 / 96.0, "valve-swing_check.svg"},
	"gate":                         {24.0 / 96.0, 20.0 / 96.0, "valve-gate.svg"},
}

// defaultNodeInches is the fallback footprint for kinds missing
// from symbolTable: Graphviz's own half-inch default box.
const defaultNodeInches = 0.5

const defaultSymbolImage = "valve-gate.svg"

// nodeSize returns the width/height in inches for a graph node kind.
func nodeSize(kind string) (w, h float64) {
	if s, ok := symbolTable[kind]; ok {
		return s.wIn, s.hIn
	}
	return defaultNodeInches, defaultNodeInches
}

// nodeImage returns the asset file name rendering a kind, or the
// generic gate-valve asset for unknown kinds.
func nodeImage(kind string) string {
	if s, ok := symbolTable[kind]; ok && s.image != "" {
		return s.image
	}
	return defaultSymbolImage
}

// symbolID returns the <symbol> id for a kind: the asset file name
// without extension.
func symbolID(kind string) string {
	return strings.TrimSuffix(nodeImage(kind), ".svg")
}

// SymbolForKind returns the symbol id for a Valve.Kind value.
// Unknown kinds fall back to "valve-gate".
func SymbolForKind(kind string) string {
	return symbolID(kind)
}
