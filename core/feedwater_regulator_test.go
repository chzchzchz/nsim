package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At construction the feedwater control valve is closed and the
// regulator is stopped with the design level setpoint and zero
// ValveDemand.
func TestFeedwaterRegulator_InitialStateHasStoppedRegulator(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	reg := p.TurbineBuilding.FeedwaterRegulator
	assert.False(t, reg.Running,
		"want Running=false at startup, got %v", reg.Running)
	assert.InDelta(t, FeedwaterLevelSetpoint, reg.LevelSetpoint, 1e-9,
		"want LevelSetpoint=FeedwaterLevelSetpoint at startup, got %v",
		reg.LevelSetpoint)
	assert.Zero(t, reg.ValveDemand,
		"want ValveDemand=0 at startup, got %v", reg.ValveDemand)
	assert.Zero(t, p.TurbineBuilding.MainFeedwaterControlValve.Open,
		"want MainFeedwaterControlValve.Open=0 at startup, got %v",
		p.TurbineBuilding.MainFeedwaterControlValve.Open)
}

// When the regulator is stopped the ValveDemand is held at zero
// regardless of the level error; the integral also freezes to
// prevent windup during shutdown.
//
// Spec justification:
//   - "Feedwater flow to each steam generator is controlled by a
//     feedwater regulator in each feedwater line. The regulator
//     is controlled by steam generator level, steam flow and
//     feedwater flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
func TestFeedwaterRegulator_StoppedRegulatorHoldsZeroDemand(t *testing.T) {
	t.Parallel()
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         0,
		Running:       false,
	}
	r2 := r.Step(DefaultTimeStep)
	assert.Zero(t, r2.ValveDemand,
		"want ValveDemand=0 when regulator is stopped, got %v",
		r2.ValveDemand)
	assert.Zero(t, r2.IntegralError,
		"want IntegralError frozen at 0 when stopped, got %v",
		r2.IntegralError)
}

// With the level below the setpoint, the regulator opens the valve
// (positive proportional action). After a single step with the
// level error = 50% - 40% = 10%, the proportional term alone gives
// Kp × e = 0.5 × 10 = 5.0 → clamped to 1.0.
func TestFeedwaterRegulator_LowLevelOpensValve(t *testing.T) {
	t.Parallel()
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         40,
		Running:       true,
	}
	r2 := r.Step(DefaultTimeStep)
	assert.InDelta(t, 1.0, r2.ValveDemand, 1e-9,
		"want ValveDemand clamped to 1.0 with low level, got %v",
		r2.ValveDemand)
}

// With the level above the setpoint, the regulator closes the
// valve (negative proportional action). With level = 80%, setpoint
// = 50%, e = -30, Kp*e = -15 → clamped to 0.
func TestFeedwaterRegulator_HighLevelClosesValve(t *testing.T) {
	t.Parallel()
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         80,
		Running:       true,
	}
	r2 := r.Step(DefaultTimeStep)
	assert.Zero(t, r2.ValveDemand,
		"want ValveDemand clamped to 0.0 with high level, got %v",
		r2.ValveDemand)
}

// The integral term accumulates while Running is true. With a
// small error (level = 49%, setpoint = 50%) the first step's
// proportional contribution is small (0.5 × 1 = 0.5) and the
// integral term grows with each step.
func TestFeedwaterRegulator_IntegralAccumulatesWhileRunning(t *testing.T) {
	t.Parallel()
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         49,
		Running:       true,
	}
	r2 := r.Step(DefaultTimeStep)
	// First step: integral = 1.0 * dt (1 ms).
	assert.InDelta(t, 1.0*DefaultTimeStep.Seconds(), r2.IntegralError, 1e-12,
		"want IntegralError = e*dt after one step, got %v",
		r2.IntegralError)
	r3 := r2.Step(DefaultTimeStep)
	// Second step: integral = 2.0 * dt.
	assert.InDelta(t, 2.0*DefaultTimeStep.Seconds(), r3.IntegralError, 1e-12,
		"want IntegralError doubled after two steps, got %v",
		r3.IntegralError)
}

