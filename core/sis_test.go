package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// The accumulators are passive: with RCS pressure above the
// precharge threshold and Level > 0 the bank stays idle. The
// Discharging flag never latches true on its own without the
// pressure drop.
//
// Spec justification:
//   - "the accumulator discharge will begin automatically as
//     the RCS pressure decreases below the accumulator tank
//     pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
func TestSIS_AccumulatorIdleAbovePrechargePressure(t *testing.T) {
	s := (&SIS{
		Accumulator: Accumulator{
			Pressure: SISAccumulatorPrechargePressure,
			Level: u.NewValue(
				SISAccumulatorPerTankCapacity.Float()*
					float64(SISAccumulatorCount),
				u.Gallon,
			),
			Capacity: u.NewValue(
				SISAccumulatorPerTankCapacity.Float()*
					float64(SISAccumulatorCount),
				u.Gallon,
			),
		},
	}).Step(float64(SISAccumulatorPrechargePressure) + 1)
	assert.False(t, s.Accumulator.Discharging,
		"want Discharging=false above precharge, got %v",
		s.Accumulator.Discharging)
	assert.Zero(t, s.AccumulatorInjectionFlow.Float(),
		"want AccumulatorInjectionFlow=0 above precharge, got %v",
		s.AccumulatorInjectionFlow.Float())
	assert.InDelta(t,
		SISAccumulatorPerTankCapacity.Float()*
			float64(SISAccumulatorCount),
		s.Accumulator.Level.Float(), 1e-9,
		"want Level unchanged at idle, got %v",
		s.Accumulator.Level.Float())
}

// The accumulators begin discharging as soon as RCS pressure
// drops below the precharge threshold. Each step drains the bank
// at SISAccumulatorDischargeRate × SISAccumulatorCount and the
// Level decreases monotonically.
//
// Spec justification:
//   - "the accumulator discharge will begin automatically as
//     the RCS pressure decreases below the accumulator tank
//     pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:31-33
//   - "This high flow is provided by the accumulators, followed
//     by the HHSI and RHR pumps, all discharging into the cold
//     legs of the RCS."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:20-23
func TestSIS_AccumulatorDischargesBelowPrecharge(t *testing.T) {
	initial := SISAccumulatorPerTankCapacity.Float() *
		float64(SISAccumulatorCount)
	s := (&SIS{
		Accumulator: Accumulator{
			Pressure: SISAccumulatorPrechargePressure,
			Level:    u.NewValue(initial, u.Gallon),
			Capacity: u.NewValue(initial, u.Gallon),
		},
	}).Step(float64(SISAccumulatorPrechargePressure) - 1)
	assert.True(t, s.Accumulator.Discharging,
		"want Discharging=true below precharge, got %v",
		s.Accumulator.Discharging)
	assert.Greater(t, s.AccumulatorInjectionFlow.Float(), 0.0,
		"want AccumulatorInjectionFlow > 0 during discharge, got %v",
		s.AccumulatorInjectionFlow.Float())
	assert.Less(t, s.Accumulator.Level.Float(), initial,
		"want Level < initial after one step, got %v",
		s.Accumulator.Level.Float())

	// Step until empty; injection flow should be non-zero while
	// Level > 0 and zero after.
	prev := s.Accumulator.Level.Float()
	for i := 0; i < int(time.Hour/TimeStep); i++ {
		s = s.Step(float64(SISAccumulatorPrechargePressure) - 1)
		if s.Accumulator.Level.Float() >= prev {
			t.Fatalf("Level did not decrease at step %d: prev=%v now=%v",
				i, prev, s.Accumulator.Level.Float())
		}
		prev = s.Accumulator.Level.Float()
		if prev == 0 {
			break
		}
	}
	// One more step after Level reaches zero: Discharging
	// should be cleared and AccumulatorInjectionFlow zero.
	s = s.Step(float64(SISAccumulatorPrechargePressure) - 1)
	assert.Zero(t, s.Accumulator.Level.Float(),
		"want Level=0 after long discharge, got %v",
		s.Accumulator.Level.Float())
	assert.False(t, s.Accumulator.Discharging,
		"want Discharging=false when Level=0, got %v",
		s.Accumulator.Discharging)
	assert.Zero(t, s.AccumulatorInjectionFlow.Float(),
		"want AccumulatorInjectionFlow=0 when Level=0, got %v",
		s.AccumulatorInjectionFlow.Float())
}

