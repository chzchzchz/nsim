package core

import u "github.com/bcicen/go-units"

// fluidNode adapters for inventories that are Volume fields rather
// than Tank structs (plans/p59_1.txt, State table). Each wraps a
// pointer so any graph mutation lands in the live plant; Step
// ledgers still own the inventory and Flow edges are audit-only
// (graph.go: recordGraphFlow). RPV is deliberately NOT an adapter:
// primary and secondary are separate fluids (graph.go:90-106).

// vctNode adapts CVCS.VCTLevel. "pressurizer level represents a
// balance of injection water flow into the RCS and water letdown
// flow into the Chemical and Volume Control System."
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
type vctNode struct{ vol *Volume }

func (n *vctNode) pressure() Pressure { return PressurizerBasePressure }
func (n *vctNode) level() Volume      { return *n.vol }
func (n *vctNode) setLevel(v Volume)  { *n.vol = v }
func (n *vctNode) capacity() Volume   { return Volume{Val: VCTHighLevel, Unit: u.Gallon} }

// rwstNode adapts SIS.RWSTLevel. "The HHSI and RHR pumps are aligned
// to take suction directly from the RWST."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
type rwstNode struct{ vol *Volume }

func (n *rwstNode) pressure() Pressure { return AtmosphericPressure }
func (n *rwstNode) level() Volume      { return *n.vol }
func (n *rwstNode) setLevel(v Volume)  { *n.vol = v }
func (n *rwstNode) capacity() Volume   { return zeroVolume }

// sumpNode adapts SIS.Sump (capacity 0 = infinite sink).
type sumpNode struct{ vol *Volume }

func (n *sumpNode) pressure() Pressure { return AtmosphericPressure }
func (n *sumpNode) level() Volume      { return *n.vol }
func (n *sumpNode) setLevel(v Volume) {
	if v.Val < 0 {
		v.Val = 0
	}
	*n.vol = v
}
func (n *sumpNode) capacity() Volume { return zeroVolume }

// csstNode adapts CSS.CSSTLevel; capacity is the technical-manual
// tank size (CSSTCapacity, parts.go:3815, b44edd3).
type csstNode struct{ vol *Volume }

func (n *csstNode) pressure() Pressure { return AtmosphericPressure }
func (n *csstNode) level() Volume      { return *n.vol }
func (n *csstNode) setLevel(v Volume)  { *n.vol = v }
func (n *csstNode) capacity() Volume   { return CSSTCapacity }

// satNode adapts CSS.SATLevel; capacity is the technical-manual
// tank size (SATCapacity, parts.go:3825, b44edd3).
type satNode struct{ vol *Volume }

func (n *satNode) pressure() Pressure { return AtmosphericPressure }
func (n *satNode) level() Volume      { return *n.vol }
func (n *satNode) setLevel(v Volume)  { *n.vol = v }
func (n *satNode) capacity() Volume   { return SATCapacity }

// efwsTankNode adapts one EFWSTank (per-subsystem; the aggregate
// EFWS is not a tank).
type efwsTankNode struct{ tank *EFWSTank }

func (n *efwsTankNode) pressure() Pressure { return AtmosphericPressure }
func (n *efwsTankNode) level() Volume {
	return n.tank.Level
}
func (n *efwsTankNode) setLevel(v Volume) { n.tank.Level = v }
func (n *efwsTankNode) capacity() Volume  { return n.tank.Capacity }

// injectionSink is the zero-capacity fluidNode sink for SIS / RHRS /
// CVCS injection edges. RPV cannot be the destination (it is not a
// fluidNode, graph.go:90-106); the sink records the tick's credited
// volume adapter-locally for tests — it never feeds back into a Step.
type injectionSink struct{ DeliveredFlow float64 }

func (s *injectionSink) pressure() Pressure { return PressurizerBasePressure }
func (s *injectionSink) level() Volume      { return zeroVolume }
func (s *injectionSink) setLevel(Volume)    {}
func (s *injectionSink) capacity() Volume   { return zeroVolume }
func (s *injectionSink) recordFlow(gal float64) {
	s.DeliveredFlow += gal
}

// cisSink is the infinite sink for CSS spray edges ("Cold water can
// be sprayed into the pressurizer relief tank...", spray to
// containment; page_114.txt:57-63). It stores no inventory.
type cisSink struct{ DeliveredFlow float64 }

func (s *cisSink) pressure() Pressure { return AtmosphericPressure }
func (s *cisSink) level() Volume      { return zeroVolume }
func (s *cisSink) setLevel(Volume)    {}
func (s *cisSink) capacity() Volume   { return zeroVolume }
func (s *cisSink) recordFlow(gal float64) {
	s.DeliveredFlow += gal
}

// accNode adapts SIS.Accumulator.Level (bank of 4 tanks, one
// combined level). Discharge rate and isolation are read by the
// edge Flow closure from AccumulatorInjectionFlow.
type accNode struct {
	vol    *Volume
	capVol *Volume
}

func (n *accNode) pressure() Pressure { return SISAccumulatorPrechargePressure }
func (n *accNode) level() Volume      { return *n.vol }
func (n *accNode) setLevel(v Volume)  { *n.vol = v }
func (n *accNode) capacity() Volume   { return *n.capVol }

// turbineNode adapts the main turbine as a pass-through flow node
// on the secondary steam path SG -> Turbine -> Condenser. The
// turbine stores no inventory; its FirstStagePressure reports
// admitted header pressure and level is zero, so Flow edges
// through it are audit-only (graph.go: recordGraphFlow) and
// never move volume in simulateFluidFlow. Zero level/capacity
// prevents the integrator from inventing inventory at the
// pass-through, matching the spec's series steam path where
// the turbine expands steam and exhausts it to the condenser
// without an intervening storage tank:
//   "The greater portion of the steam is fully expanded through
//    the turbine before being exhausted to the condenser."
//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:39-42
type turbineNode struct {
	t  *Turbine
	// DeliveredFlow is the audit-local accumulator for the
	// SG->Turbine leg (recordGraphFlow seam, plans/p59_1.txt §5):
	// the SG->Turbine Flow edge credits its tick's volume here so
	// tests can prove the turbine actually through-flowed. It
	// never feeds back into any Step ledger.
	DeliveredFlow float64
}

func (n *turbineNode) pressure() Pressure {
	return Pressure(float64(n.t.FirstStagePressure))
}
func (n *turbineNode) level() Volume      { return zeroVolume }
func (n *turbineNode) setLevel(Volume)    {}
func (n *turbineNode) capacity() Volume   { return zeroVolume }
func (n *turbineNode) recordFlow(gal float64) {
	n.DeliveredFlow += gal
}
