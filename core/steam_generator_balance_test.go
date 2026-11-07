package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The steam-generator secondary mass balance must close against the
// spec's rated steam flow: the liquid-equivalent evaporation debit is
// the anchor every level transient hangs from.
//
//   - "Steam flow per steam generator: 3,813,000 lb/hr (480 kg/sec)":
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
//   - "Feedwater flow to each steam generator is controlled by a
//     feedwater regulator [...] controlled by steam generator level,
//     steam flow and feedwater flow":
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:68-76
//   - EFWS auto-start on low-low level:
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:58-80
//   - Delivery within one minute, indefinite continuation:
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:4-11

// A1: with HeatInput at the rated per-loop duty, a no-feed step drains
// exactly one evaporation's worth of liquid stock, in gallons. The
// expected drain is computed here from the spec anchors independently
// of the production expression; the pre-fix code drained ~0.84 gal/s
// against this ~111 gal/s expectation.
func TestSteamGenerator_FullPowerDrainMatchesSpecFlow(t *testing.T) {
	t.Parallel()

	sg := SteamGenerator{
		Volume:   Volume{Val: 100000, Unit: u.Gallon},
		Water:    Volume{Val: 80000, Unit: u.Gallon},
		Pressure: SteamGeneratorShellPressure,
	}
	sg.HeatInput = SteamGeneratorFullLoadHeatInput

	before := sg.Water.Float()
	after := sg.Step(DefaultTimeStep, 1.0) // full turbine admission
	drainedPerStep := before - after.Water.Float()

	ratedKgPerSec := float64(CoreRatedThermalPower) /
		float64(SteamGeneratorLoopCount) /
		float64(LatentHeatOfSteamization)
	wantDrain := ratedKgPerSec / KilogramsPerGallon * DefaultTimeStep.Seconds()

	assert.InDelta(t, wantDrain, drainedPerStep, 0.01*wantDrain,
		"rated no-feed drain must equal the liquid-equivalent "+
			"evaporation rate, got %v gal/step want %v",
		drainedPerStep, wantDrain)
}

// Drawing steam with heat input removed does not debit the boiling
// liquid directly: the header-pressure target sags instead, while the
// vapor buffer serves the demand. Only evaporation spends Water.
func TestSteamGenerator_OfftakeDoesNotDebitLiquidStock(t *testing.T) {
	t.Parallel()

	sg := SteamGenerator{
		Volume:   Volume{Val: 100000, Unit: u.Gallon},
		Water:    Volume{Val: 80000, Unit: u.Gallon},
		Pressure: SteamGeneratorShellPressure,
	}

	before := sg.Water.Float()
	after := sg.Step(DefaultTimeStep, 1.0) // full demand, zero heat input
	assert.InDelta(t, before, after.Water.Float(), 1e-9,
		"condenser-directed offtake is internal recirculation and "+
			"must not spend shell inventory")

	after = sg.Step(DefaultTimeStep, 0.0) // nothing boiling, nothing drawing
	assert.InDelta(t, before, after.Water.Float(), 1e-9,
		"an idle shell holds its liquid stock")
}

func lossOfFeedPlant() Plant {
	p := scenario10FullPowerPlant()
	// The fixture's premise is main feed lost, so the pump is
	// secured explicitly rather than inherited from a construction
	// default.
	p.TurbineBuilding.MainFeedwaterPump.Running = false
	p.ReactorBuilding.SteamGenerator.Water =
		Volume{Val: 20000, Unit: u.Gallon}
	return p
}

