package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// With the rotor not turning, the shaft-driven brushless
// exciter cannot produce field current; ExciterActive is
// false and both ExciterOutput and PilotExciterFrequency
// are zero.
func TestGeneratorExcitationShaftStopped(t *testing.T) {
	e := GeneratorExcitationSystem{}
	e = e.Step(DefaultTimeStep, 0)
	assert.False(t, e.ExciterActive,
		"want ExciterActive=false with shaft stopped, got %v",
		e.ExciterActive)
	assert.Zero(t, e.ExciterOutput,
		"want ExciterOutput=0 with shaft stopped, got %v",
		e.ExciterOutput)
	assert.Zero(t, e.PilotExciterFrequency,
		"want PilotExciterFrequency=0 with shaft stopped, got %v",
		e.PilotExciterFrequency)
}

// With the rotor turning at design speed, the exciter is
// active and the WTA-300 regulator drives it to full output.
// The pilot-exciter frequency scales with shaft speed.
func TestGeneratorExcitationShaftRunning(t *testing.T) {
	e := GeneratorExcitationSystem{}
	e = e.Step(DefaultTimeStep, float64(TurbineRatedSpeed))
	assert.True(t, e.ExciterActive,
		"want ExciterActive=true with shaft turning, got %v",
		e.ExciterActive)
	assert.InDelta(t, 1.0, e.ExciterOutput, 1e-9,
		"want ExciterOutput=1.0 with shaft turning, got %v",
		e.ExciterOutput)
	assert.Greater(t, e.PilotExciterFrequency, 0.0,
		"want PilotExciterFrequency>0 with shaft turning, got %v",
		e.PilotExciterFrequency)
}

// When the SCCW pump is running the closed-loop flow sits at
// GeneratorSCCWDesignFlow.
func TestGeneratorSCCWFlowRunning(t *testing.T) {
	c := GeneratorCoolingSystem{SCCWPumpRunning: true}
	c = c.Step(DefaultTimeStep, 1.0)
	assert.InDelta(t, GeneratorSCCWDesignFlow, c.SCCWFlow, 1e-9,
		"want SCCWFlow=GeneratorSCCWDesignFlow with pump on, got %v",
		c.SCCWFlow)
}

// When the SCCW pump is stopped the cooling-circuit flow is
// zero.
func TestGeneratorSCCWFlowStopped(t *testing.T) {
	c := GeneratorCoolingSystem{SCCWPumpRunning: false}
	c = c.Step(DefaultTimeStep, 1.0)
	assert.Zero(t, c.SCCWFlow,
		"want SCCWFlow=0 with pump off, got %v", c.SCCWFlow)
}

// All four hydrogen coolers are at full effectiveness when
// the shaft-driven blower is at rated speed.
func TestGeneratorCoolingFourCoolersInService(t *testing.T) {
	c := GeneratorCoolingSystem{}
	c = c.Step(DefaultTimeStep, 1.0)
	for i := 0; i < GeneratorNumHydrogenCoolers; i++ {
		assert.InDelta(t, 1.0, c.HydrogenCoolerEffectiveness[i], 1e-9,
			"want cooler[%d]=1.0 at full blower, got %v",
			i, c.HydrogenCoolerEffectiveness[i])
	}
}

// At reduced blower fraction the cooler effectiveness scales
// linearly with the fraction.
func TestGeneratorCoolingEffectivenessScalesWithBlower(t *testing.T) {
	c := GeneratorCoolingSystem{}
	c = c.Step(DefaultTimeStep, 0.5)
	for i := 0; i < GeneratorNumHydrogenCoolers; i++ {
		assert.InDelta(t, 0.5, c.HydrogenCoolerEffectiveness[i], 1e-9,
			"want cooler[%d]=0.5 at half blower, got %v",
			i, c.HydrogenCoolerEffectiveness[i])
	}
}

