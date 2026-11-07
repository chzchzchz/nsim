package core

import (
	"encoding/json"
	"io"
	"math"
	"time"

	u "github.com/bcicen/go-units"
)

// secondaryExcessDrawW returns the heat pulled out of the primary
// through sinks other than the RPV's own program relaxation while the
// secondary carries cooldown duty: the steam-generator enthalpy draw
// in excess of supplied heat. The caller feeds it back into
// ReactorPressureVessel.Step so exactly one writer integrates the
// primary energy balance. Once RHRS has taken over, the takeover
// branch below owns the outlet evolution from its pre-step value and
// this must report zero so the two writers do not double-charge the
// balance.
//
// The offtake fraction is the turbine+condenser-dump draw computed by
// the previous turbine step; the atmospheric valves draw from the
// same header and are read directly.
//
// The return splits the rectified imbalance into its two regimes:
// paced (turbine offtake + atmospheric dump), which the RPV integrates
// under the operational rate envelope ("Steam dumps are adjusted to
// start a 50°F/hr (28°C/hr) cooldown rate."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:73-74,
// re-adjusted as the header falls at page_227.txt:124-127), and fault
// (main-steam-line break discharge, plans/p34_1.txt), which the RPV
// integrates against physics alone. With no break open the fault share
// is zero and every planned-evolution trajectory is unchanged.
func (r *ReactorBuilding) secondaryExcessDrawW(offtakeFraction float64, turbineAdmitting bool, op PlantOperation, dt time.Duration) (Watts, Watts) {
	if r.Aux != nil && r.Aux.ResidualHeatRemovalSystem.HasTakenOver {
		return 0, 0
	}
	// With the turbine admitted, draw and boiler duty reconcile
	// through header-pressure regulation and governor demand; their
	// rated-normalizations need not agree digit-for-digit, so a
	// residual there is bookkeeping, not primary cooling. The
	// enthalpy-deficit semantics apply only to the unloaded plant
	// outside at-power operation: a planned cooldown holds steam dump
	// against a subcritical core, while a trip stepback inside power
	// operation reconciles draw through header-pressure regulation
	// without spending primary stored heat.
	if turbineAdmitting || op.Mode == PowerOperation {
		return 0, 0
	}
	advFrac := r.SteamGenerator.AtmosphericDumpValve.Open * AtmosphericDumpFraction
	// The break path draws from the same header with the same
	// unscaled choked-blowdown convention SteamGenerator.Step uses,
	// so the deficit here matches the discharge there.
	//   "The pipe break also results in increase in steam flow from
	//    the steam generators, leading to increase in heat removal
	//    from the reactor coolant system."
	//   specs/pwr_sim/chapter_4/page_053.txt:9-11
	brkFrac := 0.0
	if r.SteamGenerator.MainSteamLineBreak.Open > 0 && r.SteamGenerator.Pressure > 0 {
		brkFrac = r.SteamGenerator.MainSteamLineBreak.Open *
			SteamLineBreakCapacityFraction
	}
	drawnW := (offtakeFraction + advFrac + brkFrac) * float64(SteamGeneratorFullLoadHeatInput)
	// Signed imbalance through a first-order filter before
	// rectification: one-step telemetry lag and rod-control noise
	// are zero-mean and must not ratchet into phantom primary
	// cooldown, while a sustained dump/RHRS-style deficit passes.
	alpha := dt.Seconds() / SecondaryImbalanceTau
	filt := float64(r.SecondaryImbalanceFiltered)*(1-alpha) +
		(drawnW-float64(r.SteamGenerator.HeatInput))*alpha
	r.SecondaryImbalanceFiltered = Watts(filt)
	net := max(0, filt)
	if drawnW <= 0 {
		return 0, 0
	}
	pacedW := (offtakeFraction + advFrac) * float64(SteamGeneratorFullLoadHeatInput)
	faultW := brkFrac * float64(SteamGeneratorFullLoadHeatInput)
	return Watts(net * pacedW / drawnW), Watts(net * faultW / drawnW)
}

func (r *ReactorBuilding) Step(dt time.Duration,
	sgSteamOutflowFraction float64,
	turbineLoadFraction float64,
	op PlantOperation,
	turbineAdmitting bool,
	turbineTripped bool,
) ReactorBuilding {
	ret := *r
	var reliefDischarge Volume
	// Normal spray authority scales with reactor coolant pump flow:
	// "To provide spray flow a RCP must be operating in one of the
	// two loops with spray flow connections." Auxiliary spray
	// arrives from the throttled CVCS charging path, which is how
	// pressure control survives securing the pumps
	// (page_227.txt:96-106). The drive reads last step's CVCS
	// outputs; the one-step lag matches the telemetry convention
	// used for the secondary imbalance.
	sprayDrive := float64(ret.ReactorCoolantPump.Velocity) /
		float64(RCPRatedSpeed)
	ret.Pressurizer, reliefDischarge = ret.Pressurizer.Step(dt,
		ret.ReactorPressureVessel.Outlet, &ret.PORV, &ret.SafetyValve, &ret.BlockValve,
		sprayDrive, ret.Aux.ChemicalVolumeControlSystem.AuxSprayDrive)
	// Hot-leg LOCA discharge: the break debits primary inventory
	// after the pressurizer's own balance so the blowdown lands on
	// top of this step's heater/spray/relief state.
	// "For large pipe ruptures, the RCS would be rapidly
	//  depressurized and voided of coolant."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
	ret.ReactorPressureVessel.applyHotLegBreak(dt, &ret.Pressurizer,
		ret.ContainmentBuilding.Pressure)
	// Route the relieved steam-equivalent volume into the PRT
	// until it fills; a full tank latches the rupture-disc burst
	// and further relief vents to containment instead.
	// "Steam from the safety and/or relief valves is piped to the
	// pressurizer relief tank which contains sufficient water to
	// condense the steam. [...] A rupture disc vents the tank to
	// the containment if design pressure is exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-38

	// Reactor makeup control valve: the operator sets Open
	// (0..1), or engages Auto to let a VCT low-level signal
	// initiate makeup; Flow is the effective rate actually
	// delivered, clamped against the RWST inventory that sources
	// it. Derived once per step so downstream consumers see a
	// stable value.
	//
	// Spec: "Low-level signals from the volume control tank initiate
	// reactor makeup control or flow from the refueling water
	// storage tank as a backup."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
	// The initiation latch reads last step's VCT level (the CVCS
	// steps later this timestep); the one-step lag matches the
	// telemetry convention used elsewhere in this step.
	mv := &ret.ReactorMakeupControlValve
	mv.applyAutoMakeup(ret.Aux.ChemicalVolumeControlSystem.VCTLevel.Float())
	makeupDemand := mv.effectiveOpen() *
		ReactorMakeupDesignFlow.Float()
	rwstLevel := ret.SafetyInjectionSystem.RWSTLevel.Float()
	makeupRate := min(makeupDemand, rwstLevel/dt.Seconds())
	ret.ReactorMakeupControlValve.Flow = Volume{Val: makeupRate, Unit: u.Gallon}
	newRWST := max(rwstLevel-makeupRate*dt.Seconds(), 0)
	ret.SafetyInjectionSystem.RWSTLevel = Volume{Val: newRWST, Unit: u.Gallon}
	// CVCS: charging, letdown, VCT level, and boron
	// concentration. The CVCS boron's reactivity contribution
	// is read by the RPV step below, so CVCS must run first.
	//
	// Spec: "The pressurizer level is controlled by balancing
	// the charging flow to the letdown flow."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
	// Spec: "The operating mode (boration or dilution) and the
	// magnitude of the boron concentration change required is
	// determined by comparison of the measured Tc (cold leg
	// temperature) with the programmed (function of power) Tc."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
	// Tavg and the measured pressurizer level fraction feed the
	// CVCS automatic level controller; the program "varies with
	// RCS average coolant temperature."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:36-40
	// Automatic boron trim: when engaged, rod-bank travel position
	// and the axial offset select the BTRS mode so chemistry relieves
	// what the rods no longer can and damps axial-flux excursions.
	// The demanded-minus-achieved power error adds the pacing tier
	// while the RPC cascade stands engaged; in turbine lead the demand
	// is the turbine's own setpoint - the walked integrator shadows
	// achieved power there and would zero the tier. Runs before CVCS.Step
	// so the selected mode's rates apply this step.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_129.txt:112-115
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:120-123
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-65
	powerError := 0.0
	if op.RodControlAuto {
		powerError = op.boronDemandPower(turbineLoadFraction) -
			float64(ret.ReactorPressureVessel.FissionPower)/
				float64(CoreFissionPowerAtRated)
	}
	if ret.Aux.ChemicalVolumeControlSystem.AutoBoronControl {
		ret.Aux.ChemicalVolumeControlSystem.applyAutoBoronControl(
			ret.ReactorPressureVessel.ControlRods.Insertion,
			ret.ReactorPressureVessel.AxialOffset,
			powerError)
	}
	tavg := Temperature{Val: (ret.ReactorPressureVessel.Inlet.Float() +
		ret.ReactorPressureVessel.Outlet.Float()) / 2, Unit: u.Fahrenheit}
	pzrFraction := 0.0
	if pzrVolume := ret.Pressurizer.Volume.Float(); pzrVolume > 0 {
		pzrFraction = ret.Pressurizer.Water.Float() / pzrVolume
	}
	// Automatic stepback boron channel: while latched, CVCS dilution
	// carries the post-stepback xenon buildup so rod margin is never
	// the binding limit (load-follow compensation duty,
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:24-27).
	if op.StepbackActive && op.StepbackCVCSDilute {
		ret.Aux.ChemicalVolumeControlSystem.Mode = CVCSDilution
		ret.Aux.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
	}
	ret.Aux.ChemicalVolumeControlSystem = ret.Aux.ChemicalVolumeControlSystem.Step(dt,
		ret.ReactorMakeupControlValve.Flow,
		tavg,
		pzrFraction,
		ret.Pressurizer.Pressure,
	)
	// Propagate the CVCS boron concentration into the
	// reactivity model. The RPV step reads
	// ControlRods.BoronConcentration to compute the boron
	// reactivity contribution.
	//
	// Spec: "the boron concentration changes significantly
	// with fuel depletion."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
	ret.ReactorPressureVessel.ControlRods.BoronConcentration =
		ret.Aux.ChemicalVolumeControlSystem.BoronConcentration

	// Charging/letdown couple to pressurizer volume so the CVCS is
	// a real volume circuit rather than a display rate.
	//
	// "pressurizer level represents a balance of injection water
	// flow into the RCS and water letdown flow into the Chemical
	// and Volume Control System."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
	cvcs := ret.Aux.ChemicalVolumeControlSystem
	netCVCS := (cvcs.ChargingFlow.Float() - cvcs.LetdownFlow.Float()) *
		dt.Seconds()
	pzWater := ret.Pressurizer.Water.Float() + netCVCS
	pzWater = min(max(pzWater, 0), ret.Pressurizer.Volume.Float())
	ret.Pressurizer.Water = vol(ret.Pressurizer.Water, pzWater)

	// Reactor Protection System step. Runs before the
	// ReactorPressureVessel step so the ReactorTripped latch
	// is available when RPV drops rods this step. The RPS
	// evaluates trip conditions from the current process
	// variables (steamline pressure, pressurizer pressure,
	// SG level, Tcold, containment pressure, forced-coolant
	// flow) and computes ESFAS outputs.
	//
	// Spec justification:
	//   - 2/4 voting: "A 2/4 logic means that a trip occurs
	//     when at least 2 signals out of 4 indicate a trip
	//     condition."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:119-121
	//   - Turbine trip interlock: "The RPS initiates a
	//     turbine trip signal whenever reactor trip occurs."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:10-11
	sgLevelPct := 0.0
	if ret.SteamGenerator.Volume.Float() > 0 {
		sgLevelPct = 100.0 * ret.SteamGenerator.Water.Float() /
			ret.SteamGenerator.Volume.Float()
	}
	pzrLevelPct := 0.0
	if pzrVolume := ret.Pressurizer.Volume.Float(); pzrVolume > 0 {
		pzrLevelPct = 100.0 * ret.Pressurizer.Water.Float() / pzrVolume
	}
	containmentGauge := ret.ContainmentBuilding.Pressure -
		AtmosphericPressure
	ret.ReactorProtectionSystem = ret.ReactorProtectionSystem.Step(dt,
		ret.SteamGenerator.Pressure,
		ret.Pressurizer.Pressure,
		sgLevelPct,
		ret.ReactorPressureVessel.Inlet.Float(),
		containmentGauge,
		// Neutron-flux channel: fission power only, unclamped. The
		// flux trip reads the fission rate, and the total-power
		// helper clamps at rated, which would mask every overpower.
		float64(ret.ReactorPressureVessel.FissionPower)/
			float64(CoreRatedThermalPower),
		float64(ret.ReactorCoolantPumps.Velocity())/float64(RCPRatedSpeed),
		pzrLevelPct,
		turbineTripped,
	)
	// Propagate the reactor trip to the reactor pressure
	// vessel so the RPV step drops rods and follows the
	// decay-heat curve this same step.
	ret.ReactorPressureVessel.Trip = ret.ReactorProtectionSystem.ReactorTripped

	// Control-rod drive rate limit (plans/x/fission_controls.md).
	// The bridge writes the operator's demand directly into
	// ControlRods.DemandInsertion; this step rate-limits the
	// transition toward that demand so the thermal model sees
	// incremental reactivity changes rather than an
	// instantaneous scram. A latched trip supersedes both and
	// forces full insertion inside ReactorPressureVessel.Step
	// (plans/x/trip.md). While the automatic stepback is latched
	// it, not the operator, owns the demand.
	//   "Rod cluster control (RCC) assemblies are used for
	//   reactor startup or shutdown, to follow load changes,
	//   and to control small transient changes in reactivity."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:88-93
	// Automatic rod control: integrate the Tavg program error
	// into the rod demand before the drive rate-limits toward it.
	// "The rod control system automatically controls RCS average
	// coolant temperature (Tavg) as a programmed function of
	// turbine load."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
	// A latched trip freezes integration: the scram forces full
	// insertion regardless of demand, and pre-spinning withdrawal
	// for a post-trip reset is not an automatic action. The
	// turbine-trip stepback outranks it while latched - the trip
	// response is the control system's first action
	// (specs/pwr_sim/chapter_3/page_041.txt:24-29).
	if ret.ReactorPressureVessel.ControlRods.AutoControl &&
		!ret.ReactorPressureVessel.Trip && !op.StepbackActive {
		powerFraction := float64(ret.ReactorPressureVessel.FissionPower) /
			float64(CoreRatedThermalPower)
		ret.ReactorPressureVessel.ControlRods.applyAutoControl(dt,
			tavg, turbineLoadFraction, powerFraction,
			float64(ret.ReactorPressureVessel.Reactivity))
	}

	if op.StepbackActive {
		ret.ReactorPressureVessel.ControlRods.DemandInsertion = op.StepbackDemand
	}
	ret.ReactorPressureVessel.ControlRods =
		ret.ReactorPressureVessel.ControlRods.Step(dt)

	// While RHRS owns primary cooling (HasTakenOver) the takeover
	// integration below must evolve the outlet from its pre-step value;
	// otherwise the quasi-steady relaxation inside ReactorPressureVessel.
	// Step pulls the coolant toward the hot no-load anchor regardless of
	// the actual heat balance ("Primary temperature is maintained through
	// the balanced use of reactor coolant pump heat input and a steam dump
	// system", specs/the_westinghouse_pressurized_water_react/chapter_16/
	// page_224.txt:109-111).
	rpvOutletPreStepF := ret.ReactorPressureVessel.Outlet.Float()
	pacedExcess, faultExcess := ret.secondaryExcessDrawW(
		sgSteamOutflowFraction, turbineAdmitting, op, dt)
	rpv := ret.ReactorPressureVessel.Step(dt, ret.ReactorCoolantPump.Velocity,
		pacedExcess)
	ret.ReactorPressureVessel = rpv
	ret.ReactorPressureVessel.applyFaultOvercool(dt, faultExcess,
		ret.SteamGenerator.Pressure)
	// Blowdown flash: while the hot-leg break discharges this tick,
	// the coolant follows the saturation temperature of the pressure
	// applyHotLegBreak just produced (plans/p35_1.txt).
	ret.ReactorPressureVessel.applyBlowdownCooling(dt, ret.Pressurizer.Pressure)
	// Surge-line coupling: the pressurizer is hydraulically common
	// with the loop, so its state is THE RCS pressure the vessel
	// reports. Carrying an independent relaxation here let the two
	// readings diverge from every operator-maintained program.
	// "First, it must keep the pressure of the Reactor Coolant
	// System (RCS) within the desired limits"
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:5-12
	ret.ReactorPressureVessel.Pressure = ret.Pressurizer.Pressure

	// RCP velocity dynamics (plans/p33_1.txt). Each loop's pump
	// ramps exponentially toward RCPRatedSpeed when its operator
	// latch is set and coasts down exponentially when not; the
	// Euler update mirrors the diesel (`core/diesel.go:17-18`)
	// and turbine (`core/turbine.go:315-319`) start/coast
	// patterns. Per-pump state is what makes the sequential
	// start evolution representable:
	//   "Reactor coolant pumps are started sequentially and pump
	//    heat is used to raise the RCS temperature."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37
	//
	// Spec justification:
	//   - "A flywheel in the motor increases the total
	//     rotating inertia, allowing the pump to coastdown
	//     slowly after deenergizing."
	//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_061.txt:152-158
	//   - "A heavy flywheel on the pump motor shaft provides
	//     long coastdown times to preclude rapid decreases in
	//     core cooling flow during pump trips."
	//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_020.txt:83-84
	//   - "the long coast down times of the reactor coolant
	//     pumps assures adequate flow during the power decay
	//     transient."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:80-94
	for _, pu := range ret.ReactorCoolantPumps.All() {
		rcpV := float64(pu.Velocity)
		if pu.Running {
			rcpV += (float64(RCPRatedSpeed) - rcpV) *
				dt.Seconds() / float64(RCPStartupTimeConstant.Seconds())
		} else {
			rcpV -= rcpV *
				dt.Seconds() / float64(RCPCoastdownTimeConstant.Seconds())
		}
		rcpV = max(rcpV, 0)
		pu.Velocity = Velocity(rcpV)
	}
	// Lumped hydraulic view for the graph, SVG, and legacy bridge
	// paths; single-writer sync from the group above.
	ret.ReactorCoolantPump.Running = ret.ReactorCoolantPumps.AnyRunning()
	ret.ReactorCoolantPump.Velocity = ret.ReactorCoolantPumps.Velocity()

	// RCP shaft-loss heat entering the primary coolant. Each
	// pump's motor and bearing waste power scales with its speed
	// squared (over the small 1 ms step this is linear in ω² at
	// the current velocity); zero when stopped. RCPRatedHeatInput
	// is per pump, so N spinning rotors contribute N shares —
	// sequential starts stage the heat input the same way they
	// stage the flow.
	//
	// Spec: "Primary temperature is maintained through the
	// balanced use of reactor coolant pump heat input and a
	// steam dump system..."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
	rcpHeat := 0.0
	for _, pu := range ret.ReactorCoolantPumps.All() {
		frac := float64(pu.Velocity) / float64(RCPRatedSpeed)
		rcpHeat += float64(RCPRatedHeatInput) * frac * frac
	}
	ret.ReactorPressureVessel.PumpHeatInput = Watts(rcpHeat)

	// RHRS: once RCS has cooled below the entrance threshold
	// (350°F, <400 psig) the RHRS steps and assumes decay-heat
	// removal. HeatTransferRate is integrated into the RCS
	// outlet temperature and SteamGenerator.HeatInput is zeroed
	// so the SG no longer carries the cooldown duty.
	//
	// Spec justification:
	//   - Entrance criteria: "When the reactor coolant
	//     temperature has dropped to 350°F (177°C) and the
	//     pressure has been reduced to less than approximately
	//     400 psig (28 bar), the RHR equipment is placed in
	//     operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	//   - Hand-off: "At this point the reactor cooling function
	//     has been taken over by the RHRS, and the steam dump
	//     system may be shutdown."
	//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:35-37
	rcsPressurePsig := float64(ret.Pressurizer.Pressure - AtmosphericPressure)
	// The CCWS was already stepped this timestep by
	// AuxiliaryBuilding.Step (single-writer discipline); the RHRS
	// reads that fresh OutletTemp here.
	ret.Aux.ResidualHeatRemovalSystem = ret.Aux.ResidualHeatRemovalSystem.Step(dt,
		ret.ReactorPressureVessel.Outlet,
		rcsPressurePsig,
		ret.Aux.ComponentCoolingWaterSystem.OutletTemp,
	)
	// Solid-plant pressure control: bleed the water-solid primary
	// toward the selected pressure through the RHR/CVC valve and
	// land the bled volume wherever letdown would land (VCT, or the
	// BRS holdup tanks while diverting) so mass is conserved.
	// "Solid plant primary pressure control utilizes a pressure
	// control valve between the RHR and CVC systems which is set to
	// maintain a selected pressure while charging makes up for any
	// coolant contraction and losses."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:115-122
	if bled := ret.Aux.ResidualHeatRemovalSystem.SolidPlantRelieve(dt,
		&ret.Pressurizer); bled.Float() > 0 {
		cvcsOut := &ret.Aux.ChemicalVolumeControlSystem
		if cvcsOut.Diverting {
			cvcsOut.BRSHoldupLevel = vol(cvcsOut.BRSHoldupLevel,
				cvcsOut.BRSHoldupLevel.Float()+bled.Float())
		} else {
			cvcsOut.VCTLevel = vol(cvcsOut.VCTLevel,
				cvcsOut.VCTLevel.Float()+bled.Float())
		}
	}
	if ret.Aux.ResidualHeatRemovalSystem.HasTakenOver {
		// While RHRS owns primary cooling the takeover integration
		// evolves the outlet from its pre-step value (net decay+pump heat
		// minus HX rejection); ReactorPressureVessel.Step's own
		// secondary-excess path is suppressed for this regime by
		// secondaryExcessDrawW, keeping one writer per regime.
		rejectionW := float64(
			ret.Aux.ResidualHeatRemovalSystem.TrainA.HeatExchanger.HeatTransferRate,
		) + float64(
			ret.Aux.ResidualHeatRemovalSystem.TrainB.HeatExchanger.HeatTransferRate,
		)
		decayW := float64(ret.ReactorPressureVessel.TotalThermalPower())
		netW := decayW - rejectionW
		deltaTK := netW / (RCSMassKg * SpecificHeatWater) * dt.Seconds()
		ret.ReactorPressureVessel.Outlet = Temperature{Val: rpvOutletPreStepF + deltaTK*9.0/5.0, Unit: u.Fahrenheit}
		ret.SteamGenerator.HeatInput = 0
	} else {
		// Per-loop SG heat input: the lumped single-SG model
		// represents SteamGeneratorLoopCount primary loops, so
		// each SG carries one loop's fraction of the core
		// thermal output. Without this division the single
		// SG would see the full CoreRatedThermalPower (~3.8
		// GW), roughly 3.5× the per-SG design rating of
		// SteamGeneratorFullLoadHeatInput (~0.95 GW, one
		// loop's share of rated core power).
		// Spec: "Number and type: 1 vertical, U-tube steam
		// generator with integral steam-drum per loop"
		// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:31-32
		// Spec: "Steam flow per steam generator: 3,813,000
		// lb/hr (480 kg/sec)"
		// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
		ret.SteamGenerator.HeatInput = Watts(
			float64(rpv.TotalThermalPower()) / float64(SteamGeneratorLoopCount),
		)
	}
	ret.SteamGenerator = ret.SteamGenerator.Step(dt, sgSteamOutflowFraction,
		ret.Pressurizer.Pressure, &ret.ReactorPressureVessel, &ret.Pressurizer)

	// Surge line: the loop "breathes" into and out of the pressurizer
	// as coolant temperature moves; the displacement itself lives in
	// Pressurizer.Step (PZRSurgeGalPerF against the previous hot-leg
	// temperature), which also owns the bubble bookkeeping.
	//   "Raise pressurizer level above no-load value in
	//    anticipation of coolant contraction"
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:70-72

	// SIS: passive accumulator discharge and HHSI pump
	// injection. The accumulator discharges into the RCS cold
	// legs whenever RCS pressure drops below the nitrogen
	// precharge; the HHSI pumps deliver borated water from the
	// RWST (or, after the low-level switchover, the containment
	// sump).
	//
	// Spec justification:
	//   - Accumulator passive discharge: "the accumulator
	//     discharge will begin automatically as the RCS
	//     pressure decreases below the accumulator tank
	//     pressure."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
	//   - HHSI pump start: "The receipt of the safety injection
	//     signal initiates the following emergency core cooling
	//     actions: [...] The HHSI pumps are started."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	//   - "This high flow is provided by the accumulators,
	//     followed by the HHSI and RHR pumps, all discharging
	//     into the cold legs of the RCS."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:20-23
	//   - "The HHSI and RHR pumps are aligned to take suction
	//     directly from the RWST."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	rcsPressureSIS := float64(ret.ReactorPressureVessel.Pressure)
	ret.SafetyInjectionSystem = ret.SafetyInjectionSystem.Step(dt, rcsPressureSIS)
	// ECCS delivery refills the primary ledger (net accounting
	// against the break's cumulative draw lives in the RPV method).
	// "All pumps will continue to operate and will deliver water to
	//  the RCS cold legs without interruption."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:175-177
	ret.ReactorPressureVessel.creditSafetyInjection(&ret.Pressurizer,
		(ret.SafetyInjectionSystem.HHSIInjectionFlow.Float()+
			ret.SafetyInjectionSystem.AccumulatorInjectionFlow.Float())*
			dt.Seconds())
	// The break's discharge spills into the containment sump, the
	// source the recirculation alignment later draws from
	// (plans/p39_1.txt).
	ret.SafetyInjectionSystem.Sump = vol(
		ret.SafetyInjectionSystem.Sump,
		ret.SafetyInjectionSystem.Sump.Float()+
			ret.ReactorPressureVessel.BreakFlow.Float())

	// EFWS: per-step tank depletion and secured-condition
	// check. The auto-start logic is in Plant.Step (it needs
	// the cross-building DieselStartSignal). The secured
	// condition fires when the hot-leg temperature has been
	// reduced to EFWSSecureTemp and the RHRS has taken over.
	//
	// Spec justification:
	//   - "the reactor coolant hot leg temperature is reduced
	//     to 350°F (176.7°C) and the RHRS has been activated,
	//     the EFWS is secured."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
	rhrsActive := ret.Aux.ResidualHeatRemovalSystem.Running &&
		ret.Aux.ResidualHeatRemovalSystem.HasTakenOver
	ret.Aux.EmergencyFeedwaterSystem = ret.Aux.EmergencyFeedwaterSystem.Step(dt,
		ret.ReactorPressureVessel.Outlet, rhrsActive, nil,
	)
	ret.EFWSFeedwaterFlow = ret.Aux.EmergencyFeedwaterSystem.EFWSFeedwaterFlow()
	// Credit the running AFW pumps' delivery against the SG
	// inventory. The flow is gal/s; tanks were already debited in
	// EFWS.Step so secondary mass is conserved overall.
	// "supply at least the minimum required flow, within one
	//  minute of the actuation signal, to at least two effective
	//  steam generators"
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:23-26
	if ret.EFWSFeedwaterFlow.Val > 0 {
		credit := ret.EFWSFeedwaterFlow.Float() * dt.Seconds()
		sgWater := min(ret.SteamGenerator.Water.Float()+credit,
			ret.SteamGenerator.Volume.Float())
		ret.SteamGenerator.Water = Volume{Val: sgWater, Unit: u.Gallon}
	}

	// Relief routing and CSS: containment pressure model. Relief
	// discharge goes to the PRT while the rupture disc holds;
	// after the disc bursts, continued relief vents to
	// containment. CSS spray condenses steam and drags pressure
	// toward atmospheric (effectiveness × spray flow × excess).
	//
	// Spec justification:
	//   - "Steam from the safety and/or relief valves is piped to
	//     the pressurizer relief tank [...]. A rupture disc vents
	//     the tank to the containment if design pressure is
	//     exceeded."
	//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-38
	//   - "Initial Injection Mode – Containment spray is
	//     initiated automatically in response to a containment
	//     high pressure signal."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-57
	if reliefDischarge.Float() > 0 {
		if !ret.PRTRuptureDiscBurst {
			lvl := min(ret.PressurizedReliefTank.Level.Float()+
				reliefDischarge.Float(), PressurizedReliefTankCapacityGal)
			ret.PressurizedReliefTank.Level = vol(
				ret.PressurizedReliefTank.Level, lvl)
			if lvl >= PressurizedReliefTankCapacityGal {
				ret.PRTRuptureDiscBurst = true
			}
		}
	}
	if ret.PRTSprayValve.Open > 0 {
		lvl := min(ret.PressurizedReliefTank.Level.Float()+
			PressurizedReliefTankSprayGPS*ret.PRTSprayValve.Open*dt.Seconds(),
			PressurizedReliefTankCapacityGal)
		ret.PressurizedReliefTank.Level = vol(
			ret.PressurizedReliefTank.Level, lvl)
	}
	// Spray drag deleted: CSS condensation acts via cssHeatOut
	// (plant.go:1724-1728); the PRT-burst increment is relocated
	// into Plant.Step after the containment step. ret.ContainmentPressure
	// remains a mirror copy from Plant.Step.
	// ret.ContainmentPressure is synced in Plant.Step after the
	// containment step; keep it for any in-ReactorBuilding-step
	// reader by copying the last structured value.
	ret.ContainmentPressure = ret.ContainmentBuilding.Pressure
	ret.Aux.ContainmentSpraySystem = ret.Aux.ContainmentSpraySystem.Step(dt)

	return ret
}