// HHSI pumps draw from the RWST during the injection phase. With
// HHSIPumpRunning=true and HHSISuction=SISSuctionRWST the RWST
// level decreases at SISHHSIDesignFlow per second, and the net
// injection flow into the RCS cold legs equals SISHHSIDesignFlow
// × (1 − SISHHSIMiniFlowFraction) per second.
//
// Spec justification:
//   - "The HHSI and RHR pumps are aligned to take suction
//     directly from the RWST."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
//   - "A continuous minimum flow path which does not require
//     isolation during the injection phase is provided so that
//     an inadvertent pump start will not result in the
//     deadheading of the HHSI pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_096.txt:26-30
func TestSIS_HHSIPumpDepletesRWST(t *testing.T) {
	s := (&SIS{
		HHSIPumpRunning: true,
		HHSIDriveReady:  true,
		HHSISuction:     SISSuctionRWST,
		RWSTLevel:       u.NewValue(SISRWSTCapacity, u.Gallon),
	}).Step(100)
	dt := TimeStep.Seconds()
	wantLoss := SISHHSIDesignFlow.Float() * dt
	assert.InDelta(t, SISRWSTCapacity-wantLoss, s.RWSTLevel.Float(), 1e-9,
		"want RWSTLevel=%v after one step, got %v",
		SISRWSTCapacity-wantLoss, s.RWSTLevel.Float())
	wantInjection := SISHHSIDesignFlow.Float() *
		(1.0 - SISHHSIMiniFlowFraction)
	assert.InDelta(t, wantInjection, s.HHSIInjectionFlow.Float(), 1e-9,
		"want HHSIInjectionFlow=%v, got %v",
		wantInjection, s.HHSIInjectionFlow.Float())
}

// Once the RWST level drops below SISRWSTLowLevel and the HHSI
// pumps are running, the protection logic aligns suction to the
// containment sump. The switchover completes after
// SISRecirculationSwitchoverDelay seconds; once latched, the
// alignment does not toggle and the RWST stops depleting.
//
// Spec justification:
//   - "The changeover from the injection mode to the cold-leg
//     recirculation mode is initiated automatically when the
//     RWST level channels indicate an RWST level less than a
//     low-level setpoint in conjunction with the initiation of
//     the engineered safeguards actuation signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:56-60
//   - "To prevent backflow to the RWST of contaminated water
//     from the containment sump, the HHSI pump miniflow
//     isolation valves are automatically closed at this time."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:61-63
func TestSIS_RecirculationSwitchoverAlignsToSump(t *testing.T) {
	s := SIS{
		HHSIPumpRunning: true,
		HHSIDriveReady:  true,
		HHSISuction:     SISSuctionRWST,
		RWSTLevel:       u.NewValue(SISRWSTLowLevel-1, u.Gallon),
	}
	delay := int(SISRecirculationSwitchoverDelay /
		TimeStep.Seconds()) + 1
	for i := 0; i < delay; i++ {
		s = s.Step(100)
	}
	assert.Equal(t, SISSuctionSump, s.HHSISuction,
		"want HHSISuction=SISSuctionSump after switchover, got %v",
		s.HHSISuction)
	assert.True(t, s.SwitchoverComplete,
		"want SwitchoverComplete=true after delay, got %v",
		s.SwitchoverComplete)
	prev := s.RWSTLevel.Float()
	s = s.Step(100)
	assert.InDelta(t, prev, s.RWSTLevel.Float(), 1e-9,
		"want RWSTLevel unchanged after switchover to sump, got %v (prev=%v)",
		s.RWSTLevel.Float(), prev)
}

// End-to-end via Plant.Step: setting the diesel start signal to
// DieselStartSafetyInjection starts the HHSI pumps and aligns
// suction to the RWST. A single Plant.Step is enough to latch
// HHSIPumpRunning = true.
//
// Spec justification:
//   - "The receipt of the safety injection signal initiates the
//     following emergency core cooling actions: [...] The HHSI
//     pumps are started."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
//   - "The HHSI and RHR pumps are aligned to take suction
//     directly from the RWST."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:24-25
func TestSIS_SafetyInjectionSignalStartsHHSIPumps(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection

	stepN(&p, 1)
	assert.True(t, p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning,
		"want HHSIPumpRunning=true on DieselStartSafetyInjection, got %v",
		p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning)
	assert.Equal(t, SISSuctionRWST,
		p.ReactorBuilding.SafetyInjectionSystem.HHSISuction,
		"want HHSISuction=SISSuctionRWST, got %v",
		p.ReactorBuilding.SafetyInjectionSystem.HHSISuction)

	// Step long enough for the RWST to deplete past the
	// low-level setpoint and complete the switchover. We
	// start the RWST close to the low-level setpoint so the
	// test runs in a tractable number of steps.
	p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel = u.NewValue(
		SISRWSTLowLevel-1, u.Gallon)
	rwstDepletionSec := 1.0 / SISHHSIDesignFlow.Float()
	totalSec := rwstDepletionSec + SISRecirculationSwitchoverDelay
	steps := int(totalSec/TimeStep.Seconds()) + 100
	stepN(&p, steps)
	assert.Equal(t, SISSuctionSump,
		p.ReactorBuilding.SafetyInjectionSystem.HHSISuction,
		"want HHSISuction=SISSuctionSump after long run, got %v",
		p.ReactorBuilding.SafetyInjectionSystem.HHSISuction)
	assert.True(t,
		p.ReactorBuilding.SafetyInjectionSystem.SwitchoverComplete,
		"want SwitchoverComplete=true after long run, got %v",
		p.ReactorBuilding.SafetyInjectionSystem.SwitchoverComplete)
}
