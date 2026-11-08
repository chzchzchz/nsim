package core

import (
	"math"
	"time"
)

type Outside struct {
	// CondensateStorageTank sizing is from the Westinghouse
	// Technical Manual (not the Westinghouse PWR document):
	// specs/westinghouse_tech_manual/chapter_5/page_0532.txt:30-41:
	// total capacity 450,000 gal; Tech Specs require 239,000 gal
	// operable, which accounts for 27,700 gal minimum vortexing
	// level and 14,400 gal instrument error, leaving 196,000 gal
	// usable; maintains hot standby 2 hours then cooldown to
	// 350°F over 4 hours at 880 gpm AFW flow.
	CondensateStorageTank Tank

	TurbineAFWPump AFWPump
	DieselAFWPump  AFWPump
	MotorAFWPump   AFWPump

	// The pump recirculation line is a 2 inch pipe
	CirculatingWaterPump Pump

	// CirculatingWaterSystem tracks the CW flow delivered to
	// the main condenser (plans/p31_1.txt). InletTemp and
	// FlowGPM flow downstream into Condenser.Step.
	// "The Circulating Water System serves as the major heat
	// sink for the power plant by providing large amounts of
	// cooling water for the main condensers."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
	CirculatingWaterSystem CirculatingWaterSystem

	// CoolingTower models the mechanical-draft cooling tower
	// as an alternate CW source with its own basin inventory
	// and cold-basin temperature. The CWS draws suction from
	// the basin when SourceIsCoolingTower is set.
	// "Depending on the plant, the source of circulating
	// water may be a river, lake, ocean, or cooling tower."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
	CoolingTower `json:"CoolingTower"`

	// AmbientWetBulbTemp drives cooling-tower evaporative
	// performance. The wet-bulb is the evaporative lower
	// bound: "effectively the lowest temperature that may be
	// achieved by evaporative cooling".
	// specs/wiki/Wet-bulb_temperature.txt:775
	AmbientWetBulbTemp Temperature

	// ServiceWaterSystem models the SWS — the ultimate heat
	// sink for the plant. The pumps live in a separate
	// intake house per
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:48-50.
	//
	// Spec justification:
	//   - "The Service Water System supplies all the cooling
	//     water for the plant, including that required for
	//     emergency shutdown. Consequently, high reliability
	//     and redundancy is required."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
	//   - "The pumps are frequently located in a separate
	//     intake house."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:48-50
	//   - "The three essential service water pumps for each
	//     unit are typically rated at approximately 20,000
	//     gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	ServiceWaterSystem ServiceWaterSystem

	RiverTemp Temperature

	// GasHoldupActivity is the activity accumulated in the
	// Waste Gas Processing System surge/holdup volume
	// (normalized units, same scale as PrimaryActivityFrac).
	// It decays with the Xe-133 half-life and is vented to the
	// plant vent after the charcoal delay. plans/p74_1.txt.
	GasHoldupActivity float64

	// LiquidHoldupActivity is the activity in the liquid
	// waste holdup tanks (normalized units). Discharge to the
	// river is monitor-gated. plans/p74_1.txt.
	LiquidHoldupActivity float64

	// GasReleasedBq / LiquidReleasedBq are the cumulative
	// activity discharged via the plant vent / monitored
	// liquid discharge to the river (Bq). One-way
	// accumulators, never decremented. plans/p74_1.txt.
	GasReleasedBq    float64
	LiquidReleasedBq float64

	// PlantVentActivityBq / LiquidMonitorBq are the
	// instantaneous monitor readings (Bq) for the
	// operator-alert criterion. plans/p74_1.txt.
	PlantVentActivityBq float64
	LiquidMonitorBq     float64

	// pendingGasInNorm / pendingLiquidInNorm are the
	// next-tick pathway-routing inputs (normalized units).
	// Plant.Step writes them after ReactorBuilding.Step;
	// Outside.Step reads them on the following tick.
	// plans/p74_1.txt Wiring.
	pendingGasInNorm    float64
	pendingLiquidInNorm float64
}

func (o *Outside) Step(dt time.Duration, electricalDriveReady bool,
	condenserOutlet Temperature, gasInNorm, liquidInNorm float64) Outside {
	ret := *o
	// Circulating Water System step. The CW pump count is
	// collapsed to the spec's "one per half-condenser"
	// architecture when the pump is running; both pumps are
	// assumed online together so the per-step flow matches
	// CirculatingWaterDesignFlowGPM.
	cwRunning := o.CirculatingWaterPump.Running
	cwPumps := 0
	if cwRunning && electricalDriveReady {
		cwPumps = CirculatingWaterPumpCount
	}
	// The tower steps first so the CWS reads this tick's
	// basin level and cold-basin temperature; it uses last
	// tick's circulating flow (a 1 ms lag).
	ret.CoolingTower = o.CoolingTower.Step(dt,
		cwRunning,
		electricalDriveReady,
		o.CirculatingWaterSystem.FlowGPM,
		condenserOutlet,
		o.AmbientWetBulbTemp,
	)
	ret.CirculatingWaterSystem = o.CirculatingWaterSystem.Step(dt,
		cwRunning,
		electricalDriveReady,
		cwPumps,
		o.RiverTemp,
		&o.CoolingTower,
	)
	ret.applyWasteChain(dt, gasInNorm, liquidInNorm)
	return ret
}

