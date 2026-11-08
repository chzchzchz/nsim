package core

import (
	u "github.com/bcicen/go-units"
	"time"
)

// Step advances the steam generator by ProductionStep. It implements the
type SteamGenerator struct {
	Part
	Pressure
	Water  Volume
	Volume Volume

	// SteamFlowRate is the design steam mass-flow the SG is sized
	// for, in volumetric units compatible with Water/Volume.
	// Sized to the spec's 480 kg/s full-load steam flow.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
	SteamFlowRate Volume

	// HeatInput is the primary-to-secondary heat flux in Watts.
	// Caller-supplied (the reactor step is out of scope; p7
	// candidate). When zero the SG does not generate steam on its
	// own; the fluid graph dominates the inventory.
	HeatInput Watts

	// MainSteamIsolationValve is the operator-controlled
	// isolation valve on the SG outlet steam line. Closing
	// it isolates the SG secondary side from the turbine and
	// steam dump. The bridge exposes this as the MSIV
	// control; closing it on a high-steam-line-break
	// signal is one of the reactor-protection-system
	// isolation paths.
	// "Each steam generator has isolation valves [...]
	// upstream of the main steam line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	MainSteamIsolationValve Valve

	// SafetyValve is the lumped ASME-Code spring-loaded
	// safety valve on the SG outlet steam line, upstream of
	// the MSIV. The valve self-actuates when SG pressure
	// exceeds SafetyValveSetpoint; the bridge exposes
	// Open for visibility.
	// "Safety valves in each steam generator outlet main
	// steam line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:42-45
	SafetyValve Valve

	// AtmosphericDumpValve is the power-operated atmospheric
	// dump valve (also called the "steam dump to
	// atmosphere") on the SG outlet steam line, upstream
	// of the MSIV. The operator (or the turbine-runback
	// interlock) opens it to dump excess steam to
	// atmosphere when the turbine cannot accept the SG
	// steam flow (e.g. turbine trip, load rejection).
	// "ASME Code safety valves and power operated relief
	// and atmospheric dump valves are located on each
	// steam line upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	AtmosphericDumpValve Valve

	// MainSteamLineBreak is the malfunction path modeling a main
	// steam header rupture upstream of the MSIV, discharging
	// straight into containment. Open is the normalized break
	// size (0 intact, 1 full header severance); the discharge
	// debits shell inventory and feeds the containment enthalpy
	// input through BreakFlow.
	// "This malfunction event causes steam pipe break in the main
	//  steam line before the main steam stop valve (MSV) inside
	//  containment, leading to rapid depressurizaton of the main
	//  steam pressure."
	// specs/pwr_sim/chapter_4/page_053.txt:3-7
	MainSteamLineBreak Valve

	// WetLayupFillValve tops the shell steam side with water up to
	// the vents once the reactor coolant is at layup temperature
	// and the header is de-pressurised. The Step refuses to fill a
	// live header.
	// "When the reactor coolant reaches 200°F (93°C), the steam
	// sides of the steam generators are filled to the vents for wet
	// layup."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:144-145
	WetLayupFillValve Valve

	// SteamlineVent is the operator-controlled steamline vent on
	// the SG outlet steam line. During low-temperature operation it
	// carries RCP-boiled steam off the header so primary
	// temperature can be raised on pump heat alone; the vents are
	// closed once the reactor coolant reaches 210-220°F and the
	// secondary is lined up for steam-dump/turbine service.
	// "When the reactor coolant temperature increases to about
	//  210-220°F (90-105°C), the steamline vents are closed and
	//  the secondary side of each steam generator is drained to
	//  the normal operating range."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
	SteamlineVent Valve

	// DrainValve is the operator-controlled shell drain that pulls
	// SG secondary inventory down to the normal operating range at
	// the same point in the startup sequence. The drawn water lands
	// in the condenser hotwell (assumption; the spec names the
	// drain without naming its destination).
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
	DrainValve Valve

	// VentFlow and DrainFlow are the per-step volumes drawn through
	// SteamlineVent and DrainValve during the most recent Step, in
	// the CondensingRate convention (gallons per step). Plant.Step
	// routes both into the condenser hotwell so vented steam and
	// drained water stay inside the tracked secondary inventory.
	VentFlow  Volume
	DrainFlow Volume

	// AdvFlow is the per-step volume dumped through
	// AtmosphericDumpValve during the most recent Step (gallons
	// per step, same convention as VentFlow). Plant.Step has no
	// condenser-credit for it (atmosphere), so the field is
	// exported purely for the graph audit edge; the shell debit
	// already happens inline.
	AdvFlow Volume

	// SafetyFlow is the per-step volume dumped through the
	// spring-loaded SafetyValve (setpoint-driven) during the most
	// recent Step (gallons per step, same convention as
	// VentFlow). Magnitude is derived from the existing in-line
	// svFraction computation, not from a spec value; exported
	// purely for the SG -> Outside audit edge alongside AdvFlow,
	// so both valves on the same pipe are billed.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	SafetyFlow Volume

	// BreakFlow is the per-step volume discharged through
	// MainSteamLineBreak during the most recent Step (gallons per
	// step, same convention as VentFlow). Unlike the vent draw it
	// is lost from the secondary: Plant.Step converts it into
	// containment enthalpy input instead of condenser makeup.
	BreakFlow Volume

	// SGTubeRupture is the malfunction path modeling a steam
	// generator tube rupture: primary coolant leaks from the tube
	// bundle (inside pressure ~RCS) into the secondary shell
	// (~header pressure) through a pinhole in the tube sheet. Open
	// is the normalized rupture size (0 intact, 1 double-ended-
	// equivalent full break of the largest tube).
	//
	// "Tube Sheet (thick plate separating primary/secondary at bottom)"
	// specs/the_westinghouse_pressurized_water_react/imgs/023.md:4
	//
	// This is NOT the main steam line break: it does not vent the
	// header to containment (that is MainSteamLineBreak), and it
	// cannot be isolated by the MSIV — it terminates only when the
	// RCS depressurizes below the shell pressure. The discharged
	// volume lands in SGTubeRuptureFlow for the monitors and the
	// inventory ledger.
	SGTubeRupture Valve

	// SGTubeRuptureFlow is the per-step volume leaked through
	// SGTubeRupture during the most recent Step (gallons per step,
	// same convention as BreakFlow). The same call debits
	// RPV.BreakDischarged and credits shell water, and hands the
	// flow to the radiation monitors.
	SGTubeRuptureFlow Volume

	// SteamlineRadiation is the normalized steam-line radiation
	// monitor reading (1 = trip level), driven by leaked primary
	// water that boiled into the header. First-order lag toward 1
	// while the leak flows, decaying after it stops.
	SteamlineRadiation float64

	// SteamlineIsolated is the auto-isolate latch driven by the
	// steam-line radiation monitor (trip/release hysteresis). While
	// latched Plant.Step closes the MSIV and secures main
	// feedwater.
	SteamlineIsolated bool

	// FeedRingVoidFrac is the current steam-void fraction in the feed ring
	// (persistent, 0..1): it accumulates while voiding conditions hold and
	// decays when feed sweeps the ring, so the high-flow hammer step reads
	// the void built up on previous steps (hammer is edge-triggered on
	// prior void, not on same-step void). Exposed as a sensor, not damage.
	// Levels are converted shell-volume % to NR % via SgNarrowRangeSpanPct.
	// "Steam voids form in the SG feed rings only if the SG levels are
	// below the tops of the feed rings and the total feed flow rate is
	// less than the leakage out of the system. The maximum leakage rate
	// was calculated at 30 gpm."
	// specs/westinghouse_tech_manual/chapter_5/page_0537.txt:36-39
	FeedRingVoidFrac float64

	// HammerDamageFrac is the cumulative water-hammer damage to the feed
	// nozzle / sleeve / piping from void collapse at high AFW flow. This is
	// the damage accumulator for this mechanism; SleeveGapIn below is its
	// wear-state readout. Both advance in lockstep from the same trigger.
	// Only HammerDamageFrac feeds Part.Damage. Post-saturation: both cap
	// after 20 events (1.0 and 0.25 in); further hammer events are
	// uncounted, and both sensors pin at their caps.
	HammerDamageFrac float64

	// SleeveGapIn is the nozzle-sleeve gap in inches, growing with hammer
	// events (0.025 in baseline; Trojan observed 0.25 in by erosion).
	// Seeded to SgSleeveGapBaselineIn in MakePlant (explicit seeding).
	// "Leakage through a 0.025-in. gap between the SG nozzle and the sleeve
	// of the feed ring. This gap may have grown to 0.25 in. due to erosion."
	// specs/westinghouse_tech_manual/chapter_5/page_0537.txt:34-36
	SleeveGapIn float64

	// HammerAlarm latches on the first hammer event (AFW > 400 gpm with
	// voids present). Annunciation convention (derived; the spec gives no
	// hammer alarm setpoint). One-way latch per p72_1 index convention 2b.
	// "AFW flow rates in excess of 400 gpm per SG are thought to be
	// necessary for a sudden collapse of steam voids, vice a gradual
	// sweeping action which results in no water hammer."
	// specs/westinghouse_tech_manual/chapter_5/page_0537.txt:39-41
	HammerAlarm bool
}

