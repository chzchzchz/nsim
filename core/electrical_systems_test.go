package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// At construction the offsite grid is healthy, both diesel
// generators are stopped, both ESF buses are energized from
// offsite, the instrument inverters are powered, and there
// is no station blackout.
func TestElecSys_InitialStateHasOffsiteAndEnergizedBuses(t *testing.T) {
	e := PlantElectricalSystems{OffsitePowerAvailable: true}
	e = e.Step()
	assert.True(t, e.OffsitePowerAvailable,
		"want OffsitePowerAvailable=true at startup, got %v",
		e.OffsitePowerAvailable)
	assert.False(t, e.Diesel1Running,
		"want Diesel1Running=false at startup, got %v",
		e.Diesel1Running)
	assert.False(t, e.Diesel2Running,
		"want Diesel2Running=false at startup, got %v",
		e.Diesel2Running)
	assert.True(t, e.ESFBus1Available,
		"want ESFBus1Available=true at startup, got %v",
		e.ESFBus1Available)
	assert.True(t, e.ESFBus2Available,
		"want ESFBus2Available=true at startup, got %v",
		e.ESFBus2Available)
	assert.True(t, e.InverterPowerAvailable,
		"want InverterPowerAvailable=true at startup, got %v",
		e.InverterPowerAvailable)
	assert.False(t, e.StationBlackout,
		"want StationBlackout=false at startup, got %v",
		e.StationBlackout)
}

// While offsite power remains available, diesel start
// timers do not accumulate and the diesels stay stopped.
func TestElecSys_DieselsStayStoppedWhileOffsiteHealthy(t *testing.T) {
	e := PlantElectricalSystems{OffsitePowerAvailable: true}
	for i := 0; i < int(60.0/TimeStep.Seconds()); i++ {
		e = e.Step()
	}
	assert.False(t, e.Diesel1Running,
		"want Diesel1Running=false with offsite healthy, got %v",
		e.Diesel1Running)
	assert.False(t, e.Diesel2Running,
		"want Diesel2Running=false with offsite healthy, got %v",
		e.Diesel2Running)
	assert.Zero(t, e.Diesel1StartTime,
		"want Diesel1StartTime=0 with offsite healthy, got %v",
		e.Diesel1StartTime)
	assert.Zero(t, e.Diesel2StartTime,
		"want Diesel2StartTime=0 with offsite healthy, got %v",
		e.Diesel2StartTime)
}

// On loss of offsite power both diesel generators receive a
// start signal; each becomes Running after DieselRatedLoadTime
// (30 s) elapses.
func TestElecSys_DieselsStartOnLossOfOffsite(t *testing.T) {
	e := PlantElectricalSystems{OffsitePowerAvailable: false}
	e = e.StepBySeconds(DieselRatedLoadTime + 1.0)
	assert.True(t, e.Diesel1Running,
		"want Diesel1Running=true after 30 s, got %v",
		e.Diesel1Running)
	assert.True(t, e.Diesel2Running,
		"want Diesel2Running=true after 30 s, got %v",
		e.Diesel2Running)
	assert.False(t, e.StationBlackout,
		"want StationBlackout=false with diesels running, got %v",
		e.StationBlackout)
}

// Before the diesel reaches rated load, the ESF bus is
// unpowered and station blackout is declared (offsite down,
// neither diesel running yet).
func TestElecSys_StationBlackoutBeforeDieselReady(t *testing.T) {
	e := PlantElectricalSystems{OffsitePowerAvailable: false}
	e = e.StepBySeconds(1.0)
	assert.False(t, e.ESFBus1Available,
		"want ESFBus1Available=false before diesel ready, got %v",
		e.ESFBus1Available)
	assert.False(t, e.ESFBus2Available,
		"want ESFBus2Available=false before diesel ready, got %v",
		e.ESFBus2Available)
	assert.False(t, e.InverterPowerAvailable,
		"want InverterPowerAvailable=false before diesel ready, got %v",
		e.InverterPowerAvailable)
	assert.True(t, e.StationBlackout,
		"want StationBlackout=true before diesel ready, got %v",
		e.StationBlackout)
}

// After the diesel reaches rated load and AutoTransferDelay
// has elapsed, the ESF bus is energized from the diesel and
// the inverter power is restored.
func TestElecSys_ESFBusEnergizedAfterDieselAndAutoTransfer(t *testing.T) {
	e := PlantElectricalSystems{OffsitePowerAvailable: false}
	e = e.StepBySeconds(DieselRatedLoadTime + AutoTransferDelay + 1.0)
	assert.True(t, e.ESFBus1Available,
		"want ESFBus1Available=true after diesel+transfer, got %v",
		e.ESFBus1Available)
	assert.True(t, e.ESFBus2Available,
		"want ESFBus2Available=true after diesel+transfer, got %v",
		e.ESFBus2Available)
	assert.True(t, e.InverterPowerAvailable,
		"want InverterPowerAvailable=true after diesel+transfer, got %v",
		e.InverterPowerAvailable)
	assert.False(t, e.StationBlackout,
		"want StationBlackout=false after transfer, got %v",
		e.StationBlackout)
}

// When offsite power returns, the diesel start timers reset
// and the diesels report stopped.
func TestElecSys_OffsiteReturnResetsDieselTimers(t *testing.T) {
	e := PlantElectricalSystems{OffsitePowerAvailable: false}
	e = e.StepBySeconds(DieselRatedLoadTime + 5.0)
	assert.True(t, e.Diesel1Running,
		"want Diesel1Running=true after start, got %v",
		e.Diesel1Running)
	e.OffsitePowerAvailable = true
	e = e.Step()
	assert.False(t, e.Diesel1Running,
		"want Diesel1Running=false after offsite returns, got %v",
		e.Diesel1Running)
	assert.Zero(t, e.Diesel1StartTime,
		"want Diesel1StartTime=0 after offsite returns, got %v",
		e.Diesel1StartTime)
}

// One ESF bus energised from its diesel is enough to power
// the inverter supply for RPS/ESF instrumentation channels.
func TestElecSys_InverterPoweredFromEitherESFBus(t *testing.T) {
	e := PlantElectricalSystems{
		OffsitePowerAvailable: false,
		Diesel1Running:        true,
		Diesel1StartTime:      DieselRatedLoadTime +
			AutoTransferDelay + 1.0,
	}
	e = e.Step()
	assert.True(t, e.ESFBus1Available,
		"want ESFBus1Available=true from diesel, got %v",
		e.ESFBus1Available)
	assert.True(t, e.InverterPowerAvailable,
		"want InverterPowerAvailable=true from ESFBus1 alone, got %v",
		e.InverterPowerAvailable)
}

// When the plant wires PlantElectricalSystems into
// Plant.Step, the initial offsite-healthy state is preserved
// and the buses remain energized.
func TestPlant_PlantElectricalSystems_InitialStatePreserved(t *testing.T) {
	p := MakePlant()
	p.Step()
	p.Update()
	e := p.ReactorBuilding.PlantElectricalSystems
	assert.True(t, e.OffsitePowerAvailable,
		"want OffsitePowerAvailable=true after Plant step, got %v",
		e.OffsitePowerAvailable)
	assert.True(t, e.ESFBus1Available,
		"want ESFBus1Available=true after Plant step, got %v",
		e.ESFBus1Available)
	assert.True(t, e.InverterPowerAvailable,
		"want InverterPowerAvailable=true after Plant step, got %v",
		e.InverterPowerAvailable)
}