// applyWasteChain advances the Chapter 14 waste-processing
// chain (plans/p74_1.txt Step 3). Units: gasInNorm/liquidInNorm
// arrive in normalized (rated-equilibrium) units; they are
// converted to Bq here by
// PrimaryActivityFullPowerActivityScaleBqPerGal so the
// boundary is normalized-in / Bq-out and every downstream
// consumer sees Bq.
//
// Gaseous: holdup decays (Xe-133 half-life); the vent stream
// is the holdup delayed by the charcoal delay as a
// first-order lag (tau = CharcoalDelayXe133Days scaled by
// WasteGasDryerEfficiency, since wet charcoal loses the
// delay), not a bookkeeping queue. H2 carrier gas is not
// delayed (chapter_14/page_190.txt:122-124); only the
// noble-gas fraction is.
// Liquid: holdup decays (tritium half-life dominates,
// external 12.3 y); discharge is monitor-gated
// (chapter_10/page_162.txt:76-82), mirroring the BRS
// auto-divert (core/brs.go:118-123). Both released streams
// accumulate one-way into GasReleasedBq / LiquidReleasedBq.
func (o *Outside) applyWasteChain(dt time.Duration, gasInNorm, liquidInNorm float64) {
	h := dt.Seconds()
	gasIn := gasInNorm * PrimaryActivityFullPowerActivityScaleBqPerGal
	liquidIn := liquidInNorm * PrimaryActivityFullPowerActivityScaleBqPerGal

	// Gaseous holdup: add inflow, decay by Xe-133 half-life.
	o.GasHoldupActivity += gasIn
	lambdaXe := math.Ln2 / (Xe133HalfLifeDays * 86400.0)
	o.GasHoldupActivity *= math.Exp(-lambdaXe * h)

	// Vent stream: first-order lag with tau = charcoal delay
	// scaled by dryer efficiency. Tests use CharcoalDelayTestDays
	// via override (see below); production uses the spec 60 d.
	tauDays := CharcoalDelayXe133Days * WasteGasDryerEfficiency
	if testOverrideCharcoalDelayDays > 0 {
		tauDays = testOverrideCharcoalDelayDays * WasteGasDryerEfficiency
	}
	tauSec := tauDays * 86400.0
	if tauSec > 0 && h > 0 {
		vented := o.GasHoldupActivity * (1 - math.Exp(-h/tauSec))
		o.GasHoldupActivity -= vented
		o.GasReleasedBq += vented
		o.PlantVentActivityBq = vented / h
	} else {
		o.PlantVentActivityBq = 0
	}

	// Liquid holdup: add inflow, decay by tritium half-life.
	o.LiquidHoldupActivity += liquidIn
	lambdaT := math.Ln2 / (TritiumHalfLifeDays * 86400.0)
	o.LiquidHoldupActivity *= math.Exp(-lambdaT * h)

	// Monitor-gated discharge: one-tick lag on the monitor
	// reading. Above trip: hold. Below release: discharge
	// at the holdup drain rate. Mirrors the BRS auto-divert
	// hysteresis (core/brs.go:118-123). The monitor reads
	// the holdup concentration (Bq per unit holdup volume),
	// normalized so 1.0 = trip (LiquidMonitorTripBq); the
	// stored value is Bq directly for the operator alert.
	monitor := o.LiquidHoldupActivity
	o.LiquidMonitorBq = monitor
	discharge := 0.0
	if monitor < LiquidMonitorTripBq {
		// Drain 1% of holdup per second when the monitor is
		// below trip (a derived drain rate; the spec names
		// the monitor, not the drain rate).
		discharge = o.LiquidHoldupActivity * 0.01 * h
		o.LiquidHoldupActivity -= discharge
		o.LiquidReleasedBq += discharge
	}
}

