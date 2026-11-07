package core

import "time"

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

	// OffsiteSource is the simplified offsite auxiliary-power
	// representation used to keep the ESF buses "normally
	// energised" at construction. The model does not track the
	// offsite grid in detail; this struct holds a single
	// energised PowerGenerator per train that the ESF bus can
	// pull from as long as OffsitePowerAvailable is true. When
	// the plant operator declares a loss of offsite power
	// (PlantElectricalSystems.OffsitePowerAvailable=false) the
	// model drops Voltage to zero, mirroring the spec's
	// "automatic transfer of supplies" path
	// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59).
	OffsiteSourceA PowerGenerator
	OffsiteSourceB PowerGenerator
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
//   - BreakerClosed — operator control to isolate one train of the ESF
//     system. The spec does not name a "bus breaker" explicitly, but the
//     two trains are required to be "physically and electrically
//     independent" up to the bus (page_165.txt:23-29), which presumes
//     some operator-controlled disconnect. Default true: closed.
type ESFBus struct {
	Part
	Voltage       float64
	Energized     bool
	BreakerClosed bool

	// OnDiesel is true while this bus's own diesel generator is
	// the serving source, and DieselFeedElapsed accumulates the
	// time since the transfer. Both reset when any non-diesel
	// source serves the bus or the bus goes dark; they drive the
	// automatic load sequencer that staggers large loads onto a
	// freshly transferred bus.
	OnDiesel          bool
	DieselFeedElapsed time.Duration
}

// StageReady reports whether a load in the given sequence stage may be
// (re-)energized from this bus: immediately when the bus is served by a
// non-diesel source (no overload constraint), otherwise only after the
// stage's delay has elapsed since the diesel picked up the bus.
func (b ESFBus) StageReady(stage int) bool {
	if !b.Energized {
		return false
	}
	if stage <= 0 || !b.OnDiesel {
		return true
	}
	return b.DieselFeedElapsed >=
		time.Duration(stage)*esfLoadSequenceStageDelay
}

// esfbusPickupVoltage is the minimum source voltage at which an ESF bus
// latches Energized. The spec does not state a number; 90% of rated is a
// standard protective-relay pickup.
const esfbusPickupVoltage = 0.9 * DieselRatedVoltage

// esfLoadSequenceStageDelay spaces successive stages of large-load
// re-energization after a diesel transfer: "Loads are automatically
// sequenced on the diesel generator buses over a short period of time to
// preclude the possibility of overload trip due to simultaneous starting
// loads." The spec gives no interval; 5 s per stage keeps the total
// sequence well inside the diesel's 30 s rated-load deadline (derived
// sizing assumption).
// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:52-55
const esfLoadSequenceStageDelay = 5 * time.Second

// Load sequence stages for ESF loads re-energized from their diesel.
// Stage 0 loads (instrument/cooling support) come back with the bus;
// stage 1 loads (the largest motors: RHR, HHSI, CSS pumps) wait one
// stage delay so simultaneous starting current cannot overload the
// freshly transferred bus. Membership is a modeling convention at this
// fidelity band.

// Step computes the per-train ESF bus state from its available sources.
// Follows the existing copy-into-nextPlant style: pure on input, writes
// only to ret.
func (e *Electrical) Step(dt time.Duration) Electrical {
	ret := *e
	ret.ESFBusA = stepESFBus(dt, ret.ESFBusA, ret.OffsiteA, ret.MainGenA, ret.DieselGenA)
	ret.ESFBusB = stepESFBus(dt, ret.ESFBusB, ret.OffsiteB, ret.MainGenB, ret.DieselGenB)
	return ret
}

// stepESFBus energizes the bus from its best source (offsite preferred,
// then the unit's own generator through the UAT, then its diesel) and
// tracks whether the diesel is serving the bus for the load sequencer.
// The source preference follows the spec's supply order: normally the
// system auxiliary transformers (offsite); with offsite lost but the
// unit on the line, the unit auxiliary transformer carries the bus from
// the main generator; the diesel is the last line.
//   - "The unit auxiliary power is transformed from the output of
//     the plant main generator through the unit auxiliary
//     transformer to the plant electrical distribution voltage."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:44-48
//   - "auxiliary power required for safe shutdown will be supplied
//     from physically and electrically independent diesel
//     generators located on the site."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:27-31
func stepESFBus(dt time.Duration, b ESFBus, offsite, main *PowerGenerator, diesel *DieselGenerator) ESFBus {
	if !b.BreakerClosed {
		b.Voltage = 0
		b.Energized = false
		b.OnDiesel = false
		b.DieselFeedElapsed = 0
		return b
	}
	src := firstEnergized(offsite, main, dieselSource(diesel))
	if src != nil && src.Voltage >= esfbusPickupVoltage && src.BreakerClosed {
		b.Voltage = src.Voltage
		b.Energized = true
		fromDiesel := diesel != nil && src == dieselSource(diesel)
		if fromDiesel {
			if !b.OnDiesel {
				b.OnDiesel = true
				b.DieselFeedElapsed = 0
			}
			b.DieselFeedElapsed += dt
		} else {
			b.OnDiesel = false
			b.DieselFeedElapsed = 0
		}
		return b
	}
	b.Voltage = 0
	b.Energized = false
	b.OnDiesel = false
	b.DieselFeedElapsed = 0
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
