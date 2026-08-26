package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 24 (reactor coolant pump trip and loss of forced flow).
// Durable coverage for the support developed in
// plans/x/scenario/24.txt:
//
//   - "The loss of one RC pump will immediately initiate reactor power
//     stepback. As the coolant flow for one loop is decreasing rapidly,
//     the low coolant flow in that loop will exceed the trip setpoint
//     and will scram the reactor."
//     specs/pwr_sim/chapter_4/page_051.txt:44-47
//   - "Primary coolant trip signals - Departure from nucleate boiling
//     ratio, kw/ft, low flow, and reactor coolant pump under-speed trip
//     signals provide trips."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:80-87
//   - "the cold leg temperature in loop #1 will increase rapidly."
//     specs/pwr_sim/chapter_4/page_052.txt:26-27
func newScenario24Plant(t *testing.T) *Plant {
	t.Helper()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(
		CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Velocity = RCPRatedSpeed
	}
	stepFor(&p, 2*time.Second)
	return &p
}

// The low-flow reactor trip votes the scram directly while the plant
// operates above the at-power permissive, is blocked below it, is
// suppressed by placing the flow channel in test (the modeled form of
// the training DISABLE, specs/pwr_sim/chapter_4/page_052.txt:59-63),
// and stays clear for a fully secured lineup.
func TestRPS_LowFlowTripVotesDirectTripAtPower(t *testing.T) {
	r := ReactorProtectionSystem{}
	// Establish healthy forced flow first - the bistable must be set
	// before a degradation can actuate (a rising startup ramp passing
	// through the band never trips).
	established := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1.0, 1.0, 0)
	assert.False(t, established.ReactorTripped,
		"full forced flow is not a trip condition")

	tripped := established.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1.0, 0.5, 0)
	assert.True(t, tripped.ReactorTripped,
		"degraded established flow at power must scram the reactor")
	assert.False(t, tripped.SafetyInjectionActuated,
		"the low-flow signal itself carries no ESFAS output")
	assert.False(t, tripped.SteamlineIsolationActuated)
	assert.False(t, tripped.ContainmentSprayActuated)

	belowPermissive := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, RPSAtPowerPermissiveFraction/2, 0.5, 0)
	assert.False(t, belowPermissive.ReactorTripped,
		"reduced-flow lineups below the permissive are normal")

	inTest := ReactorProtectionSystem{ChannelTest: RPSFlowChannelInTest}
	establishedInTest := inTest.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1.0, 1.0, 0)
	blocked := establishedInTest.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1.0, 0.5, 0)
	assert.False(t, blocked.ReactorTripped,
		"the flow channel under test must not actuate the trip")

	secured := r.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1.0, 0, 0)
	assert.False(t, secured.ReactorTripped,
		"a lineup whose flow never established cannot low-flow trip")
}

// Single-pump loss: the stepback latches immediately, the flywheel
// coastdown decays flow exponentially through the trip setpoint, and
// the scram lands with rods fully inserted.
func TestPlant_SinglePumpTripStepsBackThenScramsOnLowFlow(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)
	p := newScenario24Plant(t)
	preTrip := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower)

	p.ReactorBuilding.ReactorCoolantPumps.All()[0].Running = false

	var steppedBack, scrammed time.Duration
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		el := time.Duration(i+1) * TimeStep
		if steppedBack == 0 && p.PlantOperation.StepbackActive {
			steppedBack = el
		}
		if scrammed == 0 && p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			scrammed = el
		}
	}
	require.NotZero(t, steppedBack, "stepback must arm on the pump loss")
	require.LessOrEqual(t, steppedBack, 6*time.Second,
		"the stepback leads the scram")
	require.NotZero(t, scrammed, "decaying flow must reach the trip band")
	require.GreaterOrEqual(t, scrammed, 5*time.Second,
		"the flywheel coastdown must carry flow past ~9 s before the trip")
	require.LessOrEqual(t, scrammed, 15*time.Second)
	rpv := p.ReactorBuilding.ReactorPressureVessel
	assert.Equal(t, 1.0, rpv.ControlRods.Insertion,
		"the scram drops the rods to full insertion")
	assert.LessOrEqual(t, float64(rpv.FissionPower),
		0.10*float64(CoreRatedThermalPower),
		"fission collapses behind the inserted rods")
	_ = preTrip

	// ESFAS stays entirely clear: the coordinated turbine runback
	// riding the stepback holds the SG header above the shared
	// low-steamline setpoint ("automatic turbine runbacks which can
	// prevent a trip condition",
	// specs/the_westinghouse_pressurized_water_react/chapter_7/
	// page_138.txt:137-139), so the scram lands via the low-flow
	// channel exactly as the source attributes it, and the flow signal
	// itself carries no ESFAS output.
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationActuated)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ContainmentSprayActuated)
}

