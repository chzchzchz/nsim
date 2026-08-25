package core

import (
	u "github.com/bcicen/go-units"
)

// Step advances the Boron Recycle System by one timestep. The step
// function implements three state machines:
//
//   1. Reclaim processing. While FeedPumpRunning is latched, feed
//      draws at the evaporator duty rate from the CVCS holdup tanks
//      (plus any distillate recirculating inside the train), splitting
//      into 4-weight-percent concentrate and distillate.
//   2. Distillate radiation monitor. Evaporator carryover drives the
//      lumped scintillation-counter signal toward its high level
//      through a first-order lag; crossing the trip point latches
//      DistillateDiverted, sending distillate back to the feed
//      demineralizer instead of the reactor makeup water storage tank.
//      After the carryover clears and the signal decays below the
//      release point the divert releases and delivery resumes with the
//      recirculated inventory.
//   3. Product accounting. Concentrate accumulates as boric acid
//      product; undiverted distillate accumulates as storage-tank
//      delivery. HoldupDrawn reports how much holdup-tank inventory
//      this step consumed so Plant.Step can debit CVCS.BRSHoldupLevel
//      without a second writer on that field.
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
func (s *BoronRecycleSystem) Step(holdupGal float64) BoronRecycleSystem {
	ret := *s
	dt := TimeStep.Seconds()

	// Radiation monitor response: first-order lag toward the carryover
	// state. The signal decays whether or not the train runs, since
	// stopped production stops fresh contamination.
	target := 0.0
	if ret.Carryover {
		target = 1
	}
	alpha := dt / BRSSRadiationTauSec
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

	ret.FeedFlow = 0
	ret.HoldupDrawn = Volume{}
	if !ret.FeedPumpRunning {
		return ret
	}

	available := holdupGal + ret.Recirculating.Float()
	draw := min(BRSSEvaporatorDutyGPM*TimeStep.Seconds(), available)
	if draw <= 0 {
		return ret
	}
	ret.FeedFlow = BRSSEvaporatorDutyGPM

	// The 4 wt% product grade fixes the mass split for holdup liquid
	// at the rated-critical boron concentration: everything not left
	// in the concentrate leaves as clean distillate.
	concentrateFrac := CoreRatedCriticalBoronPpm /
		BRSSProductWeightPercentPpm
	concentrate := draw * concentrateFrac
	distillate := draw - concentrate

	// Recirculated distillate is already inside the train; it feeds
	// the draw before new holdup inventory does.
	fromRecirc := min(ret.Recirculating.Float(), draw)
	ret.Recirculating = vol(ret.Recirculating,
		ret.Recirculating.Float()-fromRecirc)
	ret.HoldupDrawn = Volume{Val: draw - fromRecirc, Unit: u.Gallon}

	ret.BoricAcidProduced = vol(ret.BoricAcidProduced,
		s.BoricAcidProduced.Float()+concentrate)
	if ret.DistillateDiverted {
		// Returned distillate rejoins the train upstream of the feed
		// demineralizer for reprocessing.
		ret.Recirculating = vol(ret.Recirculating,
			ret.Recirculating.Float()+distillate)
	} else {
		ret.DistillateDelivered = vol(ret.DistillateDelivered,
			s.DistillateDelivered.Float()+distillate)
	}

	return ret
}