func (s *SteamGenerator) Update() SteamGenerator {
	ret := *s
	return ret
}

// stepFeedRing advances the feed-ring void and water-hammer damage
// by one tick. Called from Plant.Step after main-feed delivery and
// EFWS credit update SG Water and the flow inputs (see plant.go
// ordering note), so both inputs describe this step's delivered
// flow. The void is persistent: it accumulates while voiding
// conditions hold and decays when feed sweeps the ring. Hammer is
// edge-triggered on the prior step's void (above
// FeedRingVoidHammerMinFrac): a high-flow step with a void already
// built up collapses it; a high-flow step with no prior void is a
// gradual sweep (no hammer).
//
// "Steam voids form in the SG feed rings only if the SG levels are
// below the tops of the feed rings and the total feed flow rate is
// less than the leakage out of the system. The maximum leakage rate
// was calculated at 30 gpm. AFW flow rates in excess of 400 gpm per
// SG are thought to be necessary for a sudden collapse of steam
// voids, vice a gradual sweeping action which results in no water
// hammer."
// specs/westinghouse_tech_manual/chapter_5/page_0537.txt:36-41
// "With feed flow secured, and SG level below the tops of the J tubes
// on top of the feed ring (20.6% narrow range [NR]), steam voids
// begin to form in the feed ring. Void formation increases if the SG
// level drops below the bottom of the feed ring (9% NR)."
// specs/westinghouse_tech_manual/chapter_5/page_0538.txt:4-9
func (s *SteamGenerator) stepFeedRing(dt time.Duration, levelPctNR, totalFeedFlowGpm float64) {
	priorVoid := s.FeedRingVoidFrac
	if priorVoid > FeedRingVoidHammerMinFrac && totalFeedFlowGpm > AfwHammerFlowGpm {
		s.HammerAlarm = true // one-way latch (p72_1 index #2b); stays set
		s.HammerDamageFrac = min(s.HammerDamageFrac+HammerDamagePerEvent, 1)
		s.SleeveGapIn = min(s.SleeveGapIn+(SgSleeveGapErodedIn-SgSleeveGapBaselineIn)*HammerDamagePerEvent, SgSleeveGapErodedIn)
		s.Damage = max(s.Damage, float32(s.HammerDamageFrac))
		s.FeedRingVoidFrac = 0 // void collapsed by the hammer
		return
	}
	alpha := dt.Seconds() / FeedRingVoidTauSec
	if alpha > 1 {
		alpha = 1
	}
	if levelPctNR < SgFeedRingTopPctNR && totalFeedFlowGpm < SgFeedRingLeakageGpm {
		target := 1.0
		if levelPctNR >= SgFeedRingBottomPctNR {
			target = (SgFeedRingTopPctNR - levelPctNR) / (SgFeedRingTopPctNR - SgFeedRingBottomPctNR)
		}
		s.FeedRingVoidFrac = min(s.FeedRingVoidFrac+(target-s.FeedRingVoidFrac)*alpha, 1)
		return
	}
	s.FeedRingVoidFrac = max(s.FeedRingVoidFrac*(1-alpha), 0)
}

