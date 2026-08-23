package core

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// With SourceIsCoolingTower true the CW pumps draw from the
// tower basin, so the inlet temperature is the live cold-basin
// temperature regardless of fan state.
func TestCirculatingWaterInletTempCoolingTower(t *testing.T) {
	c := CirculatingWaterSystem{SourceIsCoolingTower: true}
	ct := &CoolingTower{
		FanRunning: true,
		Basin: Tank{
			Level: Volume{
				Val:  CoolingTowerMinSuctionGal * 2,
				Unit: u.Gallon,
			},
		},
	}
	ct.ColdBasinTemp = Temperature{Val: 80.0, Unit: u.Fahrenheit}
	source := Temperature{Val: 70.0, Unit: u.Fahrenheit}
	c = c.Step(true, true, CirculatingWaterPumpCount, source, ct)
	assert.InDelta(t, 80.0, c.InletTemp.Float(), 1e-9,
		"want InletTemp=80 (live cold-basin temp), got %v",
		c.InletTemp.Float())
}

// When the basin drains below CoolingTowerMinSuctionGal the CW
// pumps lose suction: FlowGPM and PumpsInService drop to zero
// even though the pump latch is on.
func TestCirculatingWaterLosesSuctionWhenBasinEmpty(t *testing.T) {
	c := CirculatingWaterSystem{SourceIsCoolingTower: true}
	ct := &CoolingTower{
		Basin: Tank{Level: Volume{
			Val:  CoolingTowerMinSuctionGal / 2,
			Unit: u.Gallon,
		}},
	}
	source := Temperature{Val: 70.0, Unit: u.Fahrenheit}
	c = c.Step(true, true, CirculatingWaterPumpCount, source, ct)
	assert.Zero(t, c.FlowGPM,
		"want FlowGPM=0 with drained basin, got %v", c.FlowGPM)
	assert.Zero(t, c.PumpsInService,
		"want PumpsInService=0 with drained basin, got %v",
		c.PumpsInService)
}