// ValveDemand is always clamped to [0, 1] regardless of how large
// the proportional or integral terms grow.
func TestFeedwaterRegulator_ValveDemandClampedToUnitRange(t *testing.T) {
	t.Parallel()
	// Very low level to drive a large positive error.
	hi := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         0,
		Running:       true,
	}
	assert.InDelta(t, 1.0, hi.Step(DefaultTimeStep).ValveDemand, 1e-9,
		"want ValveDemand clamped to 1.0 at the high end, got %v",
		hi.Step(DefaultTimeStep).ValveDemand)
	// Very high level to drive a large negative error.
	lo := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         100,
		Running:       true,
	}
	assert.Zero(t, lo.Step(DefaultTimeStep).ValveDemand,
		"want ValveDemand clamped to 0.0 at the low end, got %v",
		lo.Step(DefaultTimeStep).ValveDemand)
}

// Conditional-integration anti-windup: a long stretch with the level
// above setpoint must not wind the integral deeply negative while the
// valve sits clamped shut, or the proportional term cannot reopen it
// when the level finally drops and the steam generator drains
// unchecked for the whole unwind time. With e = -30, two minutes
// clamped shut stores IntegralError = -3600 under plain integration;
// recovering from +1 percent error (Ki = 0.1/s^2) would then take
// ~100 minutes before the valve cracks open. With the anti-windup the
// integrator freezes and Kp*e alone reopens the demand on the first
// step below setpoint.
func TestFeedwaterRegulator_AntiWindupAllowsRecovery(t *testing.T) {
	t.Parallel()
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         80,
		Running:       true,
	}
	for range int(120 * time.Second / DefaultTimeStep) {
		r = r.Step(DefaultTimeStep)
	}
	assert.Zero(t, r.IntegralError,
		"want integrator frozen while demand is clamped past its limit, got %v",
		r.IntegralError)

	r.Level = 49
	r2 := r.Step(DefaultTimeStep)
	assert.Greater(t, r2.ValveDemand, 0.45,
		"want valve reopened by proportional action once level drops below "+
			"setpoint, got %v", r2.ValveDemand)
}

// Steam flow feedforward contributes to ValveDemand even when the
// level error is zero (load transient anticipation). With zero
// level error, the proportional and integral terms vanish and the
// feedforward drives the demand to K_ff × SteamFlow/DesignFlow.
//
// Spec justification:
//   - "allows the system to accommodate all operating conditions
//     automatically and provides control margin to accommodate
//     load transients."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:82-85
func TestFeedwaterRegulator_SteamFlowFeedforward(t *testing.T) {
	t.Parallel()
	halfDesign := FeedwaterDesignFlow.Float() / 2.0
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         FeedwaterLevelSetpoint,
		SteamFlow:     halfDesign,
		// Balanced feed keeps the third element at zero so the
		// closed form below stays feedforward-only.
		FeedwaterFlow: halfDesign,
		Running:       true,
	}
	r2 := r.Step(DefaultTimeStep)
	assert.InDelta(t, 0.5, r2.ValveDemand, 1e-9,
		"want ValveDemand=0.5 with zero error and half-design steam "+
			"flow, got %v", r2.ValveDemand)
}

// The third element trims demand by the measured steam/feed
// imbalance at zero level error: under-delivery opens the valve,
// over-delivery closes it - supply-side disturbances are countered
// before they become level error.
//
// Spec justification: the regulator is "controlled by steam generator
// level, steam flow and feedwater flow."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
func TestFeedwaterRegulator_FeedMismatchTrimsDemand(t *testing.T) {
	t.Parallel()
	// Half-design steaming keeps every variant inside the [0,1]
	// clamp so the closed forms are observable, not saturated.
	half := FeedwaterDesignFlow.Float() / 2
	base := func(feed float64) float64 {
		r := FeedwaterRegulator{
			LevelSetpoint: FeedwaterLevelSetpoint,
			Level:         FeedwaterLevelSetpoint,
			SteamFlow:     half,
			FeedwaterFlow: feed,
			Running:       true,
		}
		return r.Step(DefaultTimeStep).ValveDemand
	}
	balanced := base(half)
	under := base(half / 2)
	over := base(half * 3 / 2)
	assert.InDelta(t, 0.5, balanced, 1e-9,
		"balanced flows leave the pure feedforward demand")
	assert.InDelta(t, 0.75, under, 1e-9,
		"half delivery opens the valve by the normalized imbalance")
	assert.InDelta(t, 0.25, over, 1e-9,
		"excess delivery closes the valve by the same trim")
}

