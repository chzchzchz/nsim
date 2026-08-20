package core

// Electrical models the AC power distribution that supplies the
// Engineered Safety Features (ESF) loads. There are two physically and
// electrically independent trains per the spec.
//
// Spec justification:
//   - Train independence —
//     "Each power source, diesel generator and offsite, up to the point
//     of connection to the engineered safety features system power
//     buses, is physically and electrically independent."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:30-36
//   - Source preference (offsite > main generator > diesel) —
//     "The Engineered Safety Features buses are normally supplied from
//     the system auxiliary transformers. In the event of total loss of
//     auxiliary power from off-site sources, auxiliary power required
//     for safe shutdown will be supplied from physically and
//     electrically independent diesel generators located on the site."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:23-36
type Electrical struct {
	ESFBusA ESFBus
	ESFBusB ESFBus

	// Optional preferred sources, mirrored from the rest of the plant.
	// Kept as pointers so a nil source is treated as unavailable
	// (matching the spec's "total loss of auxiliary power" branch).
	OffsiteA   *PowerGenerator
	OffsiteB   *PowerGenerator
	MainGenA   *PowerGenerator
	MainGenB   *PowerGenerator
	DieselGenA *DieselGenerator
	DieselGenB *DieselGenerator
}

// ESFBus is one train of the Engineered Safety Features bus.
//
// Spec justification:
//   - Voltage rating —
//     "All of the engineered safety system loads are fed from two
//     6600-volt engineered safety features buses."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
//   - Energized state — the bus is considered energized when any of its
//     supplying sources is delivering voltage above the pickup threshold;
//     this matches the diesel-generator "ready to begin accepting load"
//     deadline of 10 s.
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50
type ESFBus struct {
	Part
	Voltage   float64
	Energized bool
}

// esfbusPickupVoltage is the minimum source voltage at which an ESF bus
// latches Energized. The spec does not state a number; 90% of rated is a
// standard protective-relay pickup.
const esfbusPickupVoltage = 0.9 * DieselRatedVoltage

// Step computes the per-train ESF bus state from its available sources.
// Follows the existing copy-into-nextPlant style: pure on input, writes
// only to ret.
func (e *Electrical) Step() Electrical {
	ret := *e
	ret.ESFBusA = stepESFBus(ret.ESFBusA, ret.OffsiteA, ret.MainGenA, ret.DieselGenA)
	ret.ESFBusB = stepESFBus(ret.ESFBusB, ret.OffsiteB, ret.MainGenB, ret.DieselGenB)
	return ret
}

func stepESFBus(b ESFBus, offsite, main *PowerGenerator, diesel *DieselGenerator) ESFBus {
	src := firstEnergized(offsite, main, dieselSource(diesel))
	if src != nil && src.Voltage >= esfbusPickupVoltage && src.BreakerClosed {
		b.Voltage = src.Voltage
		b.Energized = true
		return b
	}
	b.Voltage = 0
	b.Energized = false
	return b
}

func firstEnergized(candidates ...*PowerGenerator) *PowerGenerator {
	for _, c := range candidates {
		if c != nil && c.BreakerClosed && c.Voltage > 0 {
			return c
		}
	}
	return nil
}

// dieselSource returns the DieselGenerator's generator-side PowerGenerator
// as a *PowerGenerator. Pointer receiver so we can return nil for nil.
func dieselSource(d *DieselGenerator) *PowerGenerator {
	if d == nil {
		return nil
	}
	return &d.Generator
}