// The cooling-tower Step mirrors the fan-running flag when
// the upstream drive is ready; without drive power the fan is
// forced off.
func TestCoolingTowerFanRunning(t *testing.T) {
	wetBulb := Temperature{Val: 75.0, Unit: u.Fahrenheit}
	hot := Temperature{Val: 93.0, Unit: u.Fahrenheit}
	ct := CoolingTower{}
	ct = ct.Step(true, true, 0, hot, wetBulb)
	assert.True(t, ct.FanRunning,
		"want FanRunning=true after Step(true,true), got %v",
		ct.FanRunning)
	ct = ct.Step(true, false, 0, hot, wetBulb)
	assert.False(t, ct.FanRunning,
		"want FanRunning=false without drive power, got %v",
		ct.FanRunning)
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

func towerFixture(hotF float64) CoolingTower {
	return CoolingTower{
		ApproachF: CoolingTowerApproachF,
		Basin: Tank{
			Level:    Volume{Val: CoolingTowerBasinCapacityGal, Unit: u.Gallon},
			Capacity: Volume{Val: CoolingTowerBasinCapacityGal, Unit: u.Gallon},
		},
		ColdBasinTemp: Temperature{
			Val:  75.0 + CoolingTowerApproachF,
			Unit: u.Fahrenheit,
		},
		HotInletTemp:  Temperature{Val: hotF, Unit: u.Fahrenheit},
		MakeupValve:   Valve{Open: 1.0},
		BlowdownValve: Valve{Open: 1.0},
	}
}

// Acceptance 1 (wiring/dynamics check): starting away from the
// target, with fans running and design load, the basin converges
// to wet-bulb + approach. The approach constant is a modeling
// convention; this pins the relaxation wiring, not the value.
func TestCoolingTowerSteadyStateApproach(t *testing.T) {
	ct := towerFixture(93.0)
	ct.ColdBasinTemp = Temperature{Val: 90.0, Unit: u.Fahrenheit}
	wetBulb := Temperature{Val: 75.0, Unit: u.Fahrenheit}
	hot := Temperature{Val: 93.0, Unit: u.Fahrenheit}
	steps := int(5 * time.Minute / TimeStep)
	for i := 0; i < steps; i++ {
		ct = ct.Step(true, true, CirculatingWaterDesignFlowGPM, hot, wetBulb)
	}
	target := wetBulb.Float() + CoolingTowerApproachF
	assert.InDelta(t, target, ct.ColdBasinTemp.Float(), 1.0,
		"want cold basin ≈ %.0f after 5 min with fans on, got %v",
		target, ct.ColdBasinTemp.Float())
}

// Acceptance 2: losing fan drive soaks the basin toward the
// hot return water on the cooling-tower time constant, while
// fans-on keeps it at the approach target.
func TestCoolingTowerFanLossHotSoak(t *testing.T) {
	wetBulb := Temperature{Val: 75.0, Unit: u.Fahrenheit}
	hot := Temperature{Val: 95.0, Unit: u.Fahrenheit}
	ct := towerFixture(95.0)
	prev := ct.ColdBasinTemp.Float()
	steps := int(CoolingTowerTimeConstant.Seconds() / TimeStep.Seconds())
	for i := 0; i < steps; i++ {
		ct = ct.Step(false, false, CirculatingWaterDesignFlowGPM, hot, wetBulb)
		if ct.ColdBasinTemp.Float() < prev {
			t.Fatalf("cold basin cooled during hot soak: %v -> %v",
				prev, ct.ColdBasinTemp.Float())
		}
		prev = ct.ColdBasinTemp.Float()
	}
	hotF := hot.Float()
	startF := 75.0 + CoolingTowerApproachF
	want := startF + (1-1/math.E)*(hotF-startF)
	assert.InDelta(t, want, ct.ColdBasinTemp.Float(), 1.0,
		"want ~63%% of the soak to hot after one time constant (%.1f), got %v",
		want, ct.ColdBasinTemp.Float())
}

// Acceptance 3 (wiring check for the rate formula; the >0
// depletion is physics-forced): with makeup closed, the measured
// basin level slope matches the evaporation + blowdown rate
// computed from flow, range, and the derived fractions within 5%.
func TestCoolingTowerEvaporationDepletesBasin(t *testing.T) {
	wetBulb := Temperature{Val: 75.0, Unit: u.Fahrenheit}
	hot := Temperature{Val: 93.0, Unit: u.Fahrenheit}
	ct := towerFixture(93.0)
	ct.MakeupValve.Open = 0

	// Converge thermally first (makeup open clamps at capacity;
	// here we just let temps settle before closing makeup).
	for i := 0; i < 10*int(time.Second/TimeStep); i++ {
		ct = ct.Step(true, true, CirculatingWaterDesignFlowGPM, hot, wetBulb)
	}
	startLevel := ct.Basin.Level.Float()
	measureSteps := 10 * int(time.Second/TimeStep)
	for i := 0; i < measureSteps; i++ {
		ct = ct.Step(true, true, CirculatingWaterDesignFlowGPM, hot, wetBulb)
	}
	drop := startLevel - ct.Basin.Level.Float()

	rangeF := hot.Float() - ct.ColdBasinTemp.Float()
	lossGPS := CirculatingWaterDesignFlowGPM / 60.0 *
		(CoolingTowerEvapFractionPerDegF*rangeF + CoolingTowerBlowdownFraction)
	wantDrop := lossGPS * float64(measureSteps) * TimeStep.Seconds()
	assert.Greater(t, drop, 0.0, "basin must deplete without makeup")
	assert.InDelta(t, wantDrop, drop, 0.05*wantDrop,
		"want level drop ≈ %.0f gal over 10 s, got %.0f",
		wantDrop, drop)

	evapGPM := ct.EvaporationFlow.Float() * 60.0
	wantEvap := CirculatingWaterDesignFlowGPM *
		CoolingTowerEvapFractionPerDegF * rangeF
	assert.InDelta(t, wantEvap, evapGPM, 0.01*wantEvap,
		"want evaporation ≈ %.0f gpm, got %.0f gpm",
		wantEvap, evapGPM)
}

// Acceptance 4 (wiring check): the makeup valve refills the
// basin at CoolingTowerMakeupFlowGPM when open, clamped at
// capacity.
func TestCoolingTowerMakeupRefillsBasin(t *testing.T) {
	wetBulb := Temperature{Val: 75.0, Unit: u.Fahrenheit}
	hot := Temperature{Val: 93.0, Unit: u.Fahrenheit}
	ct := towerFixture(93.0)
	ct.MakeupValve.Open = 1.0
	ct.Basin.Level = vol(ct.Basin.Level, 100_000)
	steps := 60 * int(time.Second/TimeStep)
	for i := 0; i < steps; i++ {
		ct = ct.Step(true, true, 0, hot, wetBulb)
	}
	want := 100_000 + CoolingTowerMakeupFlowGPM/60.0*
		float64(steps)*TimeStep.Seconds()
	assert.InDelta(t, want, ct.Basin.Level.Float(), 1.0,
		"want basin level ≈ %.0f gal after 60 s makeup-only refill, got %v",
		want, ct.Basin.Level.Float())
	assert.LessOrEqual(t, ct.Basin.Level.Float(), CoolingTowerBasinCapacityGal)
}

// Acceptance 5 end-to-end: a drained basin trips CW flow off
// through Plant.Step, and restoring inventory recovers flow —
// the loss-of-UHS path for a tower-cooled configuration.
func TestPlant_CoolingTower_UHSLossEndToEnd(t *testing.T) {
	p := MakePlant()
	p.Outside.CirculatingWaterSystem.SourceIsCoolingTower = true

	p.Outside.CoolingTower.Basin.Level = vol(
		p.Outside.CoolingTower.Basin.Level, CoolingTowerMinSuctionGal/2)
	p.Step()
	p.Update()
	assert.Zero(t, p.Outside.CirculatingWaterSystem.FlowGPM,
		"want FlowGPM=0 with basin below suction minimum, got %v",
		p.Outside.CirculatingWaterSystem.FlowGPM)
	assert.True(t, p.Outside.CirculatingWaterPump.Running,
		"pump latch must stay set through the UHS loss")

	p.Outside.CoolingTower.Basin.Level = vol(
		p.Outside.CoolingTower.Basin.Level, CoolingTowerBasinCapacityGal)
	p.Step()
	p.Update()
	assert.Equal(t, CirculatingWaterDesignFlowGPM,
		p.Outside.CirculatingWaterSystem.FlowGPM,
		"want design flow restored once the basin refills")
}

// Acceptance 8: the new tower state survives the plant JSON
// round trip.
func TestPlantJSONRoundTripCoolingTower(t *testing.T) {
	p := MakePlant()
	p.Outside.CoolingTower.Basin.Level.Val = 4_000_000
	p.Outside.CoolingTower.MakeupValve.Open = 0.25

	b, err := json.Marshal(p)
	require.NoError(t, err)
	var p2 Plant
	require.NoError(t, json.Unmarshal(b, &p2))

	assert.InDelta(t, 4_000_000.0,
		p2.Outside.CoolingTower.Basin.Level.Float(), 1e-9,
		"basin level must survive the round trip")
	assert.InDelta(t, 0.25, p2.Outside.CoolingTower.MakeupValve.Open, 1e-9,
		"makeup valve position must survive the round trip")
	assert.InDelta(t, p.Outside.AmbientWetBulbTemp.Float(),
		p2.Outside.AmbientWetBulbTemp.Float(), 1e-9,
		"wet-bulb temperature must survive the round trip")
}