func (t *TurbineBuilding) Step(dt time.Duration) TurbineBuilding {
	ret := *t
	// SteamGenerator lives on ReactorBuilding, not TurbineBuilding;
	// the caller (Plant.Step) passes the upstream pressure in via
	// StepWithSteam. This signature stays for the legacy call path
	// where no upstream is wired; production uses StepWithSteam.
	return ret
}

// StepWithSteam advances the TurbineBuilding with the upstream steam
// generator's pressure and the RCS average temperature supplied by the
// caller. The condenser is stepped first so the turbine reads a fresh
// back-pressure; the condenser accumulates the turbine's exhaust mass flow
// via CondensingRate for the next call.
//
// Spec justification for the order (condenser → turbine):
//   - "The main condenser condenses the exhaust from the main
//     turbines [...]" — the condenser's pressure is the turbine's
//     back-pressure.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-15
//   - The dump valve is described as opening under steam-dump
//     conditions; the condenser must be stepped first so the
//     turbine can see its pressure rise:
//     "These valves open under a sudden reduction in turbine-
//     generator load (in excess of 10%) or a turbine trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:5-11
//
// steamDumpRestorationTarget opens the bypass when Tavg rides above
// its programmed value, limited by header headroom above the
// ESFAS-safe floor so restoration duty can never drag the steamline
// into the low-steamline setpoint; the equilibrium sits proportionally
// up the floor-to-arm slope while Tavg is restored, then the valves
// shut.
//
//	"The total quantity of steam dumped is minimized since the steam
//	 dump is decreased as rapidly as conditions permit."
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:75-77
//
// programTavgF is CoreReferenceInletF post-trip, where the relaxation
// target is no load,
//
//	"The steam dump system will actuate if necessary and restore
//	 Tavg to the no-load value."
//	specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:95-97
//
// and the setpoint-programmed Tref while the plant follows load.
func steamDumpRestorationTarget(programTavgF float64,
	tavg Temperature, sgPressure Pressure, floorFraction float64) float64 {
	excessF := tavg.Float() - programTavgF - SteamDumpTripDeadbandF
	term := min(max(excessF/SteamDumpTripSpanF, 0), 1)
	floor := floorFraction * float64(SteamGeneratorShellPressure)
	headroom := (float64(sgPressure) - floor) /
		(float64(SteamDumpPressureModeMargin)*float64(SteamGeneratorShellPressure) - floor)
	return min(term, min(max(headroom, 0), 1))
}

