package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 27 dropped rod bank: once one bank has dropped (large
// sustained negative reactivity), the automatic power control system
// responds by withdrawing rod demand at its capped slew rate - an
// attempt that cannot recover the worth because the dropped bank pins
// the actual insertion, and the dropped-rod signal raises the
// automatic load runback before any trip.
//
//   - Catalog: plans/x/scenarios.txt item 27.
//   - Malfunction class (secondary training source): "One bank of dark
//     rods drops [...], imparting large negative reactivity into the
//     core", and the control system withdraws rods in response.
//     specs/pwr_sim/chapter_4/page_049.txt:61-67
//     specs/pwr_sim/chapter_4/page_050.txt:1-9
//   - Withdrawal authority is the demand slew cap
//     (RodCtrlMaxDemandSlewPerSec), a named production constant.
//   - Runback initiator: "Automatic load runback initiated by a
//     dropped rod signal [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
func TestScenario27_RPCWithdrawsAtCappedRateAfterDrop(t *testing.T) {
	t.Parallel()

	// Post-drop state: actual and demanded insertion parked deep, core
	// power collapsed to 30 percent under roughly -3.9 percent delta-k,
	// turbine demand still at rated so the program point stays 100
	// percent. The atmosphere sits near the program minus the deficit
	// sag, so the measured-Tavg trim pulls withdrawal alongside the
	// power error.
	c := ControlRods{
		Insertion:       0.85,
		DemandInsertion: 0.85,
		DroppedBank:     true,
		AutoControl:     true,
	}
	steps := int(1 * time.Second / TimeStep)
	for i := 0; i < steps; i++ {
		c.applyAutoControl(
			Temperature{Val: programmedRodTavg(1.0) - 10, Unit: u.Fahrenheit},
			1.0, 0.30, -3.9)
		c = c.Step()
	}

	withdrawn := 0.85 - c.DemandInsertion
	assert.Greater(t, withdrawn, 0.0,
		"the controller must answer the drop with withdrawal")
	assert.LessOrEqual(t, withdrawn,
		RodCtrlMaxDemandSlewPerSec*float64(steps)*TimeStep.Seconds()*1.01,
		"withdrawal runs at the demand slew cap")
	assert.GreaterOrEqual(t, withdrawn,
		RodCtrlMaxDemandSlewPerSec*float64(steps)*TimeStep.Seconds()*0.99,
		"sustained large error keeps the cap pinned")

	// The dropped bank pins the actual insertion at the failure depth:
	// controller withdrawal moves only the demand, never the rods -
	// this is why the combined worth cannot recover.
	assert.Equal(t, RodDroppedBankInsertion, c.Insertion,
		"dropped bank holds the insertion floor under withdrawal")

	// Balance hold: at the program point with balanced reactivity and
	// an atmosphere sitting on the program the deadband returns before
	// any integration, so a compensated plant does not hunt.
	var held ControlRods
	held.AutoControl = true
	held.DemandInsertion = 0.30
	for i := 0; i < steps; i++ {
		held.applyAutoControl(
			Temperature{Val: programmedRodTavg(0.30), Unit: u.Fahrenheit},
			0.30, 0.30, 0)
	}
	assert.Equal(t, 0.30, held.DemandInsertion,
		"deadband must leave a balanced lineup untouched")
}

// Latching the dropped bank raises the automatic load runback on the
// very first step - before any reactor trip - so the turbine begins
// shedding toward house loads while the core folds.
func TestScenario27_DroppedBankRaisesLoadRunback(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/ControlRods/DroppedBank"].Set("1"))

	stepN(&p, 100)

	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion >=
		RodDroppedBankInsertion,
		"dropped bank sits at its failure depth immediately")
	assert.True(t, p.TurbineBuilding.Turbine.LoadRunback,
		"dropped-rod signal raises the load runback pre-trip")
	assert.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"the runback fires without needing a reactor trip")
}

// The low-pressurizer-pressure channel heads both the Safety Injection
// and Steamline Isolation trigger lists of Table 7-1 and votes the
// reactor trip behind the at-power permissive, exactly like the low
// Tcold channel beside it.
//
//   - "Safety Injection / Low pressurizer pressure / [...]":
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-15
//   - "Steamline Isolation / Low pressurizer pressure / [...]":
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:25-31
//   - At-power permissive convention:
//     specs/westinghouse_plant_operations/chapter_19/page_018.txt:3-7
func TestScenario26_LowPressurizerPressureVotesTripAndSI(t *testing.T) {
	t.Parallel()

	// At power with the pressurizer just below the setpoint: one trip
	// channel counts, SI latches on the first step and its vote lands
	// the reactor trip on the next.
	r := ReactorProtectionSystem{}
	low := RPSLowPressurizerPressureTrip - 1
	s1 := r.Step(SteamGeneratorShellPressure, low, 50,
		CoreReferenceInletF, AtmosphericPressure, 1.0, 1, 0,
		false)
	assert.Equal(t, 1, s1.TripSignalCount,
		"low pressurizer pressure contributes one voting channel")
	assert.True(t, s1.SafetyInjectionActuated,
		"low pressurizer pressure is an SI trigger")
	assert.True(t, s1.SteamlineIsolationActuated,
		"low pressurizer pressure is a steamline-isolation trigger")
	s2 := s1.Step(SteamGeneratorShellPressure, low, 50,
		CoreReferenceInletF, AtmosphericPressure, 1.0, 1, 0,
		false)
	assert.True(t, s2.ReactorTripped,
		"the SI latch votes the reactor trip")

	// Below the at-power permissive the channel stays silent so
	// planned cooldowns pass through the setpoint without ESFAS.
	cold := ReactorProtectionSystem{}
	s3 := cold.Step(SteamGeneratorShellPressure, low, 50,
		CoreReferenceInletF, AtmosphericPressure,
		RPSAtPowerPermissiveFraction/2, 1, 0,
		false)
	assert.False(t, s3.SafetyInjectionActuated)
	assert.Equal(t, 0, s3.TripSignalCount)

	// Channel-under-test isolation forces the monitored pressure
	// away from the setpoint, so calibration cannot actuate SI.
	tested := ReactorProtectionSystem{ChannelTest: RPSPressurizerChannelInTest}
	s4 := tested.Step(SteamGeneratorShellPressure, low, 50,
		CoreReferenceInletF, AtmosphericPressure, 1.0, 1, 0,
		false)
	assert.False(t, s4.SafetyInjectionActuated,
		"isolated channel cannot actuate protections")

	// Healthy pressure at power contributes nothing.
	ok := ReactorProtectionSystem{}
	s5 := ok.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, AtmosphericPressure, 1.0, 1, 0,
		false)
	assert.Equal(t, 0, s5.TripSignalCount)
	assert.False(t, s5.SafetyInjectionActuated)
}
