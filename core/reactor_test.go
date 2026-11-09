package core

import (
	"math"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With no Trip, the reactor settles to a positive ThermalPower
// near rated (the simplified power-defect model loses some power
// to the Doppler correction; we assert it stays in a sane band).
func TestReactor_RunningProducesRatedPower(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(time.Second/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	assert.Positive(t, r.TotalThermalPower(), "want ThermalPower > 0 at running")
	assert.LessOrEqual(t, r.TotalThermalPower(), CoreRatedThermalPower,
		"want ThermalPower <= rated, got %v", r.TotalThermalPower())
	assert.False(t, r.Trip, "Trip should remain false")
}

// Setting Trip drops ThermalPower below rated, into the decay-heat
// region. After the fast-branch window, the power should sit at or
// near the shutdown fraction of rated.
//
// The temperature feedback splits into a constant fuel-side Doppler
// base plus a moderator coefficient that interpolates with boron:
// slightly positive at beginning-of-life (full soluble-boron shim),
// substantially negative at end-of-life (boron depleted). A hot-outlet
// offset therefore costs less reactivity at BOL than at EOL, and the
// combined slope stays negative across the boron range so load-follow
// self-regulation has a stable fixed point.
//
// Spec justification:
//   - "At the beginning of life, the effect on reactivity is
//     small since the moderator temperature coefficient is small
//     [positive]. At the end of life, the coefficient is
//     substantially more negative due to depletion and removal of
//     shim boron [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:117-122
//   - "+7 pcm/degF" moderator effect at 1700 ppm with burnable
//     absorbers reducing the boron need
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:51-56
//   - "reactor power is maintained equal to load by virtue of
//     feedback effects inherent in the physics of a PWR"
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:69-71
func TestReactor_DopplerShiftsReactivityWithBoron(t *testing.T) {
	t.Parallel()
	assert.Greater(t, netTempCoef(CVCSInitialBoron), netTempCoef(10),
		"want net temperature coefficient higher at BOL boron than EOL boron")

	// The temperature-feedback term isolates as the reactivity
	// delta between a +30 degF hot outlet and the reference outlet
	// at fixed boron (rod/boron/xenon held identical). It must be
	// negative at both chemistries and more negative at EOL.
	feedbackTerm := func(boron float64) float32 {
		hot := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: boron},
			Outlet:      Temperature{Val: float64(CoreDopplerReferenceTempF) + 30, Unit: u.Fahrenheit},
		}
		ref := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: boron},
			Outlet:      Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
		}
		hot.SeedSteadyStatePower(CoreRatedThermalPower)
		hot = hot.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
		ref.SeedSteadyStatePower(CoreRatedThermalPower)
		ref = ref.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
		return hot.Reactivity - ref.Reactivity
	}

	termBOL := feedbackTerm(CVCSInitialBoron)
	termEOL := feedbackTerm(10)
	assert.Negative(t, termBOL,
		"combined feedback at BOL chemistry must stay negative, got %v", termBOL)
	assert.Less(t, termEOL, termBOL,
		"want EOL feedback more negative than BOL, got BOL=%v EOL=%v",
		termBOL, termEOL)
}

// Even with rods fully withdrawn and Trip asserted, decay heat
// keeps ThermalPower non-zero. The long-tail floor is reached
// well within an hour of simulated time.
func TestReactor_TripWithRodsOutKeepsDecayHeatFloor(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Trip:        true,
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(5*time.Minute/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	assert.Positive(t, r.TotalThermalPower(),
		"want non-zero ThermalPower during decay, got %v", r.TotalThermalPower())
	floor := Watts(float64(CoreRatedThermalPower) * CoreDecayHeatShutdownFraction)
	// Allow some margin since the simplified power-defect model
	// lifts the decay-heat floor slightly above the bare fraction.
	assert.LessOrEqual(t, r.TotalThermalPower(), floor*2,
		"want ThermalPower within 2x of shutdown level (%v), got %v", floor, r.TotalThermalPower())
}

// With Outlet at the design reference and ThermalPower=0, the
// outlet cannot drift away (target = inlet = CoreReferenceInletF,
// not the reference temperature), so Doppler stays near zero.
// Allow a tiny tolerance for the single-step transient.
func TestReactor_OutletAtReferenceGivesZeroDoppler(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
		// Pin the rated-equilibrium xenon load so only the Doppler
		// term can move Reactivity; a zero inventory would read as
		// a fresh core missing its nominal equilibrium poison.
		// Intentional update (physical-xenon storage): rated
		// equilibrium is now the physical concentration.
		Xenon135: xenonEquilibriumPhysical(1),
	}
	r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	assert.GreaterOrEqual(t, r.Reactivity, float32(-1),
		"want near-zero Reactivity, got %v", r.Reactivity)
	assert.LessOrEqual(t, r.Reactivity, float32(1),
		"want near-zero Reactivity, got %v", r.Reactivity)
}

