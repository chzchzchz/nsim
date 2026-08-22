package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ESF-bus power gating. With both ESF buses off, the
// safeguarded loads (HHSI, SWS, CCWS, CSS, RHR trains) cannot
// develop flow even though their pump-running intent may be
// latched. The DriveReady flag is wired from
// ESFBusA.Energized || ESFBusB.Energized (per-train for the
// redundant RHR / HHSI / EFWS motor pumps, OR for the
// shared-cooling SWS / CCWS / CSS).
//
// Spec justification:
//   - "Each power source, diesel generator and offsite, up to
//     the point of connection to the engineered safety
//     features system power buses, is physically and
//     electrically independent."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:30-36
//   - "The engineered safety features buses are normally
//     energized from system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:81-82

// With both ESF buses off, SWS PumpRunning=true produces no
// flow. DriveReady drops to false; the SWS Step zero-flow
// gate fires.
func TestESFGating_SWSNoFlowWithBusesOff(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.Outside.ServiceWaterSystem.PumpRunning = true
	stepN(&p, 1)
	assert.False(t, p.Outside.ServiceWaterSystem.DriveReady,
		"want SWS DriveReady=false with buses off, got true")
	assert.Zero(t, p.Outside.ServiceWaterSystem.Flow,
		"want SWS Flow=0 with buses off, got %v",
		p.Outside.ServiceWaterSystem.Flow)
}

// With ESFBusA off but ESFBusB on, SWS still has bus power
// (OR gate across the two buses). The pump should develop
// full design flow.
func TestESFGating_SWSFlowsWithOneBusOn(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Outside.ServiceWaterSystem.PumpRunning = true
	stepN(&p, 1)
	assert.True(t, p.Outside.ServiceWaterSystem.DriveReady,
		"want SWS DriveReady=true with ESFBusB on, got false")
	assert.InDelta(t, ServiceWaterDesignFlow,
		p.Outside.ServiceWaterSystem.Flow, 1e-9,
		"want SWS Flow=ServiceWaterDesignFlow with one bus on, got %v",
		p.Outside.ServiceWaterSystem.Flow)
}

// With both ESF buses off, CCWS DriveReady=false and the
// outlet temperature tracks inlet (no recovery-lag
// integration).
func TestESFGating_CCWSNoRecoveryWithBusesOff(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp =
		u.NewValue(120, u.Fahrenheit)
	stepN(&p, int(10*time.Second/TimeStep))
	assert.False(t, p.AuxiliaryBuilding.ComponentCoolingWaterSystem.DriveReady,
		"want CCWS DriveReady=false with buses off, got true")
	assert.InDelta(t, 120.0,
		p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp.Float(),
		1e-9,
		"want CCWS InletTemp unchanged with buses off, got %v",
		p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp.Float())
}

// With both ESF buses off, the SI signal still auto-latches
// HHSIPumpRunning=true (operator intent) but the actual
// injection flow is gated to zero. This mirrors the
// AFWPump pattern: the Running flag preserves operator
// intent while DriveReady gates effective flow.
func TestESFGating_HHSINoFlowWithBusesOff(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel =
		u.NewValue(SISRWSTCapacity, u.Gallon)
	rwstInitial := p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	stepN(&p, 1)
	require.True(t, p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning,
		"precondition: HHSIPumpRunning should latch on SI signal")
	assert.False(t, p.ReactorBuilding.SafetyInjectionSystem.HHSIDriveReady,
		"want HHSIDriveReady=false with buses off, got true")
	assert.Zero(t,
		p.ReactorBuilding.SafetyInjectionSystem.HHSIInjectionFlow.Float(),
		"want HHSIInjectionFlow=0 with buses off, got %v",
		p.ReactorBuilding.SafetyInjectionSystem.HHSIInjectionFlow.Float())
	assert.InDelta(t, rwstInitial,
		p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float(), 1e-9,
		"RWST should not deplete with HHSI flow gated, got %v (was %v)",
		p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float(),
		rwstInitial)
}

// RHR trains are on dedicated ESF buses (TrainA on BusA,
// TrainB on BusB). With BusA off, TrainA cannot develop flow
// even if Pump.Running=true; TrainB is unaffected.
func TestESFGating_RHRTrainGatedByOwnBus(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	r := &p.AuxiliaryBuilding.ResidualHeatRemovalSystem
	r.Running = true
	r.TrainA = RHRSTrain{
		Pump:           Pump{Running: true},
		BypassValve:    Valve{Open: 0},
		WarmupComplete: true,
		SuctionValves:  [2]Valve{{Open: 1}, {Open: 1}},
	}
	r.TrainB = r.TrainA

	stepN(&p, 1)
	assert.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.DriveReady,
		"want TrainA DriveReady=false with BusA off, got true")
	assert.True(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.DriveReady,
		"want TrainB DriveReady=true with BusB on, got false")
	assert.Zero(t,
		p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.ReturnFlow.Float(),
		"want TrainA ReturnFlow=0 with BusA off, got %v",
		p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.ReturnFlow.Float())
	assert.Greater(t,
		p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.ReturnFlow.Float(),
		0.0,
		"want TrainB ReturnFlow>0 with BusB on, got %v",
		p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.ReturnFlow.Float())
}