// routeContamination routes entrained primary activity per
// Paths A/B/C (plans/p74_1.txt Step 2). It mutates the
// ContainmentBuilding activity fields in place and returns
// the normalized gas/liquid inflows for Outside.Step (next
// tick's pending inputs).
//
// Path A (tube rupture): entrained activity in the secondary
// header. Destination depends on the secondary path:
// AtmosphericDumpValve / SafetyValve / MainSteamLineBreak
// -> outside (accident bypass, no charcoal delay);
// SteamlineVent -> condenser hotwell (internal); turbine
// admission -> hotwell. Verified: VentFlow + DrainFlow are
// credited to the hotwell at core/plant.go:2153-2156, so
// secondary mass does reach the condenser. When
// SteamlineIsolated is latched the turbine path is secured
// and the only remaining secondary sink is the dump/relief
// path (outside).
// Path B (relief / hot-leg break): entrained activity enters
// the containment atmosphere. CSS running entrains iodine to
// the sump at CSSIodineScrubRatePerSec scaled by
// SprayFlow/CSSDesignFlow; the remainder is subject to CIS
// leakage (EngineeredLeakageFracPerDay isolated,
// OpenPenetrationLeakageFracPerDay open). Sump activity
// returns to the RCS by long-term recirculation.
// Path C (containment failure): only on ContainmentFailed,
// remaining airborne activity drains at
// OpenPenetrationLeakageFracPerDay (a measurable transient,
// not a delta function) and bypasses charcoal delay.
func routeContamination(
	rb *ReactorBuilding,
	css *ContainmentSpraySystem,
	nobleEntrained, iodineEntrained float64,
) (gasInNorm, liquidInNorm float64) {
	cb := &rb.ContainmentBuilding
	sg := &rb.SteamGenerator
	rpv := &rb.ReactorPressureVessel

	totalEntrained := nobleEntrained + iodineEntrained
	// Fraction of entrained activity that is Path B vs A is
	// proportional to the volume split (BreakFlow vs
	// SGTubeRuptureFlow) — the lumped model cannot track
	// per-path chemistry, so volume proportionality is the
	// honest choice.
	volTotal := rpv.BreakFlow.Float() + sg.SGTubeRuptureFlow.Float()
	pathBFrac := 0.0
	if volTotal > 0 {
		pathBFrac = rpv.BreakFlow.Float() / volTotal
	}
	pathBActivity := totalEntrained * pathBFrac
	cb.ContainmentAirActivity += pathBActivity

	// CSS iodine scrub to the sump (chapter_6/page_115.txt:
	// 12-16). Rate scales with SprayFlow.
	if css != nil && css.Running && css.SprayHeaderOpen {
		scrubFrac := CSSIodineScrubRatePerSec *
			(css.SprayFlow / (CSSDesignFlow / 60.0))
		scrubFrac = min(max(scrubFrac, 0), 1)
		scrubbed := cb.ContainmentAirActivity * scrubFrac
		cb.ContainmentAirActivity -= scrubbed
		cb.ContainmentSumpActivity += scrubbed
	}

	// Sump recirculation return to the RCS: a fraction of
	// sump activity is carried back by long-term
	// recirculation. Derived rate: half-life-equivalent
	// 1/day when recirculating (SuctionSump true).
	if css != nil && css.SuctionSump {
		ret := cb.ContainmentSumpActivity * 0.01
		cb.ContainmentSumpActivity -= ret
			// The returned activity re-enters the primary
		// inventory (plans/p74_1.txt State).
		rpv.PrimaryActivityFrac += ret
	}

	// Leakage past the CIS.
	isolated := cb.ContainmentIsolationSystem.PhaseAIsolated &&
		cb.ContainmentIsolationSystem.PhaseBIsolated
	leakFracPerDay := OpenPenetrationLeakageFracPerDay
	if isolated {
		leakFracPerDay = EngineeredLeakageFracPerDay
	}
	leakPerSec := leakFracPerDay / 86400.0
	leaked := cb.ContainmentAirActivity * leakPerSec
	cb.ContainmentAirActivity -= leaked

	// Path C: containment failure drains the remainder at
	// the open-penetration rate.
	if cb.ContainmentFailed {
		pathC := cb.ContainmentAirActivity * OpenPenetrationLeakageFracPerDay / 86400.0
		cb.ContainmentAirActivity -= pathC
		leaked += pathC
	}

	// Path A: secondary-side entrained activity.
	pathAActivity := totalEntrained * (1 - pathBFrac)
	accidentBypass := 0.0
	if sg.AtmosphericDumpValve.Open > 0 ||
		sg.SafetyValve.Open > 0 ||
		sg.MainSteamLineBreak.Open > 0 {
		accidentBypass = pathAActivity
	} else if sg.SteamlineIsolated {
		accidentBypass = pathAActivity * 0.5
	}
	// Normal secondary path (vent to hotwell) is internal:
	// no outside inflow.
	gasInNorm = leaked + accidentBypass
	liquidInNorm = 0
	// Tritium (liquid class) is the noble-gas fraction
	// already counted; liquidInNorm carries the tritium
	// split. With one lumped inventory the split is
	// proportional: 10% of entrained activity is the liquid
	// (tritium) class (derived; the spec names tritium
	// segregation but no fraction).
	liquidInNorm = pathBActivity * 0.10
	gasInNorm -= liquidInNorm
	if gasInNorm < 0 {
		gasInNorm = 0
	}
	return gasInNorm, liquidInNorm
}