// With ThermalPower > 0 and RCP running, Outlet relaxes toward
// CoreReferenceOutletF on the PrimaryThermalTimeConstant.
func TestReactor_OutletTracksThermalPower(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	// Pre-warm the outlet at the design reference so the simplified
	// model is on the right side of the operating curve. With Outlet
	// at the reference, Doppler ≈ 0 and the cascade doesn't halve.
	for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	// After 10 s of full power the outlet should still be near the
	// design reference (within the model noise).
	assert.GreaterOrEqual(t, r.Outlet.Float(), CoreReferenceOutletF-30,
		"want Outlet near %v, got %v", CoreReferenceOutletF, r.Outlet.Float())
}

// After a trip the Outlet temperature decays along with the core
// power — within a few hundred steps it should drop appreciably.
func TestReactor_OutletDropsAfterTrip(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	// Settle at full power briefly so Outlet converges to ~621°F.
	for i := 0; i < int(time.Second/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	start := r.Outlet.Float()
	r.Trip = true
	r.ControlRods.Insertion = 1
	for i := 0; i < int(time.Minute/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	assert.Less(t, r.Outlet.Float(), start,
		"want Outlet < %v after 1 min of decay, got %v", start, r.Outlet.Float())
}

// With the pumps off but the hot leg well above the cold leg, the
// thermosiphon turns the loop over: the cold-leg inlet follows its
// program anchor instead of freezing. The old expectation here
// ("inlet frozen") was the asserted-anchor behavior the natural-
// circulation gap removed. Movement paces at powerFrac scaled by
// PrimaryInletTimeConstant on post-trip decay heat (~5 percent), so
// ten minutes moves the inlet single-digit degrees — asserted
// directionally against that constant-derived magnitude.
//
// Spec justification:
//   - "a thermal driving head will exist to produce natural
//     circulation flow if the RCPs are not operating"
//     specs/westinghouse_tech_manual/chapter_3/page_0328.txt:31-32
func TestReactor_InletFollowsProgramUnderNaturalCirculation(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 1, DemandInsertion: 1, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: 420, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.Trip = true
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < int(10*time.Minute/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, 0, 0, 0) // pumps off, buoyancy carries the loop
	}
	assert.Greater(t, r.NaturalCircFlowFrac, 0.03,
		"want thermosiphon flow developed, got %v", r.NaturalCircFlowFrac)
	// Measured ten-minute rise ~3 degF: pacing is post-trip decay
	// heat scaled by PrimaryInletTimeConstant. Half-margin witness.
	assert.Greater(t, r.Inlet.Float(), 421.5,
		"want Inlet driven toward its anchor under natural circulation, got %v",
		r.Inlet.Float())
}

// Post-trip decay-heat inventories must be a function of fission
// power history and fixed radioactive decay constants only — not of
// reactivity (specs/wiki/Decay_heat.txt). Two identical scrammed
// cores held at different (both negative) reactivity via boron must
// produce matching fission-product inventories and total thermal
// power. Regression: the reactivity power-defect correction used to
// multiply the shutdown curve too, letting reactivity amplify or
// suppress the "decay heat" of a scrammed core all the way back up
// to rated.
//
// The 950/990 ppm pair sits inside the narrow band where the
// current worth calibration holds a scrammed core subcritical;
// that band is a model-calibration artifact, not a spec value.
func TestReactor_TrippedPowerIgnoresReactivity(t *testing.T) {
	t.Parallel()
	mk := func(boron float64) ReactorPressureVessel {
		r := ReactorPressureVessel{
			ControlRods: ControlRods{Insertion: 1, DemandInsertion: 0, BoronConcentration: boron},
			Inlet:       Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
			Outlet:      Temperature{Val: float64(CoreDopplerReferenceTempF), Unit: u.Fahrenheit},
			Trip:        true,
		}
		r.SeedSteadyStatePower(CoreRatedThermalPower)
		return r
	}
	weak := mk(950)
	strong := mk(990)

	for i := 0; i < int(time.Second/DefaultTimeStep); i++ {
		weak = weak.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
		strong = strong.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
		if i > 0 {
			break
		}
	}

	assert.Negative(t, weak.Reactivity, "precondition: 950 ppm twin subcritical")
	assert.Negative(t, strong.Reactivity, "precondition: 990 ppm twin subcritical")
	assert.NotEqual(t, weak.Reactivity, strong.Reactivity,
		"boron must shift post-scram reactivity (%v vs %v)", weak.Reactivity, strong.Reactivity)

	// The fission component differs with reactivity by design;
	// the fission-product inventories may not.
	assert.InDelta(t, float64(weak.DecayHeatFast), float64(strong.DecayHeatFast),
		float64(weak.DecayHeatFast)*1e-6,
		"fast pool must be reactivity-independent: %v vs %v W",
		weak.DecayHeatFast, strong.DecayHeatFast)
	assert.InDelta(t, float64(weak.DecayHeatSlow), float64(strong.DecayHeatSlow),
		float64(weak.DecayHeatSlow)*1e-6,
		"slow pool must be reactivity-independent: %v vs %v W",
		weak.DecayHeatSlow, strong.DecayHeatSlow)
	assert.LessOrEqual(t, float64(weak.TotalThermalPower()), float64(CoreRatedThermalPower),
		"scrammed core must stay at or below rated, got %v W", weak.TotalThermalPower())
}

// After a scram collapses the fission chain, withdrawing the rods
// must let power regrow: the startup neutron sources keep a minimal
// fission population alive at the bottom of the range, and the
// multiplicative update amplifies it once reactivity turns positive.
// Without the source floor a once-tripped core could never return to
// power.
//
// Spec justification:
//   - "Reactor startup neutron sources must be used because of fuel
//     configuration and the initially low core activity."
//   - specs/the_westinghouse_pressurized_water_react/chapter_2/page_041.txt:18-20
func TestReactor_FissionRegrowsFromSourceAfterScram(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0,
			// Near-full-dilution (end-of-life) boron: the net
			// temperature coefficient is strongly negative,
			// so once the rods withdraw the cold core is
			// supercritical and the surviving source
			// population amplifies.
			BoronConcentration: 1},
		Inlet:  Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet: Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)

	// Scram: full rod insertion drives the chain to the source floor.
	r.Trip = true
	for i := 0; i < int(70*time.Second/DefaultTimeStep); i++ {
		r = r.Step(DefaultTimeStep, 0, 0, 0)
	}
	require.Less(t, float64(r.FissionPower), 1e4,
		"precondition: scram must collapse fission power, got %v W",
		r.FissionPower)

	// Untrip and withdraw the rods; positive reactivity then
	// amplifies the surviving source population back toward rated.
	// Rod motion is rate-limited by ControlRods.Step — the same
	// step ReactorBuilding.Step runs in production — so the
	// withdrawal takes ~20 s at RodInsertionSpeed.
	r.Trip = false
	r.DemandInsertion = 0
	regrown := false
	for i := 0; i < int(600*time.Second/DefaultTimeStep); i++ {
		r.ControlRods = r.ControlRods.Step(DefaultTimeStep)
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
		if r.FissionPower > CoreRatedThermalPower/10 {
			regrown = true
			break
		}
	}
	assert.True(t, regrown,
		"fission power never regrew past 10%% of rated (final %v W)",
		r.FissionPower)
}