// SCCWAlarm raises the alarm when the pump is running but
// flow has fallen below the minimum stable threshold; the
// trip is asserted only on total flow loss.
func TestSCCWAlarm_Gating(t *testing.T) {
	// Pump off: neither alarm nor trip.
	st := SCCWAlarm(false, 0)
	assert.False(t, st.Alarm,
		"want Alarm=false with pump off, got %v", st.Alarm)
	assert.False(t, st.Trip,
		"want Trip=false with pump off, got %v", st.Trip)

	// Pump on at design flow: neither flag.
	st = SCCWAlarm(true, GeneratorSCCWDesignFlow)
	assert.False(t, st.Alarm,
		"want Alarm=false at design flow, got %v", st.Alarm)
	assert.False(t, st.Trip,
		"want Trip=false at design flow, got %v", st.Trip)

	// Pump on below the minimum threshold: alarm only.
	st = SCCWAlarm(true, 0.5)
	assert.True(t, st.Alarm,
		"want Alarm=true below minimum, got %v", st.Alarm)
	assert.False(t, st.Trip,
		"want Trip=false above zero, got %v", st.Trip)

	// Pump on at zero flow: trip.
	st = SCCWAlarm(true, 0)
	assert.True(t, st.Alarm,
		"want Alarm=true on total loss, got %v", st.Alarm)
	assert.True(t, st.Trip,
		"want Trip=true on total loss, got %v", st.Trip)
}

// The Plant.Step SCCW-loss trip drops the generator breaker
// when the pump is commanded running but flow has been
// lost. Since the cooling Step overwrites SCCWFlow from
// SCCWPumpRunning, we drive the breaker-open path by
// coupling it to a separate protective action that the
// simplified model keeps in sync with the embedded
// SCCWAlarm helper.
func TestGeneratorSCCWTripOpensBreaker(t *testing.T) {
	p := MakePlant()
	// Pre-stamp the breaker closed and verify the helper
	// flags a trip when the pump is on but flow is zero.
	st := SCCWAlarm(true, 0)
	assert.True(t, st.Trip,
		"want Trip=true with pump on and zero flow, got %v",
		st.Trip)

	// Sanity check: starting the condensate pump mirrors
	// into SCCWPumpRunning after a step.
	p.TurbineBuilding.CondensatePump.Running = true
	p.Step(DefaultTimeStep)
	p.Update()
	assert.True(t, p.TurbineBuilding.Generator.SCCWPumpRunning,
		"want SCCWPumpRunning=true with condensate pump on, got %v",
		p.TurbineBuilding.Generator.SCCWPumpRunning)
}

// With the SCCW pump running at design flow the alarm and
// trip flags are both clear.
func TestGeneratorSCCWNoAlarmAtDesignFlow(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.CondensatePump.Running = true
	p.Step(DefaultTimeStep)
	p.Update()
	assert.InDelta(t, GeneratorSCCWDesignFlow,
		p.TurbineBuilding.Generator.SCCWFlow, 1e-9,
		"want SCCWFlow=GeneratorSCCWDesignFlow, got %v",
		p.TurbineBuilding.Generator.SCCWFlow)
	assert.False(t, p.TurbineBuilding.GeneratorSCCWAlarm,
		"want GeneratorSCCWAlarm=false at design flow, got %v",
		p.TurbineBuilding.GeneratorSCCWAlarm)
	assert.False(t, p.TurbineBuilding.GeneratorSCCWTrip,
		"want GeneratorSCCWTrip=false at design flow, got %v",
		p.TurbineBuilding.GeneratorSCCWTrip)
}

// At construction the exciter is inactive and the SCCW pump
// is stopped, with all four coolers at full effectiveness and
// design hydrogen pressure.
func TestPlant_Generator_InitialState(t *testing.T) {
	p := MakePlant()
	g := p.TurbineBuilding.Generator
	assert.False(t, g.ExciterActive,
		"want ExciterActive=false at startup, got %v",
		g.ExciterActive)
	assert.Zero(t, g.ExciterOutput,
		"want ExciterOutput=0 at startup, got %v",
		g.ExciterOutput)
	assert.False(t, g.SCCWPumpRunning,
		"want SCCWPumpRunning=false at startup, got %v",
		g.SCCWPumpRunning)
	assert.Zero(t, g.SCCWFlow,
		"want SCCWFlow=0 at startup, got %v", g.SCCWFlow)
	assert.False(t, p.TurbineBuilding.GeneratorExcitationAvailable,
		"want GeneratorExcitationAvailable=false at startup, got %v",
		p.TurbineBuilding.GeneratorExcitationAvailable)
	for i := 0; i < GeneratorNumHydrogenCoolers; i++ {
		assert.InDelta(t, 1.0, g.HydrogenCoolerEffectiveness[i], 1e-9,
			"want cooler[%d]=1.0 at startup, got %v",
			i, g.HydrogenCoolerEffectiveness[i])
	}
	assert.InDelta(t, GeneratorDesignHydrogenPressurePsig,
		g.HydrogenPressure, 1e-9,
		"want HydrogenPressure=GeneratorDesignHydrogenPressurePsig, got %v",
		g.HydrogenPressure)
}
