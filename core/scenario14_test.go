package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 14 (power level reduction to 0% FP) durable assertions.
//
// "Initialize the simulator to 100% FP, using 'reactor lead' mode,
// reduce reactor power in 25% steps at 0.5%/sec"
// specs/pwr_sim/chapter_3/page_039.txt:25-27
//
// The evolution runs on the modeled reactor-lead cascade: the operator
// steps the reactor-power target and the rod loop walks fission onto
// it while the turbine load follows the walked demand. Chemistry is
// the slow actuator: the armed automatic boron trim carries the
// standing deficit (boration on this descent) so the bank stays near
// its withdrawal limit, per the Power Control System division of labor
// ("through control rod motions, boron concentration changes, and
// control of the moderator temperature",
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_129.txt:112-115).
// The condenser dump pressure-modulates the transient surplus; its
// settled openings sit well inside the 0.20 band while step transients
// peak near 0.30 (measured at a uniform 10 ms tick).

// scenario14FullPowerPlant extends the shared full-power lineup with
// the scenario's CVCS lineup: the automatic pressurizer level
// controller balances charging against letdown around the
// Tavg-programmed level.
//
// "it must maintain the RCS mass inventory, that is,
// the pressurizer level, by balancing the charging
// flow to the letdown flow."
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:9-12
func scenario14FullPowerPlant() Plant {
	p := scenario10FullPowerPlant()
	p.ReactorBuilding.Pressurizer.HeatersOn = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = true
	return p
}