func (t *TurbineBuilding) StepWithSteam(dt time.Duration,
	sgPressure Pressure,
	tavg Temperature,
	cwPumpRunning bool,
	reactorTripped bool,
	msivOpen float64,
	tcoldF float64,
) TurbineBuilding {
	ret := *t

	// Feedwater recirculation (plans/p59_1.txt §3): the bridge-only
	// RecirculateValve now has a Step effect. Derived rate sized
	// so the full-open path moves one design full-load steam
	// load per second, matching the HDT suction rate. CST lives
	// on Outside (outside.go:14) and is wired by Plant.Step as
	// nextPlant.CSTForRecirc; the in-TurbineBuilding path keeps
	// the CSTLevel credit here so a test that steps the building
	// directly still sees conservation.
	if ret.RecirculateValve.Open > 0 {
		moved := min(FeedPumpSuctionFlow*ret.RecirculateValve.Open*dt.Seconds(),
			ret.Condenser.HotwellLevel.Float())
		ret.Condenser.HotwellLevel = vol(ret.Condenser.HotwellLevel,
			ret.Condenser.HotwellLevel.Float()-moved)
		if ret.CSTForRecirc != nil {
			*ret.CSTForRecirc = vol(*ret.CSTForRecirc,
				ret.CSTForRecirc.Float()+moved)
		}
	}
	// RejectValve: hotwell -> drain sink, loses secondary inventory.
	if ret.RejectValve.Open > 0 {
		dump := min(FeedPumpSuctionFlow*ret.RejectValve.Open*dt.Seconds(),
			ret.Condenser.HotwellLevel.Float())
		ret.Condenser.HotwellLevel = vol(ret.Condenser.HotwellLevel,
			ret.Condenser.HotwellLevel.Float()-dump)
	}

	cwIn := ret.Condenser.CirculatingWaterIn
	c := ret.Condenser.Step(dt, cwIn, ret.Condenser.VacuumPumpRunning, cwPumpRunning)
	ret.Condenser = c

	// The MSIV sits between the shell and the header: shut, it
	// cuts both the turbine admission and the dump delivery while
	// the atmospheric dumps and any line break - upstream of the
	// stop valves - keep their own paths.
	//   "The main steam stop valves automatically close on low
	//    steam line pressure or on a high-high containment
	//    pressure signal [...]"
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:71-75
	msivAdmission := min(max(msivOpen, 0), 1)
	stopOpen := ret.Turbine.StopValve.Open * msivAdmission
	prevSetpoint := ret.Turbine.LastSetpoint
	ret.Turbine = ret.Turbine.Step(dt,
		sgPressure,
		c.Pressure,
		tavg,
		false,
		&stopOpen,
	)
	ret.Turbine.LastSetpoint = ret.Turbine.Setpoint

	// Generator excitation system. The brushless exciter is
	// shaft-driven: it can only produce field current when
	// the rotor is turning.
	//
	// Spec justification:
	//   - "Excitation: Shaft-driven, air-cooled brushless
	//     exciter of the rotating rectifier type."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
	//   - "The Type WTA-300 voltage regulating system is of
	//     the latest design incorporating all solid state
	//     circuitry."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_212.txt:5-7
	ret.Generator.GeneratorExcitationSystem =
		ret.Generator.GeneratorExcitationSystem.Step(dt,
			float64(ret.Turbine.Velocity),
		)
	ret.GeneratorExcitationAvailable =
		ret.Generator.ExciterActive

	// Generator cooling system. SCCW pump mirrors the
	// feedwater/condensate pump availability in the
	// simplified model (real plant has dedicated redundant
	// pumps; for p30 the gating flag is "at least one
	// pump-running boolean").
	//
	// Spec justification:
	//   - "Demineralized cooling water is provided for the
	//     stator by a closed recirculating water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	//   - "Redundancy is provided in the water pumps, water
	//     coolers, and the alarm system so that safe,
	//     reliable operation is assured under all conditions."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:55-61
	ret.Generator.SCCWPumpRunning = ret.MainFeedwaterPump.Running ||
		ret.CondensatePump.Running
	// H₂ blower fraction tracks shaft speed relative to
	// rated; the four cooler effectiveness slots scale
	// linearly with this fraction.
	h2Blower := float64(ret.Turbine.Velocity) /
		float64(TurbineRatedSpeed)
	h2Blower = min(h2Blower, 1)
	h2Blower = max(h2Blower, 0)
	ret.Generator.GeneratorCoolingSystem =
		ret.Generator.GeneratorCoolingSystem.Step(dt, h2Blower)

	// SCCW alarm and trip gating (plans/p30_1.txt). When
	// the SCCW circuit is active but the flow has dropped
	// below the minimum stable threshold, the alarm is
	// raised. Total loss of flow while the pump is
	// commanded running drops the generator breaker.
	sccwState := SCCWAlarm(ret.Generator.SCCWPumpRunning,
		ret.Generator.SCCWFlow)
	ret.GeneratorSCCWAlarm = sccwState.Alarm
	ret.GeneratorSCCWTrip = sccwState.Trip
	if ret.GeneratorSCCWTrip && ret.Generator.BreakerClosed {
		ret.Generator.BreakerClosed = false
	}

	// Generator terminal state. Terminal frequency tracks shaft
	// speed and terminal voltage builds under the regulator whenever
	// the rotor spins with the exciter active. The breaker gates
	// real/reactive power export, not the presence of terminal
	// voltage: a spinning generator with field applied is live.
	//
	// Spec: "Output voltage: 24,000 to 26,000 volts, three-phase,
	// 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	// "Speed: 1800 rpm"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:22
	spinFrac := float64(0)
	if TurbineRatedSpeed > 0 {
		spinFrac = float64(ret.Turbine.Velocity) / float64(TurbineRatedSpeed)
	}
	spinFrac = min(max(spinFrac, 0), 1)
	if spinFrac >= 0.01 && ret.Generator.ExciterActive {
		ret.Generator.Frequency = spinFrac * GridRatedFrequency
		voltageTarget := GeneratorRatedVoltage
		ret.Generator.Voltage += (voltageTarget - ret.Generator.Voltage) *
			GeneratorExciterFieldForcingFraction
		if ret.Generator.Voltage > GeneratorRatedVoltage*0.99 {
			ret.Generator.Voltage = GeneratorRatedVoltage
		}
	} else {
		ret.Generator.Voltage = 0
		ret.Generator.Frequency = 0
	}

	// RealPower and ReactivePower are zero whenever the breaker
	// is open, regardless of the turbine's mechanical output. The
	// breaker gates grid connection.
	//
	// Spec: "output power, the power factor, and speed to ensure
	// that the limits of the generator are not exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
	// "Power factor: 0.9, short circuit ratio: 0.58"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
	if ret.Generator.BreakerClosed && ret.Turbine.RealPower > 0 {
		gross := float64(ret.Turbine.RealPower)
		pf := ret.Generator.PowerFactor
		if pf <= 0 {
			pf = GeneratorPowerFactor
		}
		q := gross * math.Sqrt(1.0/(pf*pf)-1.0)
		gross, q = clampMVA(&ret.Generator, gross, q)
		ret.Generator.RealPower = Watts(gross)
		ret.Generator.ReactivePower = q
	} else {
		ret.Generator.RealPower = 0
		ret.Generator.ReactivePower = 0
	}

	// OPC trip pops the breaker. The spec describes OPC as part
	// of the protective trip layer that closes steam valves AND
	// disconnects the generator from the grid.
	//
	// Spec: "OPC trip (110%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:72
	if ret.Turbine.OPCTrip {
		ret.Generator.BreakerClosed = false
	}

	// Steam dump controller. The dump valves open on a sudden load
	// rejection (in excess of 10%), a turbine trip, or pressure-mode
	// operation during startup/cooldown; they reclose automatically as
	// steam is readmitted to the turbine and pressure falls.
	//
	// Spec: "These valves open under a sudden reduction in turbine-
	// generator load (in excess of 10%) or a turbine trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
	// "These valves are also utilized in the pressure control mode
	// during plant startup and plant cooldown."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-25
	// "The dump valves close automatically as steam is admitted to
	// the turbine."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:32-33
	tripped := ret.Turbine.State == TurbineTripped
	loadReject := prevSetpoint-ret.Turbine.Setpoint >= LoadRejectDetectFraction
	pressureMode := sgPressure > Pressure(SteamDumpPressureModeMargin*float64(SteamGeneratorShellPressure))
	if loadReject {
		ret.Turbine.DumpHoldTimer = SteamDumpHoldTime
	} else if ret.Turbine.DumpHoldTimer > 0 {
		ret.Turbine.DumpHoldTimer -= dt
	}
	// A turbine trip holds the bypass in service while the trip
	// lasts; a load rejection holds it for a fixed ride-through
	// window instead. Two trip regimes share the valves:
	//   - Turbine-trip stepback with the reactor still critical:
	//     the bypass "temporarily replace[s] the turbine as the
	//     steam load", opening once on the trip and then modulating
	//     around the full-load shell pressure to pass what
	//     production keeps making.
	//     "turbine bypass valves open automatically on turbine trip,
	//      then modulate to pass steam to the condenser and hold
	//      boiler pressure at the constant setpoint"
	//     specs/pwr_sim/chapter_3/page_041.txt:37-45
	//   - Reactor trip: fission folds to decay heat, so holding
	//     boiler pressure at the full-load setpoint would dump far
	//     more than production and drag the header into protective
	//     setpoints. The demand follows the RCS average temperature
	//     excess above no load instead, limited by header headroom
	//     above an ESFAS-safe floor, and the valves shut as Tavg
	//     restores.
	//     "Steam dump is controlled by signals from the RCS
	//      (essentially reactor coolant average temperature) [...];
	//      the total quantity of steam dumped is minimized"
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/
	//     page_233.txt:70-77
	//     "The steam dump system will actuate if necessary and
	//      restore Tavg to the no-load value."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/
	//     page_233.txt:95-96
	// Table 7-1: "Block Steam Dump: Low Tcold". With the cold leg
	// below the low-Tcold setpoint the automatic dump branches -
	// trip kick, post-scram restoration, rejection hold - stand
	// down; the pressure-control branch stays armed because plant
	// cooldown deliberately dumps to the condensers for its heat
	// sink.
	//   "Block Steam Dump: Low Tcold"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:48-49
	//   "During plant cooldown, steam is dumped to the condensers
	//    to provide a heat sink rather than to atmosphere through
	//    the atmospheric dump valves."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:33-37
	tcoldBlocksDump := tcoldF < RPSLowTcoldTrip
	var target float64
	switch {
	case tripped && !reactorTripped:
		if tcoldBlocksDump {
			ret.Turbine.DumpPressureIntegral = 0
			ret.Turbine.DumpBypassKicked = false
			target = 0
			break
		}
		if !ret.Turbine.DumpBypassKicked {
			// One-shot opening on the trip itself; the pressure
			// loop below throttles from there.
			ret.Turbine.DumpBypassKicked = true
			ret.Turbine.DumpPressureIntegral = 1
		}
		ret.Turbine.DumpHoldTimer = 0
		target = ret.Turbine.dumpModulationDemand(sgPressure, dt)
	case tripped && reactorTripped:
		if tcoldBlocksDump {
			target = 0
			break
		}
		target = steamDumpRestorationTarget(CoreReferenceInletF, tavg, sgPressure,
			SteamDumpTripHeaderFloorFraction)
		if pressureMode {
			target = 1.0
		}
		// Leave the stepback episode state reset so a later
		// turbine-only trip kicks its pressure loop cleanly.
		ret.Turbine.DumpPressureIntegral = 0
		ret.Turbine.DumpBypassKicked = false
	case ret.Turbine.DumpHoldTimer > 0:
		// Rejection ride-through: the bypass pins fully open for
		// the hold window as the short-term artificial load.
		// "The steam dump serves as a short-term artificial load which
		// allows the reactor control system time to automatically cut
		// back power."
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:27-31
		if tcoldBlocksDump {
			target = 0
			break
		}
		ret.Turbine.DumpPressureIntegral = 0
		ret.Turbine.DumpBypassKicked = false
		target = 1.0
	case ret.Turbine.State == TurbineRunning && !reactorTripped &&
		ret.Turbine.Setpoint <= SteamDumpRestorationMaxLoadFrac:
		// Deep-band park sink: with both machines untripped and the
		// turbine admitting, production exceeds the governor draw at
		// house loads and Tavg would park above its program; the
		// bypass carries the surplus against the programmed Tref
		// under the shared headroom limiter. Above the band the
		// governor draw services production and the dump yields as
		// the control system reduces power, so tracking lag must not
		// open it ("[the dump] is reduced as rapidly as the reactor
		// control system reduces core power",
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:72-74).
		// The pressure loop only has authority while the header is
		// above the arm point; below it the loop state resets so a
		// trip-phase kick cannot leak in as a phantom demand.
		var pl float64
		if pressureMode {
			pl = ret.Turbine.dumpModulationDemand(sgPressure, dt)
		} else {
			ret.Turbine.DumpPressureIntegral = 0
			ret.Turbine.DumpBypassKicked = false
		}
		programTavgF := programmedRodTavg(ret.Turbine.Setpoint)
		target = min(max(pl, steamDumpRestorationTarget(
			programTavgF, tavg, sgPressure,
			SteamDumpRestorationHeaderFloorFraction)), 1)
	case pressureMode:
		// Pressure-control mode without a trip (startup, cooldown,
		// load-chase surplus): modulate to hold the header at the
		// shell-pressure setpoint instead of slamming full open.
		//   "These valves are also utilized in the pressure control mode
		//    during plant startup and plant cooldown."
		//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-26
		//   "...modulate their opening to pass sufficient steam flow to
		//    the condenser, in order to maintain boiler pressure at the
		//    constant setpoint."
		//   specs/pwr_sim/chapter_3/page_041.txt:40-42
		ret.Turbine.DumpBypassKicked = false
		target = ret.Turbine.dumpModulationDemand(sgPressure, dt)
	default:
		ret.Turbine.DumpPressureIntegral = 0
		ret.Turbine.DumpBypassKicked = false
	}
	travel := SteamDumpValveTravelRate * dt.Seconds()
	switch {
	case ret.Turbine.DumpValve.Open < target:
		ret.Turbine.DumpValve.Open = min(ret.Turbine.DumpValve.Open+travel, target)
	case ret.Turbine.DumpValve.Open > target:
		ret.Turbine.DumpValve.Open = max(ret.Turbine.DumpValve.Open-travel, target)
	}

	// CondensingRate: the condenser sees the turbine exhaust plus
	// whatever the dump valves route into it. The SG-side draw
	// (exhaust + dump) is exported so Plant.Step can debit the
	// steam generator inventory and header pressure.
	// "The main condenser condenses the exhaust from the main
	// turbines [...] There is also sufficient surface to condense
	// steam from the steam dump system with an acceptable increase
	// in turbine back pressure."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-19
	condenseGalPerStep := 0.0
	turbineFrac := 0.0
	if ret.Turbine.State == TurbineRunning {
		// SteamDemandGalPerStep is already MSIV-gated inside
		// Turbine.Step via stopOpen (above); multiplying here
		// again would count the MSIV twice.
		condenseGalPerStep = ret.Turbine.SteamDemandGalPerStep()
		if CondenserDesignSteamLoadGalPer > 0 {
			turbineFrac = condenseGalPerStep / CondenserDesignSteamLoadGalPer
		}
	}
	// Dump delivery scales with available header pressure: an open
	// dump valve cannot pass design steam against a depressurised SG.
	pressureRatio := min(max(float64(sgPressure)/float64(SteamGeneratorShellPressure), 0), 1)
	dumpedGalPerStep := ret.Turbine.DumpValve.Open * SteamDumpCapacityGalPerStep *
		pressureRatio * msivAdmission
	ret.Turbine.dumpedGalPerStep = dumpedGalPerStep
	dumpFrac := ret.Turbine.DumpValve.Open * SteamDumpDesignFraction *
		pressureRatio * msivAdmission
	ret.Condenser.CondensingRate = Volume{Val: condenseGalPerStep + dumpedGalPerStep, Unit: u.Gallon}
	ret.SteamOfftakeFraction = turbineFrac + dumpFrac

	// Feedwater heater train: four LP stages between the
	// condenser hotwell and the deaerator, then two HP stages on
	// the main-feed-pump discharge side. The hotwell feedwater
	// temperature is the condenser drain temperature (~120°F at
	// design vacuum).
	//
	// Spec justification for the topology:
	//   - "The condensate pumps take suction from the condenser
	//     hotwell and pump condensate [...] through four stages of
	//     low-pressure feedwater heaters to the deaerator."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
	//   - "The water discharge from the feedwater pumps flows
	//     through two stages of high-pressure heaters then into
	//     the steam generators."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
	//
	// The per-step extraction steam flow is sized at the design
	// extraction rate derived from the turbine's load fraction
	// (plans/p11_1.txt). The MSR reheater section uses the same
	// extraction supply for both the LP sixth-stage taps and the
	// HP heater train, so the per-stage temperature rise
	// converges to LPTemperatureRisePerStage /
	// HPTemperatureRisePerStage at design operating point.
	fwIn := Temperature{Val: 120, Unit: u.Fahrenheit}
	lpSteam := ret.Turbine.ExtractionSteamFlow.Float() * MSRSixthStageExtractionFraction
	hpSteam := ret.Turbine.ExtractionSteamFlow.Float() * MSRDesignExtractionFraction
	lp := ret.LowPressureHeater
	for i := 0; i < 4; i++ {
		lp = lp.Step(dt, fwIn.Float(), lpSteam)
		fwIn = Temperature{Val: lp.Inlet.Float() + lp.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	// LP heater drains cascade back to the condenser instead of
	// accumulating inside the heater shell.
	// "Drains from the high-pressure feedwater heaters are routed
	// to the deaerator. Drains from the four low-pressure
	// feedwater heaters are cascaded back to the condenser."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:88-90,
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:1-2
	ret.Condenser.creditHotwell(lp.DrainLevel.Float())
	lp.DrainLevel = Volume{Val: 0, Unit: u.Gallon}
	ret.LowPressureHeater = lp

	// MSR moisture drain to hotwell (feedwater system).
	// Spec: "where the water is removed and drained to the
	// feedwater system."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:62-63
	msrDrain := ret.Turbine.MoistureSeparatorReheater.DrainFlow.Float()
	ret.Condenser.creditHotwell(msrDrain)

	// HP train: two stages.
	hp := ret.HighPressureHeater
	for i := 0; i < 2; i++ {
		hp = hp.Step(dt, fwIn.Float(), hpSteam)
		fwIn = Temperature{Val: hp.Inlet.Float() + hp.TemperatureRise.Float(), Unit: u.Fahrenheit}
	}
	ret.HighPressureHeater = hp

	// HP drains route to the deaerator tank. The
	// HeaterDrainTank plays the role of the deaerator water
	// storage tank:
	// "The booster pumps draw suction from the deaerator water
	// storage tank, which is mounted directly below the
	// deaerator [...]."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:47-49
	// "Drains from the high-pressure feedwater heaters are
	// routed to the deaerator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-14
	hpDrain := hp.DrainLevel
	newHDTLevel := ret.HeaterDrainTank.Level.Float() + hpDrain.Float()
	ret.HeaterDrainTank.Level = Volume{Val: newHDTLevel, Unit: u.Gallon}

	// Minimum-flow recirculation. When the main feed pump is
	// running but the net flow to the HP heaters is below
	// FeedwaterMinFlow, the discharge recirculates to the
	// deaerator.
	// "The discharge from the pumps is automatically recirculated
	// back to the deaerator whenever the flow to the high-pressure
	// feedwater heaters falls below the minimum."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
	// We approximate "net flow" with the pump's Running flag: when
	// the pump is stopped (or no SG-side demand exists), the
	// minimum-flow line moves FeedwaterMinFlow per step back to
	// the deaerator tank.
	if !ret.MainFeedwaterPump.Running {
		recirc := FeedwaterMinFlow.Float() * float64(dt.Seconds())
		newHDTLevel := ret.HeaterDrainTank.Level.Float() + recirc
		ret.HeaterDrainTank.Level = Volume{Val: newHDTLevel, Unit: u.Gallon}
	}

	// Drain-tank accounting: clamp at capacity, draw suction while
	// the main feed pump runs, and open the dump valve to the
	// condenser on a high level.
	//
	// "The feedwater booster pumps draw suction from the deaerator
	// water storage tank, which is mounted directly below the
	// deaerator, and supply the main feedwater pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:46-49
	// "A dump valve which taps off the cross-connection drains the
	// tanks to condenser A on a high level in either tank."
	// specs/westinghouse_tech_manual/chapter_7/page_0636.txt:36-38
	dtSec := dt.Seconds()
	hdt := min(ret.HeaterDrainTank.Level.Float(), HeaterDrainTankCapacityGal)
	if ret.MainFeedwaterPump.Running {
		hdt = max(hdt-FeedPumpSuctionFlow*dtSec, 0)
	}
	if hdt > HeaterDrainTankCapacityGal*HDTHighLevelFraction {
		dumped := min(HDTDumpFlow*dtSec,
			hdt-HeaterDrainTankCapacityGal*HDTHighLevelFraction)
		hdt -= dumped
		// Dump adds to the hotwell through the shared clamp; the
		// tech-manual citation is already on the block comment.
		ret.Condenser.creditHotwell(dumped)
	}
	ret.HeaterDrainTank.Level = Volume{Val: hdt, Unit: u.Gallon}

	return ret
}

// generatorReactive returns reactive power for real power p at power
// factor pf: Q = P*tan(acos(pf)). At the design pf of 0.9 that is
// 0.484*p.
// "Power factor: 0.9, short circuit ratio: 0.58"
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
func generatorReactive(p float64, pf float64) float64 {
	return p * math.Sqrt(1.0/(pf*pf)-1.0)
}

// clampMVA scales apparent power back onto the generator rating
// circle and raises GeneratorMVALimitAlarm when clamping engages.
// "It controls valve changes and monitors output power, the power
// factor, and speed to ensure that the limits of the generator are
// not exceeded."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
func clampMVA(gen *PowerGenerator, realWatts, reactiveVars float64) (float64, float64) {
	s := math.Hypot(realWatts, reactiveVars)
	limitW := GeneratorRatedMVA * 1000.0 // kVA rating expressed in watts
	if s > limitW && s > 0 {
		scale := limitW / s
		gen.GeneratorMVALimitAlarm = true
		return realWatts * scale, reactiveVars * scale
	}
	gen.GeneratorMVALimitAlarm = false
	return realWatts, reactiveVars
}

// Plant represents all state relevant to the facility.
type Plant struct {
	ReactorBuilding   `json:"ReactorBuilding"`
	AuxiliaryBuilding `json:"AuxiliaryBuilding"`
	TurbineBuilding   `json:"TurbineBuilding"`
	Outside           `json:"Outside"`
	Electrical        `json:"Electrical"`
	// PlantOperation is the Chapter 16 operational-mode
	// layer (plans/p32_1.txt) that tracks the mode state
	// machine (ColdShutdown → HotStandby → PowerOperation)
	// and enforces rate limits, the auxiliary-to-main
	// feedwater transition, and the hot-standby RCP cap.
	//
	// Spec justification:
	//   - Operational modes: chapter 16 covers startup,
	//     normal operation, load follow, shutdown.
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
	//   - Rate limits: 5%/min loading, 50°F/hr heatup/cooldown.
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
	PlantOperation PlantOperation
	nextPlant      *Plant
	cachedGraph    *PlantGraph
	cachedTarget   *Plant
	cachedPumps    [7]*Pump
}

func (p *Plant) Step(dt time.Duration) {
	if p.nextPlant == nil {
		p.nextPlant = &Plant{}
	}
	// Step the electrical distribution first so the ESF bus
	// Energized state for this step is visible to every
	// downstream consumer (CW pump, SFPC pump, RHRS, CCWS,
	// CSS, HHSI, SWS). The Electrical.Step call is pure on
	// its input and only writes to nextPlant.Electrical.
	p.mirrorOffsiteAvailability()
	// Wire the ESF-bus source candidates before Electrical.Step so
	// the very first tick sees the live plant instead of zero
	// scratch state. The candidates read the CURRENT plant's values
	// (the same one-tick telemetry lag the rest of the model uses):
	// offsite from the declared-availability mirror above, the unit
	// generator through the UAT, and each train's own diesel
	// (independent trains,
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:30-36).
	p.Electrical.DieselGenA = &p.AuxiliaryBuilding.DieselGenerator
	p.Electrical.DieselGenB = &p.AuxiliaryBuilding.DieselGeneratorB
	p.Electrical.MainGenA = &p.TurbineBuilding.Generator
	p.Electrical.MainGenB = &p.TurbineBuilding.Generator
	p.nextPlant.Electrical = p.Electrical.Step(dt)
	plantDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized

	// Run Outside.Step first so the CW pump / cooling-tower
	// state is in nextPlant. Step the AuxiliaryBuilding next so
	// its diesel / RHRS / CCWS state is fresh for the
	// ReactorBuilding step (CVCS / RHRS / CCWS / CSS / RCFC /
	// EFWS live on the auxiliary building and read the
	// pointer populated below).
	//
	// The cross-building source pointers must point at
	// nextPlant (where the fresh state is being written), not
	// at p (the current state being read).
	// The tower's hot inlet is the condenser CW outlet stepped
	// in the previous tick's TurbineBuilding pass (a 1 ms lag,
	// same as the tower's flow input).
	p.nextPlant.Outside = p.Outside.Step(dt,
		plantDriveReady, p.TurbineBuilding.Condenser.CWOut)
	// Latch the CW pump DriveReady (naming-convention
	// counterpart to ServiceWaterSystem.DriveReady) so the
	// bridge can publish it as a sensor. The pump motors ride
	// on the 6600 V plant distribution bus fed by UAT or SAT
	// (see plans/x/uat_sat_gating.md); a blackout drops it.
	p.nextPlant.Outside.CirculatingWaterPump.DriveReady = plantDriveReady
	// SWS flow at this point is the prior step's delivered flow
	// in gpm (the SWS Step runs later, after the cross-building
	// DriveReady wiring). PumpRunning alone is not enough — the
	// 6600V ESF breaker must be closed for the pump motor to
	// develop flow, so the diesel jacket cooler sees Flow > 0
	// only when power is actually delivered to the pump.
	// The SFPC pump reads the same plantDriveReady signal
	// (480 V MCC downstream of an ESF bus).
	// Emergency diesel auto-start (chapter 16 emergency response):
	// "backup emergency diesel generators are automatically started"
	// when their safeguards bus loses power
	// (specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-109);
	// diesel start is also item 1 of the SI actuation list
	// (specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:18-22).
	// The trigger is the SENSED bus state, not the operator's offsite
	// declaration: buses still fed from the unit's own generator while
	// offsite is declared lost stay live, so their diesels idle until
	// the unit comes off the line. Each train's diesel answers only
	// its own bus (independent trains,
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:30-36).
	// Set before AuxiliaryBuilding.Step so the engines crank in the
	// same tick; clearing a latch stays an explicit operator action
	// after the supply is restored.
	if !p.nextPlant.Electrical.ESFBusA.Energized &&
		p.AuxiliaryBuilding.DieselGenerator.StartSignal == DieselStartNone {
		p.AuxiliaryBuilding.DieselGenerator.StartSignal =
			DieselStartLossOfOffsitePower
	}
	if !p.nextPlant.Electrical.ESFBusB.Energized &&
		p.AuxiliaryBuilding.DieselGeneratorB.StartSignal == DieselStartNone {
		p.AuxiliaryBuilding.DieselGeneratorB.StartSignal =
			DieselStartLossOfOffsitePower
	}

	// Safety-injection auto-start: item 1 of the SI actuation list
	// starts the diesels whenever the ESFAS output latches, with or
	// without a loss of offsite power. The committed RPS latch is
	// read here (this sits before ReactorBuilding.Step in the tick),
	// so the engines crank the tick after actuation and an operator
	// standdown sticks immediately.
	// "The receipt of the safety injection signal initiates the
	//  following emergency core cooling actions: The onsite emergency
	//  diesel generators are started."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-114
	if p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated {
		if p.AuxiliaryBuilding.DieselGenerator.StartSignal == DieselStartNone {
			p.AuxiliaryBuilding.DieselGenerator.StartSignal =
				DieselStartSafetyInjection
		}
		if p.AuxiliaryBuilding.DieselGeneratorB.StartSignal == DieselStartNone {
			p.AuxiliaryBuilding.DieselGeneratorB.StartSignal =
				DieselStartSafetyInjection
		}
	}

	p.nextPlant.AuxiliaryBuilding = p.AuxiliaryBuilding.Step(dt,
		p.Outside.ServiceWaterSystem.InletTemp,
		p.Outside.ServiceWaterSystem.Flow,
		plantDriveReady,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel.Float(),
	)
	// Latch the SFPC DriveReady so the bridge can publish it
	// as a sensor (plans/x/uat_sat_gating.md).
	p.nextPlant.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.DriveReady =
		plantDriveReady
	p.ReactorBuilding.Aux = &p.nextPlant.AuxiliaryBuilding

	// RHR TrainA on BusA, TrainB on BusB. The spec assigns
	// RHR pumps to their own ESF train, mirroring the EFWS
	// motor-pump assignment.
	// "Loads important to plant safety are split and
	// diversified between independent engineered safety
	// features system switchgear groups."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:33-36
	p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.DriveReady =
		p.nextPlant.Electrical.ESFBusA.StageReady(1)
	p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.DriveReady =
		p.nextPlant.Electrical.ESFBusB.StageReady(1)

	// Automatic reactor-power stepback. The control system latches on
	// a tripped turbine or degraded forced flow (the pump-trip runback
	// of "The loss of one RC pump will immediately initiate reactor
	// power stepback", specs/pwr_sim/chapter_4/page_051.txt:44-46)
	// and owns the rod demand while it holds neutron power at the
	// runback level; the ReactorBuilding step below applies that
	// demand ahead of its rod-drive rate limit.
	p.nextPlant.PlantOperation = p.PlantOperation.Stepback(dt,
		p.TurbineBuilding.Turbine.State,
		float64(p.ReactorBuilding.ReactorCoolantPumps.Velocity())/
			float64(RCPRatedSpeed),
		p.ReactorBuilding.ReactorPressureVessel,
	)
	// Coordinated turbine runback riding the stepback: while the runback
	// holds and the machine is still on the line, steam demand descends
	// toward the park level at the same rate the rods fold power. Without
	// it, production outpaces a fixed demand only in the downward
	// direction and the sagging SG header crosses the shared
	// low-steamline setpoint - "automatic turbine runbacks which can
	// prevent a trip condition"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:137-139
	// The walk floors at StepbackPowerFraction so demand lands where the
	// reactor parks; handing back to the operator afterwards mirrors the
	// rod-demand handback on release.
	if op := p.nextPlant.PlantOperation; op.StepbackActive &&
		p.TurbineBuilding.Turbine.State == TurbineRunning {
		tb := &p.TurbineBuilding.Turbine
		if tb.Setpoint > StepbackPowerFraction {
			tb.Setpoint = max(tb.Setpoint-RodInsertionSpeed*dt.Seconds(),
				StepbackPowerFraction)
		}
	}
	p.nextPlant.ReactorBuilding = p.ReactorBuilding.Step(dt,
		p.TurbineBuilding.SteamOfftakeFraction,
		p.TurbineBuilding.Turbine.Setpoint,
		p.nextPlant.PlantOperation,
		p.TurbineBuilding.Turbine.State == TurbineRunning,
		p.TurbineBuilding.Turbine.State == TurbineTripped,
	)
	// Debit the holdup tanks for what the reclaim train drew this
	// step. The CVCS stepped after the BRS and added its own letdown
	// contribution, so the subtraction lands on top of it without a
	// second writer inside either subsystem.
	if drawn := p.nextPlant.AuxiliaryBuilding.BoronRecycleSystem.HoldupDrawn.Float(); drawn > 0 {
		cv := &p.nextPlant.AuxiliaryBuilding.ChemicalVolumeControlSystem
		cv.BRSHoldupLevel = vol(cv.BRSHoldupLevel,
			max(cv.BRSHoldupLevel.Float()-drawn, 0))
	}
	// Latch the RCP DriveReady after the ReactorBuilding step so
	// the assignment above (which copies the prior state) cannot
	// clobber it. The bridge sensor then reflects the actual
	// upstream power state, matching the pattern used for the CW
	// and SWS pumps. The RCP motors ride on the 6600 V ESF buses:
	//   "All of the engineered safety system loads are fed from
	//    two 6600-volt engineered safety features buses."
	//   specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
	p.nextPlant.ReactorBuilding.ReactorCoolantPump.DriveReady = plantDriveReady
	for _, pu := range p.nextPlant.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.DriveReady = plantDriveReady
	}
	// Loss of the pump power supply de-energizes the reactor coolant
	// pumps and the trip latches: on its loss the flywheel coastdown
	// carries core flow through the power decay. The diesel-backed
	// ESF buses do not restart the pumps; bringing one back is an
	// explicit operator action once a supply exists.
	//   "Reactor coolant pumps supplied from the offsite electrical
	//    source will maintain forced flow cooling to the reactor
	//    core."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:66-68
	//   "If offsite power is not available, the long coast down times
	//    of the reactor coolant pumps assures adequate flow during
	//    the power decay transient."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:80-84
	if !plantDriveReady {
		p.nextPlant.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(false)
		p.nextPlant.ReactorBuilding.ReactorCoolantPump.Running = false
	}

	// Refueling operations step. The phase machine advances
	// based on phase-relative time; the cavity flood/drain
	// booleans are operator-driven inputs (set by the test
	// harness or higher-fidelity HMI). The pit water level
	// and boron concentration are held constant at the
	// refueling-cavity values per the spec.
	//
	// Spec justification:
	//   - Five-phase cycle: "The refueling operation is divided
	//     into five major phases: preparation, reactor
	//     disassembly, fuel handling, reactor reassembly, and
	//     preoperational checks, tests, and startup."
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_183.txt:32-34
	//   - Pit always full of water: "the fuel storage pit,
	//     which is kept full of water and is always accessible
	//     to operating personnel."
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:25-29
	p.nextPlant.ReactorBuilding.RefuelingOperations =
		p.nextPlant.ReactorBuilding.RefuelingOperations.Step(dt)

	// SIS actuation: receipt of the safety-injection signal
	// starts the HHSI pumps and aligns them to the RWST. The
	// signal is the same DieselStartSafetyInjection that
	// starts the diesel (the spec ties both actions to the
	// same actuation event).
	//
	// Spec justification:
	//   - "The receipt of the safety injection signal initiates
	//     the following emergency core cooling actions: The
	//     onsite emergency diesel generators are started. The
	//     HHSI pumps are started."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	//   - "The HHSI and RHR pumps are aligned to take suction
	//     directly from the RWST."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	if p.AuxiliaryBuilding.anyDieselStartSignal(DieselStartSafetyInjection) {
		// HHSI pumps need ESF bus power; DriveReady
		// reflects whether at least one ESF bus is
		// energised past its sequence stage. The actual
		// injection flow is gated on HHSIPumpRunning &&
		// HHSIDriveReady, so the auto-start latches
		// PumpRunning on the safety signal (operator intent)
		// and the bus-energised state decides whether flow
		// actually develops.
		p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSIDriveReady =
			p.nextPlant.Electrical.ESFBusA.StageReady(1) ||
				p.nextPlant.Electrical.ESFBusB.StageReady(1)
		p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = true
		if p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSISuction !=
			SISSuctionSump {
			p.nextPlant.ReactorBuilding.SafetyInjectionSystem.HHSISuction =
				SISSuctionRWST
		}
		// The same actuation list starts the RHR pumps; they develop
		// flow only once the RCS is inside the RHRS entrance
		// envelope (the pump shutoff head cannot inject into a
		// pressurized primary), so the latch waits for entry
		// conditions exactly like an operator line-up would.
		// "The RHR pumps are started."
		// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:140
		// "When the reactor coolant temperature has dropped to 350°F
		//  [...] and the pressure has been reduced to less than
		//  approximately 400 psig [...]"
		// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
		if p.nextPlant.ReactorBuilding.ReactorPressureVessel.Outlet.Float() <= RHRSTempThreshold &&
			float64(p.nextPlant.ReactorBuilding.Pressurizer.Pressure-AtmosphericPressure) <
				RHRSPressureThreshold {
			p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Running = true
			p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Running = true
		}
	}

	// EFWS auto-start. The pump-start logic is the most
	// load-bearing safety function of the EFWS; the spec
	// enumerates three independent actuation signals.
	//
	// Spec justification:
	//   - Safety-injection signal: "Safety injection / Motor
	//     driven pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:94-98
	//   - Low-low level in any one SG: "Low-low level in 2/4
	//     level channels in any one steam generator / Motor
	//     driven pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:71-79
	//   - Low-low level in any two SGs: "Low-low level in 2/4
	//     level channels in any two steam generators / Turbine
	//     drive pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:82-92
	//   - Diverse power sources: "The use of both motor driven
	//     and turbine driven pumps satisfies the requirement
	//     that the pumps be powered by diverse power sources."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:3-5
	efws := &p.nextPlant.AuxiliaryBuilding.EmergencyFeedwaterSystem
	if !efws.Active {
		sgWaterLow := p.ReactorBuilding.SteamGenerator.Water.Float() <
			EFWSLevelLowLow
		// The simplified model uses a single combined
		// SteamGenerator field, so "low-low in any one SG"
		// reduces to a single boolean and "low-low in any
		// two SGs" becomes equivalent to "low-low in any
		// one SG". The per-loop SGs are not modelled
		// separately at this fidelity band.
		if p.AuxiliaryBuilding.anyDieselStartSignal(DieselStartSafetyInjection) ||
			p.nextPlant.ReactorBuilding.ReactorProtectionSystem.AuxiliaryFeedPumpsStartActuated ||
			sgWaterLow {
			if !efws.SubsystemA.MotorPump.Running {
				efws.SubsystemA.MotorPump.Running = true
				efws.MotorPumpStartCount++
			}
			if !efws.SubsystemB.MotorPump.Running {
				efws.SubsystemB.MotorPump.Running = true
				efws.MotorPumpStartCount++
			}
			efws.Active = true
			efws.AutoStartElapsed = 0
		}
		if sgWaterLow {
			// Single-SG low-low simultaneously satisfies
			// the "two SG" condition in the simplified
			// model, so the turbine pump starts too.
			if !efws.SubsystemA.TurbinePump.Running {
				efws.SubsystemA.TurbinePump.Running = true
				efws.TurbinePumpStartCount++
			}
			if !efws.SubsystemB.TurbinePump.Running {
				efws.SubsystemB.TurbinePump.Running = true
				efws.TurbinePumpStartCount++
			}
		}
	}

	// EFWS: turbine-driven pump steam admission. The
	// SteamAdmissionValve is fail-open on loss of air or
	// electrical power; whenever an SG header has pressure the
	// valve is open and the pump is ready to drive.
	//
	// Spec justification:
	//   - "The steam supply line for each turbine driven pump
	//     is connected to the main steam line from a steam
	//     generator. This line is fitted with a steam admission
	//     valve which is a pneumatically operated valve arranged
	//     to fail-open on loss of air or electrical power."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
	turbineReady := p.ReactorBuilding.SteamGenerator.Pressure > SteamGeneratorShellPressure/100
	if turbineReady {
		efws.SubsystemA.TurbinePump.SteamAdmissionValve.Open = 1.0
		efws.SubsystemB.TurbinePump.SteamAdmissionValve.Open = 1.0
	} else {
		// Pressure-threshold latch clears on SG depressurization:
		// the fail-open direction covers loss of air/power, not
		// loss of SG pressure (plans/p62_1.txt).
		efws.SubsystemA.TurbinePump.SteamAdmissionValve.Open = 0
		efws.SubsystemB.TurbinePump.SteamAdmissionValve.Open = 0
	}
	efws.SubsystemA.TurbinePump.DriveReady = turbineReady
	efws.SubsystemB.TurbinePump.DriveReady = turbineReady
	// Motor-driven pump DriveReady tracks the ESF bus for
	// each subsystem. The spec assigns each subsystem's
	// motor pump to its corresponding ESF train.
	// "Each subsystem [of the EFWS] receives electrical power
	// from one of two separate Safety Class 1E electrical power
	// trains."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:61-65
	efws.SubsystemA.MotorPump.DriveReady =
		p.nextPlant.Electrical.ESFBusA.Energized
	efws.SubsystemB.MotorPump.DriveReady =
		p.nextPlant.Electrical.ESFBusB.Energized

	// CCWS pumps derive power from the ESF bus; the diesel
	// energises them through the same path as the EFWS motor
	// pumps. DriveReady reflects whether at least one ESF bus
	// is energised; PumpRunning auto-latches when the bus is
	// up so the CCWS Step applies the recovery-lag to the
	// inlet.
	//
	// Spec justification:
	//   - "The CCWS Safeguards subsystem is considered an
	//     engineered safeguards system because it is required
	//     to remove decay heat during postaccident and to
	//     provide cooling water to safeguards equipment."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:135-138
	ccwsDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized
	p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.DriveReady =
		ccwsDriveReady
	// CCWS pump auto-start: latch PumpRunning on first entry
	// to "DriveReady=true" so the Step applies the recovery-
	// lag. DriveReady stays true only while an ESF bus is
	// energised, so power loss simply drops the pump back
	// to "intent without power" without operator action.
	if ccwsDriveReady &&
		!p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning {
		p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	}

	// CSS auto-start: containment spray is initiated in
	// response to a containment high-pressure signal. DriveReady
	// requires at least one ESF bus energised (CSS pumps are
	// 6600V ESF loads); the auto-start only fires when both the
	// actuation signal is present AND DriveReady is true.
	//
	// Spec justification:
	//   - "Initial Injection Mode – Containment spray is
	//     initiated automatically in response to a containment
	//     high pressure signal. This signal starts the spray
	//     pumps, opens the spray header isolation valves "A",
	//     and opens the SAT isolation valves "B"."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
	css := &p.nextPlant.AuxiliaryBuilding.ContainmentSpraySystem
	cssDriveReady := p.nextPlant.Electrical.ESFBusA.StageReady(1) ||
		p.nextPlant.Electrical.ESFBusB.StageReady(1)
	css.DriveReady = cssDriveReady
	// CSS auto-start: latch on the actuation signal
	// regardless of bus state; DriveReady gates the actual
	// spray contribution in the Step function. This matches
	// the AFWPump pattern (latch Running on the start signal;
	// DriveReady gates effective flow).
	if !css.Running &&
		(p.ReactorBuilding.ContainmentBuilding.Pressure > CSSContainmentPressureSetpoint ||
			p.nextPlant.ReactorBuilding.ReactorProtectionSystem.ContainmentSprayActuated) {
		css.Running = true
		css.SprayHeaderOpen = true
		css.SATIsolationValveBOpen = true
	}

	// SWS pumps are fed from the 6600V engineered safety
	// features bus, so they receive power automatically when
	// the diesel (or off-site power) energises the bus on a
	// safety-injection signal. The simplified model latches
	// PumpRunning when the SI signal is present, so the
	// diesel jacket cooling and the RCFC / CCWS heat-rejection
	// paths become active without operator action.
	//
	// The operator control (Outside/ServiceWaterSystem/PumpRunning)
	// remains the manual-test override for normal operation.
	//
	// Spec justification:
	//   - "The larger auxiliaries such as the service water
	//     pumps [...] are fed directly from the 6600-volt
	//     [engineered safety features] buses."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
	//   - "The onsite emergency diesel generators are
	//     started. [...] The HHSI pumps are started." (SI
	//     actuation list — SWS pumps follow the same ESF
	//     bus-energised path.)
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	//   - "The system auxiliary transformers and the standby
	//     diesel generator constitute the normal and redundant
	//     sources of power for the engineered safety features
	//     system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:91-94
	sws := &p.nextPlant.Outside.ServiceWaterSystem
	swsDriveReady := p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized
	sws.DriveReady = swsDriveReady
	// SWS pumps are fed from the 6600V ESF bus, so they
	// only develop flow when the bus is energised. The
	// auto-start latches PumpRunning on the safety signal
	// (operator intent); the actual flow contribution is
	// gated on PumpRunning && DriveReady in SWS.Step.
	if !sws.PumpRunning &&
		p.AuxiliaryBuilding.anyDieselStartSignal(DieselStartSafetyInjection) ||
		!sws.PumpRunning &&
			p.AuxiliaryBuilding.anyDieselStartSignal(DieselStartLossOfOffsitePower) {
		sws.PumpRunning = true
	}

	// RCFC step: each running unit integrates heat rejection
	// against the live containment temperature. The cross-
	// building pass has access to the diesel state (the
	// "LOCA sensed" trigger for the low-speed switchover).
	//
	// Spec justification:
	//   - "When a LOCA is sensed, the fan motors are
	//     automatically switched to low speed to provide proper
	//     flow of the steam-air mixture."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-57
	//   - "The cooling coils remove heat from the air with the
	//     fan providing the required air flow rates."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:84-85
	locaSensed := p.AuxiliaryBuilding.anyDieselStartSignal(DieselStartSafetyInjection)
	cbTemp := p.ReactorBuilding.ContainmentBuilding.Temperature
	for i := 0; i < RCFCUnitCount; i++ {
		p.nextPlant.AuxiliaryBuilding.RCFCUnits[i] =
			p.nextPlant.AuxiliaryBuilding.RCFCUnits[i].Step(dt,
				locaSensed, cbTemp,
				p.Outside.ServiceWaterSystem.InletTemp,
				p.Outside.ServiceWaterSystem.Flow > 0,
			)
	}
	var rcfcHeatOut Watts
	for i := 0; i < RCFCUnitCount; i++ {
		rcfcHeatOut += Watts(
			p.nextPlant.AuxiliaryBuilding.RCFCUnits[i].HeatTransferRate,
		)
	}
	// CSS spray condensation: spray drops condensing steam from
	// the containment atmosphere remove latent heat at spray mass
	// flow × heat of vaporisation.
	//
	// Spec justification:
	//   - "the steam is condensed and falls to the containment
	//     floor as drops of a borated sodium hydroxide solution."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:4-7
	//   - Pressure limiting alongside the fan coolers:
	//     "independently capable of delivering the necessary flow
	//     to limit containment pressure in conjunction with the
	//     Reactor Containment Fan Cooler System."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:29-32
	spray := p.nextPlant.AuxiliaryBuilding.ContainmentSpraySystem
	var cssHeatOut Watts
	if spray.Running && spray.DriveReady && spray.SprayHeaderOpen {
		massKgPerS := spray.SprayFlow * KilogramsPerGPM
		cssHeatOut = Watts(massKgPerS * float64(LatentHeatOfSteamization))
	}
	// Main-steam-line-break enthalpy release: the discharge flashes
	// into the containment atmosphere at the latent heat of
	// vaporisation, the same convention the spray sink uses.
	//   "This malfunction event causes steam pipe break in the main
	//    steam line before the main steam stop valve (MSV) inside
	//    containment"
	//   specs/pwr_sim/chapter_4/page_053.txt:5-6
	breakGalPerStep := p.nextPlant.ReactorBuilding.SteamGenerator.BreakFlow.Float()
	steamBreakHeatIn := Watts(breakGalPerStep / dt.Seconds() *
		KilogramsPerGallon * float64(LatentHeatOfSteamization))
	// Hot-leg LOCA enthalpy release: the primary discharge flashes
	// into containment through the same latent-heat convention.
	// "This break causes a loss of coolant accident (LOCA) event."
	// specs/pwr_sim/chapter_4/page_053.txt:44-45
	locaGalPerStep := p.nextPlant.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float()
	locaHeatIn := Watts(locaGalPerStep / dt.Seconds() *
		KilogramsPerGallon * float64(LatentHeatOfSteamization))
	p.nextPlant.ReactorBuilding.ContainmentBuilding =
		p.nextPlant.ReactorBuilding.ContainmentBuilding.Step(dt,
			steamBreakHeatIn+locaHeatIn-cssHeatOut, rcfcHeatOut,
		)
	if p.nextPlant.ReactorBuilding.PRTRuptureDiscBurst {
		p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure +=
			Pressure(ContainmentVentRatePSIGPerS * dt.Seconds())
	}
	// CSS spray condensation: pressure drag toward atmospheric,
	// the pressure-side companion of the cssHeatOut enthalpy
	// withdrawal above. ContainmentBuilding.Step derives
	// pressure from the thermal state alone; without this drag
	// the PRT-burst term above could never be caught by a
	// running spray (pressure would climb monotonically and
	// the CSS auto-start's catch could never fire). The drag
	// is proportional to the excess over atmosphere, so it
	// removes no mass when already at atmosphere.
	{
		css3 := &p.nextPlant.AuxiliaryBuilding.ContainmentSpraySystem
		if css3.Running && css3.SprayHeaderOpen {
			excess := float64(p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure) -
				float64(AtmosphericPressure)
			if excess > 0 {
				p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure -=
					Pressure(CSSSprayEffectiveness * css3.SprayFlow * excess * dt.Seconds())
				if p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure < AtmosphericPressure {
					p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure
				}
			}
		}
	}
	p.nextPlant.ReactorBuilding.ContainmentPressure =
		p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure
	// CSS auto-start runs after the containment step in
	// Plant.Step; reading the prior tick's mirror here cannot
	// see this tick's burst presssure, so the check runs on the
	// structured field just synced.
	{
		css2 := &p.nextPlant.AuxiliaryBuilding.ContainmentSpraySystem
		if !css2.Running &&
			(p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure > CSSContainmentPressureSetpoint ||
				p.nextPlant.ReactorBuilding.ReactorProtectionSystem.ContainmentSprayActuated) {
			css2.Running = true
			css2.SprayHeaderOpen = true
			css2.SATIsolationValveBOpen = true
		}
	}

	// RPS-driven CIS manual flags. The Reactor Protection
	// System is the "brain" that decides when to actuate
	// containment isolation per Table 7-1; the wiring here
	// sets the CIS ManualPhaseA / ManualPhaseB flags so the
	// CIS step can latch the corresponding phases. This
	// mirrors the existing safety-injection propagation path
	// and extends it with RPS-driven containment spray.
	//
	// Spec justification:
	//   - ESFAS Table 7-1: CIS Phase A on SI or manual.
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:78-80
	//   - CIS Phase B on high containment pressure or manual
	//     containment spray.
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:68-71
	rps := p.nextPlant.ReactorBuilding.ReactorProtectionSystem
	cis := &p.nextPlant.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem
	// The manual flags mirror the RPS ESFAS outputs in both
	// directions so an operator reset of the protection system also
	// releases the isolation demand. The CIS step still requires the
	// underlying condition (SI signal gone, pressure below setpoint)
	// before each phase actually unlatches:
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:68-80
	cis.ManualPhaseA = rps.ContainmentIsolationPhaseAActuated
	cis.ManualPhaseB = rps.ContainmentIsolationPhaseBActuated

	// Containment Isolation System step. Latches Phase A on
	// safety-injection signal or operator manual command;
	// latches Phase B on high containment pressure or operator
	// manual command. The simplified model assumes
	// instantaneous valve closure (no stroke time).
	//
	// Spec justification:
	//   - Phase A actuation: "Containment Isolation Phase A:
	//     Safety injection, Manual."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:74-76
	//   - Phase B actuation: "Containment Isolation Phase B:
	//     High containment pressure, Manual actuation of
	//     containment spray."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_141.txt:64-67
	p.nextPlant.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem =
		p.nextPlant.ReactorBuilding.ContainmentBuilding.ContainmentIsolationSystem.Step(dt,
			locaSensed,
			p.nextPlant.ReactorBuilding.ContainmentBuilding.Pressure-AtmosphericPressure,
		)

	// Hydrogen Detection and Suppression step. Generation is
	// active when the safety-injection signal has fired (LOCA
	// detected); recombiners and ventilation activate
	// automatically via the HDS step once the hydrogen
	// concentration crosses their thresholds with hysteresis.
	//
	// Spec justification:
	//   - Hydrogen source: "Under accident conditions [...]
	//     the Zirconium water reaction can result in the
	//     release of hydrogen gas."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-33
	cb := &p.nextPlant.ReactorBuilding.ContainmentBuilding
	hds := &cb.HydrogenDetectionSuppression
	// H2 generation: thermal-availability- and inventory-driven
	// rather than SI-latched. Source term is zero when the SI signal
	// fires but the primary is still liquid-covered at saturation
	// temperature (small break, injection holding), and nonzero on
	// blowdown-opened uncovery (large break, injection denied,
	// pressure fell). Decay heat (incl. SBO) feeds the ramp only
	// once the gate has opened; without a pressure drop there is
	// no steam to react with.
	//   - LOCA phenomenology: "In the unlikely event of a LOCA, the
	//     SIS is designed to limit increases in fuel clad
	//     temperatures, core geometry distortion, and metal-water
	//     reaction for all breaks up to and including the
	//     double-ended severance of a reactor coolant loop."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_094.txt:52-55
	//   - Source: "Under accident conditions, coolant
	//     radioactivity and the Zirconium water reaction can result
	//     in the release of hydrogen gas to the containment
	//     atmosphere."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:30-33
	rpv := &p.nextPlant.ReactorBuilding.ReactorPressureVessel
	pzr := &p.nextPlant.ReactorBuilding.Pressurizer
	var generationRate float64
	inventoryFrac := rpv.BreakDischarged.Float() / RCSDrainableInventoryGal
	coreHeatW := float64(rpv.TotalThermalPower())
	superheatF := rpv.Outlet.Float() -
		PressurizerSatTemperatureF(float64(pzr.Pressure))
	if superheatF <= HydrogenSuperheatGateF && inventoryFrac < HydrogenInventoryGateFraction {
		generationRate = 0.0
	} else {
		// HydrogenHeatNormW ~= decay heat shortly post-trip at rated
		// power (3800 MWt rated, 6.5% shutdown decay fraction =
		// ~247 MW at full trip). Reads ~0.49 initially, decaying
		// toward ~0.11 by 4h (spec 1.5% decay at 1h); the conservative
		// normalisation deliberately saturates at HydrogenHeatNormW.
		generationRate = HydrogenGenerationRate *
			min(max(inventoryFrac+max(coreHeatW/HydrogenHeatNormW, 0), 0), 1)
	}
	// Containment vent isolation (Table 7-1): with either phase
	// latched, the atmosphere-exchange path through the wall is
	// shut and hydrogen control shifts to the recombiners, which
	// circulate inside containment. The operator flag resumes
	// command when both phases clear.
	//   "Containment Vent Isolation: Safety injection, [...]
	//    Manual actuation of Phase A isolation"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:51-54
	if p.nextPlant.ReactorBuilding.ContainmentBuilding.
		ContainmentIsolationSystem.IsIsolated() {
		hds.VentilationActive = false
	}
	hds.stepHydrogen(cb, dt, generationRate)

	// SWS: the service water picks up the heat rejected by the
	// RCFC coils plus the component cooling heat exchangers, and
	// its inlet tracks Outside.RiverTemp (shared intake house).
	// In the simplified model the lumped SWS flow
	// is the design maximum (all three essential pumps running)
	// at construction; the outlet temperature rises with the
	// imposed heat load.
	//
	// Spec justification:
	//   - "The three essential service water pumps for each
	//     unit are typically rated at approximately 20,000
	//     gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	//   - "Component cooling heat exchangers",
	//     "Containment for ventilation coolers" are typical
	//     SWS loads.
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:81-93
	//   - Shared river intake: "serves one circulating water
	//     pump and one service water pump."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:37-38
	//   - "Diesel generator coolers" join the component cooling HXs
	//     as essential loads when the engine runs with cooler flow.
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89
	ccwDuty := p.nextPlant.AuxiliaryBuilding.ComponentCoolingWaterSystem.HXHeatDuty
	jacketDuty := p.nextPlant.AuxiliaryBuilding.DieselGenerator.JacketHeatDuty +
		p.nextPlant.AuxiliaryBuilding.DieselGeneratorB.JacketHeatDuty
	p.nextPlant.Outside.ServiceWaterSystem =
		p.nextPlant.Outside.ServiceWaterSystem.Step(dt,
			p.Outside.RiverTemp,
			rcfcHeatOut+ccwDuty+jacketDuty,
		)

	// SFPCS: pump auto-start and purification-flow update
	// happen inside AuxiliaryBuilding.Step above, where the
	// upstream-drive readiness signal is plumbed in.
	//
	// Spec justification:
	//   - "Normal operation requires two pumps with the third
	//     pump serving as a standby."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	//   - "passing approximately 100 gpm (23 m3/hr) of each
	//     loop's flow through a filter and demineralizer."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	_ = p.nextPlant.AuxiliaryBuilding.SpentFuelPoolCoolingSystem

	// Flow-modulating valves. Inside the assumed 30-minute
	// no-operator-action window after an automatic start the
	// valves are full open; afterwards they hold the SG level in
	// a narrow band below the high-level trip. Manual starts and
	// the secured state leave the operator's position standing.
	// See EFWS.ApplyModulatingValveLineup for citations.
	efws.ApplyModulatingValveLineup(100.0 * p.ReactorBuilding.SteamGenerator.Water.Float() /
		p.ReactorBuilding.SteamGenerator.Volume.Float())

	// Reactor-to-turbine trip cascade. The spec's protective feature
	// list includes "Turbine trip following a reactor trip" (item 2)
	// and describes the auto-stop-oil mechanism that closes all steam
	// control valves on any trip signal. We therefore latch the
	// turbine into TurbineTripped the same step the reactor trips,
	// matching the auto-stop-oil "hydraulic fluid dumped to drain"
	// behaviour described in the spec.
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:33-35
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
	if p.ReactorBuilding.ReactorPressureVessel.Trip ||
		p.TurbineBuilding.Turbine.State == TurbineTripped {
		p.TurbineBuilding.Turbine.StopValve.Open = 0
		if p.TurbineBuilding.Turbine.State == TurbineRunning {
			p.TurbineBuilding.Turbine.State = TurbineTripped
		}
	}

	// Table 7-1's Turbine-Trip row actuates the machine trip on
	// high steam generator level or safety injection directly -
	// not only through the reactor trip above. A high level alone
	// carries no scram vote beyond the single collapsed channel,
	// so without this consumer the carryover protection never
	// reaches the machine.
	//   "Turbine Trip / Reactor trip / High steam generator level /
	//    Safety injection"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:35-38
	//   "High water level trip signals protect against excessive
	//    water carryover into the turbine."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:27-29
	if p.nextPlant.ReactorBuilding.ReactorProtectionSystem.TurbineTripActuated &&
		p.TurbineBuilding.Turbine.State == TurbineRunning {
		p.TurbineBuilding.Turbine.StopValve.Open = 0
		p.TurbineBuilding.Turbine.State = TurbineTripped
	}

	// Load-runback propagation. The spec's protective-layer output
	// for the DEHM is a "controlled selectable rate" runback on
	// dropped rod / high kw/ft / low DNBR / rapid nuclear flux
	// decrease / rod bottom circuitry. We collapse those four
	// signals into a single LoadRunback bit set when the reactor
	// trips (the most aggressive of the four signals); the
	// per-signal wiring is in scope for a later plan.
	//
	// Spec: "Automatic load runback initiated by a dropped rod
	// signal, high kw/ft signal or low departure from nucleate
	// boiling ratio signal."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
	// The dropped-rod initiator is wired to its named signal: a
	// latched DroppedBank raises the runback immediately, before any
	// trip, so the turbine begins shedding toward house loads while
	// the core folds. The kw/ft, low-DNBR, and flux-rate channels
	// remain deferred (no such instrumentation channels are modeled);
	// the reactor trip keeps raising the same bit as the most
	// aggressive member of the family.
	if p.ReactorBuilding.ReactorPressureVessel.Trip ||
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.DroppedBank {
		p.TurbineBuilding.Turbine.LoadRunback = true
	}

	// Reactor trip also opens the generator breaker so the unit
	// stops exporting power to the grid. Combined with the
	// stop-valve close above, this matches the spec's protective
	// cascade of "Turbine trip following a reactor trip" and the
	// DEHM's automatic load-runback behaviour.
	//
	// Spec: "Turbine trip following a reactor trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:33-35
	if p.ReactorBuilding.ReactorPressureVessel.Trip {
		p.TurbineBuilding.Generator.BreakerClosed = false
	}

	// Grid interface: advance the infinite bus, then latch the
	// grid frequency into the turbine so the droop governor can
	// see it during the next turbine step. Plant.Step is the only
	// writer of GridFrequency; the turbine step only reads.
	//
	// Spec: "computes signals comparing turbine speed and
	// first-stage pressure with reference values established by
	// the dispatcher or initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	p.nextPlant.ReactorBuilding.Grid = p.ReactorBuilding.Grid.Step(dt)
	p.TurbineBuilding.Turbine.GridFrequency = p.nextPlant.ReactorBuilding.Grid.Frequency
	// Dispatch demand reaches the turbine only under the operator's
	// dispatching permissive. LoadDemand is stored in MW (bridge
	// kind "mw"); the conversion to the Setpoint-scale fraction is
	// this division and only this division, so 600 MW on the 1000 MW
	// machine lands at 0.6.
	// "to act as a permissive network so that unsafe demands from
	// the computer [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:10-12
	if p.PlantOperation.LoadFollowActive {
		p.TurbineBuilding.Turbine.LoadDemandRef =
			p.nextPlant.ReactorBuilding.Grid.LoadDemand * WattsPerMW /
				float64(TurbineRatedkW)
	}
	p.TurbineBuilding.Turbine.Synchronized =
		p.nextPlant.TurbineBuilding.Generator.BreakerClosed

	cwPumpRunning := p.Outside.CirculatingWaterPump.Running
	// Condenser flow is gated on both the operator latch
	// (cwPumpRunning) and the upstream-drive readiness
	// (plantDriveReady). A blackout that drops the ESF bus
	// also drops CW flow; the secondary side then has no
	// heat sink and the turbine can no longer accept steam.
	cwFlowing := cwPumpRunning && plantDriveReady
	tavgAfterStep := Temperature{Val: (p.nextPlant.ReactorBuilding.ReactorPressureVessel.Inlet.Float() +
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.Outlet.Float()) / 2,
		Unit: u.Fahrenheit}
	// Steamline isolation forces the main steam stop valves shut
	// until the operator clears the latch; the atmospheric dump
	// valves and any line break sit upstream of these valves and
	// are untouched.
	//   "The main steam stop valves automatically close on low
	//    steam line pressure or on a high-high containment
	//    pressure signal [...]"
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:71-75
	sgIsolated := p.nextPlant.ReactorBuilding.SteamGenerator.SteamlineIsolated
	if p.nextPlant.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationActuated ||
		sgIsolated {
		p.nextPlant.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 0
	}
	// RecirculateValve credits the CST on Outside (plans/p59_1.txt §3).
	p.TurbineBuilding.CSTForRecirc = &p.nextPlant.Outside.CondensateStorageTank.Level
	p.nextPlant.TurbineBuilding = p.TurbineBuilding.StepWithSteam(dt,
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure,
		tavgAfterStep,
		cwFlowing,
		p.nextPlant.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		p.nextPlant.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.Inlet.Float(),
	)
	// Steamline vent and SG shell drain receipts. Both draws land in
	// the condenser hotwell: vented steam condenses on the tube
	// bundle alongside the dump-path steam, and drained shell water
	// is condensate-grade, so secondary mass stays tracked.
	// "the steamline vents are closed and the secondary side of
	//  each steam generator is drained to the normal operating
	//  range."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
	sgDrawn := p.nextPlant.ReactorBuilding.SteamGenerator.VentFlow.Float() +
		p.nextPlant.ReactorBuilding.SteamGenerator.DrainFlow.Float()
	if sgDrawn > 0 {
		p.nextPlant.TurbineBuilding.Condenser.creditHotwell(sgDrawn)
	}
	// Residual invariant: every production credit now routes through
	// creditHotwell, except simulateFluidFlow's setLevel (graph.go:140)
	// which only debits; the exhaustive writer sweep (CHECK_SKILL §9)
	// shows no unrouted positive writer remains.

	// Condenser-effluent radiation monitor: updated here, after
	// StepWithSteam (which runs Condenser.Step), because the
	// condenser step cannot see this tick's leak. One-tick lag is
	// intentional; the monitor is diagnostic-only with a slow decay
	// and never drives the isolate latch.
	{
		leakedGal := p.nextPlant.ReactorBuilding.SteamGenerator.SGTubeRuptureFlow.Float()
		target := 0.0
		if leakedGal > 0 {
			target = 1.0
		}
		alpha := dt.Seconds() / SGTubeRuptureEffluentTauSec
		cond := &p.nextPlant.TurbineBuilding.Condenser
		cond.HotwellRadiation = cond.HotwellRadiation*(1-alpha) + target*alpha
	}

	// Feedwater regulator step. The simplified model collapses
	// the spec's per-SG regulators into a single shared
	// regulator that reads the lumped SteamGenerator (plans/p22_1.txt).
	//
	// The level input is derived from the SG water inventory as
	// a fraction of the design capacity; this is the process
	// variable the regulator drives toward FeedwaterLevelSetpoint.
	//
	// Spec justification:
	//   - "Feedwater flow to each steam generator is controlled
	//     by a feedwater regulator in each feedwater line. The
	//     regulator is controlled by steam generator level,
	//     steam flow and feedwater flow."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
	sg := p.nextPlant.ReactorBuilding.SteamGenerator
	sgLevelPct := 0.0
	if sg.Volume.Float() > 0 {
		sgLevelPct = 100.0 * sg.Water.Float() / sg.Volume.Float()
	}
	fwi := p.nextPlant.ReactorBuilding.ReactorProtectionSystem.
		FeedwaterIsolationActuated || sgIsolated
	reg := p.nextPlant.TurbineBuilding.FeedwaterRegulator
	priorDemand := reg.ValveDemand
	reg.Level = sgLevelPct
	// Both flow inputs read live measurements now: steam side from
	// this tick's measured SG offtake, feed side from the last
	// committed valve demand (the regulator's documented linear
	// flow-vs-position characteristic). The sizing constant fed
	// both inputs a static design value before.
	reg.SteamFlow = p.TurbineBuilding.SteamOfftakeFraction *
		FeedwaterDesignFlow.Float()
	reg.FeedwaterFlow = priorDemand * FeedwaterDesignFlow.Float()
	// Feedwater isolation stands the regulator down: a stopped
	// regulator decays its demand and freezes the integrator, so
	// retransfer to automatic does not bump the valve.
	reg.Running = p.nextPlant.TurbineBuilding.MainFeedwaterPump.Running &&
		!p.nextPlant.TurbineBuilding.Turbine.LoadRunback &&
		!fwi
	reg = reg.Step(dt)
	p.nextPlant.TurbineBuilding.FeedwaterRegulator = reg
	valveDemand := reg.ValveDemand
	if fwi {
		// Belt and suspenders: even a manual-mode regulator cannot
		// hold the valve open against an active isolation.
		valveDemand = 0
	}
	p.nextPlant.TurbineBuilding.MainFeedwaterControlValve.Open = valveDemand

	// Drive the main feedwater control valve to the regulator's
	// demand. The valve position is the regulator's output and
	// gates the effective flow into the SG. RPS tracks
	// FeedwaterIsolationActuated per Table 7-1 (FWI on high SG
	// level, safety injection, or coincidence of reactor trip
	// and low Tcold) and while it stands both the regulator and
	// the valve are forced shut above - main feed returns only
	// after the operator resets the ESFAS latch.
	//
	// Spec justification:
	//   - ESFAS Table 7-1: Feedwater Isolation triggers.
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:73-76
	// Main feedwater replenishment. With the pump running and the
	// regulator in service, the valve demand meters FeedwaterDesign-
	// Flow into the lumped SG and out of the condenser hotwell, so
	// secondary mass stays conserved: steam offtake condenses back
	// into the hotwell, and feed draws it onward to the SG. This
	// closes the gap where only EFWS could add SG inventory.
	//   "The condensate pumps take suction from the condenser
	//    hotwell and pump condensate [...]; the water discharge from
	//    the feedwater pumps flows [...] into the steam generators."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-51
	if reg.Running && !fwi &&
		p.nextPlant.TurbineBuilding.MainFeedwaterPump.Running {
		delivery := reg.ValveDemand *
			FeedwaterDesignFlow.Float() * dt.Seconds()
		sgWater := min(sg.Water.Float()+delivery,
			sg.Volume.Float())
		p.nextPlant.ReactorBuilding.SteamGenerator.Water =
			Volume{Val: sgWater, Unit: u.Gallon}
		hotwell := max(p.nextPlant.TurbineBuilding.Condenser.HotwellLevel.Float()-delivery, 0)
		p.nextPlant.TurbineBuilding.Condenser.HotwellLevel =
			Volume{Val: hotwell, Unit: u.Gallon}
	}

	p.nextPlant.Electrical = p.Electrical.Step(dt)

	// Plant electrical systems step (plans/p27_1.txt). The
	// simplified model rolls offsite availability, diesel
	// start timing, ESF bus transfer, inverter power, and
	// station-blackout declaration into a single struct
	// that lives on ReactorBuilding. Its inputs are
	// OffsitePowerAvailable (typically true at construction)
	// and the auto-start signal handled in the Step function
	// below; its outputs feed downstream consumers (e.g. the
	// RPS and EFWS logic can gate on InverterPowerAvailable
	// when the simplified model is extended).
	//
	// Spec justification:
	//   - "The Engineered Safety Features buses are normally
	//     supplied from the system auxiliary transformers.
	//     In the event of total loss of auxiliary power from
	//     off-site sources, auxiliary power required for
	//     safe shutdown will be supplied from [...] diesel
	//     generators."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:28-33
	p.nextPlant.ReactorBuilding.PlantElectricalSystems =
		p.ReactorBuilding.PlantElectricalSystems.Step(dt,
			p.nextPlant.Electrical.ESFBusA,
			p.nextPlant.Electrical.ESFBusB,
			&p.nextPlant.AuxiliaryBuilding.DieselGenerator,
			&p.nextPlant.AuxiliaryBuilding.DieselGeneratorB,
		)

	// Plant control room step (plans/p28_1.txt). The control
	// room is a passive aggregator: its availability is
	// gated on electrical power, its safety-station status
	// mirrors the ESF bus state, and its alarm is the OR
	// of the RPS trip and the hydrogen-alarm signals.
	//
	// Spec justification:
	//   - Power dependency: "No single failure in the power
	//     distribution system will result in degradation of
	//     the control area functions, except for ancillary
	//     or nonessential performance features."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_146.txt:11-16
	//   - Alarm acknowledgement: "Turned off by operator
	//     acknowledgement."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	pes := &p.nextPlant.ReactorBuilding.PlantElectricalSystems
	alarmCondition :=
		p.nextPlant.ReactorBuilding.ReactorProtectionSystem.ReactorTripped ||
			p.nextPlant.ReactorBuilding.ContainmentBuilding.
				HydrogenDetectionSuppression.HydrogenAlarm ||
			p.nextPlant.TurbineBuilding.GeneratorSCCWAlarm ||
			p.nextPlant.TurbineBuilding.GeneratorSCCWTrip
	pcrIn := p.ReactorBuilding.PlantControlRoom
	pcrIn.PowerAvailable = pes.InverterPowerAvailable
	p.nextPlant.ReactorBuilding.PlantControlRoom = pcrIn.Step(dt,
		alarmCondition,
		p.Alarms(),
		pes.ESFBus1Available,
		pes.ESFBus2Available,
	)

	// Plant computer step (plans/p29_1.txt). The computer
	// is a passive aggregator: it derives data-acquisition
	// availability from the control-room power state, drives
	// its SPDS from the aggregate safety-system status, and
	// arms the post-trip review / sequence-of-events
	// recording on a reactor trip.
	//
	// Spec justification:
	//   - Computer mission: "collects data from many
	//     locations within the plant and then processes it
	//     to provide a variety of information."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
	//   - SPDS: "Presents a succinct display account of the
	//     overall plant safety status."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	//   - Post-trip review:
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_156.txt:1-3
	pcIn := p.ReactorBuilding.PlantComputer
	pcPower := p.nextPlant.ReactorBuilding.
		PlantElectricalSystems.InverterPowerAvailable
	rps = p.nextPlant.ReactorBuilding.ReactorProtectionSystem
	pcAlarm := rps.ReactorTripped ||
		p.nextPlant.ReactorBuilding.ContainmentBuilding.
			HydrogenDetectionSuppression.HydrogenAlarm ||
		p.nextPlant.TurbineBuilding.GeneratorSCCWAlarm ||
		p.nextPlant.TurbineBuilding.GeneratorSCCWTrip
	// Aggregate "safety systems available" is the AND of the
	// RPS being operable, both ESF buses being energised,
	// and the major safeguard systems being active. The
	// simplified model collapses per-system availability
	// flags into this single boolean.
	safetyAvailable := !rps.ReactorTripped &&
		pes.ESFBus1Available && pes.ESFBus2Available
	p.nextPlant.ReactorBuilding.PlantComputer = pcIn.Step(dt,
		pcPower,
		rps.ReactorTripped,
		rps.ReactorTripped && !rps.ManualTrip,
		pcAlarm,
		safetyAvailable,
		p.nextPlant.ReactorBuilding.
			PlantControlRoom.AlarmAcknowledged,
	)

	// Station transformer and house load. The station transformer
	// step reads the generator's terminal voltage (post step)
	// and the grid voltage to decide whether UAT and SAT are
	// energised. The house load step sums fixed demand plus a
	// variable demand that scales with turbine load fraction
	// when the generator is online.
	//
	// Spec: "the plant distribution buses are split and part are
	// supplied by the plant's own generator and the other part
	// are supplied by the system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:54-56
	// "supply power for its own auxiliaries."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
	p.nextPlant.ReactorBuilding.StationTransformer =
		p.nextPlant.ReactorBuilding.StationTransformer.Step(dt,
			float64(p.nextPlant.TurbineBuilding.Generator.Voltage),
			p.nextPlant.ReactorBuilding.Grid.Voltage,
			p.nextPlant.TurbineBuilding.Generator.BreakerClosed,
		)
	loadFraction := 0.0
	if TurbineRatedkW > 0 {
		loadFraction = float64(p.nextPlant.TurbineBuilding.Turbine.RealPower) /
			float64(TurbineRatedkW)
		loadFraction = min(max(loadFraction, 0), 1)
	}
	p.nextPlant.ReactorBuilding.HouseLoad =
		p.nextPlant.ReactorBuilding.HouseLoad.Step(dt,
			p.nextPlant.TurbineBuilding.Generator.BreakerClosed,
			loadFraction,
		)

	// Net generator output after house load. When the breaker is
	// closed, the gross generator output minus house load is what
	// flows to the grid. When the breaker is open, house load is
	// still drawn — from SAT, not the generator — so
	// Generator.RealPower stays at zero (the generator isn't
	// powering anything).
	//
	// Spec: "house loads automatically transferred to the offsite
	// sources of power."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64-66
	if p.nextPlant.TurbineBuilding.Generator.BreakerClosed {
		gross := float64(p.nextPlant.TurbineBuilding.Turbine.RealPower)
		// TotalMW is in MW; convert once to the generator's watt
		// scale here so the subtraction stays one-unit-per-sum.
		net := gross -
			p.nextPlant.ReactorBuilding.HouseLoad.TotalMW*1_000_000
		net = max(net, 0)
		pf := p.nextPlant.TurbineBuilding.Generator.PowerFactor
		if pf <= 0 {
			pf = GeneratorPowerFactor
		}
		q := generatorReactive(net, pf)
		net, q = clampMVA(&p.nextPlant.TurbineBuilding.Generator, net, q)
		p.nextPlant.TurbineBuilding.Generator.RealPower = Watts(net)
		p.nextPlant.TurbineBuilding.Generator.ReactivePower = q
	}
	p.nextPlant.TurbineBuilding.Generator.CumulativeEnergy.
		Add(p.nextPlant.TurbineBuilding.Generator.RealPower, dt)
	p.nextPlant.TurbineBuilding.Turbine.CumulativeEnergy.
		Add(p.nextPlant.TurbineBuilding.Turbine.RealPower, dt)
	p.nextPlant.ReactorBuilding.ReactorPressureVessel.CumulativeThermalEnergy.
		Add(p.nextPlant.ReactorBuilding.ReactorPressureVessel.TotalThermalPower(), dt)

	// Synchronism check: the generator breaker may stay closed only
	// while terminal frequency and voltage match the grid within
	// tolerance. An out-of-step close (or a grid disturbance that
	// pulls the unit out of tolerance) pops the breaker.
	//
	// Spec: DEHM supports "automatic synchronizers" as remote
	// equipment.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:90-92
	gen := &p.nextPlant.TurbineBuilding.Generator
	gridRef := p.nextPlant.ReactorBuilding.Grid
	if gen.BreakerClosed {
		inSync := math.Abs(gen.Frequency-gridRef.Frequency) <= SyncFreqToleranceHz &&
			math.Abs(gen.Voltage-gridRef.Voltage) <= SyncVoltageTolFraction*gridRef.Voltage
		if !inSync {
			gen.BreakerClosed = false
		}
	}
	p.closeDieselBreakersInSync()
	// Diesel cumulative energy records delivered electrical energy:
	// count it after the plant-level synchronizer sets BreakerClosed.
	// A running but unconnected diesel has RealPower capability but
	// delivers no energy to its ESF bus.
	for _, diesel := range []*DieselGenerator{
		&p.nextPlant.AuxiliaryBuilding.DieselGenerator,
		&p.nextPlant.AuxiliaryBuilding.DieselGeneratorB,
	} {
		if diesel.Generator.BreakerClosed {
			diesel.Generator.CumulativeEnergy.Add(diesel.Generator.RealPower, dt)
		}
	}

	// Cross-building DriveReady wiring for the AFW pumps. The fluid
	// graph (see simulateFluidFlow) treats `Running && DriveReady` as
	// the gate for effective flow, so each pump's DriveReady must be
	// latched from its upstream before the graph runs.
	//
	// Spec justification:
	//   - DieselAFWPump: "one steam-driven and one diesel-engine-driven
	//     auxiliary feedwater pump (each of which is safety related)"
	//     specs/westinghouse_tech_manual/chapter_5/page_0527.txt:30-36
	//   - MotorAFWPump: ESF bus is fed by the diesel after LossOfOffsite
	//     (specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:23-36);
	//     until ESF is Energized the motor-driven AFWP cannot deliver flow.
	//   - TurbineAFWPump: "The steam supply line for each turbine driven
	//     pump is connected to the main steam line from a steam
	//     generator. This line is fitted with a steam admission valve
	//     […] arranged to fail-open on loss of air or electrical power."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
	p.nextPlant.Outside.DieselAFWPump.DriveReady =
		p.nextPlant.AuxiliaryBuilding.DieselGenerator.ReadyToLoad ||
			p.nextPlant.AuxiliaryBuilding.DieselGeneratorB.ReadyToLoad
	p.nextPlant.Outside.MotorAFWPump.DriveReady =
		p.nextPlant.Electrical.ESFBusA.Energized
	p.nextPlant.Outside.TurbineAFWPump.DriveReady =
		p.nextPlant.ReactorBuilding.SteamGenerator.Pressure > SteamGeneratorShellPressure/100

	// Plumb the CW inlet temperature from the outside loop. The
	// CirculatingWaterSystem.InletTemp field (plans/p31_1.txt)
	// is the source-side temperature after any cooling-tower
	// approach has been applied; in steady state this tracks
	// RiverTemp for once-through systems.
	//
	// Spec justification:
	//   - "The Circulating Water System serves as the major
	//     heat sink for the power plant by providing large
	//     amounts of cooling water for the main condensers."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:10-13
	p.nextPlant.TurbineBuilding.Condenser.CirculatingWaterIn =
		p.nextPlant.Outside.CirculatingWaterSystem.InletTemp

	// Plant Operation step (plans/p32_1.txt). Drives the
	// operational-mode state machine, the 5%/min loading
	// ramp, the aux-to-main feedwater transition, and the
	// hot-standby RCP cap. The per-SG steam flow fraction is
	// approximated as SG shell pressure over its design value
	// (920 psia at full load) because the simplified model
	// collapses all four SGs into a single lumped
	// SteamGenerator and does not track per-SG mass flow
	// separately.
	//
	// Spec justification:
	//   - 5%/min ramp: "plant loading is continued to full
	//     power at a rate not to exceed 5 percent of full power
	//     per minute."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
	//   - Aux-to-main: "Before reaching the flow capacity of
	//     the auxiliary feedwater system, equivalent to about
	//     3.5-percent steam flow, the steam generators feedwater
	//     supply is switched to the main feedwater system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:129-133
	//   - Hot-standby RCP cap: "a maximum of two out of four
	//     reactor coolant pumps (RCPs) may be secured."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-108
	runningRCPs := p.nextPlant.ReactorBuilding.ReactorCoolantPumps.RunningCount()
	// Steam-flow fraction as an actual offtake fraction of rated
	// capacity: the pressure-ratio proxy formerly used here read
	// "main feedwater in service" whenever the SG sat near design
	// pressure, which is the normal state of every hot lineup.
	steamFlowFraction := p.nextPlant.TurbineBuilding.SteamOfftakeFraction
	// nextPlant is a zero-valued scratch pad; seed the operational
	// layer from the copy this step already advanced (the seed
	// preserves the stepback latch written earlier in the step) so
	// Step advances the lineup the plant actually sits in (mode
	// definitions:
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86).
	p.nextPlant.PlantOperation = p.nextPlant.PlantOperation.Step(dt,
		p.nextPlant.PlantOperation.Mode,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.Outlet,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.Pressure,
		p.nextPlant.ReactorBuilding.ReactorPressureVessel.TotalThermalPower(),
		CoreRatedThermalPower,
		p.nextPlant.AuxiliaryBuilding.ResidualHeatRemovalSystem.Running,
		runningRCPs,
		steamFlowFraction,
		p.nextPlant.TurbineBuilding.Turbine.Setpoint,
	)

	// Reactor power control step (specs/pwr_sim chapter 2.8).
	// Advances the demanded-power-setpoint integrator and, when
	// the operator has engaged automatic rod control, trims rod
	// demand so outlet temperature tracks the Tref program within
	// the deadband. Runs after the RPV/turbine steps so the loop
	// sees this tick's temperatures; the write lands in nextPlant
	// and reaches the rods on the next tick's ControlRods step.
	// Zero-value state (turbine lead, auto off) touches nothing,
	// preserving the pre-RPC behavior.
	p.nextPlant.PlantOperation.StepRPC(
		&p.nextPlant.ReactorBuilding.ReactorPressureVessel,
		&p.nextPlant.TurbineBuilding.Turbine, dt)

	// Pressure-driven fluid flow integrates the freshly-stepped parts.
	// Graph nodes hold pointers into nextPlant; the mutations land in
	// nextPlant directly so Update() copies them into p.
	// Interlock (plans/p62_1.txt): EFWS and the Outside AFW graph
	// edges must not both credit SG.Water in the same tick. EFWS
	// wins; the AFW edges are skipped when EFWS delivered flow.
	simulateFluidFlow(p.BuildGraphFor(p.nextPlant), dt, p.nextPlant.ReactorBuilding.EFWSFeedwaterFlow.Val > 0)
}