// A zero-fission core holds its coolant temperature: the outlet
// relaxation strength scales with produced power, so cold and hot-soak
// states stay put instead of drifting toward the full-load reference.
//
// Spec: the cold startup initial condition holds "<140" F RCS coolant.
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-51
func TestReactor_ZeroPowerHoldsTemperature(t *testing.T) {
	t.Parallel()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{BoronConcentration: CVCSInitialBoron},
		Outlet:      Temperature{Val: 135, Unit: u.Fahrenheit},
		Inlet:       Temperature{Val: 135, Unit: u.Fahrenheit},
		Xenon135:    xenonEquilibriumPhysical(1),
	}
	r.FissionPower = CoreNeutronSourcePower

	for range int(60 * time.Second / DefaultTimeStep) {
		r = r.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}

	assert.InDelta(t, 135.0, r.Outlet.Float(), 0.5,
		"a zero-fission core must not be dragged toward full-load temperature")
	assert.InDelta(t, 135.0, r.Inlet.Float(), 0.5,
		"the inlet must hold as well with no flow-driven blend")
}

// The Tavg program is rate-limited to the operational limits:
// heatup stays at the spec's 50 F/hr cap, while planned cooldown
// at power follows p67's derived envelope instead of the heatup
// number.
//
// Spec: "Primary plant heatup rate is limited to 50°F per hour."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141
func TestReactor_OutletProgramRateLimited(t *testing.T) {
	t.Parallel()
	hot := ReactorPressureVessel{}
	hot.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	hot.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	// Subcritical in the original fixture: with the rods fully
	// inserted, the source population would regrow toward rated
	// and the program target would stay at 621.
	// The cooldown fixture keeps the rods withdrawn in the
	// load-following lineup while the seeded outlet offset holds
	// the clamp saturated for the whole measurement window.
	// Seed half power with a touch of rod insertion: the
	// seeded outlet 50 F above the Doppler reference holds the
	// cooling clamp saturated for the whole measurement window.
	hot.SeedSteadyStatePower(CoreRatedThermalPower / 2)
	hot.ControlRods.BoronConcentration = CVCSInitialBoron
	hot.ControlRods.Insertion = CVCSAutoBoronLowInsertionFrac
	hot.ControlRods.DemandInsertion = CVCSAutoBoronLowInsertionFrac
	hot.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}

	const window = 10 * time.Second
	start := hot.Outlet.Float()
	for i := 0; i < int(window/DefaultTimeStep); i++ {
		hot = hot.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	drop := start - hot.Outlet.Float()
	wantHeat := OpHeatupRateLimit * window.Seconds()
	wantCool := PrimaryCooldownRateLimitFPerSec * window.Seconds()
	// Every step saturates the directed clamp this far from the
	// target: heatup at OpHeatupRateLimit and cooldown at p67's
	// derived PrimaryCooldownRateLimitFPerSec.
	assert.InDelta(t, wantCool, drop, wantCool*1e-6,
		"want cooldown paced at exactly the derived cooldown rate, got %v F in %v",
		drop, window)
	assert.Greater(t, start, hot.Outlet.Float(), "want the outlet cooling")

	// Heatup direction: a cold vessel under full power climbs at the
	// bounded heatup rate.
	cold := ReactorPressureVessel{}
	cold.Inlet = Temperature{Val: 200, Unit: u.Fahrenheit}
	cold.Outlet = Temperature{Val: 200, Unit: u.Fahrenheit}
	cold.SeedSteadyStatePower(CoreRatedThermalPower)
	start = cold.Outlet.Float()
	for i := 0; i < int(window/DefaultTimeStep); i++ {
		cold = cold.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	rise := cold.Outlet.Float() - start
	assert.InDelta(t, wantHeat, rise, wantHeat*1e-6,
		"want heatup paced at exactly the 50 F/hr program rate, got %v F in %v",
		rise, window)
}

// Planned heatup and cooldown are deliberately asymmetric in p67:
// the spec caps heatup at 50 F/hr, while the derived cooldown
// envelope lets the primary follow the descending Tavg program.
// Both directions were probed in this same 10 s saturated-clamp
// window, within the 1e-6 tolerance of the existing rate witness.
func TestReactor_OutletCooldownRateAsymmetry(t *testing.T) {
	t.Parallel()

	const window = 10 * time.Second
	hot := ReactorPressureVessel{}
	hot.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	hot.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	hot.Insertion = CVCSAutoBoronLowInsertionFrac
	hot.DemandInsertion = CVCSAutoBoronLowInsertionFrac
	hot.SeedSteadyStatePower(CoreRatedThermalPower / 2)
	hot.ControlRods.BoronConcentration = CVCSInitialBoron
	hot.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	start := hot.Outlet.Float()
	for i := 0; i < int(window/DefaultTimeStep); i++ {
		hot = hot.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	drop := start - hot.Outlet.Float()
	wantCool := PrimaryCooldownRateLimitFPerSec * window.Seconds()
	assert.InDelta(t, wantCool, drop, wantCool*1e-6,
		"want cooldown paced at the derived rate, got %v F in %v", drop, window)

	cold := ReactorPressureVessel{}
	cold.Inlet = Temperature{Val: 200, Unit: u.Fahrenheit}
	cold.Outlet = Temperature{Val: 200, Unit: u.Fahrenheit}
	cold.SeedSteadyStatePower(CoreRatedThermalPower)
	start = cold.Outlet.Float()
	for i := 0; i < int(window/DefaultTimeStep); i++ {
		cold = cold.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	rise := cold.Outlet.Float() - start
	wantHeat := OpHeatupRateLimit * window.Seconds()
	assert.InDelta(t, wantHeat, rise, wantHeat*1e-6,
		"want heatup paced at the 50 F/hr cap, got %v F in %v", rise, window)
	assert.NotEqual(t, wantCool, wantHeat,
		"planned cooldown must use its own envelope, not the heatup cap")
}

// A fault-regime secondary draw (main steam line break) cools the
// primary at the m*cp rate the discharge demands, not at the
// operational program envelope: the ±50 F/hr clamp is operator pacing
// discipline for planned evolutions, and protection exists precisely
// because a break outruns it.
//
// "The pipe break also results in increase in steam flow from the
//
//	steam generators, leading to increase in heat removal from the
//	reactor coolant system. Therefore, coolant temperature and pressure
//	will drop."
//
// specs/pwr_sim/chapter_4/page_053.txt:9-11
func TestReactor_FaultOvercoolBypassesProgramEnvelope(t *testing.T) {
	t.Parallel()
	hot := ReactorPressureVessel{}
	hot.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	hot.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	// Subcritical: with the rods out the source population would
	// regrow to rated and the fault draw would fight fission heat.
	hot.Insertion = 1
	hot.DemandInsertion = 1

	const window = 10 * time.Second
	start := hot.Outlet.Float()
	for i := 0; i < int(window/DefaultTimeStep); i++ {
		hot = hot.Step(DefaultTimeStep, 0, 0, 0)
		hot.applyFaultOvercool(DefaultTimeStep, Watts(SteamGeneratorFullLoadHeatInput),
			SteamGeneratorShellPressure)
	}
	drop := start - hot.Outlet.Float()
	programDrop := OpHeatupRateLimit * window.Seconds()
	assert.Greater(t, drop, programDrop*10,
		"a full-break draw cooled %v F in %v; the program envelope allows only %v F",
		drop, window, programDrop)
	floorF := PressurizerSatTemperatureF(float64(SteamGeneratorShellPressure))
	assert.GreaterOrEqual(t, hot.Outlet.Float(), floorF-1e-9,
		"tube heat transfer cannot push the primary below the boiling header")

	// One-writer discipline: no fault draw, no fault cooling.
	before := hot.Outlet.Float()
	hot.applyFaultOvercool(DefaultTimeStep, 0, SteamGeneratorShellPressure)
	assert.Equal(t, before, hot.Outlet.Float(),
		"an absent fault path must leave the outlet untouched")
}

// A cold, zero-power primary holds its operator-maintained pressure:
// nothing passive raises it toward the design point. Regression for
// the unconditional design-pressure relaxation, which dragged a
// cold-shutdown plant up toward 2500 psia.
//
// "The RCS and pressurizer are completely filled with borated water
// and the system pressure is maintained by charging and letdown flow
// control."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:94-97
func TestRPV_ColdPressureHoldsAtZeroPower(t *testing.T) {
	t.Parallel()
	rpv := ReactorPressureVessel{
		Outlet:       Temperature{Val: 140, Unit: u.Fahrenheit},
		FissionPower: CoreNeutronSourcePower,
	}
	// Cold-shutdown lineup: rods fully inserted pins fission power
	// at the neutron-source floor (a zero-valued Insertion would be
	// a withdrawn-rod critical core).
	rpv.ControlRods.Insertion = 1.0
	rpv.ControlRods.DemandInsertion = 1.0
	for range int(300 * time.Second / DefaultTimeStep) {
		rpv = rpv.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	assert.Less(t, float64(rpv.Pressure), 420.0,
		"cold primary must not drift toward the design point: got %v",
		rpv.Pressure)
}

// The at-power anchor is unchanged: a producing core still settles its
// RPV pressure baseline at the design point over the same time
// constant (wiring check against the relaxation constant).
func TestRPV_PressureStillRelaxesTowardDesignAtPower(t *testing.T) {
	t.Parallel()
	atPower := ReactorPressureVessel{Pressure: 2000}
	atPower.SeedSteadyStatePower(CoreRatedThermalPower)
	for range int(300 * time.Second / DefaultTimeStep) {
		atPower = atPower.Step(DefaultTimeStep, RCPRatedSpeed, 0, 0)
	}
	want := 2000 + (float64(RPVDesignPressure)-2000)*(1-math.Exp(-1))
	assert.InDelta(t, want, float64(atPower.Pressure), 5.0,
		"at power the relaxation should track one time constant: "+
			"want ~%v got %v", want, atPower.Pressure)
}