// When the regulator is wired into the plant and the pump is
// running, the main feedwater control valve is positioned at the
// regulator's demand.
func TestPlant_FeedwaterRegulatorDrivesControlValve(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	sgVol := p.ReactorBuilding.SteamGenerator.Volume.Float()
	p.ReactorBuilding.SteamGenerator.Water =
		Volume{Val: 0.4 * sgVol, Unit: u.Gallon}
	p.Step(DefaultTimeStep)
	p.Update()
	reg := p.TurbineBuilding.FeedwaterRegulator
	// SG level = 40% < 50% setpoint → valve demand should
	// rise above zero on the proportional action.
	assert.Greater(t, reg.ValveDemand, 0.0,
		"want ValveDemand > 0 with low SG level and pump running, "+
			"got %v", reg.ValveDemand)
	assert.InDelta(t, reg.ValveDemand,
		p.TurbineBuilding.MainFeedwaterControlValve.Open, 1e-9,
		"want MainFeedwaterControlValve.Open = reg.ValveDemand, "+
			"got %v vs %v",
		p.TurbineBuilding.MainFeedwaterControlValve.Open,
		reg.ValveDemand)
}

// Transferring to manual freezes both the computed demand and the
// integral: ValveDemand holds the operator's written position while
// the level error persists, and retransfer to automatic resumes from
// the frozen integral without bumping the valve.
// "reactor control and steam generator water level control are
//
//	transferred to manual operation"
//
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:156-159
func TestFeedwaterRegulator_ManualHoldsOperatorDemand(t *testing.T) {
	t.Parallel()
	// Small error so the anti-windup does not pin the integrator
	// after retransfer (kp*e would exceed the output clamp).
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         49,
		Running:       true,
		ManualControl: true,
		ValveDemand:   0.42,
	}
	r2 := r.Step(DefaultTimeStep)
	assert.InDelta(t, 0.42, r2.ValveDemand, 1e-12,
		"manual mode must hold the operator's demand, got %v",
		r2.ValveDemand)
	assert.Zero(t, r2.IntegralError,
		"the integral must freeze in manual mode")

	r3 := r2
	r3.ManualControl = false
	r4 := r3.Step(DefaultTimeStep)
	assert.Greater(t, r4.IntegralError, r2.IntegralError,
		"retransfer to automatic must resume integrating")
}

// The manual transfer is operator-reachable on the bridge and the
// control valve follows the written demand instead of the PI law.
func TestPlant_FeedwaterManualTransferHoldsValve(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.MainFeedwaterPump.Running = true

	m := GetControlMap(&p)
	mc, ok := m["TurbineBuilding/FeedwaterRegulator/ManualControl"]
	require.Truef(t, ok, "want feedwater manual-transfer control registered")
	require.NoErrorf(t, mc.Set("1"), "Set ManualControl")
	vd, ok := m["TurbineBuilding/FeedwaterRegulator/ValveDemand"]
	require.Truef(t, ok, "want regulator demand control registered")
	require.NoErrorf(t, vd.Set("0.3"), "Set ValveDemand")

	stepN(&p, 10, DefaultTimeStep)

	reg := p.TurbineBuilding.FeedwaterRegulator
	assert.True(t, reg.ManualControl)
	assert.InDelta(t, 0.3, reg.ValveDemand, 1e-9,
		"manual demand must survive plant steps, got %v", reg.ValveDemand)
	assert.InDelta(t, 0.3,
		p.TurbineBuilding.MainFeedwaterControlValve.Open, 1e-9,
		"the control valve must follow the manual demand, got %v",
		p.TurbineBuilding.MainFeedwaterControlValve.Open)
}