// closeDieselBreakersInSync applies each standby diesel's automatic
// synchronizer after the parts have stepped. Against a dark ESF bus the
// breaker closes dead-bus the moment the engine is ready to load,
// preserving the loss-of-offsite-power transfer; against a live bus - the
// paralleled test run - it closes only once terminal voltage and
// frequency match the system nominal, and a closed breaker held out of
// sync pops, mirroring the main-generator out-of-step check above.
//
// The synchronism reference is DieselRatedVoltage / DieselRatedFrequency
// because the lumped model holds every serving source at nominal:
// 6600-V ESF buses
// (specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74)
// at system frequency. Tolerances reuse the main-generator synchronizer
// band; the spec gives no diesel-specific values.
//
// Spec justification:
//   - "Automatic synchronizing equipment is provided to permit parallel
//     operation of each diesel with the normal auxiliary power source
//     during testing and exercising periods."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:65-68
//   - Dead-bus emergency close keeps the safe-shutdown transfer: "auxiliary
//     power required for safe shutdown will be supplied from physically and
//     electrically independent diesel generators located on the site."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:26-29
func (p *Plant) closeDieselBreakersInSync() {
	trains := []struct {
		diesel *DieselGenerator
		bus    *ESFBus
	}{
		{&p.nextPlant.AuxiliaryBuilding.DieselGenerator, &p.nextPlant.Electrical.ESFBusA},
		{&p.nextPlant.AuxiliaryBuilding.DieselGeneratorB, &p.nextPlant.Electrical.ESFBusB},
	}
	for _, t := range trains {
		d := t.diesel
		if d.State != DieselRunning || !d.ReadyToLoad {
			continue
		}
		inSync := math.Abs(d.Generator.Frequency-DieselRatedFrequency) <= SyncFreqToleranceHz &&
			math.Abs(d.Generator.Voltage-DieselRatedVoltage) <= SyncVoltageTolFraction*DieselRatedVoltage
		if !t.bus.Energized || inSync {
			d.Generator.BreakerClosed = true
		} else {
			d.Generator.BreakerClosed = false
		}
	}
}