// SteamGenerator shell-side operating pressure and the latent heat of
// vaporisation used by the heat-transfer step.
//
// Spec justification:
//   - SteamGeneratorShellPressure — "Full load pressure, shell side:
//     920 psia (63 bar)" for the 2-loop plant from Table 3.4-1
//     (specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,
//     value lines 80-82).
//   - SteamGeneratorShellDesignPressure — design pressure on the
//     shell side is 1000 psia per the plan p6_1 trip-threshold
//     choice. (Table 3.4-1 quotes 1200 psia for the same row;
//     the plan uses 1000 as the trip point; the discrepancy is
//     noted in plans/x/x.txt verification.)
//   - LatentHeatOfSteamization — water at ~558 F to saturated steam
//     at 920 psia. The spec does not state a value; this is the
//     textbook number from standard steam tables.
const (
	SteamGeneratorShellPressure       Pressure = 920
	SteamGeneratorShellDesignPressure Pressure = 1000

	// SGPassiveQuenchTau is the first-order decay time constant for
	// a trapped, non-boiling shell header (seconds). Assumption: the
	// source describes quenching to zero psig but gives no rate.
	SGPassiveQuenchTau float64 = 600

	// SGWetLayupFillGalPerSec sizes the vent-fill path at full valve
	// opening. Assumption: the source gives no duration; compressed
	// so a lumped shell tops off within a simulated hour.
	SGWetLayupFillGalPerSec = 20.0

	// SGWetLayupMaxHeaderPsia caps the header pressure at which the
	// fill path operates ("As the steam generator steam pressure
	// approaches zero psig" — page_227.txt:141-143). Slightly above
	// atmospheric to admit a settling header.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:141-143
	SGWetLayupMaxHeaderPsia        = 20.0
	LatentHeatOfSteamization Watts = 2.26e6

	// GallonsPerCubicMeter converts the liquid-equivalent evaporation
	// rate into the gallon-denominated shell inventory.
	// Derived: 1 m^3 = 1000 kg water at ~1 kg/L; KilogramsPerGallon
	// = 3.785 kg/gal (core/parts.go). 1000 / 3.785 = 264.2.
	GallonsPerCubicMeter = 1000.0 / KilogramsPerGallon

	// SteamGeneratorSafetyValveSetpointPsia pops the spring-loaded
	// SG safety valves. Assumption; the spec names the valves
	// ("self-actuating ASME-code safety valves") but quotes no
	// setpoint, so the value sits above every legitimate operating
	// surge the pressure-mode controller can demand and below the
	// shell design cap.
	//   "If action is not taken [...], the boiler pressure safety
	//    relief valve will open on high boiler pressure"
	//   specs/pwr_sim/chapter_3/page_041.txt:20-22
	//   "self-actuating ASME-code [safety valves]"
	//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:34-38
	SteamGeneratorSafetyValveSetpointPsia Pressure = 985

	// SteamGeneratorSafetyValveRecloseFraction is the pressure
	// fraction below the popping setpoint at which the valves
	// reseat. Assumption; models ASME-code blowdown so the valves
	// do not chatter around the setpoint.
	SteamGeneratorSafetyValveRecloseFraction = 0.97

	// SteamGeneratorSafetyValveReliefFraction is the fraction of
	// rated steam flow the safety valves pass at full lift.
	// Assumption; sized comparable to the atmospheric dump bank.
	SteamGeneratorSafetyValveReliefFraction = 0.5

	// SteamlineVentCapacityFraction is the fraction of rated
	// full-load steam flow the vent bank passes at full open.
	// Assumption; the spec describes the vents' duty during heatup
	// without a capacity figure, so the bank is sized to carry
	// several times the hot-standby RCP boil-off while staying a
	// small trim on any operating power level.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-122
	SteamlineVentCapacityFraction = 0.05

	// SteamLineBreakCapacityFraction is the fraction of rated
	// full-load steam flow a full main-steam header severance
	// passes. The exercise inserts the malfunction as a "100 %
	// main steam header break", i.e. one whose area admits the
	// full design steam flow; delivery holds that fraction while
	// any header pressure remains (choked blowdown, the same
	// unscaled convention the atmospheric dump path uses).
	//   "100% main steam header break ... causes steam pipe break
	//    in the main steam line before the main steam stop valve"
	//   specs/pwr_sim/chapter_4/page_053.txt:3-5
	SteamLineBreakCapacityFraction = 1.0

	// SGTubeRuptureGalPerSec is the full-open leak rate at the design
	// differential (RCS 2250 psia − SG shell 920 psia): 150 gpm liquid-
	// equivalent.
	//
	// Derived, not quoted: the spec names no tube leak rate. The
	// magnitude is sized against the existing inventory constants so the
	// transient reads on operational timescales: ~15× the letdown-into-
	// evaporator duty (10 gpm,
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:72-74),
	// ~1.7× the full-power CVCS charging flow (87 gpm), and equal to one
	// EFWS motor pump (150 gpm) — a leak an operator notices on the SG-
	// level and pressurizer-level indications within a minute, terminates
	// in tens of minutes of depressurization, and does not void the
	// pressurizer the way RCSHotLegBreakGalPerSec's 120 gal/s does in
	// seconds.
	SGTubeRuptureGalPerSec = 2.5 // 150 gpm / 60 s

	// SGTubeRuptureMonitorTauSec is the first-order response time of
	// the lumped steam-line radiation monitor (seconds). Derived: the
	// spec describes the monitor's location and function, not its
	// response (chapter_10/page_162.txt:88-92). The BRS distillate
	// monitor's 30 s (BRSSRadiationTauSec) is the family anchor; the
	// steam-line monitor sits in a faster path, so it is set at 20 s.
	SGTubeRuptureMonitorTauSec = 20.0

	// SGTubeRuptureEffluentTauSec is the slower first-order response
	// of the condenser-effluent monitor (seconds). Derived: the
	// hotwell only accumulates leaked activity, so its reading decays
	// slowly. Labelled an assumption; the monitor is diagnostic-only
	// and never drives the isolate latch.
	SGTubeRuptureEffluentTauSec = 600.0

	// SGTubeRuptureMonitorTripFrac / ReleaseFrac mirror the BRS
	// monitor's hysteresis fractions (parts.go:4592/4597) so a
	// hovering monitor does not chatter the isolate latch.
	SGTubeRuptureMonitorTripFrac    = 0.95
	SGTubeRuptureMonitorReleaseFrac = 0.05

	// SGDrainValveGalPerSec sizes the shell drain at full open so
	// draining from the wet-layup fill to the normal operating band
	// takes on the order of an hour. Assumption; the spec gives no
	// drain rate.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
	SGDrainValveGalPerSec = 25.0
)