// Fidelity note: this glide used to require the default TimeStep.
// The reactor's per-step reactivity compounding folded power more
// slowly under a coarser tick against an unchanged setpoint chase, and
// the growing offtake surplus lifted SG header pressure until the
// condenser dump left its <=0.20 pressure-modulation band (measured at
// a uniform 10 ms tick). The compounding is now anchored to
// DefaultTimeStep (see TestReactorFissionPower_WidthInvariance), so
// the whole transient runs wide. Must not run in parallel:
// overrideTimeStep mutates global simulation state.
func TestScenario14_ReactorLeadGlideToZeroFP(t *testing.T) {
	// The descent is one coupled maneuver (rod loop, chemistry, dump
	// modulation) with no passive windows, sampled against measured
	// trajectories at a uniform 10 ms tick. Must not run in parallel:
	// overrideTimeStep mutates global simulation state.
	overrideTimeStep(t, 10*time.Millisecond)
	p := scenario14FullPowerPlant()

	// Reactor-lead power control owns the descent: the operator steps
	// the reactor-power target and the cascade moves rods while the
	// turbine load follows the walked demand
	// (specs/pwr_sim/chapter_3/page_039.txt:25-27; actual-setpoint
	// rule specs/pwr_sim/chapter_2/page_023.txt:70-71).
	setControl(t, &p, "PlantOperation/LeadLagMode", "1")
	// Chemistry follows on the slow path so the bank stays near its
	// withdrawal limit while boration carries the deficit
	// (chapter_7/page_129.txt:112-115).
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoBoronControl",
		"true")

	// The descent legitimately carries the header through protection
	// setpoints late in the glide, so the SI initiation signals are
	// blocked first, as an operator would before a planned cooldown
	// crossing them.
	// "Prior to reaching the automatic safety injection setpoints
	// during cooldown and depressurization, the safety injection
	// initiation signals are blocked."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	setControl(t, &p,
		"ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock",
		"true")

	// Bumpless handoff: shadow the integrator onto achieved power,
	// then engage automatics and enter the descent rate.
	for range int(5 * time.Second / TimeStep) {
		p.Step()
		p.Update()
	}
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	require.InDelta(t, fissionFraction(rpv.FissionPower),
		p.PlantOperation.DemandedPowerSetpoint, 1e-6,
		"shadow phase parked the integrator at achieved power")
	setControl(t, &p, "PlantOperation/LeadLagMode", "1")
	setControl(t, &p, "PlantOperation/RodControlAuto", "true")
	setControl(t, &p, "PlantOperation/PowerRateDemand", "0.005")

	// Guards are sampled every checkEvery-th step: the descent takes
	// minutes while the trip/pressure physics move on second scales,
	// so a 200 ms sample rate cannot miss a regime.
	const checkEvery = 200

	maxDump := 0.0
	prevPF := fissionFraction(rpv.FissionPower)
	for _, tgt := range []struct {
		text  string
		value float64
	}{
		{"0.75", 0.75}, {"0.50", 0.50}, {"0.25", 0.25}, {"0.00", 0.00},
	} {
		setControl(t, &p, "PlantOperation/PowerSetpointTarget", tgt.text)
		settledAt := -1
		var settledPF, settledDump float64
		for i := range int(300 * time.Second / TimeStep) {
			p.Step()
			p.Update()
			if i%checkEvery != 0 {
				continue
			}
			dump := p.TurbineBuilding.Turbine.DumpValve.Open
			if dump > maxDump {
				maxDump = dump
			}
			assert.False(t, rpv.Trip,
				"reactor must not trip during the glide")
			assert.Zero(t, p.ReactorBuilding.SteamGenerator.SafetyValve.Open,
				"SG safety valves stay seated through the controlled descent")
			dem := p.PlantOperation.DemandedPowerSetpoint
			if absF(dem-tgt.value) <= 1e-6 &&
				absF(fissionFraction(rpv.FissionPower)-dem) <= 0.05 {
				settledAt = i
				settledPF = fissionFraction(rpv.FissionPower)
				settledDump = dump
				break
			}
		}
		require.GreaterOrEqual(t, settledAt, 0,
			"power must settle onto the %v target within the horizon", tgt.text)
		assert.LessOrEqual(t, settledPF, prevPF-0.15,
			"each 25-point demand step must walk the plateau down")
		// Settled surplus is ~zero, so the modulating dump stands
		// back down inside its band once production tracks demand;
		// transient step peaks ride higher by design (measured 0.295)
		// but must never pin at capacity.
		assert.LessOrEqual(t, settledDump, 0.20,
			"settled condenser dump stays inside its modulation band")
		assert.Less(t, maxDump, 0.999,
			"the modulating dump never pins at capacity")
		prevPF = settledPF
	}
	require.LessOrEqual(t, prevPF, 0.10,
		"the glide reaches the 0% FP neighborhood")

	// Guarantee the subcritical end state: take the reactor loop back
	// to manual and insert to shutdown margin while unloading the
	// turbine to house-load level.
	//
	// "If the plant is to be placed in a subcritical hot standby
	// condition, the control rods are inserted to reduce power level"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:20-23
	//
	// Subcritical hot standby needs real shutdown margin, not a
	// trim worth: a couple percent of insertion holds criticality
	// once the moderator warms toward no-load temperature, so the
	// demand drives the banks deep enough that rod worth dominates
	// the end-of-cooldown positive Doppler swing.
	setControl(t, &p, "PlantOperation/RodControlAuto", "false")
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl",
		"false")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoBoronControl",
		"false")
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion",
		"0.60")
	setControl(t, &p, "TurbineBuilding/Turbine/Setpoint", "0.04")
	// The hold outlasts the chain's decay, not the rod travel:
	// rods seat inside ~0.2 s, and at rho ~ -0.07 pct dk/k the
	// multiplicative fission update folds power with an e-fold
	// near 1.4 s, so reaching the source floor from the post-glide
	// ~1.4 MW takes ~14 s. The Tavg pacing (outlet temperature
	// moves at most 50 F/hr) holds the doppler term fixed through
	// the window, so no faster collapse arrives from cooling.
	stepN(&p, int(15*time.Second/TimeStep))

	// Honest end state: house-load steam draw exceeds what decay
	// heat can raise, so the header must depart the shell pressure
	// (or the turbine must trip on steam loss) instead of pinning
	// at an infinite-energy 920 psia.
	tailPressureOK := false
	for i := 0; i < int(120*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		if float64(p.ReactorBuilding.SteamGenerator.Pressure) < 900 ||
			p.TurbineBuilding.Turbine.State == TurbineTripped {
			tailPressureOK = true
			break
		}
	}
	assert.True(t, tailPressureOK,
		"header must sag below 900 psia or trip the turbine on steam loss")

	// The subcritical fold rides the small negative reactivity and
	// needs tens of seconds to drain the fission chain to the
	// neutron-source floor.
	stepN(&p, int(60*time.Second/TimeStep))

	assert.LessOrEqual(t, float64(rpv.FissionPower), 100.0,
		"fission chain collapsed to the neutron-source floor")
	assert.InDelta(t, rpv.Insertion, rpv.DemandInsertion, 1e-9,
		"rods seated at the shutdown-margin demand")
	assert.InDelta(t, 0.04, p.TurbineBuilding.Turbine.Setpoint, 1e-9,
		"turbine unloaded to house-load level")
	// A deep-unload cooldown can carry the header through the
	// low-steamline setpoint: the stop valves then auto-close and
	// the admission-less machine dies on its steam-loss deadline,
	// transferring house loads to the offsite source. Without the
	// crossing the unit stays synchronized on house load.
	if p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationActuated {
		assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State,
			"post-isolation starvation trips the machine")
		assert.Equal(t, "SAT",
			p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
			"house loads transfer to the offsite source")
	} else {
		assert.True(t, p.TurbineBuilding.Generator.BreakerClosed,
			"breaker stays closed supplying station auxiliaries")
	}
	assert.False(t, p.TurbineBuilding.Turbine.OPCTrip)
	assert.False(t, p.TurbineBuilding.Turbine.TripByVacuum)
}

// Critical hot-zero-power operation became expressible when the
// temperature feedback split into a power-weighted Doppler base plus
// the moderator interpolation: at zero power the Doppler term vanishes,
// so criticality at the no-load temperature is a pure boron solve.
// The solved concentration sits just under the reference shim, which
// is what makes "hold critical HZP" an operator-expressible lineup.
//
//   - Power defect definition (why the Doppler term vanishes at
//     zero power):
//     specs/the_westinghouse_pressurized_water_react/back_matter/page_244.txt:60-62
func TestScenario14_HZPCriticalityExpressible(t *testing.T) {
	const hzpBoron = 975.7 // solves rho = 0 at 558 F, rods out, xe at rated eq

	r := ReactorPressureVessel{
		ControlRods:        ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: hzpBoron},
		Inlet:              Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:             Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
	}
	r.SeedSteadyStatePower(0)
	r.Iodine135 = 1.0
	r.Xenon135 = xenonEquilibriumPhysical(1.0)
	r.Step(RCPRatedSpeed, 0)

	assert.InDelta(t, 0.0, float64(r.Reactivity), 1e-3,
		"critical hot-zero-power must be expressible near the reference shim, rho=%v",
		r.Reactivity)
	assert.Equal(t, hzpBoron,
		float64(r.ControlRods.BoronConcentration),
		"reactivity must be evaluated at the sampled boron value")
}
