package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// With the CW pump running and the pump count at design
// (CirculatingWaterPumpCount), the total flow equals the
// design flow rate and PumpsInService is the full count.
func TestCirculatingWaterPumpRunning(t *testing.T) {
	c := CirculatingWaterSystem{}
	c = c.Step(true, true, CirculatingWaterPumpCount,
		Temperature{Val: CirculatingWaterSourceTempF, Unit: u.Fahrenheit}, nil)
	assert.Equal(t, CirculatingWaterPumpCount, c.PumpsInService,
		"want PumpsInService=%d, got %d",
		CirculatingWaterPumpCount, c.PumpsInService)
	assert.InDelta(t, CirculatingWaterDesignFlowGPM, c.FlowGPM, 1e-9,
		"want FlowGPM=%.0f, got %v",
		CirculatingWaterDesignFlowGPM, c.FlowGPM)
}

// With the CW pump stopped the total flow is zero and
// PumpsInService is zero.
func TestCirculatingWaterPumpStopped(t *testing.T) {
	c := CirculatingWaterSystem{}
	c = c.Step(false, true, 0,
		Temperature{Val: CirculatingWaterSourceTempF, Unit: u.Fahrenheit}, nil)
	assert.Zero(t, c.PumpsInService,
		"want PumpsInService=0 with pump off, got %v",
		c.PumpsInService)
	assert.Zero(t, c.FlowGPM,
		"want FlowGPM=0 with pump off, got %v", c.FlowGPM)
}

// With SourceIsCoolingTower false the inlet temperature
// tracks the source temperature directly (no approach
// applied).
func TestCirculatingWaterInletTempRiver(t *testing.T) {
	c := CirculatingWaterSystem{SourceIsCoolingTower: false}
	source := Temperature{Val: 65.0, Unit: u.Fahrenheit}
	c = c.Step(true, true, CirculatingWaterPumpCount, source, nil)
	assert.InDelta(t, 65.0, c.InletTemp.Float(), 1e-9,
		"want InletTemp=65 with river source, got %v",
		c.InletTemp.Float())
}

// With SourceIsCoolingTower true and the cooling-tower fan
// running, the inlet temperature is the source (wet-bulb)
// plus the cooling-tower approach.
func TestCirculatingWaterInletTempCoolingTower(t *testing.T) {
	c := CirculatingWaterSystem{SourceIsCoolingTower: true}
	ct := &CoolingTower{FanRunning: true, ApproachF: 5.0}
	source := Temperature{Val: 70.0, Unit: u.Fahrenheit}
	c = c.Step(true, true, CirculatingWaterPumpCount, source, ct)
	assert.InDelta(t, 75.0, c.InletTemp.Float(), 1e-9,
		"want InletTemp=75 (source+approach), got %v",
		c.InletTemp.Float())
}

// When the cooling-tower fan is off, the cooling-tower source
// degenerates to the raw source temperature (the CWS does
// not get cooled basin water when the tower fans are off).
func TestCirculatingWaterInletTempCoolingTowerFanOff(t *testing.T) {
	c := CirculatingWaterSystem{SourceIsCoolingTower: true}
	ct := &CoolingTower{FanRunning: false, ApproachF: 5.0}
	source := Temperature{Val: 70.0, Unit: u.Fahrenheit}
	c = c.Step(true, true, CirculatingWaterPumpCount, source, ct)
	assert.InDelta(t, 70.0, c.InletTemp.Float(), 1e-9,
		"want InletTemp=70 (raw source) with tower fan off, got %v",
		c.InletTemp.Float())
}

// The cooling-tower Step mirrors the fan-running flag and
// initialises the approach temperature when zero.
func TestCoolingTowerFanRunning(t *testing.T) {
	ct := CoolingTower{}
	ct = ct.Step(true, true)
	assert.True(t, ct.FanRunning,
		"want FanRunning=true after Step(true,true), got %v",
		ct.FanRunning)
	assert.InDelta(t, CoolingTowerApproachF, ct.ApproachF, 1e-9,
		"want ApproachF=%.1f (default), got %v",
		CoolingTowerApproachF, ct.ApproachF)
}

// After Plant.Step the CirculatingWaterSystem reflects the
// running CW pump: FlowGPM matches the design flow and
// InletTemp tracks RiverTemp.
func TestPlant_CirculatingWaterSystem_InitialState(t *testing.T) {
	p := MakePlant()
	p.Step()
	p.Update()
	cws := p.Outside.CirculatingWaterSystem
	assert.True(t, p.Outside.CirculatingWaterPump.Running,
		"want CirculatingWaterPump.Running=true at startup, got %v",
		p.Outside.CirculatingWaterPump.Running)
	assert.InDelta(t, CirculatingWaterDesignFlowGPM, cws.FlowGPM, 1e-9,
		"want FlowGPM=%.0f at startup, got %v",
		CirculatingWaterDesignFlowGPM, cws.FlowGPM)
	assert.InDelta(t, 75.0, cws.InletTemp.Float(), 1e-9,
		"want InletTemp=75 (RiverTemp default) at startup, got %v",
		cws.InletTemp.Float())
	assert.False(t, cws.SourceIsCoolingTower,
		"want SourceIsCoolingTower=false at startup, got %v",
		cws.SourceIsCoolingTower)
}

// Stopping the CW pump drops FlowGPM and PumpsInService to
// zero on the next step; the condenser falls back to its
// no-CW path.
func TestPlant_CirculatingWaterSystem_PumpStop(t *testing.T) {
	p := MakePlant()
	p.Outside.CirculatingWaterPump.Running = false
	p.Step()
	p.Update()
	cws := p.Outside.CirculatingWaterSystem
	assert.Zero(t, cws.FlowGPM,
		"want FlowGPM=0 with pump stopped, got %v", cws.FlowGPM)
	assert.Zero(t, cws.PumpsInService,
		"want PumpsInService=0 with pump stopped, got %v",
		cws.PumpsInService)
}