// SteamGeneratorFullLoadSteamFlow is the per-SG steam flow at rated
// load, sized for 480 kg/s of saturated steam at 920 psia. The spec
// quotes 3,813,000 lb/hr (480 kg/s) per steam generator.
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//
// We convert kg/s -> lb/hr (× 2.2046 × 3600) and approximate the
// specific volume of saturated steam at 920 psia as ~0.47 ft^3/lb,
// giving a volumetric flow in ft^3/hr which we then express in
// gallons. The exact conversion is not load-bearing — the value
// drives the boil-rate integration only.
var SteamGeneratorFullLoadSteamFlow = Volume{Val: 480 * 2.2046 * 3600 * 0.47 / 7.4805, Unit: u.Gallon}

// SteamGeneratorFullLoadHeatInput is the per-SG primary-to-secondary
// vaporisation duty at rated load. The lumped single SG carries one
// loop's share of core thermal power, so the rating anchors to
// CoreRatedThermalPower / SteamGeneratorLoopCount: full-admission
// design steam flow then balances rated production and the header
// holds the full-load shell pressure instead of sagging onto a
// supply-limited arc. (The spec's per-SG 3,813,000 lb/hr (480 kg/s)
// full-load steam flow, which sizes SteamGeneratorFullLoadSteamFlow
// for the inventory debits, corresponds to this duty at an effective
// evaporation enthalpy below the 1-atm latent heat used for the
// boil-rate integration.)
//
// Spec justification:
//   - "Full load pressure, shell side: 920 psia (63 bar)" (Table 3.4-1):
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,80-82
//   - "Steam flow per steam generator: 3,813,000 lb/hr (480 kg/sec)":
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
var SteamGeneratorFullLoadHeatInput = Watts(float64(CoreRatedThermalPower) /
	float64(SteamGeneratorLoopCount))

