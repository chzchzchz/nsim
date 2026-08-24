package core

import (
	"math"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// At construction the CCWS pumps are off, the inlet temperature
// sits at the steady-state setpoint (105°F), and the surge tank
// is full.
func TestCCWS_InitialStateIsIdle(t *testing.T) {
	p := MakePlant()
	ccws := p.AuxiliaryBuilding.ComponentCoolingWaterSystem
	assert.False(t, ccws.PumpRunning,
		"want PumpRunning=false at startup, got %v", ccws.PumpRunning)
	assert.InDelta(t, CCWSInletSteadyStateTemp,
		ccws.InletTemp.Float(), 1e-9,
		"want InletTemp=CCWSInletSteadyStateTemp at startup, got %v",
		ccws.InletTemp.Float())
	assert.InDelta(t, CCWSInletSteadyStateTemp,
		ccws.OutletTemp.Float(), 1e-9,
		"want OutletTemp=InletTemp at startup, got %v",
		ccws.OutletTemp.Float())
	assert.InDelta(t, CCWSSurgeTankCapacity.Float(),
		ccws.SurgeTankLevel.Float(), 1e-9,
		"want SurgeTankLevel=CCWSSurgeTankCapacity at startup, got %v",
		ccws.SurgeTankLevel.Float())
}

// With the pumps off the inlet temperature freezes and the
// outlet equals the inlet (no flow, no heat transfer).
func TestCCWS_PumpsOffFreezeInletAndOutletEqualInlet(t *testing.T) {
	c := CCWS{
		PumpRunning: false,
		InletTemp:   Temperature{Val: 110.0, Unit: u.Fahrenheit},
		OutletTemp:  Temperature{Val: 120.0, Unit: u.Fahrenheit},
	}
	c2 := c.Step(Temperature{Val: 70, Unit: u.Fahrenheit}, true)
	assert.InDelta(t, 110.0, c2.InletTemp.Float(), 1e-9,
		"want InletTemp=110 with pumps off, got %v", c2.InletTemp.Float())
	assert.InDelta(t, c2.InletTemp.Float(), c2.OutletTemp.Float(), 1e-9,
		"want OutletTemp=InletTemp with pumps off, got %v",
		c2.OutletTemp.Float())
}

// With the pumps running and the inlet above the setpoint, the
// inlet relaxes toward CCWSInletSteadyStateTemp. The first-order
// lag moves the inlet by an exponentially small fraction per
// millisecond step; after one step the gap shrinks by roughly
// (1 - exp(-dt/tau)) ≈ 2.78e-7.
//
// Spec justification:
//   - "The component cooling water inlet temperature to various
//     components during normal cooldown is permitted to increase
//     to 120°F (49°C), however it must return to 105°F (40°C)
//     after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
func TestCCWS_PumpsRunningInletRecoversTowardSetpoint(t *testing.T) {
	c := CCWS{
		PumpRunning: true,
		DriveReady:  true,
		InletTemp:   Temperature{Val: 120.0, Unit: u.Fahrenheit},
		OutletTemp:  Temperature{Val: 120.0, Unit: u.Fahrenheit},
	}
	c2 := c.Step(Temperature{Val: 70, Unit: u.Fahrenheit}, true)
	gap := 120.0 - CCWSInletSteadyStateTemp
	frac := 1.0 - (c2.InletTemp.Float()-CCWSInletSteadyStateTemp)/gap
	// Per-step fraction should match the analytical exponential
	// within rounding; assert it's in [0, 1] and positive.
	assert.Greater(t, frac, 0.0,
		"want positive recovery fraction, got %v", frac)
	assert.LessOrEqual(t, frac, 1.0,
		"want recovery fraction <= 1, got %v", frac)
	assert.Less(t, c2.InletTemp.Float(), 120.0,
		"want InletTemp < 120 after one step, got %v",
		c2.InletTemp.Float())
}

// With the pumps running the outlet temperature equals the
// inlet plus CCHXApproachTemp. The approach is the simplified
// representation of the heat load imposed by the cooled
// components.
func TestCCWS_PumpsRunningOutletAboveInletByApproachTemp(t *testing.T) {
	c := CCWS{
		PumpRunning: true,
		DriveReady:  true,
		InletTemp:   Temperature{Val: 110.0, Unit: u.Fahrenheit},
		OutletTemp:  Temperature{Val: 110.0, Unit: u.Fahrenheit},
	}
	c2 := c.Step(Temperature{Val: 70, Unit: u.Fahrenheit}, true)
	assert.InDelta(t, c2.InletTemp.Float()+CCHXApproachTemp.Float(),
		c2.OutletTemp.Float(), 1e-9,
		"want OutletTemp=InletTemp+CCHXApproachTemp, got %v",
		c2.OutletTemp.Float())
}

// After roughly one tau (1 hour) the inlet temperature moves
// about 63% of the way toward the setpoint. We exercise the
// closed-form lag over many steps to verify the cumulative
// behaviour matches the analytical solution.
//
// Spec justification:
//   - "The component cooling water inlet temperature to various
//     components during normal cooldown is permitted to increase
//     to 120°F (49°C), however it must return to 105°F (40°C)
//     after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
func TestCCWS_InletRecoversWithinFourHours(t *testing.T) {
	// The recovery tau is one hour, orders of magnitude wider than
	// the step, and Step folds the closed-form lag on every call,
	// so a coarse step cannot distort the relaxation trajectory.
	// Must not run in parallel: overrideTimeStep mutates global
	// simulation state.
	overrideTimeStep(t, time.Minute)
	c := CCWS{
		PumpRunning: true,
		DriveReady:  true,
		InletTemp:   Temperature{Val: 120.0, Unit: u.Fahrenheit},
		OutletTemp:  Temperature{Val: 120.0, Unit: u.Fahrenheit},
	}
	// 4 hours of simulated time at 1 min per step.
	swSupply := Temperature{Val: 70, Unit: u.Fahrenheit}
	steps := int(4 * time.Hour / TimeStep)
	for i := 0; i < steps; i++ {
		c = c.Step(swSupply, true)
	}
	// After 4 tau the gap should have shrunk by 1 - exp(-4) ≈ 0.982.
	// Allow a small slack to absorb per-step rounding.
	assert.Less(t, c.InletTemp.Float(), CCWSInletSteadyStateTemp+0.5,
		"want InletTemp close to 105°F after 4h, got %v",
		c.InletTemp.Float())
	assert.Greater(t, c.InletTemp.Float(), CCWSInletSteadyStateTemp-0.1,
		"want InletTemp >= ~105°F after 4h, got %v",
		c.InletTemp.Float())
}

// With a hot river the component cooling HX cannot cool the
// loop to the spec's 105°F design value; the inlet target lifts
// to service water supply + approach (110°F at 100°F supply),
// so the inlet moves above 105°F instead of holding.
func TestCCWS_HotRiverRaisesInletTarget(t *testing.T) {
	c := CCWS{
		PumpRunning: true,
		DriveReady:  true,
		InletTemp:   Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
	}
	hot := c.Step(Temperature{Val: 100, Unit: u.Fahrenheit}, true)
	assert.Greater(t, hot.InletTemp.Float(), CCWSInletSteadyStateTemp,
		"want InletTemp > 105 with 100°F supply, got %v",
		hot.InletTemp.Float())
	assert.LessOrEqual(t, hot.InletTemp.Float(), 100.0+CCHXApproachTemp.Float(),
		"want InletTemp <= supply+approach, got %v",
		hot.InletTemp.Float())
}

// Without service water flow there is no heat sink across the
// component cooling HX: the loop soaks toward the maximum
// permitted cooldown inlet and no duty is rejected to the
// service water.
//
// Spec justification:
//   - "The component cooling water inlet temperature to various
//     components during normal cooldown is permitted to increase
//     to 120°F (49°C), however it must return to 105°F (40°C)
//     after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-112
func TestCCWS_LossOfServiceWaterSoaksLoopAndZeroesDuty(t *testing.T) {
	c := CCWS{
		PumpRunning: true,
		DriveReady:  true,
		InletTemp:   Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
	}
	Soaked := c.Step(Temperature{Val: 75, Unit: u.Fahrenheit}, false)
	assert.Greater(t, Soaked.InletTemp.Float(), CCWSInletSteadyStateTemp,
		"want InletTemp > 105 soaking toward 120 without SW flow, got %v",
		Soaked.InletTemp.Float())
	assert.Zero(t, Soaked.HXHeatDuty,
		"want HXHeatDuty=0 without SW flow, got %v",
		Soaked.HXHeatDuty)
}

// End-to-end via Plant.Step: a safety-injection actuation
// signal starts the diesel; once the diesel reaches
// ReadyToLoad the CCWS PumpRunning flag flips true and the
// inlet temperature begins recovering.
//
// Spec justification:
//   - "The CCWS Safeguards subsystem is considered an
//     engineered safeguards system because it is required to
//     remove decay heat during postaccident and to provide
//     cooling water to safeguards equipment."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:135-138
func TestCCWS_EndToEndSafetyInjectionStartsPumps(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	// The diesel takes ~10 s to reach ReadyToLoad in this model
	// (per existing diesel tests).
	stepN(&p, int(15*time.Second/TimeStep))
	ccws := p.AuxiliaryBuilding.ComponentCoolingWaterSystem
	assert.True(t, ccws.PumpRunning,
		"want CCWS PumpRunning=true after SI+diesel ready, got %v",
		ccws.PumpRunning)
}

// Plant.Step must advance the CCWS exactly one timestep per call.
// ReactorBuilding.Step and AuxiliaryBuilding.Step both used to step
// it, doubling the recovery-lag rate (single-writer discipline,
// plans/CHECK_SKILL.md section 5).
func TestCCWS_SteppedOncePerPlantStep(t *testing.T) {
	p := MakePlant()
	ccws := &p.AuxiliaryBuilding.ComponentCoolingWaterSystem
	ccws.PumpRunning = true
	ccws.DriveReady = true
	ccws.InletTemp = Temperature{Val: 120, Unit: u.Fahrenheit}
	stepN(&p, 1)

	dtTau := TimeStep.Seconds() / CCWSRecoveryTimeConstant.Seconds()
	want := 120.0 - (120.0-CCWSInletSteadyStateTemp)*(1.0-math.Exp(-dtTau))
	got := p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp.Float()
	assert.InDelta(t, want, got, 1e-9,
		"want InletTemp moved one lag-step to %v, got %v", want, got)
}
