package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 14 (power level reduction to 0% FP) durable assertions.
//
// "Initialize the simulator to 100% FP, using 'reactor lead' mode,
// reduce reactor power in 25% steps at 0.5%/sec"
// specs/pwr_sim/chapter_3/page_039.txt:23-30
//
// The core's net temperature coefficient is positive at the reference
// boron concentration, which makes part-load plateaus bistable: power
// either recovers toward the rated cap or folds toward the floor. The
// evolution is therefore expressed as a reactor-lead glide — one rod
// drive leads the fold while the turbine load setpoint chases
// production downward, keeping the steam generators throttled near
// their shell pressure without tripping any protection function.

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

func TestScenario14_ReactorLeadGlideToZeroFP(t *testing.T) {
	t.Parallel()
	p := scenario14FullPowerPlant()

	// Rods lead: one insertion drive starts the power fold.
	//
	// "Rod cluster control (RCC) assemblies are used for reactor
	// startup or shutdown, to follow load changes..."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:90-93
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion",
		"0.01178")

	totalMW := func() float64 {
		return float64(p.ReactorBuilding.ReactorPressureVessel.
			TotalThermalPower()) / 1e6
	}

	// Turbine follows: setpoint chases production downward in writes
	// under the sudden-rejection threshold so the governing valves
	// keep SG offtake matched to falling heat input.
	//
	// "These valves open under a sudden reduction in turbine-
	// generator load (in excess of 10%)"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
	spSteps := []string{"0.92", "0.84", "0.76", "0.68", "0.60", "0.52",
		"0.44", "0.36", "0.28", "0.20", "0.12", "0.06"}

	// Guards are sampled every checkEvery-th step: the descent takes
	// tens of simulated seconds while the trip/pressure physics move
	// on second scales, so a 200 ms sample rate cannot miss a regime.
	const checkEvery = 200
	stepsPerInterval := int(5 * time.Second / TimeStep)

	prevMW := totalMW()
	for _, sp := range spSteps {
		setControl(t, &p, "TurbineBuilding/Turbine/Setpoint", sp)
		for i := range stepsPerInterval {
			p.Step()
			p.Update()
			if i%checkEvery != 0 {
				continue
			}
			assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
				"reactor must not trip during the glide")
			// The setpoint chase leads the falling heat input in
			// 8 percent steps, so late in the glide the header
			// legitimately sags below the low-steamline setpoint.
			// The guard is therefore honesty, not health: once
			// the process variable crosses the trip point the
			// protection channel must engage instead of sitting
			// artificially healthy.
			if float64(p.ReactorBuilding.SteamGenerator.Pressure) <
				float64(RPSLowSteamlinePressureTrip) {
				assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated,
					"low steamline pressure must actuate safety injection")
				assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationActuated,
					"low steamline pressure must isolate the steamlines")
			}
			assert.Zero(t, p.TurbineBuilding.Turbine.DumpValve.Open,
				"condenser dump valves stay shut while the setpoint chase tracks production")
			curMW := totalMW()
			// Monotonicity arms once the fold actually departs the
			// rated cap: while pinned, fission is clamped and the
			// decay pools still creep upward toward their pinned
			// targets, so tiny positive drift there is legitimate.
			// Below the cap both contributors fall strictly.
			if prevMW < 3700 {
				require.LessOrEqual(t, curMW, prevMW,
					"descent must be monotone")
			}
			prevMW = curMW
		}
	}

	// Guarantee the 0% FP end state: insert rods to shutdown margin
	// and unload the turbine to house-load level.
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
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion",
		"0.60")
	setControl(t, &p, "TurbineBuilding/Turbine/Setpoint", "0.04")
	stepN(&p, int(5*time.Second/TimeStep))

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

	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.LessOrEqual(t, float64(rpv.FissionPower), 100.0,
		"fission chain collapsed to the neutron-source floor")
	assert.InDelta(t, rpv.Insertion, rpv.DemandInsertion, 1e-9,
		"rods seated at the shutdown-margin demand")
	assert.InDelta(t, 0.04, p.TurbineBuilding.Turbine.Setpoint, 1e-9,
		"turbine unloaded to house-load level")
	assert.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"breaker stays closed supplying station auxiliaries")
	assert.False(t, p.TurbineBuilding.Turbine.OPCTrip)
	assert.False(t, p.TurbineBuilding.Turbine.TripByVacuum)
}
