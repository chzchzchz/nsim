package core

import (
	u "github.com/bcicen/go-units"
	"time"
)

// Step advances the Boron Recycle System by one timestep. The step
// function implements three state machines:
//
//  1. Reclaim processing. While FeedPumpRunning is latched, feed
//     draws at the evaporator duty rate from the CVCS holdup tanks
//     (plus any distillate recirculating inside the train), splitting
//     into 4-weight-percent concentrate and distillate.
//  2. Distillate radiation monitor. Evaporator carryover drives the
//     lumped scintillation-counter signal toward its high level
//     through a first-order lag; crossing the trip point latches
//     DistillateDiverted, sending distillate back to the feed
//     demineralizer instead of the reactor makeup water storage tank.
//     After the carryover clears and the signal decays below the
//     release point the divert releases and delivery resumes with the
//     recirculated inventory.
//  3. Product accounting. Concentrate accumulates as boric acid
//     product; undiverted distillate accumulates as storage-tank
//     delivery. HoldupDrawn reports how much holdup-tank inventory
//     this step consumed so Plant.Step can debit CVCS.BRSHoldupLevel
//     without a second writer on that field.
//
// Spec justification:
//   - Feed pump and stripping column: "The borated liquid is then
//     pumped to the recycle evaporator package by one of the recycle
//     evaporator feed pumps. Here hydrogen and residual fission gases
//     are removed in the stripping column before the liquid enters the
//     evaporator shell. The evaporative process produces a batch of
//     4-weight-percent boric acid solution."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:1-7
//   - Distillate path and radiation auto-divert: "Distillate from the
//     evaporator is pumped to the evaporator condensate demineralizer
//     and then to the reactor makeup water storage tank. [...] A high
//     radiation level signal at the monitor directs distillate back to
//     the recycle evaporator feed demineralizer."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:20-30
//   - Manual panel operation and duty: "The BRS is manually operated
//     from the boron recycle panel. [...] use of the BTRS reduces the
//     average evaporator duty from approximately 10 gpm to less than
//     2 gpm per unit."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:66-74
func (s *BoronRecycleSystem) Step(dt time.Duration, holdupGal float64) BoronRecycleSystem {
	ret := *s
	dtSec := dt.Seconds()

	// Processing runs first so the radiation monitor samples this
	// step's actual distillate flow: the scintillation counter sits in
	// the line leading into the storage tank, so it only sees
	// contamination in distillate the train is producing.
	// "a scintillation counter is located in the line leading into the
	// tank."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:26-28
	ret.FeedFlow = 0
	ret.HoldupDrawn = Volume{}
	producing := false
	if ret.FeedPumpRunning {
		available := holdupGal + ret.Recirculating.Float()
		draw := min(BRSSEvaporatorDutyGPM*dtSec, available)
		if draw > 0 {
			producing = true
			// Honest rate: an availability-limited draw reports a
			// proportional feed flow instead of the duty figure.
			ret.FeedFlow = draw / dtSec

			// The 4 wt% product grade fixes the mass split for holdup
			// liquid at the rated-critical boron concentration:
			// everything not left in the concentrate leaves as clean
			// distillate.
			concentrateFrac := CoreRatedCriticalBoronPpm /
				BRSSProductWeightPercentPpm
			concentrate := draw * concentrateFrac
			distillate := draw - concentrate

			// Recirculated distillate is already inside the train; it
			// feeds the draw before new holdup inventory does.
			fromRecirc := min(ret.Recirculating.Float(), draw)
			ret.Recirculating = vol(ret.Recirculating,
				ret.Recirculating.Float()-fromRecirc)
			ret.HoldupDrawn = Volume{Val: draw - fromRecirc, Unit: u.Gallon}

			ret.BoricAcidProduced = vol(ret.BoricAcidProduced,
				s.BoricAcidProduced.Float()+concentrate)
			if ret.DistillateDiverted {
				// Returned distillate rejoins the train upstream of the
				// feed demineralizer for reprocessing.
				ret.Recirculating = vol(ret.Recirculating,
					ret.Recirculating.Float()+distillate)
			} else {
				ret.DistillateDelivered = vol(ret.DistillateDelivered,
					s.DistillateDelivered.Float()+distillate)
			}
		}
	}

	// Radiation monitor response: first-order lag toward the carryover
	// state, driven only while contaminated distillate is actually
	// flowing. The signal decays whether or not the train runs.
	target := 0.0
	if ret.Carryover && producing {
		target = 1
	}
	alpha := dtSec / BRSSRadiationTauSec
	ret.DistillateRadiation = ret.DistillateRadiation*(1-alpha) + target*alpha

	// Auto-divert latch with hysteresis: trip above the high-signal
	// fraction, release only after the source cleared and the signal
	// decayed below the release fraction, so a hovering monitor cannot
	// chatter the distillate path.
	// "A high radiation level signal at the monitor directs distillate
	// back to the recycle evaporator feed demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:28-30
	if ret.DistillateRadiation >= BRSSRadiationTripFrac {
		ret.DistillateDiverted = true
	}
	if !ret.Carryover && ret.DistillateRadiation < BRSSRadiationReleaseFrac {
		ret.DistillateDiverted = false
	}

	return ret
}