// mirrorOffsiteAvailability applies the declared offsite-power state
// (ReactorBuilding.PlantElectricalSystems) to the ESF-bus source model:
// declaring a loss of normal electrical power drops both offsite source
// voltages to zero so stepESFBus transfers each bus to its diesel,
// mirroring "automatic transfer of supplies is provided on sensing loss
// of power" (specs/the_westinghouse_pressurized_water_react/
// chapter_11/page_165.txt:56-59) for the condition chapter_16/page_233.txt
// describes at :103-105. The OffsiteA/OffsiteB aliases are re-pointed
// each tick the same way as DieselGenA/B: a JSON round-trip deserializes
// them into separate PowerGenerator values, so a stale alias would
// disconnect the declared state from the buses.
func (p *Plant) mirrorOffsiteAvailability() {
	p.Electrical.OffsiteA = &p.Electrical.OffsiteSourceA
	p.Electrical.OffsiteB = &p.Electrical.OffsiteSourceB
	voltage := DieselRatedVoltage
	if !p.ReactorBuilding.PlantElectricalSystems.OffsitePowerAvailable {
		voltage = 0
	}
	p.Electrical.OffsiteSourceA.Voltage = voltage
	p.Electrical.OffsiteSourceB.Voltage = voltage
}

func (p *Plant) Update() {
	p.ReactorBuilding = p.nextPlant.ReactorBuilding
	p.AuxiliaryBuilding = p.nextPlant.AuxiliaryBuilding
	p.TurbineBuilding = p.nextPlant.TurbineBuilding
	p.Outside = p.nextPlant.Outside
	p.Electrical = p.nextPlant.Electrical
	p.PlantOperation = p.nextPlant.PlantOperation
}

