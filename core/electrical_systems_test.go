package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// pesStepIn builds the sensed inputs for PlantElectricalSystems.Step:
// two ESF buses with the given energized states and two diesels.
func pesStepIn(busAE, busBE bool, dieselA, dieselB *DieselGenerator) (ESFBus, ESFBus, *DieselGenerator, *DieselGenerator) {
	busA := ESFBus{Energized: busAE, Voltage: 0}
	if busAE {
		busA.Voltage = DieselRatedVoltage
	}
	busB := ESFBus{Energized: busBE, Voltage: 0}
	if busBE {
		busB.Voltage = DieselRatedVoltage
	}
	return busA, busB, dieselA, dieselB
}

// carryingDiesel returns a diesel in its Running state with enough
// elapsed start time to be at rated load
// ("be capable of carrying rated load within 30 seconds",
// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-51).
func carryingDiesel() *DieselGenerator {
	return &DieselGenerator{
		State:          DieselRunning,
		CrankStartTime: DieselRatedLoadTime * time.Second,
	}
}

// At construction the offsite grid is healthy, both diesel
// generators are stopped, both ESF buses are energized from
// offsite, the instrument inverters are powered, and there
// is no station blackout.
func TestElecSys_InitialStateHasOffsiteAndEnergizedBuses(t *testing.T) {
	e := PlantElectricalSystems{}
	{
		busA, busB, da, db := pesStepIn(true, true, nil, nil)
		e = e.Step(DefaultTimeStep, busA, busB, da, db)
	}
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

// While the ESF buses stay energized, diesel start
// timers do not accumulate and the diesels stay stopped.
func TestElecSys_DieselsStayStoppedWhileBusesHealthy(t *testing.T) {
	e := PlantElectricalSystems{}
	for i := 0; i < int(60.0/DefaultTimeStep.Seconds()); i++ {
		{
			busA, busB, da, db := pesStepIn(true, true, nil, nil)
			e = e.Step(DefaultTimeStep, busA, busB, da, db)
		}
	}
	assert.False(t, e.Diesel1Running,
		"want Diesel1Running=false with buses healthy, got %v",
		e.Diesel1Running)
	assert.False(t, e.Diesel2Running,
		"want Diesel2Running=false with buses healthy, got %v",
		e.Diesel2Running)
	assert.Zero(t, e.Diesel1StartTime,
		"want Diesel1StartTime=0 with buses healthy, got %v",
		e.Diesel1StartTime)
	assert.Zero(t, e.Diesel2StartTime,
		"want Diesel2StartTime=0 with buses healthy, got %v",
		e.Diesel2StartTime)
}

// With both buses dark and their diesels started, each
// diesel reports Running once it has carried rated load for
// DieselRatedLoadTime (30 s).
func TestElecSys_DieselsStartOnLossOfOffsite(t *testing.T) {
	e := PlantElectricalSystems{}
	d := carryingDiesel()
	busA, busB, da, db := pesStepIn(false, false, d, d)
	e = e.StepBySeconds(DefaultTimeStep, 1.0, busA, busB, da, db)
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
// unpowered and station blackout is declared (buses dark,
// neither diesel carrying yet).
func TestElecSys_StationBlackoutBeforeDieselReady(t *testing.T) {
	e := PlantElectricalSystems{}
	cranking := &DieselGenerator{State: DieselCranking}
	busA, busB, da, db := pesStepIn(false, false, cranking, cranking)
	e = e.StepBySeconds(DefaultTimeStep, 1.0, busA, busB, da, db)
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

// Once the diesel picks up its bus, the ESF bus is energized
// from the diesel and the inverter power is restored.
func TestElecSys_ESFBusEnergizedAfterDieselAndAutoTransfer(t *testing.T) {
	e := PlantElectricalSystems{}
	d := carryingDiesel()
	busA, busB, _, _ := pesStepIn(true, true, d, d)
	busA.OnDiesel = true
	busB.OnDiesel = true
	e = e.Step(DefaultTimeStep, busA, busB, d, d)
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

// When a bus re-energizes from a non-diesel source, its diesel
// start timer resets.
func TestElecSys_OffsiteReturnResetsDieselTimers(t *testing.T) {
	e := PlantElectricalSystems{}
	da, db := carryingDiesel(), carryingDiesel()
	busA, busB, _, _ := pesStepIn(false, false, da, db)
	e = e.StepBySeconds(DefaultTimeStep, DieselRatedLoadTime+5.0, busA, busB, da, db)
	assert.Positive(t, e.Diesel1StartTime,
		"want Diesel1StartTime>0 while the bus is dark")
	{
		busA, busB, da, db := pesStepIn(true, true, nil, nil)
		e = e.Step(DefaultTimeStep, busA, busB, da, db)
	}
	assert.Zero(t, e.Diesel1StartTime,
		"want Diesel1StartTime=0 after the bus re-energizes, got %v",
		e.Diesel1StartTime)
}

// One ESF bus energised from its diesel is enough to power
// the inverter supply for RPS/ESF instrumentation channels.
func TestElecSys_InverterPoweredFromEitherESFBus(t *testing.T) {
	e := PlantElectricalSystems{}
	d := carryingDiesel()
	busA, busB, _, _ := pesStepIn(true, false, d, d)
	busA.OnDiesel = true
	e = e.Step(DefaultTimeStep, busA, busB, d, d)
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
	p.Step(DefaultTimeStep)
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