// Two-machine loop-group loss: flow decays toward half of rated, so
// both the runback and the scram arrive sooner than the single-pump
// case, and the settled aggregate flow sits near N/4 staging.
func TestPlant_TwoPumpLossScramsSoonerWithLowerSettledFlow(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)

	scramTime := func(tripCount int) (time.Duration, Velocity) {
		p := newScenario24Plant(t)
		pumps := p.ReactorBuilding.ReactorCoolantPumps.All()
		for i := 0; i < tripCount; i++ {
			pumps[i].Running = false
		}
		var scrammed time.Duration
		for i := 0; i < int(60*time.Second/TimeStep); i++ {
			p.Step()
			p.Update()
			if scrammed == 0 &&
				p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
				scrammed = time.Duration(i+1) * TimeStep
			}
		}
		return scrammed, p.ReactorBuilding.ReactorCoolantPump.Velocity
	}

	singleT, singleV := scramTime(1)
	doubleT, doubleV := scramTime(2)
	require.NotZero(t, singleT)
	require.NotZero(t, doubleT)
	assert.Less(t, doubleT, singleT,
		"losing two machines crosses the trip band sooner")
	assert.InDelta(t, 0.50*float64(RCPRatedSpeed), float64(doubleV),
		0.03*float64(RCPRatedSpeed),
		"two remaining machines settle near half of rated flow")
	assert.InDelta(t, 0.75*float64(RCPRatedSpeed), float64(singleV),
		0.03*float64(RCPRatedSpeed),
		"three remaining machines settle near three quarters of rated")
}

// With the stepback malfunction inserted the shutdown system still
// answers: no runback arms, but the decaying flow scrams the reactor
// anyway ("the reactor shutdown system (SDS) is always poised to act",
// specs/pwr_sim/chapter_4/page_051.txt:8-9).
func TestPlant_StepbackFailureStillScramsViaSDS(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)
	p := newScenario24Plant(t)
	setControl(t, p, "PlantOperation/StepbackFailed", "1")

	pumps := p.ReactorBuilding.ReactorCoolantPumps.All()
	pumps[0].Running = false
	pumps[1].Running = false

	var scrammed time.Duration
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		if scrammed == 0 &&
			p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			scrammed = time.Duration(i+1) * TimeStep
		}
	}
	assert.False(t, p.PlantOperation.StepbackActive,
		"the failed-stepback malfunction must block the runback")
	require.NotZero(t, scrammed,
		"SDS must scram on the low-flow signal without the stepback")
	assert.LessOrEqual(t, scrammed, 15*time.Second)
}

// Unprotected variant: with stepback failed AND the flow channel in
// test, neither protection answers; power rides high on degraded flow
// and the per-pass temperature rise drives the outlet upward inside the
// model's rate envelope.
func TestPlant_UnprotectedLossOfFlowChallengesMargin(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)
	p := newScenario24Plant(t)
	setControl(t, p, "PlantOperation/StepbackFailed", "1")
	setControl(t, p,
		"ReactorBuilding/ReactorProtectionSystem/ChannelTest", "Flow")

	outletBefore := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	pumps := p.ReactorBuilding.ReactorCoolantPumps.All()
	pumps[0].Running = false
	pumps[1].Running = false

	for i := 0; i < int(120*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
	}
	assert.False(t, p.PlantOperation.StepbackActive,
		"no runback may arm with the malfunction inserted")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"the disabled flow channel must not scram")
	assert.GreaterOrEqual(t,
		float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower),
		0.85*float64(CoreRatedThermalPower),
		"power stays high without either protection")
	assert.Greater(t, p.ReactorBuilding.ReactorPressureVessel.Outlet.Float(),
		outletBefore,
		"the degraded per-pass rise must push the outlet upward")
}

// Restarting the tripped machine restores forced flow above the
// initiation band and releases the stepback latch before any scram.
func TestPlant_StepbackReleasesWhenFlowRestores(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)
	p := newScenario24Plant(t)

	p.ReactorBuilding.ReactorCoolantPumps.All()[0].Running = false
	released := false
	scrammed := false
	for i := 0; i < int(40*time.Second/TimeStep); i++ {
		el := time.Duration(i) * TimeStep
		if el >= 3*time.Second {
			p.ReactorBuilding.ReactorCoolantPumps.All()[0].Running = true
		}
		p.Step()
		p.Update()
		if !p.PlantOperation.StepbackActive && el > 3*time.Second {
			released = true
		}
		if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			scrammed = true
			break
		}
	}
	assert.True(t, released,
		"restored flow must release the stepback latch")
	assert.False(t, scrammed,
		"flow recovery must stay clear of the trip band")
}

// The stretch is scoped to the degraded-forced-flow regime: zero flow
// (natural circulation) follows exactly the full-flow trajectory, since
// the gated divisor leaves both anchored.
func TestRPV_FlowDegradedTargetRaisesOutletOnlyWhenForced(t *testing.T) {
	overrideTimeStep(t, 10*time.Millisecond)

	outletAfter := func(speed Velocity) float64 {
		r := ReactorPressureVessel{}
		r.SeedSteadyStatePower(CoreRatedThermalPower)
		for i := 0; i < int(120*time.Second/TimeStep); i++ {
			r = r.Step(speed, 0)
		}
		return r.Outlet.Float()
	}
	full := outletAfter(RCPRatedSpeed)
	none := outletAfter(0)
	assert.InDelta(t, full, none, 1e-9,
		"zero flow is natural circulation, not a degraded-forced state")
}