func MakePlant() Plant {
	p := Plant{nextPlant: &Plant{}}
	p.ReactorBuilding.PlantControlRoom.AlarmSet = make(map[AlarmID]Alarm)
	p.ReactorBuilding.SteamGenerator.Volume = Volume{Val: 100000, Unit: u.Gallon}
	p.ReactorBuilding.SteamGenerator.Water = Volume{Val: 80000, Unit: u.Gallon}
	// Initialise the RCS to the design full-load operating point so
	// the Doppler / moderator feedback loop starts in a sane state.
	// "Reactor coolant inlet temperature: 621°F (327°C); outlet:
	// 558°F (292°C)" — values here are inverted for RPV-side use
	// (cold leg from SG inlet / hot leg to SG outlet) per Table 3.4-1.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
	p.ReactorBuilding.ReactorPressureVessel.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	p.ReactorBuilding.ReactorPressureVessel.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	p.ReactorBuilding.ReactorPressureVessel.Pressure = RPVDesignPressure
	// Seed the fission/fission-product inventories to match those
	// full-load coolant conditions: a core holding full-load
	// temperatures must be producing full-load power. Without the
	// seed the multiplicative fission update can never raise power
	// from its zero value, and no operator action could ever start
	// the plant.
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		CoreRatedThermalPower)
	// The secondary side starts at its full-load shell pressure.
	// Leaving it at the zero value arms the low-steamline SI trigger
	// (500 psia) on every fresh plant: "Full load pressure, shell
	// side ... 920 psia (63 bar)".
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_066.txt:36,80
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	// The main steam stop valves start open: the construction
	// default is a lineup delivering steam, and a zero-valued valve
	// would read as isolated.
	// "During operation, the four steam generators deliver
	//  saturated steam through four steam lines to the main
	//  turbine."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:20-23
	p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 1
	// The full-load operating point spins the RCP rotors at rated
	// speed: leaving them at zero would read as a loss of forced
	// flow for the first spin-up seconds and stretch the per-pass
	// temperature rise toward the floored 4x value. Fixtures that
	// model a startup or pump trip drive the speed back down
	// themselves.
	// "Operating speed 1189 rpm"
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:175-177
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = true
		pu.Velocity = RCPRatedSpeed
	}
	p.ReactorBuilding.ReactorCoolantPump.Running = true
	p.ReactorBuilding.ReactorCoolantPump.Velocity = RCPRatedSpeed

	// Pressurizer design defaults: 60% liquid, 40% steam
	// bubble, pressure held by the heaters at
	// PressurizerHeaterTarget. Hot standby holds ~2235 psig
	// (≈2250 psia) with heater-and-spray operation maintaining
	// pressure; without an initialised bubble or heat input the
	// idle-condensation path settles Pressure on the saturation
	// floor of the default coolant temperature, below the
	// RPSLowPressurizerPressureTrip (1800 psi) setpoint.
	// HeatersOn is seeded on so a fresh plant holds its no-load
	// pressure while idled; the heater block only raises toward
	// the target and stands inert above it, so fixtures that
	// override Pressure to a high value keep their state.
	//   "As the reactor coolant temperature increases, operation
	//    of pressurizer heaters and spray maintains the system
	//    pressure in accordance with the applicable
	//    temperature-pressure curves."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:141-145
	p.ReactorBuilding.Pressurizer = Pressurizer{
		Volume:      Volume{Val: 1500, Unit: u.Gallon},
		Water:       Volume{Val: 900, Unit: u.Gallon}, // 60% liquid
		Pressure:    PressurizerHeaterTarget,
		SteamBubble: Volume{Val: 600, Unit: u.Gallon},
		HeatersOn:   true,
	}
	p.TurbineBuilding.HeaterDrainTank = Tank{
		Level:    Volume{Val: 0, Unit: u.Gallon},
		Capacity: Volume{Val: HeaterDrainTankCapacityGal, Unit: u.Gallon},
		Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
	}
	p.Outside = Outside{
		CondensateStorageTank: Tank{
			// Seed above the 239,000-gal Tech Specs operable
			// volume per
			// specs/westinghouse_tech_manual/chapter_5/page_0532.txt:30-41,
			// so the tank is operable at construction.
			Level:    Volume{Val: 300000, Unit: u.Gallon},
			Capacity: Volume{Val: 450000, Unit: u.Gallon},
			Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
		},
		RiverTemp:          Temperature{Val: 75, Unit: u.Fahrenheit},
		AmbientWetBulbTemp: Temperature{Val: OutsideDefaultWetBulbF, Unit: u.Fahrenheit},
	}
	p.AuxiliaryBuilding.DieselGenerator = DieselGenerator{
		Speed: 0,
		Fuel: Tank{
			// Initialise full so the diesel can crank on the
			// first manual start. The state machine
			// (core/diesel.go:47) requires Fuel.Level > 0 to
			// transition Off -> Cranking.
			// Spec minimum "450 gallons at start of day"
			// (core/parts.go:1110-1111); the capacity is sized
			// above that so we leave head-room for the 7-day
			// continuous-rated-load requirement
			// (page_165.txt:120-122).
			Level:    Volume{Val: 500, Unit: u.Gallon},
			Capacity: Volume{Val: 500, Unit: u.Gallon},
			Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
		},
		CoolantTemp: Temperature{Val: 80, Unit: u.Fahrenheit},
	}
	p.AuxiliaryBuilding.DieselGeneratorB = p.AuxiliaryBuilding.DieselGenerator
	p.Outside.TurbineAFWPump.DriveSource = DriveSteam
	p.Outside.MotorAFWPump.DriveSource = DriveElectric
	p.Outside.DieselAFWPump.DriveSource = DriveDiesel

	// Condenser / steam-system defaults (plans/p9_1.txt).
	// "Under normal operation the air is removed by steam jet air
	// ejectors or vacuum pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
	p.TurbineBuilding.Condenser.VacuumPumpRunning = true
	p.TurbineBuilding.Condenser.Pressure = CondenserDesignVacuum
	p.TurbineBuilding.Turbine.StopValve = Valve{Open: 1.0}
	p.Outside.CirculatingWaterPump = Pump{Running: true}

	// Circulating Water System defaults (plans/p31_1.txt).
	// At construction both CW pumps are running at design
	// flow, the source is the once-through river intake (no
	// cooling tower), the cooling-tower fan mirrors the CW
	// pump state, and the inlet temperature tracks
	// Outside.RiverTemp (set below in this MakePlant).
	//
	// Spec justification:
	//   - "Depending on the plant, the source of circulating
	//     water may be a river, lake, ocean, or cooling
	//     tower."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
	//   - "Circulating water for each main condenser is
	//     supplied by vertical, mixed flow, volute type
	//     circulating water pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:56-58
	p.Outside.CirculatingWaterSystem = CirculatingWaterSystem{
		SourceIsCoolingTower: false,
	}
	p.Outside.CoolingTower = CoolingTower{
		FanRunning: false,
		ApproachF:  CoolingTowerApproachF,
		Basin: Tank{
			Level:    Volume{Val: CoolingTowerBasinCapacityGal, Unit: u.Gallon},
			Capacity: Volume{Val: CoolingTowerBasinCapacityGal, Unit: u.Gallon},
		},
		ColdBasinTemp: Temperature{
			Val:  OutsideDefaultWetBulbF + CoolingTowerApproachF,
			Unit: u.Fahrenheit,
		},
		MakeupValve:   Valve{Open: 1.0},
		BlowdownValve: Valve{Open: 1.0},
	}

	// Generator defaults (plans/p12_1.txt).
	//   - BreakerClosed = false at startup; the operator closes
	//     it after synchronisation.
	//   - PowerFactor = 0.9 per design point.
	//   - RatedMVA = 1370 MVA per the spec.
	// Spec: "Rating: 1370 mva"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
	// Spec: "Power factor: 0.9, short circuit ratio: 0.58"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:19
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.TurbineBuilding.Generator.PowerFactor = GeneratorPowerFactor
	p.TurbineBuilding.Generator.RatedMVA = GeneratorRatedMVA

	// Engineered Safety Features bus defaults.
	// "The engineered safety features buses are normally energized
	// from system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:81-82
	// Both train breakers default to closed so the buses follow the
	// spec's "automatic transfer of supplies" path
	// (page_165.txt:57-59) out of the box; the operator can open one
	// to isolate a train.
	p.Electrical.ESFBusA.BreakerClosed = true
	p.Electrical.ESFBusB.BreakerClosed = true

	// Offsite-source generators (one per train). At construction
	// both offsite supplies are available and energised, so the
	// ESF buses are powered from offsite; the diesel acts as a
	// redundant backup if offsite is lost.
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:23-36
	p.Electrical.OffsiteSourceA = PowerGenerator{
		Voltage:       DieselRatedVoltage,
		BreakerClosed: true,
	}
	p.Electrical.OffsiteSourceB = PowerGenerator{
		Voltage:       DieselRatedVoltage,
		BreakerClosed: true,
	}
	p.Electrical.OffsiteA = &p.Electrical.OffsiteSourceA
	p.Electrical.OffsiteB = &p.Electrical.OffsiteSourceB

	// Generator excitation and cooling defaults (plans/p30_1.txt).
	// At construction the exciter is not producing field current
	// (rotor not turning), the SCCW pump is not running, all four
	// hydrogen coolers are at full effectiveness, and the
	// design hydrogen pressure is held. The GeneratorExcitation
	// System and GeneratorCoolingSystem are embedded structs so
	// these assignments reach into the PowerGenerator directly.
	//
	// Spec justification:
	//   - "Excitation: Shaft-driven, air-cooled brushless
	//     exciter of the rotating rectifier type."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:23-24
	//   - "Hydrogen pressure: 60 or 75 psig"
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:20
	//   - "Demineralized cooling water is provided for the
	//     stator by a closed recirculating water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	p.TurbineBuilding.Generator.RegulatorVoltageSetpoint =
		GeneratorRatedVoltage
	p.TurbineBuilding.Generator.ExciterOutput = 0
	p.TurbineBuilding.Generator.ExciterActive = false
	p.TurbineBuilding.Generator.PilotExciterFrequency = 0
	p.TurbineBuilding.Generator.SCCWPumpRunning = false
	p.TurbineBuilding.Generator.SCCWFlow = 0
	p.TurbineBuilding.Generator.HydrogenPressure =
		GeneratorDesignHydrogenPressurePsig
	for i := range p.TurbineBuilding.Generator.
		HydrogenCoolerEffectiveness {
		p.TurbineBuilding.Generator.
			HydrogenCoolerEffectiveness[i] = 1.0
	}

	// Turbine setpoint defaults: 1.0 (full-load demand) when the
	// operator closes the breaker and 0.0 when off-line.
	// RampRate is set to the spec's "controlled selectable rate"
	// (5% per minute, ≈0.000833/s) at construction time.
	p.TurbineBuilding.Turbine.Setpoint = 0
	p.TurbineBuilding.Turbine.RampRate = TurbineLoadRunbackRate

	// Governing valves default wide open so admission follows the
	// stop valve until the DEHM trim takes over at a nonzero
	// reference. The lube-oil pump runs and the header sits at
	// design pressure, matching the pre-startup lineup where the
	// bearing oil pump is verified before rolling the turbine.
	p.TurbineBuilding.Turbine.GoverningValve.Open = 1
	p.TurbineBuilding.Turbine.LubeOilPumpRunning = true
	p.TurbineBuilding.Turbine.BearingOilPressure = BearingOilDesignPressure

	// Grid interface defaults (plans/p13_1.txt). At construction
	// the off-site grid is available (SAT can be energised),
	// the grid sits at 60 Hz / 24 kV, and the house load has
	// the design 3% fixed-plus-variable split.
	// Spec: "Output voltage: 24,000 to 26,000 volts, three-phase,
	// 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	p.ReactorBuilding.Grid = Grid{
		Frequency: GridRatedFrequency,
		Voltage:   GridRatedVoltage,
	}
	p.ReactorBuilding.StationTransformer = StationTransformer{}
	p.ReactorBuilding.HouseLoad = HouseLoad{
		FixedMW:    HouseLoadFixed,
		VariableMW: HouseLoadFixed * HouseLoadVariableFraction,
	}

	// CVCS defaults (plans/p14_1.txt). At construction the
	// charging pump is secured and the letdown valve is shut so
	// a fresh plant neither drains nor fills the pressurizer
	// while idle; an open letdown path with the design flows
	// unbalances the inventory and empties the pressurizer
	// within minutes. The operator lines both up manually to
	// obtain the desired pressurizer water level during
	// startup.
	//   "The charging pump flow and low-pressure letdown valve
	//    are controlled manually to obtain the desired
	//    pressurizer water level."
	//   specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem = CVCS{
		Mode:               CVCSVolumeControl,
		VCTLevel:           Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration: CoreRatedCriticalBoronPpm,
	}

	// RHRS defaults (plans/p15_1.txt). Both pumps are off and
	// the hot-leg isolation valves are closed at construction;
	// the operator aligns the trains and starts the pumps once
	// RCS temp ≤ 350°F and pressure < 400 psig.
	// Spec: "When the reactor coolant temperature has dropped
	// to 350°F (177°C) and the pressure has been reduced to
	// less than approximately 400 psig (28 bar), the RHR
	// equipment is placed in operation."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
	p.AuxiliaryBuilding.ResidualHeatRemovalSystem = RHRS{
		TrainA: RHRSTrain{Pump: Pump{Running: false}},
		TrainB: RHRSTrain{Pump: Pump{Running: false}},
	}

	// SIS defaults (plans/p16_1.txt). The accumulators are
	// full and pressurised at construction; the HHSI pumps
	// are off and aligned to RWST suction; the RWST is full.
	// The model starts the HHSI pumps on the safety-injection
	// actuation signal in Plant.Step.
	//
	// Spec justification:
	//   - "The accumulator tanks are passive components
	//     normally requiring no maintenance, and are located
	//     inside the containment to provide rapid injection of
	//     water following a LOCA."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:11-14
	//   - "The HHSI and RHR pumps are aligned to take suction
	//     directly from the RWST."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
	p.ReactorBuilding.SafetyInjectionSystem = SIS{
		Accumulator: Accumulator{
			Pressure: SISAccumulatorPrechargePressure,
			Level: Volume{Val: SISAccumulatorPerTankCapacity.Float() *
				float64(SISAccumulatorCount), Unit: u.Gallon},
			Capacity: Volume{Val: SISAccumulatorPerTankCapacity.Float() *
				float64(SISAccumulatorCount), Unit: u.Gallon},
		},
		HHSIPumpRunning: false,
		HHSISuction:     SISSuctionRWST,
		RWSTLevel:       Volume{Val: SISRWSTCapacity, Unit: u.Gallon},
	}

	// EFWS defaults (plans/p17_1.txt). At construction the
	// primary tanks are full, both pumps in each subsystem
	// are off, and the EFWS is not active. The auto-start
	// logic in Plant.Step activates the system on low-low SG
	// level or a safety-injection signal.
	//
	// Spec justification:
	//   - "A primary emergency feedwater supply tank, to
	//     which the suction of the emergency feedwater pumps
	//     are normally aligned, is provided in each
	//     subsystem."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:48-51
	//   - "Each tank contains a quantity of condensate
	//     quality water sufficient to allow the plant to be
	//     maintained in hot standby for 13 hours then allow a
	//     5-hour cooldown of the plant to 350°F (176.7°C).
	//     The maximum permissible water temperature in the
	//     primary emergency feedwater supply tank is 120°F
	//     (48.9°C)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-59
	efwsSubsystem := func() EFWSSubsystem {
		return EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				Temperature: EFWSPrimaryTankMaxTemp,
				IsPrimary:   true,
			},
			MotorPump: EFWSPump{
				AFWPump: AFWPump{
					Pump: Pump{Running: false},
				},
				FlowModulatingValve: Valve{Open: 0},
				RecirculationValve:  Valve{Open: 1},
			},
			TurbinePump: EFWSPump{
				AFWPump: AFWPump{
					Pump:        Pump{Running: false},
					DriveSource: DriveSteam,
				},
				FlowModulatingValve: Valve{Open: 0},
				RecirculationValve:  Valve{Open: 1},
			},
		}
	}
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem = EFWS{
		SubsystemA: efwsSubsystem(),
		SubsystemB: efwsSubsystem(),
	}

	// CCWS defaults (plans/p18_1.txt). At construction the
	// inlet temperature sits at the steady-state setpoint
	// (105°F), no pumps are running, and the surge tank is
	// full. The simplified model brings pumps online in
	// Plant.Step when the safety-injection actuation signal
	// arrives.
	//
	// Spec justification:
	//   - "The component cooling water inlet temperature to
	//     various components during normal cooldown is permitted
	//     to increase to 120°F (49°C), however it must return to
	//     105°F (40°C) after four hours."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
	//   - "a standby pump in each train of the Safeguards
	//     subsystem is provided. A spare pump is also available
	//     in the Auxiliary subsystem."
	//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:93-96
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem = CCWS{
		PumpRunning:    false,
		InletTemp:      Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
		OutletTemp:     Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
		SurgeTankLevel: CCWSSurgeTankCapacity,
	}

	// CSS defaults (plans/p19_1.txt). At construction the CSS
	// pumps are off, both the CSST and SAT are full, the spray
	// header is closed, and the containment sits at atmospheric
	// pressure. The simplified model starts the CSS in
	// Plant.Step when containment pressure rises above
	// CSSContainmentPressureSetpoint and the diesel reaches
	// ReadyToLoad.
	//
	// Spec justification:
	//   - "The system consists of a containment spray storage
	//     tank (CSST), a spray additive tank (SAT), two pumps,
	//     two heat exchangers, and a set of spray ring headers."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:14-18
	//   - "(T)he storage tanks are sized to provide approximately
	//     30 minutes of spray with spray additive."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-18
	p.AuxiliaryBuilding.ContainmentSpraySystem = ContainmentSpraySystem{
		Running:                false,
		SprayHeaderOpen:        false,
		SATIsolationValveBOpen: false,
		CSSTLevel:              CSSTCapacity,
		SATLevel:               SATCapacity,
		CSSTLowLevelSetpoint:   Volume{Val: CSSLowLevelSetpoint, Unit: u.Gallon},
		SuctionSump:            false,
		SumpPHAdjusted:         false,
		SprayFlow:              0,
	}
	p.ReactorBuilding.ContainmentPressure = AtmosphericPressure
	p.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure

	// Pressurizer relief path defaults: the PRT starts half full
	// ("contains sufficient water to condense the steam") with an
	// intact rupture disc; the relief block valve is open so the
	// PORV/safety-valve path is available; the PRT spray and both
	// relief valves start closed.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-38
	p.ReactorBuilding.PressurizedReliefTank = Tank{
		Level: Volume{
			Val: PressurizedReliefTankInitialFraction *
				PressurizedReliefTankCapacityGal, Unit: u.Gallon,
		},
		Capacity: Volume{
			Val: PressurizedReliefTankCapacityGal, Unit: u.Gallon,
		},
	}
	p.ReactorBuilding.BlockValve = Valve{Open: 1.0}

	// RCFC defaults (plans/p20_1.txt). At construction all
	// four RCFC units are running at full speed, satisfying
	// the "minimum of two units" safeguard requirement. The
	// service water coil inlet is at the design service-water
	// temperature (60°F).
	//
	// Spec justification:
	//   - "In a containment, the system includes four fan units
	//     that operate in parallel. A minimum of two units must
	//     function to satisfy design requirements for both
	//     normal operation and post-accident operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:19-24
	//   - "Cooling water is supplied by the essential service
	//     water system."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87
	for i := 0; i < RCFCUnitCount; i++ {
		p.AuxiliaryBuilding.RCFCUnits[i] = RCFCUnit{
			Running:             true,
			LowSpeed:            false,
			HeatTransferRate:    0,
			CoilInletTemp:       p.Outside.RiverTemp,
			CoilOutletTemp:      p.Outside.RiverTemp,
			BackdraftDamperOpen: true,
		}
	}

	// ContainmentBuilding defaults. Initialise at atmospheric
	// pressure and 120°F (typical normal-operation atmosphere
	// temperature); design pressure and free volume are the
	// spec-stated values.
	//
	// Spec justification:
	//   - "Transients resulting from the DBA serve as the basis
	//     for a containment design pressure of 60 psig (4.46
	//     bar)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_110.txt:27-30
	//   - "The internal net free volume approximates 2,000,000
	//     cubic feet (56,600 m³)."
	//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_109.txt:11-12
	p.ReactorBuilding.ContainmentBuilding = ContainmentBuilding{
		Pressure:             AtmosphericPressure,
		Temperature:          Temperature{Val: 120.0, Unit: u.Fahrenheit},
		UpperTemperature:     Temperature{Val: 120.0, Unit: u.Fahrenheit},
		LowerTemperature:     Temperature{Val: 120.0, Unit: u.Fahrenheit},
		UpperHydrogenPercent: 0,
		LowerHydrogenPercent: 0,
		DesignPressure:       ContainmentDesignPressure,
		FreeVolume:           ContainmentFreeVolume,
		// CIS defaults (plans/p23_1.txt). At construction
		// neither phase is latched, both manual flags are
		// false, and the design-qualification flags
		// (MissileProtected, SeismicClass1) are recorded
		// as true.
		//
		// Spec justification:
		//   - "missile protected, Seismic Class 1 and
		//     designed for a pressure equal to or greater
		//     than the design pressure of the containment."
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_113.txt:135-138
		ContainmentIsolationSystem: ContainmentIsolationSystem{
			MissileProtected: true,
			SeismicClass1:    true,
		},
		// HDS defaults (plans/p25_1.txt). At construction the
		// containment atmosphere has 0% hydrogen, no alarm,
		// and no active recombiners or ventilation. Generation
		// becomes active in Plant.Step when the
		// safety-injection signal arrives; recombiners and
		// ventilation activate automatically through the HDS
		// step logic.
		//
		// Spec justification:
		//   - Hydrogen alarm: "Alarms at proper setpoints
		//     are provided."
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:14-15
		//   - Three-function defense: detect, ventilate,
		//     recombine.
		//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_117.txt:11-26
		HydrogenDetectionSuppression: HydrogenDetectionSuppression{},
	}

	// SWS defaults (plans/p21_1.txt). At construction all
	// three essential service water pumps are running, the
	// inlet is at the source temperature, and the nonessential
	// headers are aligned (no isolation).
	//
	// Spec justification:
	//   - "The three essential service water pumps for each
	//     unit are typically rated at approximately 20,000
	//     gpm."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	//   - "The nonessential equipment coolers are fed from
	//     separate branch headers that can be isolated from
	//     the main headers under accident conditions."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:23-26
	p.Outside.ServiceWaterSystem = ServiceWaterSystem{
		PumpRunning:          true,
		Flow:                 ServiceWaterDesignFlow,
		InletTemp:            p.Outside.RiverTemp,
		OutletTemp:           p.Outside.RiverTemp,
		NonessentialIsolated: false,
	}

	// SFPCS defaults (plans/p21_1.txt). Two pumps running,
	// header pressure in the normal range (55-75 psig), pool
	// temperature at the design value (100°F), purification
	// flow at 100 gpm.
	//
	// Spec justification:
	//   - "Normal operation requires two pumps with the third
	//     pump serving as a standby."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	//   - "The system pressure is maintained at 55 to 75 psig"
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-46
	p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem = SpentFuelPoolCoolingSystem{
		LoopAPumpRunning: true,
		LoopBPumpRunning: true,
		StandbyRunning:   false,
		HeaderPressure:   SFPCPressureNormalMin,
		PoolTemp:         SFPCPoolDesignTemp,
		PurificationFlow: SFPCPurificationFlow,
	}

	// BRS defaults: the reclaim train stands secured until the
	// operator starts it from the boron recycle panel; holdup tanks
	// start empty and the monitor reads clean.
	// "The BRS is manually operated from the boron recycle panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:66-68
	p.AuxiliaryBuilding.BoronRecycleSystem = BoronRecycleSystem{}

	// Refueling operations defaults (plans/p24_1.txt). At
	// construction the cycle is complete (Phase 5), no cavity
	// flooding, no head/internals removed, the pit is full of
	// borated water at the refueling-cavity concentration,
	// and the core carries FuelAssembliesPerReactor
	// assemblies. A refueling outage starts by operator
	// action (reset Phase to RefuelingPhasePreparation and
	// set CavityFlooding).
	//
	// Spec justification:
	//   - Pit always full of water:
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:25-29
	//   - Borated water for subcriticality:
	//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:17-19
	//   - First-core assembly count:
	//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_040.txt:37-39
	p.ReactorBuilding.RefuelingOperations = RefuelingOperations{
		Phase:                 RefuelingPhaseComplete,
		CavityFloodDepth:      0,
		CavityFlooded:         false,
		PitWaterLevel:         RefuelingCavityMaxDepth,
		PitBoronConcentration: RefuelingPitBoronConcentration,
		FuelAssembliesInCore:  FuelAssembliesPerReactor,
		FuelAssembliesInPit:   0,
		FuelAssembliesShipped: 0,
		HeadRemoved:           false,
		InternalsRemoved:      false,
		WeightSafetyTripped:   false,
		FuelTransferred:       false,
	}

	// RPS defaults (plans/p26_1.txt). At construction the
	// reactor is not tripped, no manual trip has been
	// pressed, no ESFAS output is active.
	//
	// Spec justification:
	//   - "The RPS provides for manual initiation of reactor
	//     trip by operator action in the control room."
	//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
	p.ReactorBuilding.ReactorProtectionSystem = ReactorProtectionSystem{}

	// Plant Electrical Systems defaults (plans/p27_1.txt).
	// At construction offsite power is available, both
	// diesel generators are stopped, both ESF buses are
	// energized from offsite, the instrument inverters are
	// powered, and there is no station blackout.
	//
	// Spec justification:
	//   - "The system auxiliary power is supplied from
	//     independent and electrically isolated buses of
	//     the off-site main distribution grid."
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:45-47
	//   - "automatic transfer of supplies is provided on
	//     sensing loss of power"
	//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
	p.ReactorBuilding.PlantElectricalSystems = PlantElectricalSystems{
		OffsitePowerAvailable:  true,
		Diesel1Running:         false,
		Diesel2Running:         false,
		ESFBus1Available:       true,
		ESFBus2Available:       true,
		InverterPowerAvailable: true,
		StationBlackout:        false,
	}

	// Plant Control Room defaults (plans/p28_1.txt). At
	// construction the control room has electrical power,
	// the PCC and auxiliary panels are operational, all
	// displays are active, control inputs are accepted,
	// and no annunciator alarm is sounding.
	//
	// Spec justification:
	//   - PCC: "all controls, displays and instrumentation
	//     required for normal power operation."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_147.txt:23-26
	//   - Operator controls: "Controls have been segregated
	//     into operating areas which optimize the location
	//     of frequently used and important controls."
	//     specs/the_westinghouse_pressurized_water_react/chapter_8/page_146.txt:1-6
	p.ReactorBuilding.PlantControlRoom = PlantControlRoom{
		PowerAvailable:                true,
		PCCAvailable:                  true,
		AuxiliaryPanelsAvailable:      true,
		GraphicDisplaysAvailable:      true,
		DedicatedDisplaysAvailable:    true,
		AnnunciatorAlarm:              false,
		AlarmAcknowledged:             false,
		ControlInputsAvailable:        true,
		SafetySystemStationsAvailable: true,
	}

	// Plant Computer defaults (plans/p29_1.txt). At
	// construction the computer is on, all display formats
	// are active, SPDS is active, post-trip review has not
	// fired, and no alarm is sounding.
	//
	// Spec justification:
	//   - "The computer collects data from many locations
	//     within the plant and then processes it to provide
	//     a variety of information."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_150.txt:6-12
	//   - SPDS: "Presents a succinct display account of the
	//     overall plant safety status."
	//     specs/the_westinghouse_pressurized_water_react/chapter_9/page_157.txt:34-35
	p.ReactorBuilding.PlantComputer = PlantComputer{
		DataAcquisitionAvailable:           true,
		MimicDisplaysAvailable:             true,
		BarChartDisplaysAvailable:          true,
		TabularDisplaysAvailable:           true,
		GraphicalDisplaysAvailable:         true,
		DataTrendsAvailable:                true,
		PolarDisplaysAvailable:             true,
		SPCSActive:                         true,
		PostTripReviewPrinted:              false,
		SequenceOfEventsRecording:          true,
		AlarmActive:                        false,
		BypassedInoperableStatusIndication: true,
	}

	// Reactor makeup control valve defaults: closed at startup
	// (the operator opens it during cooldown / VCT-level
	// recovery).
	// Spec: "Because of the influx of makeup water from the
	// reactor makeup control system [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
	p.ReactorBuilding.ReactorMakeupControlValve = MakeupValve{
		Open: 0,
		Flow: Volume{Val: 0, Unit: u.Gallon},
	}

	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration =
		CoreRatedCriticalBoronPpm
	// Control rods start fully withdrawn (Insertion=0) for normal
	// operation. A trip forces full insertion (Insertion=1.0) via
	// the clamp in ReactorPressureVessel.Step (plans/x/trip.md).
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0.0

	// Main Feedwater Control System defaults (plans/p22_1.txt).
	// At construction the main feedwater control valve is
	// closed and the regulator's demand is zero until the first
	// Step computes it; the level setpoint sits at FeedwaterLevel-
	// Setpoint. The seeded lineup is the design full-load operating
	// point, so the main feedwater pump runs: without it the seeded
	// steam offtake drains the generators to their low-level trips
	// inside a quarter hour. Plant.Step latches the regulator's
	// Running state from MainFeedwaterPump.Running.
	//
	// Spec justification:
	//   - "Feedwater flow to each steam generator is controlled
	//     by a feedwater regulator in each feedwater line. The
	//     regulator is controlled by steam generator level,
	//     steam flow and feedwater flow."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
	//   - Feed path in service: "The condensate pumps take suction
	//     from the condenser hotwell and pump condensate [...]
	//     The water discharge from the feedwater pumps flows
	//     through two stages of highpressure heaters then into
	//     the steam generators."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-52
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	p.TurbineBuilding.MainFeedwaterControlValve = Valve{Open: 0}
	// The regulator's first Step latches the healthy-lineup position
	// from ValveDemand; a zero at construction would block the
	// HDT->SG edge that now carries inventory (plans/p59_1.txt).
	p.TurbineBuilding.FeedwaterRegulator = FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Running:       false,
		ValveDemand:   0,
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
// (cold leg, hot leg, hot standby) and
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt
// (AFWS pumps, common pipe header).
func (p *Plant) BuildGraphFor(target *Plant) *PlantGraph {
	if p.cachedGraph != nil && p.cachedTarget == target {
		g := p.cachedGraph
		for _, n := range g.Nodes {
			if rec, ok := n.Ref.(flowRecorder); ok {
				rec.resetFlow()
			}
		}
		aux := &target.AuxiliaryBuilding
		sis := &target.ReactorBuilding.SafetyInjectionSystem
		cvcs := &aux.ChemicalVolumeControlSystem
		css := &aux.ContainmentSpraySystem
		ccws := &aux.ComponentCoolingWaterSystem
		sfpc := &aux.SpentFuelPoolCoolingSystem
		genCooling := &target.TurbineBuilding.Generator.GeneratorCoolingSystem
		cvcsDrive := target.Electrical.ESFBusA.Energized || target.Electrical.ESFBusB.Energized
		sfpcRunning := sfpc.LoopAPumpRunning || sfpc.LoopBPumpRunning || sfpc.StandbyRunning
		setPump := func(i int, running, drive bool) {
			p.cachedPumps[i].Running = running
			p.cachedPumps[i].DriveReady = drive
		}
		setPump(0, cvcs.ChargingPumpRunning, cvcsDrive)
		setPump(1, sis.HHSIPumpRunning, sis.HHSIDriveReady)
		setPump(2, css.Running, css.DriveReady)
		setPump(3, cvcs.BTRSRecirculationPumpRunning, cvcsDrive)
		setPump(4, ccws.PumpRunning, ccws.DriveReady)
		setPump(5, sfpcRunning, sfpc.DriveReady)
		setPump(6, genCooling.SCCWPumpRunning, true)
		return g
	}
	g := &PlantGraph{Nodes: map[string]GraphNode{}}

	add := func(id, kind string, ref interface{}) {
		g.Nodes[id] = GraphNode{ID: id, Kind: kind, Ref: ref}
	}
	edge := func(from, to string, valve *Valve, pump *Pump) {
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Valve: valve, Pump: pump})
	}
	flowEdge := func(from, to string, valve *Valve, pump *Pump, flow func(time.Duration) float64) {
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Valve: valve, Pump: pump, Flow: flow})
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
	add("CoolingTower", "CoolingTower", &target.Outside.CoolingTower)
	// Hot return water condenser → tower basin; makeup refills
	// the basin. The CW pumps draw from the basin when the
	// tower is the configured source.
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:1-3
	edge("Condenser", "CoolingTower", nil, nil)
	edge("CoolingTower", "CoolingTower",
		&target.Outside.CoolingTower.MakeupValve, nil)
	add("HDT", "HeaterDrainTank", &target.TurbineBuilding.HeaterDrainTank)
	add("LPHeaterDrain", "LowPressureHeaterDrain",
		&target.TurbineBuilding.LowPressureHeater)
	add("HPHeaterDrain", "HighPressureHeaterDrain",
		&target.TurbineBuilding.HighPressureHeater)
	// Secondary steam path through the turbine (Figure 15.3-1):
	// the turbine stores no inventory, so both legs are
	// Step-owned audit-only Flow edges. The turbine draw is the
	// throttled steam demand; dump delivery bypasses the turbine
	// to the condenser.
	//   "During operation, the four steam generators deliver
	//    saturated steam through four steam lines to the main
	//    turbine. [...] A flow diagram of a typical steam,
	//    condensate, and feedwater system is shown in Figure
	//    15.3-1. [...] The greater portion of the steam is fully
	//    expanded through the turbine before being exhausted to
	//    the condenser."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:20-42
	//   "The main steam lines are provided with a stop valve and a
	//    check valve in each line just outside the containment.
	//    The main steam stop valves automatically close on low steam
	//    line pressure or on a high-high containment pressure signal,
	//    and can be actuated manually from the main control room or
	//    local panels."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:70-79
	// Audit-only legs: the Step already debit/credited the
	// volumes; simulateFluidFlow runs last and must not move
	// them again. The throttled demand is the Step's steam
	// demand; the dump is its dump-delivery. MSIV is a derived
	// lumped-model gate with no direct spec text: the spec
	// stop valves close on low pressure and isolate the header
	// (page_214.txt:70-79 above), and the MSIV admission used
	// for the model matches that isolation behaviour. Assumption;
	// source is silent on a single lumped MSIV field - disclosed
	// here so an injected derived gate is not mistaken for spec
	// backing.
	// Valve wiring: the turbine legs carry the MSIV in the edge's
	// Valve field so classifyPipe sees it as a bearing the flow and
	// DrawPipePathDead shows it shut, and recordGraphFlow zeroes a
	// closed valve. The Flow closure itself must NOT multiply by
	// MSIV again - Turbine.Step already gated admission via
	// stopOpen (plant.go:729-737), so a second factor would square
	// a throttled MSIV. The condenser dump leg carries no Valve
	// field: the DumpValve already gated dumpedGalPerStep inside
	// StepWithSteam, and putting MakeupValve there blocked it in
	// recordGraphFlow whenever makeup was shut. The exhaust leg is
	// unvalved; DumpValve.Open has no bearing on turbine exhaust -
	// dumps bypass via SG->Condenser, dumps close as steam is
	// admitted (page_215.txt:108-113). Exhaust is credited iff the
	// turbine actually through-flowed: Running plus positive
	// demand.
	//
	// Spec: "The greater portion of the steam is fully expanded
	// through the turbine before being exhausted to the condenser."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:39-42
	add("Turbine", "Turbine", &turbineNode{t: &target.TurbineBuilding.Turbine})
	flowEdge("SG", "Turbine", &target.ReactorBuilding.SteamGenerator.MainSteamIsolationValve, nil,
		func(time.Duration) float64 {
			if target.TurbineBuilding.Turbine.State != TurbineRunning {
				return 0
			}
			return target.TurbineBuilding.Turbine.SteamDemandGalPerStep()
		})
	flowEdge("Turbine", "Condenser", nil, nil,
		func(time.Duration) float64 {
			if target.TurbineBuilding.Turbine.State != TurbineRunning {
				return 0
			}
			return target.TurbineBuilding.Turbine.SteamDemandGalPerStep()
		})
	// Steam-dump leg: dumps steam to the condenser when turbine
	// demand cannot take inertial heat.
	//   "This heat is rejected to the condenser through the steam
	//    dump valves and to the atmosphere for the initial phase of
	//    large load reductions. These valves open under a sudden
	//    reduction in turbine-generator load (in excess of 10%) or a
	//    turbine trip."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:85-91
	//   "The main condenser condenses the exhaust from the main
	//    turbines and the two feedwater pump turbines. There is also
	//    sufficient surface to condense steam from the steam dump
	//    system with an acceptable increase in turbine back pressure."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:93-99
	flowEdge("SG", "Condenser", nil, nil,
		func(time.Duration) float64 {
			// Already MSIV- and pressure-gated inside
			// StepWithSteam; return the Step's own value
			// verbatim so audit and ledger cannot diverge.
			if target.TurbineBuilding.Turbine.DumpValve.Open <= 0 {
				return 0
			}
			return target.TurbineBuilding.Turbine.DumpedGalPerStep()
		})
	edge("Condenser", "CST", &target.TurbineBuilding.RecirculateValve, nil)
	// The condensate pumps take suction from the condenser hotwell
	// (the Condenser graph node carries HotwellLevel) and deliver
	// through the LP heater train to the deaerator tank; the pump
	// latch gates the delivery head.
	// "The condensate pumps take suction from the condenser
	//  hotwell and pump condensate [...] through four stages of
	//  low-pressure feedwater heaters to the deaerator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-46
	edge("Condenser", "HDT", nil, &target.TurbineBuilding.CondensatePump)
	// The main-feedwater line is gated by the feedwater control
	// valve so the regulator's demand throttles delivery; without
	// the valve the drain tank either siphons the SG dry through
	// the depressurised suction or overfills it at full pump head.
	// "Feedwater flow to each steam generator is controlled by a
	//  feedwater regulator in each feedwater line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
	edge("HDT", "SG", &target.TurbineBuilding.MainFeedwaterControlValve,
		&target.TurbineBuilding.MainFeedwaterPump)

	// LP heater drains cascade back to the condenser.
	// "Drains from the four low-pressure feedwater heaters are
	// cascaded back to the condenser."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:14-15
	edge("LPHeaterDrain", "Condenser", nil, nil)

	// HP heater drains route to the deaerator tank (HeaterDrainTank).
	// "Drains from the high-pressure feedwater heaters are routed
	// to the deaerator."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-14
	edge("HPHeaterDrain", "HDT", nil, &target.TurbineBuilding.HeaterDrainPump)

	add("DieselAFWPump", "DieselAFWPump", &target.Outside.DieselAFWPump.Pump)
	add("MotorAFWPump", "MotorAFWPump", &target.Outside.MotorAFWPump.Pump)
	add("TurbineAFWPump", "TurbineAFWPump", &target.Outside.TurbineAFWPump.Pump)
	edge("CST", "DieselAFWPump", &target.Outside.DieselAFWPump.CSTValve, nil)
	edge("DieselAFWPump", "SG", &target.Outside.DieselAFWPump.ServiceWaterValve, nil)
	edge("CST", "MotorAFWPump", &target.Outside.MotorAFWPump.CSTValve, nil)
	edge("MotorAFWPump", "SG", &target.Outside.MotorAFWPump.ServiceWaterValve, nil)
	edge("CST", "TurbineAFWPump", &target.Outside.TurbineAFWPump.CSTValve, nil)
	edge("TurbineAFWPump", "SG", &target.Outside.TurbineAFWPump.ServiceWaterValve, nil)

	// Standalone nodes without simulation edges, included so the
	// P&ID graph and Graphviz layout are complete.
	add("EFWSTank", "EmergencyFeedwaterSystem", &target.AuxiliaryBuilding.EmergencyFeedwaterSystem)
	add("CCWSHX", "ComponentCoolingWaterSystem", &target.AuxiliaryBuilding.ComponentCoolingWaterSystem)
	add("RHRS_A", "ResidualHeatRemovalSystem", &target.AuxiliaryBuilding.ResidualHeatRemovalSystem)
	add("RHRS_B", "ResidualHeatRemovalSystem", &target.AuxiliaryBuilding.ResidualHeatRemovalSystem)
	add("SFP", "SpentFuelPool", &target.AuxiliaryBuilding.SpentFuelPoolCoolingSystem)
	add("BRS", "BoronRecycleSystem", &target.AuxiliaryBuilding.BoronRecycleSystem)
	add("Diesel", "DieselGenerator", &target.AuxiliaryBuilding.DieselGenerator)
	add("FuelTank", "FuelTank", nil)
	add("Transformer", "StationTransformer", &target.ReactorBuilding.StationTransformer)
	add("ESF_A", "ESFBus", &target.Electrical.ESFBusA)
	add("ESF_B", "ESFBus", &target.Electrical.ESFBusB)
	add("Grid", "OffsiteGrid", &target.ReactorBuilding.Grid)

	// --- p59_1: Step-owned flow edges (audit-only, plans/p59_1.txt) ---
	// Flow edges are attached only for the integer-layout build;
	// their inventory is already moved by the per-part Steps and
	// simulateFluidFlow runs last (plant.go:2362), so moving it
	// again here would double-count. Wiring captures the ledger's
	// pointers through target, so simulateFluidFlow records on
	// the sink without moving inventory.
	aux := &target.AuxiliaryBuilding
	sis := &target.ReactorBuilding.SafetyInjectionSystem
	cvcs := &aux.ChemicalVolumeControlSystem
	css := &aux.ContainmentSpraySystem
	efws := &aux.EmergencyFeedwaterSystem
	rhrs := &aux.ResidualHeatRemovalSystem

	// Adapter nodes (nodes.go).
	add("RWST", "RefuelingWaterStorageTank", &rwstNode{vol: &sis.RWSTLevel})
	add("Sump", "ContainmentSump", &sumpNode{vol: &sis.Sump})
	add("VCT", "VolumeControlTank", &vctNode{vol: &cvcs.VCTLevel})
	add("CSSTank", "ContainmentSprayStorageTank", &csstNode{vol: &css.CSSTLevel})
	add("SATTank", "SprayAdditiveTank", &satNode{vol: &css.SATLevel})
	add("EFWSTankA", "EmergencyFeedwaterSystem", &efwsTankNode{tank: &efws.SubsystemA.Tank})
	add("EFWSTankB", "EmergencyFeedwaterSystem", &efwsTankNode{tank: &efws.SubsystemB.Tank})
	add("SISAccumulators", "SafetyInjectionSystem", &accNode{vol: &sis.Accumulator.Level, capVol: &sis.Accumulator.Capacity})
	add("PRT", "PressurizerReliefTank", &target.ReactorBuilding.PressurizedReliefTank)
	add("CIS", "ContainmentIsolationSystem", &cisSink{})
	add("InjectionSink", "InjectionSink", &injectionSink{})
	// p63_1: audit nodes for loop circuits whose pumps were
	// renderer-only. All are zero-volume passThroughNodes
	// (nodes.go); Generator is a generatorNode (not a recorder).
	add("BTRS", "BTRSDemineralizer", &passThroughNode{})
	add("SFPHX", "SpentFuelPoolHeatExchanger", &passThroughNode{})
	add("SCCWPump", "StatorCoolingPump", &passThroughNode{})
	add("CCWSSurge", "CCWSSurgeTank", &passThroughNode{})
	add("Generator", "PowerGenerator", &passThroughNode{})

	// EFWS per-subsystem flow edges (replaces Outside AFW edges
	// CST->AFWPump->SG; specs/the_westinghouse_pressurized_water_
	// react/chapter_5/page_099.txt:12-21 — EFWS pumps take
	// suction from the primary tanks, not CST). Teaches the
	// renderer the real topology; Step ledger already debits
	// the tanks (efws.go:117-208).
	flowEdge("EFWSTankA", "SG", &efws.SubsystemA.MotorPump.FlowModulatingValve,
		&efws.SubsystemA.MotorPump.Pump, func(time.Duration) float64 {
			if efws.SubsystemA.MotorPump.Pump.IsRunning() {
				return EFWSMotorPumpFlow.Float() * efws.SubsystemA.MotorPump.FlowModulatingValve.Open
			}
			return 0
		})
	flowEdge("EFWSTankA", "SG", &efws.SubsystemA.TurbinePump.FlowModulatingValve,
		&efws.SubsystemA.TurbinePump.Pump, func(time.Duration) float64 {
			if efws.SubsystemA.TurbinePump.Pump.IsRunning() {
				return EFWSTurbinePumpFlow.Float() * efws.SubsystemA.TurbinePump.FlowModulatingValve.Open
			}
			return 0
		})
	flowEdge("EFWSTankB", "SG", &efws.SubsystemB.MotorPump.FlowModulatingValve,
		&efws.SubsystemB.MotorPump.Pump, func(time.Duration) float64 {
			if efws.SubsystemB.MotorPump.Pump.IsRunning() {
				return EFWSMotorPumpFlow.Float() * efws.SubsystemB.MotorPump.FlowModulatingValve.Open
			}
			return 0
		})
	flowEdge("EFWSTankB", "SG", &efws.SubsystemB.TurbinePump.FlowModulatingValve,
		&efws.SubsystemB.TurbinePump.Pump, func(time.Duration) float64 {
			if efws.SubsystemB.TurbinePump.Pump.IsRunning() {
				return EFWSTurbinePumpFlow.Float() * efws.SubsystemB.TurbinePump.FlowModulatingValve.Open
			}
			return 0
		})

	// SIS accumulator edge (passive, no pump).
	flowEdge("SISAccumulators", "InjectionSink", nil, nil, func(time.Duration) float64 {
		return sis.AccumulatorInjectionFlow.Float()
	})
	// sfp/ccws shortcuts for the p63_1 loop edges below.
	ccws := &aux.ComponentCoolingWaterSystem
	sfpc := &aux.SpentFuelPoolCoolingSystem
	genCooling := &target.TurbineBuilding.Generator.GeneratorCoolingSystem
	// boolPump snapshots (nodes.go); rebuilt against live target
	// on every BuildGraphFor call, so each tick sees current latches.
	// CVCS has no DriveReady field; charging pumps feed from the
	// ESF bus (parts.go:2388-2391), so drive is ESF energised.
	cvcsDrive := target.Electrical.ESFBusA.Energized || target.Electrical.ESFBusB.Energized
	getPump := func(i int, running, drive bool) *Pump {
		if p.cachedPumps[i] == nil {
			p.cachedPumps[i] = &Pump{}
		}
		p.cachedPumps[i].Running = running
		p.cachedPumps[i].DriveReady = drive
		return p.cachedPumps[i]
	}
	chgPump := getPump(0, cvcs.ChargingPumpRunning, cvcsDrive)
	hhsiPump := getPump(1, sis.HHSIPumpRunning, sis.HHSIDriveReady)
	cssPump := getPump(2, css.Running, css.DriveReady)
	btrsPump := getPump(3, cvcs.BTRSRecirculationPumpRunning, cvcsDrive)
	ccwsPump := getPump(4, ccws.PumpRunning, ccws.DriveReady)
	// One loop, three latches: duty A/B share the SFPPA hop;
	// Standby is the SFPPB label on the same edge. Gate is "any".
	sfpcRunning := sfpc.LoopAPumpRunning || sfpc.LoopBPumpRunning || sfpc.StandbyRunning
	sfpcPump := getPump(5, sfpcRunning, sfpc.DriveReady)
	// SCCW drive: generator MCC, derived, spec silent (p63_1).
	// GeneratorCoolingSystem has no DriveReady field; the pump
	// is ready iff commanded on, so drive mirrors the latch.
	sccwPump := getPump(6, genCooling.SCCWPumpRunning, true)

	// HHSI RWST / Sump phase edges share one guard shape; they
	// differ only in the suction source being aligned.
	hsiiGuard := func(src SISSuctionSource) bool {
		return sis.HHSISuction == src && sis.HHSIPumpRunning && sis.HHSIDriveReady
	}
	flowEdge("RWST", "InjectionSink", nil, hhsiPump, func(time.Duration) float64 {
		if hsiiGuard(SISSuctionRWST) {
			return sis.HHSIInjectionFlow.Float()
		}
		return 0
	})
	flowEdge("Sump", "InjectionSink", nil, nil, func(time.Duration) float64 {
		if hsiiGuard(SISSuctionSump) {
			return sis.HHSIInjectionFlow.Float()
		}
		return 0
	})

	// RHRS train edges. ReturnFlow is per-step gallons
	// (rhrs.go:76-78 accumulates * dt.Seconds()); dividing
	// by dt.Seconds() yields gal/s so recordGraphFlow's
	// * dt.Seconds() lands the per-step amount exactly once
	// (CHECK_SKILL.md §2: one scale per sum).
	flowEdge("RHRS_A", "InjectionSink", nil, &rhrs.TrainA.Pump, func(dt time.Duration) float64 {
		if rhrs.TrainA.SuctionValves[0].Open == 1 && rhrs.TrainA.SuctionValves[1].Open == 1 {
			// ReturnFlow is per-step gallons (rhrs.go:76-78
			// accumulates * dt.Seconds()); convert to gal/s so
			// recordGraphFlow's * dt.Seconds() lands the per-step
			// amount exactly once (CHECK_SKILL.md §2: one scale
			// per sum).
			return rhrs.TrainA.ReturnFlow.Float() / dt.Seconds()
		}
		return 0
	})
	flowEdge("RHRS_B", "InjectionSink", nil, &rhrs.TrainB.Pump, func(dt time.Duration) float64 {
		if rhrs.TrainB.SuctionValves[0].Open == 1 && rhrs.TrainB.SuctionValves[1].Open == 1 {
			// Per-step gallons -> gal/s (see TrainA above).
			return rhrs.TrainB.ReturnFlow.Float() / dt.Seconds()
		}
		return 0
	})

	// CVCS letdown / charging / makeup.
	flowEdge("Pressurizer", "VCT", nil, nil, func(time.Duration) float64 {
		if cvcs.LetdownValveOpen {
			return cvcs.LetdownFlow.Float()
		}
		return 0
	})
	flowEdge("VCT", "InjectionSink", nil, chgPump, func(dt time.Duration) float64 {
		if cvcs.ChargingPumpRunning {
			// ChargingFlow is stored gal/s (CVCS.Step sets it
			// directly and integrates * dt.Seconds() at
			// cvcs.go:197); the closure already returns gal/s.
			// Bridge presentation multiplies by 60 for gpm
			// display only. Return raw; recordGraphFlow
			// multiplies by dt.Seconds().
			return cvcs.ChargingFlow.Float()
		}
		return 0
	})
	flowEdge("RWST", "VCT", nil, nil, func(time.Duration) float64 {
		return target.ReactorBuilding.ReactorMakeupControlValve.Flow.Float()
	})

	// CSS CSST/SAT to containment spray.
	flowEdge("CSSTank", "CIS", nil, cssPump, func(time.Duration) float64 {
		if css.SprayHeaderOpen && css.Running && css.DriveReady {
			return css.SprayFlow
		}
		return 0
	})
	flowEdge("SATTank", "CIS", nil, nil, func(time.Duration) float64 {
		if css.SATIsolationValveBOpen && css.SprayHeaderOpen && css.Running && css.DriveReady {
			return css.SprayFlow * (SATCapacity.Float() / CSSTCapacity.Float())
		}
		return 0
	})

	// Pressurizer relief to PRT (PRT is a Tank, already fluidNode).
	flowEdge("Pressurizer", "PRT", nil, nil, func(time.Duration) float64 {
		open := max(target.ReactorBuilding.PORV.Open, target.ReactorBuilding.SafetyValve.Open) * target.ReactorBuilding.BlockValve.Open
		if open <= 0 {
			return 0
		}
		return PressurizerReliefDischargeGalPerSec * open
	})
	flowEdge("CCWSHX", "PRT", nil, nil, func(time.Duration) float64 {
		return PressurizedReliefTankSprayGPS * target.ReactorBuilding.PRTSprayValve.Open
	})

	// p63_1: new audit edges (audit-only; Steps already moved
	// inventory, CHECK_SKILL §5). One scale per sum (§2).
	// CW supply: CoolingTower -> Condenser. FlowGPM is gal/min
	// (circulating_water.go:60); closure converts gpm -> gal/s.
	// Never CondenserDesignCWFlowGPM (20,000 thermal fudge).
	// Spec: "Circulating water for each main condenser is
	// supplied by vertical, mixed flow, volute type circulating
	// water pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:51-58
	flowEdge("CoolingTower", "Condenser", nil, &target.Outside.CirculatingWaterPump,
		func(time.Duration) float64 {
			if target.Outside.CirculatingWaterPump.Running {
				return target.Outside.CirculatingWaterSystem.FlowGPM / 60.0
			}
			return 0
		})
	// BTRS loop: forward Flow audits recirculated volume; return
	// is plain edge() to avoid 2x double-credit (CHECK_SKILL §2).
	// Derived flow VCTBTRSCirculation (gal/s, parts.go:1019),
	// labelled derived; spec silent on numeric rate.
	// "The BTRS recirculation pump provides the motive force
	// to route flow from the VCT through the demineralizers
	// during boration and dilution modes."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-40
	flowEdge("VCT", "BTRS", nil, btrsPump, func(time.Duration) float64 {
		if cvcs.BTRSRecirculationPumpRunning {
			return VCTBTRSCirculation
		}
		return 0
	})
	// Flow (audit-only, returns 0) not plain edge(): passThroughNode
	// pressure() is 0 while vctNode reports PressurizerBasePressure,
	// so a plain edge would compute deltaP != 0 and simulateFluidFlow
	// would drain VCT into the zero-volume sink every tick.
	flowEdge("BTRS", "VCT", nil, nil, func(time.Duration) float64 { return 0 })
	// CCWS loop: no self-loop. Pump seats on surge -> HX.
	// Flow back-solved from HXHeatDuty (ccws.go:108-110):
	// dutyK = CCHXApproachTemp*5/9; massKgPerS = gal/s*60*kg/gpm.
	// gal/s = duty/(cp*dutyK*60*kg/gpm), clamped [0, CCWSDesignFlow].
	// Derived flow; label per plan.
	// "The component cooling water flows from the pump,
	// through the shell side of the component cooling heat
	// exchangers, through the components being cooled, and
	// back to the pump. The surge tank is connected to the
	// suction side of the component cooling pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:51-56
	flowEdge("CCWSSurge", "CCWSHX", nil, ccwsPump, func(time.Duration) float64 {
		if !ccws.PumpRunning {
			return 0
		}
		dutyK := CCHXApproachTemp.Float() * 5.0 / 9.0
		galPerS := float64(ccws.HXHeatDuty) / (SpecificHeatWater * dutyK * 60.0 * KilogramsPerGPM)
		if galPerS < 0 {
			return 0
		}
		if galPerS > CCWSDesignFlow.Float() {
			return CCWSDesignFlow.Float()
		}
		return galPerS
	})
	// SFP loop: pump draws from pool through HX, returns to pool.
	// One loop, two glyphs, one shared hop (SFPPA on forward).
	// Flow derived: spec silent on full circulation rate; only
	// spec-backed figure is 100 gpm/loop purification sidestream
	// (page_171.txt:68-71). Audit uses SFPCPurificationFlow/60.
	// "The pump draws water from the pool, circulates it
	// through the heat exchanger and returns it to the pool."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:53-55
	flowEdge("SFP", "SFPHX", nil, sfpcPump, func(time.Duration) float64 {
		// Gate on IsRunning() (any latch + DriveReady), not the
		// raw PurificationFlow which is seeded non-zero at
		// MakePlant and would otherwise read 1.67 gal/s while idle.
		if sfpcPump.IsRunning() {
			return sfpc.PurificationFlow / 60.0
		}
		return 0
	})
	edge("SFPHX", "SFP", nil, nil)
	// SCCW loop: Generator -> SCCWPump Flow + plain return.
	// Generator is not a flowRecorder; return must not carry Flow.
	// Derived flow GeneratorSCCWDesignFlow (gal/s).
	// "Demineralized cooling water is provided for the stator
	// by a closed recirculating water system"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-49
	flowEdge("Generator", "SCCWPump", nil, sccwPump, func(time.Duration) float64 {
		if genCooling.SCCWPumpRunning {
			return GeneratorSCCWDesignFlow
		}
		return 0
	})
	edge("SCCWPump", "Generator", nil, nil)

	// Cache for the hot Step loop: the graph structure (nodes,
	// edges, closures, adapter pointers) is only a function of
	// the target Plant's address, not of tick-varying state.
	// Reusing it avoids rebuilding the map, slice, ~30 closures
	// and adapter wrappers every tick.
	p.cachedGraph = g
	p.cachedTarget = target
	return g
}