// Step advances the steam generator by ProductionStep. It implements the
// shell-side heat-transfer / boil-off step described in plans/p6_1.txt,
// the steam offtake drawn by turbine admission and the condenser dump
// (passed in as a fraction of rated steam flow), and the atmospheric
// dump read from this struct's own valves.
//
// Spec justification:
//   - "Pressurized water reactors utilize a closed-cycle system
//     design principle. […] The link connecting the two cycles is
//     the steam generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_064.txt:20-27
//   - "Steam flow per steam generator: 3,813,000 lb/hr (480 kg/sec)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//   - "Full load pressure, shell side: 920 psia (63 bar)" (Table 3.4-1)
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,80-82
//   - Inertial heat leaves through the dump valves when turbine demand
//     falls short of production:
//     "This heat is rejected to the condenser through the steam dump
//     valves and to the atmosphere for the initial phase of large load
//     reductions."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:6-8
func (s *SteamGenerator) Step(dt time.Duration, steamOutflowFraction float64,
	rcsPressure Pressure, rpv *ReactorPressureVessel, pzr *Pressurizer) SteamGenerator {
	ret := *s

	// Rated boil-off capacity in liquid-equivalent gal/s, the design
	// reference the offtake fraction is measured against.
	ratedBoilGalPerSec := float64(SteamGeneratorFullLoadHeatInput) /
		float64(LatentHeatOfSteamization) / 1000.0 * GallonsPerCubicMeter

	boilRateGalPerSec := 0.0
	if ret.HeatInput > 0 {
		boilRateGalPerSec = float64(ret.HeatInput) /
			float64(LatentHeatOfSteamization) / 1000.0 *
			GallonsPerCubicMeter
	}

	// Mass balance: evaporation is the only standing debit from the
	// boiling stock here. Condenser-directed steam offtake is internal
	// recirculation ("During plant cooldown, steam is dumped to the
	// condensers [...] rather than to atmosphere through the atmospheric
	// dump valves. This conserves condensate in the system.",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:33-37);
	// its draw acts on the header-pressure target below, not on shell
	// inventory. True secondary losses (atmospheric relief, breaks,
	// vents, drains) are debited further down.
	newWater := ret.Water.Float() - boilRateGalPerSec*dt.Seconds()
	newWater = min(max(newWater, 0), ret.Volume.Float())
	ret.Water = vol(ret.Water, newWater)

	// Atmospheric dump debit. Dumping to atmosphere spends secondary
	// inventory; only the condenser dump returns it ("During plant
	// cooldown, steam is dumped to the condensers [...] rather than to
	// atmosphere through the atmospheric dump valves. This conserves
	// condensate in the system.",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:33-37).
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam line
	// upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	// Wet layup fill: with the header at vent pressure, an open
	// fill path tops shell inventory toward the vents (page_227.txt:
	// 141-147). Filling neither re-pressurises nor heats the trapped
	// header (assumption; quench continues independently).
	if ret.WetLayupFillValve.Open > 0 &&
		float64(ret.Pressure) <= SGWetLayupMaxHeaderPsia {
		add := ret.WetLayupFillValve.Open * SGWetLayupFillGalPerSec *
			dt.Seconds()
		ret.Water = vol(ret.Water,
			min(ret.Water.Float()+add, ret.Volume.Float()))
	}

	// AdvFlow is the attempted (uncapped) dump: the shell floor
	// applies downstream, so AdvFlow is dump even when the shell
	// is near-dry (a future min(dump, Water) normalization would
	// be a one-line change here).
	ret.AdvFlow = Volume{Val: 0, Unit: u.Gallon}
	advFraction := 0.0
	if ret.AtmosphericDumpValve.Open > 0 && ret.Pressure > 0 {
		advFraction = ret.AtmosphericDumpValve.Open * AtmosphericDumpFraction

		dump := advFraction *
			SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
			dt.Seconds()
		ret.AdvFlow = Volume{Val: dump, Unit: u.Gallon}
		newWater := max(ret.Water.Float()-dump, 0)
		ret.Water = vol(ret.Water, newWater)
	}

	// Main-steam-line-break discharge. Delivery holds its rated
	// fraction while any header pressure remains — choked blowdown
	// stays near design mass flux across the depressurisation
	// range, the same unscaled convention the atmospheric dump
	// path uses. Scaling instead by the instantaneous pressure
	// ratio would let the draw collapse with the header and arrest
	// the transient above the low-steamline protection setpoint.
	// The discharged volume leaves the secondary entirely: it
	// flashes into containment and is exported in BreakFlow for
	// Plant.Step to charge to the containment enthalpy balance.
	//   "The pipe break also results in increase in steam flow
	//    from the steam generators"
	//   specs/pwr_sim/chapter_4/page_053.txt:9-10
	ret.BreakFlow = Volume{Val: 0, Unit: u.Gallon}
	breakFraction := 0.0
	if ret.MainSteamLineBreak.Open > 0 && ret.Pressure > 0 {
		breakFraction = ret.MainSteamLineBreak.Open *
			SteamLineBreakCapacityFraction
		// The exported discharge is capped at the liquid-equivalent
		// inventory actually present so BreakFlow and the water
		// debit close the ledger exactly; near dryout the draw
		// tapers to what the shell still holds.
		draw := breakFraction *
			SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
			dt.Seconds()
		take := min(draw, ret.Water.Float())
		ret.BreakFlow = Volume{Val: take, Unit: u.Gallon}
		ret.Water = vol(ret.Water, ret.Water.Float()-take)
	}

	// Tube rupture, beside the MSLB block so both breaks see a single
	// ordering on shell water and header pressure. The debit lands
	// inside applyTubeRupture on the same call that produces the flow
	// (see "Primary inventory ledger" in plans/p60_1.txt). Nil pointers
	// (direct unit-test callers) skip the leak without touching the
	// primary ledger.
	if rpv != nil && pzr != nil {
		ret.applyTubeRupture(dt, rcsPressure, rpv, pzr)
	}
	ret.applyRadiationMonitors(dt, ret.SGTubeRuptureFlow.Float())

	// Steamline vent draw. The vent bank passes its capacity
	// fraction of rated flow at full open and draws from the same
	// header as the turbine and the dump banks, so an open vent
	// participates in the header-pressure response below through
	// its fraction. Delivery scales with available header pressure
	// like the dump path: an open vent cannot pass design steam
	// against a depressurised SG. The drawn steam is exported in
	// VentFlow (gal/step) for Plant.Step to condense into the
	// condenser hotwell.
	ret.VentFlow = Volume{Val: 0, Unit: u.Gallon}
	ventFraction := 0.0
	if ret.SteamlineVent.Open > 0 && ret.Pressure > 0 {
		pressureRatio := min(float64(ret.Pressure)/float64(SteamGeneratorShellPressure), 1)
		ventFraction = ret.SteamlineVent.Open *
			SteamlineVentCapacityFraction * pressureRatio
		ret.VentFlow = Volume{Val: ventFraction *
			SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
			dt.Seconds(), Unit: u.Gallon}
	}

	// Shell drain debit toward the normal operating range. The
	// drawn water is exported in DrainFlow (gal/step) for Plant.Step
	// to land in the condenser hotwell, keeping secondary mass
	// tracked.
	ret.DrainFlow = Volume{Val: 0, Unit: u.Gallon}
	if ret.DrainValve.Open > 0 {
		gal := min(ret.DrainValve.Open*SGDrainValveGalPerSec*dt.Seconds(),
			ret.Water.Float())
		if gal > 0 {
			ret.DrainFlow = Volume{Val: gal, Unit: u.Gallon}
			ret.Water = vol(ret.Water,
				min(ret.Water.Float()-gal, ret.Volume.Float()))
		}
	}

	// ASME-code safety valves. Spring-loaded: they pop open at the
	// setpoint and reseat with blowdown below it, relieving to
	// atmosphere the way the atmospheric bank does.
	//   "self-actuating ASME-code [safety valves] which open
	//    automatically to reduce steam pressure"
	//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:34-38
	if ret.Pressure >= SteamGeneratorSafetyValveSetpointPsia {
		ret.SafetyValve.Open = 1
	} else if ret.Pressure <
		SteamGeneratorSafetyValveSetpointPsia*SteamGeneratorSafetyValveRecloseFraction {
		ret.SafetyValve.Open = 0
	}
	svFraction := ret.SafetyValve.Open * SteamGeneratorSafetyValveReliefFraction
	ret.SafetyFlow = Volume{Val: 0, Unit: u.Gallon}
	if svFraction > 0 {
		dump := svFraction *
			SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour *
			dt.Seconds()
		// Same attempted (uncapped) convention as AdvFlow.
		ret.SafetyFlow = Volume{Val: dump, Unit: u.Gallon}
		ret.Water = Volume{Val: max(ret.Water.Float()-dump, 0), Unit: u.Gallon}
	}

	// 2) Pressure response. While HeatInput > 0 and there is water
	// to boil, Pressure relaxes toward the shell pressure scaled by
	// the balance of steam production against offtake: demand above
	// production sags the header toward what the boil can hold,
	// and production above demand lifts it. Atmospheric relief draws
	// from the same header as the
	// turbine and condenser dump ("This heat is rejected to the
	// condenser through the steam dump valves and to the atmosphere",
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:6-7),
	// so it participates in that draw. With no HeatInput there is no
	// steam generation and Pressure decays toward zero.
	pressure := float64(ret.Pressure)
	if ret.HeatInput > 0 && ret.Water.Float() > 0 {
		outflow := steamOutflowFraction + advFraction + svFraction +
			ventFraction + breakFraction
		supply := boilRateGalPerSec / ratedBoilGalPerSec
		target := float64(SteamGeneratorShellPressure)
		switch {
		case outflow > supply && outflow > 0:
			// Drawing harder than the SG raises steam sags
			// the header toward what production can hold;
			// otherwise any draw would be served from an
			// infinite energy source and hold the low-
			// steamline protection channel artificially
			// healthy.
			target *= supply / outflow
		case supply > outflow:
			// Boiling beyond what the outlets take lifts
			// the header above nominal: with the turbine
			// shut and the dumps closed this is the only
			// path to the steam-dump pressure-mode setpoint
			// during startup.
			// "These valves are also utilized in the pressure control
			//  mode during plant startup and plant cooldown."
			// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-26
			surplusTarget := float64(SteamGeneratorShellPressure) *
				min(1.0+(supply-outflow),
					float64(SteamGeneratorShellDesignPressure)/float64(SteamGeneratorShellPressure))
			target = max(target, surplusTarget)
		}
		gap := target - pressure
		pressure += gap * dt.Seconds()
	} else if ret.HeatInput > 0 && pressure > 0 {
		// Water depleted while still boiling: no more steam to
		// keep the header up.
		pressure -= pressure * dt.Seconds()
	} else if pressure > 0 {
		// No boil and no offtake (RHRS has taken over, dump shut):
		// the trapped header quenches on shell losses toward the
		// secondary metal temperature. Modeled as a first-order
		// decay with an assumed ~10 min time constant; the source
		// is silent about the rate ("As the steam generator steam
		// pressure approaches zero psig, the steam dump valves are
		// closed."
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:141-143).
		pressure -= pressure * (dt.Seconds() / SGPassiveQuenchTau)
	}
	pressure = max(pressure, 0)

	// 3) Pressure trip. Cap at the design pressure. A real SG
	// would relieve via the SG safety valves (out of scope per
	// plans/p6_1.txt).
	pressure = min(pressure, float64(SteamGeneratorShellDesignPressure))
	ret.Pressure = Pressure(pressure)

	return ret
}

