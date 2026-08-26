package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 28 safety injection actuation and post-LOCA recirculation:
// a hot-leg LOCA rapidly depressurizes and voids the RCS; the low
// pressurizer pressure channel votes the reactor trip and actuates SI,
// which starts the diesel-backed HHSI and RHR pumps; the accumulators
// discharge passively below their precharge; RWST depletion aligns the
// pumps to the containment sump automatically; and the operator later
// realigns to simultaneous cold- and hot-leg injection from the control
// board.
//
//   - Catalog: plans/x/scenarios.txt item 28.
//   - "For large pipe ruptures, the RCS would be rapidly depressurized
//     and voided of coolant."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
//   - "The RHR pumps are started."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:140
func TestScenario28_HotLegBreakBlowsDownTripsAndActuatesSI(t *testing.T) {
	t.Parallel()

	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))

	sawTripPressure := 0.0
	for i := 0; i < int(10*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		pzrP := float64(p.ReactorBuilding.Pressurizer.Pressure)
		if !p.ReactorBuilding.ReactorPressureVessel.Trip &&
			pzrP <= float64(RPSLowPressurizerPressureTrip) {
			sawTripPressure = pzrP
		}
		if p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning &&
			p.AuxiliaryBuilding.DieselGenerator.StartSignal == DieselStartSafetyInjection {
			break
		}
	}

	rpv := p.ReactorBuilding.ReactorPressureVessel
	require.True(t, rpv.Trip, "blowdown must vote the reactor trip")
	assert.Greater(t, sawTripPressure, 0.0,
		"trip lands while pressure crosses the setpoint")
	assert.LessOrEqual(t, sawTripPressure, float64(RPSLowPressurizerPressureTrip)+50,
		"scram pressure sits at the low-pressurizer-pressure setpoint")

	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.True(t, rps.SafetyInjectionActuated,
		"low pressurizer pressure actuates safety injection")
	assert.True(t, p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning,
		"the SI signal starts the HHSI pumps")
	assert.Equal(t, DieselStartSafetyInjection,
		p.AuxiliaryBuilding.DieselGenerator.StartSignal,
		"the SI signal auto-starts the emergency diesels")
}

// The actuation list also starts the RHR pumps, but they can only
// develop flow once the RCS is inside the RHRS entrance envelope, so
// the auto-start latch waits for those conditions exactly like an
// operator line-up would.
//
//   - "The RHR pumps are started."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:140
//   - "When the reactor coolant temperature has dropped to 350°F [...]
//     and the pressure has been reduced to less than approximately 400
//     psig [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
func TestScenario28_RHRPumpsAutoStartInsideEntryEnvelope(t *testing.T) {
	t.Parallel()

	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: RHRSTempThreshold - 50, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: RHRSTempThreshold - 50, Unit: u.Fahrenheit}
	p.ReactorBuilding.Pressurizer.Pressure =
		Pressure(RHRSPressureThreshold + AtmosphericPressure - 100)
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorProtectionSystem/ManualSI"].Set("1"))
	stepN(&p, 5)

	rhr := p.AuxiliaryBuilding.ResidualHeatRemovalSystem
	assert.True(t, rhr.TrainA.Running && rhr.TrainB.Running,
		"the SI signal starts the RHR pumps inside the entry envelope")
}

