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
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.Outside.ServiceWaterSystem.PumpRunning = true
	stepN(&p, 1, DefaultTimeStep)
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
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Outside.ServiceWaterSystem.PumpRunning = true
	stepN(&p, 1, DefaultTimeStep)
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
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp =
		Temperature{Val: 120, Unit: u.Fahrenheit}
	stepN(&p, int(10*time.Second/DefaultTimeStep), DefaultTimeStep)
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
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel =
		Volume{Val: SISRWSTCapacity, Unit: u.Gallon}
	rwstInitial := p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	stepN(&p, 1, DefaultTimeStep)
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
	t.Parallel()
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

	stepN(&p, 1, DefaultTimeStep)
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

// With both ESF buses off, the CW pump cannot develop flow even
// though the operator latch Running=true is preserved. The CFW
// pumps ride on the 6600 V plant distribution bus fed by UAT or
// SAT, mirroring the SWS readiness gate
// (plans/x/uat_sat_gating.md); a blackout drops the bus so the
// pump spins down and the cooling-tower fan stops.
func TestESFGating_CWNoFlowWithBusesOff(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.Outside.CirculatingWaterPump.Running = true
	stepN(&p, 1, DefaultTimeStep)
	assert.False(t, p.Outside.CirculatingWaterPump.DriveReady,
		"want CW pump DriveReady=false with buses off, got true")
	assert.Zero(t, p.Outside.CirculatingWaterSystem.FlowGPM,
		"want CW FlowGPM=0 with buses off, got %v",
		p.Outside.CirculatingWaterSystem.FlowGPM)
	assert.Zero(t, p.Outside.CirculatingWaterSystem.PumpsInService,
		"want CW PumpsInService=0 with buses off, got %v",
		p.Outside.CirculatingWaterSystem.PumpsInService)
	assert.False(t, p.Outside.CoolingTower.FanRunning,
		"want cooling-tower FanRunning=false with buses off, got true")
	assert.True(t, p.Outside.CirculatingWaterPump.Running,
		"want CW pump Running latch preserved (operator intent), got false")
}

// With one ESF bus (ESFBusB) still energised the CW pump
// develops full design flow: the plant distribution bus is fed
// by an OR of the two ESF buses, so a single-bus supply keeps
// the CW pump motors energised.
func TestESFGating_CWFlowsWithOneBusOn(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Outside.CirculatingWaterPump.Running = true
	stepN(&p, 1, DefaultTimeStep)
	assert.True(t, p.Outside.CirculatingWaterPump.DriveReady,
		"want CW pump DriveReady=true with ESFBusB on, got false")
	assert.InDelta(t, CirculatingWaterDesignFlowGPM,
		p.Outside.CirculatingWaterSystem.FlowGPM, 1e-9,
		"want CW FlowGPM=design with one bus on, got %v",
		p.Outside.CirculatingWaterSystem.FlowGPM)
}

// With both ESF buses off the SFPCS pump loses power: the 480 V
// MCC downstream of an ESF bus is de-energised, so
// PurificationFlow drops to zero and the standby auto-start is
// cleared even though PumpRunning stays latched
// (plans/x/uat_sat_gating.md).
func TestESFGating_SFPCSNoFlowWithBusesOff(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	sf := &p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	sf.LoopAPumpRunning = true
	sf.LoopBPumpRunning = true
	stepN(&p, 1, DefaultTimeStep)
	assert.False(t, p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.DriveReady,
		"want SFPC DriveReady=false with buses off, got true")
	assert.Zero(t, p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PurificationFlow,
		"want SFPC PurificationFlow=0 with buses off, got %v",
		p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PurificationFlow)
	assert.False(t, p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.StandbyRunning,
		"want SFPC StandbyRunning=false with buses off, got true")
	assert.True(t, p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.LoopAPumpRunning &&
		p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.LoopBPumpRunning,
		"want SFPC duty-loop latches preserved (operator intent), got false")
}

// When power returns (bus re-energised) the CW pump resumes flow
// automatically: the Running latch was preserved during the
// blackout, so the readiness gate alone gates flow.
func TestESFGating_CWRecoversWhenPowerReturns(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	p.Outside.CirculatingWaterPump.Running = true
	stepN(&p, 1, DefaultTimeStep)
	assert.Zero(t, p.Outside.CirculatingWaterSystem.FlowGPM,
		"precondition: no CW flow during blackout")
	// Restore bus power and step again.
	p.Electrical.ESFBusA.BreakerClosed = true
	stepN(&p, 1, DefaultTimeStep)
	assert.True(t, p.Outside.CirculatingWaterPump.DriveReady,
		"want CW pump DriveReady=true after power restored, got false")
	assert.InDelta(t, CirculatingWaterDesignFlowGPM,
		p.Outside.CirculatingWaterSystem.FlowGPM, 1e-9,
		"want CW FlowGPM=design after power restored, got %v",
		p.Outside.CirculatingWaterSystem.FlowGPM)
}