// MarshalJSON omits the internal nextPlant pointer to avoid
// circular references. All public state is included.
func (p *Plant) MarshalJSON() ([]byte, error) {
	type plant Plant
	p2 := plant(*p)
	p2.nextPlant = nil
	return json.Marshal(&p2)
}

// UnmarshalJSON restores a Plant from JSON. The internal
// nextPlant pointer is left nil (rebuilt by Step).
//
// Three redundancies are reconciled so a plant JSON only has to seed
// one copy of each value:
//
//   - ReactorBuilding.Aux aliases the top-level AuxiliaryBuilding
//     during simulation (Step re-points it at nextPlant every tick),
//     but a JSON round-trip decodes it as a detached copy. It is
//     re-pointed at the decoded master here, matching the Step
//     aliasing, so pre-step reads and writes through the mirror hit
//     live state instead of a discarded duplicate.
//   - The RCS boron concentration is stored on the CVCS (master)
//     and on ControlRods (reactivity-model input); Plant.Step
//     propagates the CVCS value every tick, so the ControlRods copy
//     is refreshed from the master here and a stale divergent copy
//     cannot silently win.
//   - Turbine.LastSetpoint is the edge-detector input the dump
//     controller compares against Setpoint; a recorded state whose
//     memory disagrees with its setpoint would replay a phantom or
//     suppressed load-rejection edge. Ingest resumes edge-neutral so
//     the decision belongs to commands written after the load.
func (p *Plant) UnmarshalJSON(data []byte) error {
	type plant Plant
	var p2 plant
	if err := json.Unmarshal(data, &p2); err != nil {
		return err
	}
	*p = Plant(p2)
	p.nextPlant = nil
	if p.ReactorBuilding.Aux != nil {
		*p.ReactorBuilding.Aux = p.AuxiliaryBuilding
	}
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration =
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	// Recorded states are hand-authored or regenerated artifacts, so
	// the turbine's between-step setpoint memory cannot be trusted to
	// match its setpoint. The dump controller reads that memory to
	// detect a sudden load reduction, and a stale value would arm or
	// suppress the rejection on the first step after ingest instead of
	// leaving the decision to commands written after the load.
	//   "These valves open under a sudden reduction in turbine-
	//    generator load (in excess of 10%) or a turbine trip."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
	p.TurbineBuilding.Turbine.LastSetpoint =
		p.TurbineBuilding.Turbine.Setpoint
	return nil
}

// ExportDot writes the plant's graph in Graphviz DOT format.
func (p *Plant) ExportDot(w io.Writer) error {
	return p.BuildGraph().ExportDot(w)
}