// The break debits primary inventory (pressurizer level falls), flashes
// enthalpy into containment (pressure rises above the seeded ambient),
// and keeps depressurizing through the steam space as the primary voids,
// while an intact plant shows no break flow at all.
//
//   - "Record the break flow in Table V."
//     specs/pwr_sim/chapter_4/page_053.txt:64-65
//   - "For large pipe ruptures, the RCS would be rapidly depressurized
//     and voided of coolant."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
func TestScenario28_BreakDebitsInventoryVoidsAndPressurizesContainment(t *testing.T) {
	t.Parallel()

	baseline := MakePlant()
	stepN(&baseline, 10)
	closedWater := baseline.ReactorBuilding.Pressurizer.Water.Float()
	closedContainment := baseline.ReactorBuilding.ContainmentBuilding.Pressure
	require.Zero(t, baseline.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float(),
		"an intact plant has no break flow")

	p := baseline
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	for i := 0; i < int(45*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		if float64(p.ReactorBuilding.Pressurizer.Pressure) <=
			float64(SISAccumulatorPrechargePressure) {
			break
		}
	}
	require.Less(t, float64(p.ReactorBuilding.ReactorPressureVessel.Pressure),
		float64(SISAccumulatorPrechargePressure),
		"the blowdown must carry the RCS below the accumulator precharge")

	assert.Less(t, p.ReactorBuilding.Pressurizer.Water.Float(), closedWater,
		"break discharge drains primary liquid inventory")
	assert.Greater(t, p.ReactorBuilding.ContainmentBuilding.Pressure, closedContainment,
		"flashing break flow heats the containment atmosphere")
	assert.Greater(t, p.ReactorBuilding.ContainmentBuilding.Pressure,
		AtmosphericPressure+1.0,
		"containment pressure response is observable")

	sis := &p.ReactorBuilding.SafetyInjectionSystem
	require.True(t, sis.Accumulator.Discharging,
		"passive accumulator stage runs below precharge")
	// Wiring check: expected flow recomputed from the same production
	// constants the SIS step uses.
	assert.Equal(t, SISAccumulatorDischargeRate.Float()*
		float64(SISAccumulatorCount), sis.AccumulatorInjectionFlow.Float())
	levelBefore := sis.Accumulator.Level.Float()
	*sis = sis.Step(float64(AtmosphericPressure))
	assert.Less(t, sis.Accumulator.Level.Float(), levelBefore,
		"discharge draws the accumulator bank down")
}

// With the break isolated, ECCS delivery refills the primary ledger
// and retires the same volume from the cumulative break draw - net
// extraction, not gross spill, is what the drain budget bounds.
//
//   - "All pumps will continue to operate and will deliver water to
//     the RCS cold legs without interruption."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:175-177
func TestScenario28_RefloodCreditedAfterBreakIsolation(t *testing.T) {
	t.Parallel()

	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	for i := 0; i < int(45*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
	}
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	require.Less(t, float64(rpv.Pressure),
		float64(SISAccumulatorPrechargePressure),
		"precondition: blowdown must void the primary")
	require.True(t, p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning,
		"precondition: HHSI must be delivering")

	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0"))
	require.NoError(t, cm["ReactorBuilding/Pressurizer/HeatersOn"].Set("0"))
	water0 := p.ReactorBuilding.Pressurizer.Water.Float()
	drawn0 := rpv.BreakDischarged.Float()
	stepN(&p, int(20*time.Second/TimeStep))

	waterGain := p.ReactorBuilding.Pressurizer.Water.Float() - water0
	netRetired := drawn0 - rpv.BreakDischarged.Float()
	assert.Greater(t, waterGain, 100.0,
		"ECCS delivery must refill a voided primary")
	// The surge line shares the ledger while residual cooldown drift
	// moves loop water in and out, so allow a few degrees' worth of
	// surge exchange around the one-for-one retirement.
	surgeSlack := PZRSurgeGalPerF * 5
	assert.InDelta(t, netRetired, waterGain, surgeSlack+1e-6,
		"refilled water retires the break draw one for one")
}

// The break is a bounded drain, not an infinite source: once the
// cumulative discharge reaches the drainable primary inventory the
// per-step BreakFlow must reach zero even with the break wide open.
func TestScenario28_BreakStopsAtDrainableInventory(t *testing.T) {
	t.Parallel()

	r := &ReactorPressureVessel{}
	r.HotLegBreak.Open = 1
	pzr := &Pressurizer{
		Volume: Volume{Val: 1500, Unit: u.Gallon},
		Water:  Volume{Val: 900, Unit: u.Gallon},
		Pressure: RPSLowPressurizerPressureTrip,
	}
	r.BreakDischarged = vol(r.BreakDischarged, RCSDrainableInventoryGal)
	r.applyHotLegBreak(pzr, AtmosphericPressure)

	assert.Zero(t, r.BreakFlow.Float(),
		"an exhausted primary delivers no further break flow")
	assert.InDelta(t, RCSDrainableInventoryGal, r.BreakDischarged.Float(),
		RCSDrainableInventoryGal*1e-9,
		"the cumulative draw never exceeds the drainable inventory")
}