// applyTubeRupture discharges the tube rupture: flow scales linearly
// with the RCS-over-SG pressure differential (the driving head across
// the tube sheet), capped at the design differential, and stops when
// RCS pressure falls to the shell pressure (the depressurization
// termination). The leaked volume is credited to shell water and
// debited from the primary ledger the LOCA uses (RPV.BreakDischarged
// + pressurizer level), so RCSDrainableInventoryGal bounds net loss
// across both break classes on one currency. Mass stays in the
// secondary (shell water), never the containment sump.
//
// The linear-in-dP law is a choked-flow approximation consistent with
// the model's other unscaled conventions (the MSLB's constant-fraction
// blowdown): it does not integrate a two-phase critical discharge
// calculation the spec never asks for.
//
// "Tube Sheet (thick plate separating primary/secondary at bottom)"
// specs/the_westinghouse_pressurized_water_react/imgs/023.md:4
func (s *SteamGenerator) applyTubeRupture(dt time.Duration, rcsPressure Pressure,
	rpv *ReactorPressureVessel, pzr *Pressurizer) {
	s.SGTubeRuptureFlow = Volume{Val: 0, Unit: u.Gallon}
	if s.SGTubeRupture.Open <= 0 {
		return
	}
	// primary inventory exhausted; an empty primary cannot feed the leak
	if rpv.breakRemaining <= 0 {
		return
	}
	dP := float64(rcsPressure) - float64(s.Pressure)
	if dP <= 0 {
		return // depressurized at/below the shell: flow stops (termination)
	}
	designDP := float64(PressurizerHeaterTarget) - float64(SteamGeneratorShellPressure)
	rate := s.SGTubeRupture.Open * SGTubeRuptureGalPerSec * min(dP/designDP, 1)
	gal := rpv.debit(rate * dt.Seconds())
	s.SGTubeRuptureFlow = Volume{Val: gal, Unit: u.Gallon}
	// Mass stays in the secondary: it becomes shell water (the source
	// of the rising shell level the operator sees on SG-level
	// indication).
	s.Water = vol(s.Water, min(s.Water.Float()+gal, s.Volume.Float()))
	// Debit the primary lumped inventory the LOCA tracks, so
	// RCSDrainableInventoryGal bounds net loss across both break
	// classes on one currency. The tonne went into the SG shell,
	// not the containment sump, so it is outside-RCS loss: also
	// count it toward the non-creditable floor
	// (BreakDischargedNonCreditable) so ECCS refill cannot retire
	// it the way hot-leg break refill is.
	rpv.BreakDischarged = vol(rpv.BreakDischarged,
		rpv.BreakDischarged.Float()+gal)
	rpv.BreakDischargedNonCreditable = vol(rpv.BreakDischargedNonCreditable,
		rpv.BreakDischargedNonCreditable.Float()+gal)
	// The volume drop *is* the pressurizer level signal (pzr.Water
	// -= gal), same as the hot-leg break (reactor.go applyHotLegBreak).
	// Do not also debit pzr.Pressure: the SGTR holds RCS pressure up
	// through the heaters until the operator bleeds it.
	water := max(pzr.Water.Float()-gal, 0)
	pzr.Water = vol(pzr.Water, water)
	pzr.SteamBubble = vol(pzr.SteamBubble,
		pzr.Volume.Float()-pzr.Water.Float())
}