// A2: at full power with main feed available, the level walks down
// from the 80 percent seed until the regulator opens near its 50
// percent setpoint, then holds inside the band over a multi-minute
// window: equilibrium delivery equals the boil (~111 gal/s), well
// inside FeedwaterDesignFlow authority.
//
// The tick is widened to keep the multi-minute window fast while
// staying fine enough that no trip limiter aliases (a 1 s tick
// spuriously collapses the fresh-plant header); overrideTimeStep
// mutates global state, so this test must not run in parallel.
func TestPlant_FeedwaterRegulatesLevelAtFullPower(t *testing.T) {
	testDt := 100 * time.Millisecond
	p := scenario10FullPowerPlant()
	p.TurbineBuilding.MainFeedwaterPump.Running = true

	stepN(&p, int(480*time.Second/testDt), testDt) // eight simulated minutes
	water := p.ReactorBuilding.SteamGenerator.Water.Float()
	assert.GreaterOrEqual(t, water, 45000.0,
		"level fell below the regulation band, water=%v", water)
	assert.LessOrEqual(t, water, 55000.0,
		"level rose above the regulation band, water=%v", water)

	held := water
	stepN(&p, int(240*time.Second/testDt), testDt) // four more minutes on automatics
	water = p.ReactorBuilding.SteamGenerator.Water.Float()
	assert.GreaterOrEqual(t, water, 45000.0,
		"regulator lost the band on the long hold, water=%v", water)
	assert.LessOrEqual(t, water, 55000.0,
		"regulator lost the band on the long hold, water=%v", water)
	assert.Less(t, water-held, 5000.0,
		"level must hold, not keep draining, delta=%v", water-held)
}

// A3: securing main feed at full power genuinely drains the generator
// through the EFWS low-low setpoint, which latches the auto-start and
// puts both motor pumps on the generator within the one-minute
// window.
//
// Same widened-tick rationale as the regulation test above; must not
// run in parallel.
func TestPlant_LossOfFeedReachesEFWSLowLowTrigger(t *testing.T) {
	testDt := 100 * time.Millisecond
	p := lossOfFeedPlant()

	steps := int(100 * time.Millisecond / testDt)
	crossed := false
	for i := 0; i < 2000 && !crossed; i++ {
		stepN(&p, steps, testDt)
		if p.ReactorBuilding.SteamGenerator.Water.Float() <
			EFWSLevelLowLow {
			crossed = true
		}
	}
	require.True(t, crossed,
		"loss of feed must reach the EFWS low-low setpoint, "+
			"water=%v",
		p.ReactorBuilding.SteamGenerator.Water.Float())

	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.True(t, efws.Active, "want EFWS autostart latch set")
	assert.True(t, efws.SubsystemA.MotorPump.Running)
	assert.True(t, efws.SubsystemB.MotorPump.Running)

	stepN(&p, int(EFWSDeliveryTime/0.1), testDt)
	assert.GreaterOrEqual(t, p.ReactorBuilding.EFWSFeedwaterFlow.Float(),
		2*EFWSMotorPumpFlow.Float(),
		"both motor pumps delivering within one minute of the trigger")
}

// A4: post-trip hot standby on EFW alone. With the reactor tripped to
// decay heat and the motor-pump pair delivering, the generator must
// not dry out over the window and the pressurizer heaters must hold
// primary pressure above the low-pressurizer trip.
//
// Manual SI is the trip driver: it inserts rods and auto-starts the
// EFW motor pumps through the implemented actuation path.
//
// Same widened-tick rationale as above; must not run in parallel.
func TestPlant_EFWHoldsDecayHeatBoil(t *testing.T) {
	testDt := 100 * time.Millisecond
	p := scenario10FullPowerPlant()
	cm := GetControlMap(&p)
	c, ok := cm["ReactorBuilding/ReactorProtectionSystem/ManualSI"]
	require.True(t, ok, "ManualSI control not registered")
	require.NoError(t, c.Set("1"), "set ManualSI")

	waterBefore := p.ReactorBuilding.SteamGenerator.Water.Float()
	stepN(&p, int(300*time.Second/testDt), testDt) // five minutes of hot standby

	waterAfter := p.ReactorBuilding.SteamGenerator.Water.Float()
	assert.GreaterOrEqual(t, waterAfter-waterBefore, 0.0,
		"EFW must cover decay-heat boil, delta=%v",
		waterAfter-waterBefore)

	pzr := p.ReactorBuilding.Pressurizer
	assert.Greater(t, float64(pzr.Pressure),
		float64(RPSLowPressurizerPressureTrip),
		"pressurizer heaters must hold hot-standby pressure, got %v",
		pzr.Pressure)
}