// The recirculation switchover latches once RWST level falls below the
// low-level setpoint with the pumps running, and the subsequent manual
// realignment routes half of the HHSI discharge to the hot legs without
// changing the total injection flow.
//
//   - "initiated automatically when the RWST level channels indicate an
//     RWST level less than a low-level setpoint in conjunction with the
//     initiation of the engineered safeguards actuation signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:149-154
//   - "simultaneous flow to the RCS cold legs and hot legs [...]
//     Switchover to this phase is performed manually from the control
//     board."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:122-138
func TestScenario28_SwitchoverAtRWSTLowThenHotLegRealignment(t *testing.T) {
	t.Parallel()

	stepsPerSecond := int(time.Second / TimeStep)
	// Post-blowdown pressure sits below the HHSI shutoff head, so
	// injection develops and the switchover logic runs.
	s := SIS{
		HHSIPumpRunning: true,
		HHSIDriveReady:  true,
		HHSISuction:     SISSuctionRWST,
		RWSTLevel:       Volume{Val: SISRWSTLowLevel + 10, Unit: u.Gallon},
	}
	for range stepsPerSecond {
		s = s.Step(float64(SISHHSIShutoffHeadPsia - 100))
	}
	assert.False(t, s.SwitchoverComplete,
		"switchover waits for the RWST low-level signal")

	for range 5 * stepsPerSecond {
		s = s.Step(float64(SISHHSIShutoffHeadPsia - 100))
	}
	assert.True(t, s.SwitchoverComplete,
		"RWST low level latches the switchover")
	assert.Equal(t, SISSuctionSump, s.HHSISuction,
		"post-switchover suction draws from the containment sump")

	net := SISHHSIDesignFlow.Float() * (1.0 - SISHHSIMiniFlowFraction)
	assert.InDelta(t, net, s.HHSIInjectionFlow.Float(), net*1e-9)
	assert.Zero(t, s.HHSIHotLegFlow.Float(),
		"no hot-leg share before the manual realignment")

	s.HotLegInjection = true
	s = s.Step(float64(SISHHSIShutoffHeadPsia - 100))
	// Wiring checks: shares recomputed from the same production
	// constants (design flow minus miniflow, half-split fraction).
	assert.InDelta(t, net*SISHotLegInjectionFraction, s.HHSIHotLegFlow.Float(),
		net*1e-9, "half the discharge serves the hot legs after realignment")
	assert.InDelta(t, net, s.HHSIInjectionFlow.Float(), net*1e-9,
		"total HHSI delivery is unchanged by the realignment")
}

// While the break discharges, flashing cools the residual liquid toward
// the saturation temperature of the reduced RCS pressure - the
// mechanism that makes a LOCA transient rapid instead of paced like a
// planned cooldown. With no discharge this tick the writer is inert,
// and a target above the outlet never heats the coolant.
//
//   - "For large pipe ruptures, the RCS would be rapidly
//     depressurized and voided of coolant."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
//   - "Both the vapor and the residual liquid are cooled to the
//     saturation temperature of the liquid at the reduced pressure."
//     specs/wiki/Flash_evaporation.txt:651
func TestRPV_BlowdownCoolingTracksSaturationWhileDischarging(t *testing.T) {
	r := &ReactorPressureVessel{}
	r.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	r.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}

	before := r.Outlet.Float()
	r.applyBlowdownCooling(Pressure(300))
	assert.Equal(t, before, r.Outlet.Float(),
		"no discharge this tick, no flash cooling")

	r.BreakFlow = vol(r.BreakFlow, 100)
	target := PressurizerSatTemperatureF(300)
	const window = 30 * time.Second
	for i := 0; i < int(window/TimeStep); i++ {
		r.applyBlowdownCooling(Pressure(300))
	}
	drop := before - r.Outlet.Float()
	envelope := OpHeatupRateLimit * window.Seconds()
	assert.Greater(t, drop, envelope*10,
		"flashing cooled %v F in %v; the program envelope allows only %v F",
		drop, window, envelope)
	assert.GreaterOrEqual(t, r.Outlet.Float(), target-1e-9,
		"flash cooling relaxes toward saturation without crossing it")

	r.BreakFlow = vol(r.BreakFlow, 50)
	parked := r.Outlet.Float()
	r.applyBlowdownCooling(RPVDesignPressure)
	assert.Equal(t, parked, r.Outlet.Float(),
		"a saturation target above the outlet must never heat")
}