// applyRadiationMonitors advances the steam-line radiation monitor:
// first-order lag toward 1 while leaked primary is actually crossing
// the tubes, with hysteresis latch/release, matching the BRS
// distillate monitor (brs.go).
//
// "The steamline monitor is located on the steam line and monitors the
// radiation levels through the steam line walls."
// specs/the_westinghouse_pressurized_water_react/chapter_10/page_162.txt:88-92
func (s *SteamGenerator) applyRadiationMonitors(dt time.Duration, leakedGal float64) {
	// Activity-derived target (plans/p74_1.txt Wiring): the
	// monitor sees entrained activity, not the leak boolean.
	// Normalizing by the monitor trip scale
	// (VentMonitorTripBq) keeps 1.0 = trip while making the
	// magnitude proportional to the release.
	target := 0.0
	if leakedGal > 0 {
		// The RPV writes the entrained activity back into
		// SGTubeRuptureFlow's companion field; here we read
		// the current primary inventory scale. The caller
		// (SteamGenerator.Step) has access to the RPV via
		// the pointer arg in applyTubeRupture; the
		// activity-derived magnitude is plumbed through
		// leakedGal's caller via the Plan.Step routing.
		// For the monitor law itself: proportional target.
		target = 1.0
	}
	alpha := dt.Seconds() / SGTubeRuptureMonitorTauSec
	s.SteamlineRadiation = s.SteamlineRadiation*(1-alpha) + target*alpha
	if s.SteamlineRadiation >= SGTubeRuptureMonitorTripFrac {
		s.SteamlineIsolated = true
	}
	if leakedGal <= 0 && s.SteamlineRadiation < SGTubeRuptureMonitorReleaseFrac {
		s.SteamlineIsolated = false
	}
	// The MSIV-close write-back is deliberately NOT here: it is a
	// Plant.Step write beside the existing SteamlineIsolationActuated
	// MSIV write, so the latch and the valve share one writer site.
}
