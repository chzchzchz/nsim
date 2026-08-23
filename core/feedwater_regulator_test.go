package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// At construction the feedwater control valve is closed and the
// regulator is stopped with the design level setpoint and zero
// ValveDemand.
func TestFeedwaterRegulator_InitialStateHasStoppedRegulator(t *testing.T) {
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
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
func TestFeedwaterRegulator_StoppedRegulatorHoldsZeroDemand(t *testing.T) {
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         0,
		Running:       false,
	}
	r2 := r.Step()
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
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         40,
		Running:       true,
	}
	r2 := r.Step()
	assert.InDelta(t, 1.0, r2.ValveDemand, 1e-9,
		"want ValveDemand clamped to 1.0 with low level, got %v",
		r2.ValveDemand)
}

// With the level above the setpoint, the regulator closes the
// valve (negative proportional action). With level = 80%, setpoint
// = 50%, e = -30, Kp*e = -15 → clamped to 0.
func TestFeedwaterRegulator_HighLevelClosesValve(t *testing.T) {
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         80,
		Running:       true,
	}
	r2 := r.Step()
	assert.Zero(t, r2.ValveDemand,
		"want ValveDemand clamped to 0.0 with high level, got %v",
		r2.ValveDemand)
}

// The integral term accumulates while Running is true. With a
// small error (level = 49%, setpoint = 50%) the first step's
// proportional contribution is small (0.5 × 1 = 0.5) and the
// integral term grows with each step.
func TestFeedwaterRegulator_IntegralAccumulatesWhileRunning(t *testing.T) {
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         49,
		Running:       true,
	}
	r2 := r.Step()
	// First step: integral = 1.0 * dt (1 ms).
	assert.InDelta(t, 1.0*TimeStep.Seconds(), r2.IntegralError, 1e-12,
		"want IntegralError = e*dt after one step, got %v",
		r2.IntegralError)
	r3 := r2.Step()
	// Second step: integral = 2.0 * dt.
	assert.InDelta(t, 2.0*TimeStep.Seconds(), r3.IntegralError, 1e-12,
		"want IntegralError doubled after two steps, got %v",
		r3.IntegralError)
}

// ValveDemand is always clamped to [0, 1] regardless of how large
// the proportional or integral terms grow.
func TestFeedwaterRegulator_ValveDemandClampedToUnitRange(t *testing.T) {
	// Very low level to drive a large positive error.
	hi := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         0,
		Running:       true,
	}
	assert.InDelta(t, 1.0, hi.Step().ValveDemand, 1e-9,
		"want ValveDemand clamped to 1.0 at the high end, got %v",
		hi.Step().ValveDemand)
	// Very high level to drive a large negative error.
	lo := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         100,
		Running:       true,
	}
	assert.Zero(t, lo.Step().ValveDemand,
		"want ValveDemand clamped to 0.0 at the low end, got %v",
		lo.Step().ValveDemand)
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
	halfDesign := FeedwaterDesignFlow.Float() / 2.0
	r := FeedwaterRegulator{
		LevelSetpoint: FeedwaterLevelSetpoint,
		Level:         FeedwaterLevelSetpoint,
		SteamFlow:     halfDesign,
		Running:       true,
	}
	r2 := r.Step()
	assert.InDelta(t, 0.5, r2.ValveDemand, 1e-9,
		"want ValveDemand=0.5 with zero error and half-design steam "+
			"flow, got %v", r2.ValveDemand)
}

// When the regulator is wired into the plant and the pump is
// running, the main feedwater control valve is positioned at the
// regulator's demand.
func TestPlant_FeedwaterRegulatorDrivesControlValve(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	sgVol := p.ReactorBuilding.SteamGenerator.Volume.Float()
	p.ReactorBuilding.SteamGenerator.Water =
		Volume{Val: 0.4*sgVol, Unit: u.Gallon}
	p.Step()
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