// Full-plant acceptance: during the blowdown the outlet follows the
// collapsing RCS pressure down by orders more than the operational
// envelope permits in the same window (pre-fix code rode the clamp).
func TestScenario28_BlowdownCoolsCoolantPastProgramEnvelope(t *testing.T) {
	t.Parallel()

	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	startF := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()

	const window = 45 * time.Second
	for i := 0; i < int(window/TimeStep); i++ {
		p.Step()
		p.Update()
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	endF := rpv.Outlet.Float()

	drop := startF - endF
	envelope := OpHeatupRateLimit * window.Seconds()
	satEnd := PressurizerSatTemperatureF(float64(p.ReactorBuilding.Pressurizer.Pressure))
	t.Logf("measured: outlet %.1f -> %.1f F (%.1f F drop, envelope %.2f F), final sat target %.1f F",
		startF, endF, drop, envelope, satEnd)
	assert.Greater(t, drop, envelope*20,
		"the blowdown thermal response must outrun the planned-cooldown pace (pre-fix code rides at 1x)")
	assert.Greater(t, drop, 20.0,
		"the coolant must meaningfully follow the depressurization down")
	assert.GreaterOrEqual(t, endF, satEnd-2.0,
		"the outlet relaxes toward, never below, the current saturation point")
}

// ECCS delivery carries storage-temperature enthalpy, not just volume:
// each delivered gallon joins the lumped loop inventory at storage
// temperature, so continued safety-injection flow drives coolant
// temperature toward storage instead of leaving it parked at its
// blowdown value (plans/p36_1.txt).
//
//   - "All pumps will continue to operate and will deliver water to
//     the RCS cold legs without interruption."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:175-177
func TestReactor_ECCSInjectionMixesStorageTemperature(t *testing.T) {
	r := &ReactorPressureVessel{}
	r.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	r.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	pzr := &Pressurizer{
		Volume: Volume{Val: 1500, Unit: u.Gallon},
		Water:  Volume{Val: 900, Unit: u.Gallon},
	}

	before := r.Outlet.Float()
	r.creditSafetyInjection(pzr, 0)
	assert.Equal(t, before, r.Outlet.Float(),
		"zero delivery must not move the energy ledger")

	// Wiring check: expectation recomputed from the same production
	// constants; a tenth of the loop inventory mixes exactly one
	// mixing-fraction of the way toward storage temperature.
	tenth := RCSDrainableInventoryGal / 10
	r.creditSafetyInjection(pzr, tenth)
	want := before + tenth/RCSDrainableInventoryGal*
		(ECCSInjectionTempF-before)
	assert.InDelta(t, want, r.Outlet.Float(), 0.5,
		"the mix must be the exact mass-weighted convex combination")

	for i := 0; i < int(600*time.Second/TimeStep); i++ {
		r.creditSafetyInjection(pzr, tenth/1000)
	}
	assert.Greater(t, r.Outlet.Float(), ECCSInjectionTempF-1e-9,
		"mixing converges toward storage temperature without crossing it")
	assert.Less(t, r.Outlet.Float(), want,
		"sustained delivery keeps pulling the coolant toward storage")
}

// Full-plant differential acceptance: twin plants through an identical
// blowdown and isolated refill differ only in whether their
// accumulators held injectable inventory, so the end-temperature
// separation is attributable to the ECCS volume credit arriving at
// storage temperature instead of as pure volume. The crediting plant
// must end colder by a strictly positive margin - design-flow mixing
// is gentle by derivation (~0.2 F per 30 s against ~419400 gal), so
// the window is sized to accumulate a clear signal. Securing HHSI
// manually is deliberately inexpressible here: while the actuation
// signal stands the model re-latches its pumps.
func TestScenario28_ECCSCreditCoolsRefilledPrimary(t *testing.T) {
	t.Parallel()

	runLeg := func(seedAccumulators bool) float64 {
		p := MakePlant()
		if seedAccumulators {
			p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level =
				Volume{Val: 0, Unit: u.Gallon}
		}
		cm := GetControlMap(&p)
		require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
		for i := 0; i < int(45*time.Second/TimeStep); i++ {
			p.Step()
			p.Update()
		}
		require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0"))
		for i := 0; i < int(120*time.Second/TimeStep); i++ {
			p.Step()
			p.Update()
		}
		return p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	}

	creditedF := runLeg(false)
	dryF := runLeg(true)
	t.Logf("measured: credited %.3f F vs dry-accumulator %.3f F (separation %.3f F)",
		creditedF, dryF, dryF-creditedF)
	assert.Greater(t, dryF-creditedF, 0.3,
		"the ECCS-crediting plant must end measurably colder")
	assert.Greater(t, creditedF, ECCSInjectionTempF,
		"mixing approaches storage temperature asymptotically")
}
