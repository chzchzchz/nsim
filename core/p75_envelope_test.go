package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G1 (Slice 1): seal standby success twin. With ChargingPumpRunning=0,
// standby (CCWS running + AuxSealCoolingStandby) freezes SealHeatDamageFrac;
// without standby the failure twin reaches Damage=1.
//
// "Provides high-pressure seal water for the reactor coolant pump (RCP) seals"
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:76-77
// "The seal water heat exchanger is also used to cool… This heat exchanger
// is cooled by the component cooling water system."
// specs/westinghouse_tech_manual/chapter_4/page_0380.txt:27-29
func TestEnvelope03_SealStandbySuccessTwin(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AuxSealCoolingStandby = true
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.DriveReady = true
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = true
	}
	for i := 0; i < 1200; i++ {
		p.Step(time.Second)
		p.Update()
	}
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.Less(t, pu.SealHeatDamageFrac, 0.05,
			"standby must freeze seal damage, got %v", pu.SealHeatDamageFrac)
	}

	p2 := MakePlant()
	p2.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	for _, pu := range p2.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = true
	}
	for i := 0; i < 1100; i++ {
		p2.Step(time.Second)
		p2.Update()
	}
	for _, pu := range p2.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.Equal(t, float32(1), pu.Damage,
			"without standby, seal damage must reach 1.0")
	}
}

// G2 (Slice 2): ejection drives the flux-rate trip via the EjectedEvent pulse.
//
// "High flux rate trips protect the reactor in case of rod ejection or
// accidental rod drop"
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:163-168
// "worth about one-percent in reactivity"
// specs/the_westinghouse_pressurized_water_react/chapter_2/page_039.txt:28-30
func TestEnvelope02_RodEjectionTripsReactor(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Ejected = true
	tripped := false
	powerSeen := false
	for i := 0; i < 2000 && !tripped; i++ {
		p.Step(time.Second)
		p.Update()
		if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
			tripped = true
		}
		if p.ReactorBuilding.ReactorPressureVessel.ControlRods.EjectedReactivityPct == RodEjectionWorthPct {
			powerSeen = true
		}
	}
	assert.True(t, powerSeen, "EjectedReactivityPct must equal RodEjectionWorthPct while latched")
	assert.True(t, tripped, "ejection must trip the reactor within 2000 ticks")

	// Seed guard: a fresh RPS must not trip on its first step.
	r := ReactorProtectionSystem{}
	r2 := r.Step(DefaultTimeStep, 100, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, 0, 1, 1, 0, false, false)
	assert.False(t, r2.ReactorTripped, "first-step seed must not trip")
}

// G3 (Slice 3): CV22 routing flips the verdict from PRT to condenser.
//
// "Steam from the safety and/or relief valves is piped to the pressurizer
// relief tank"
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:31-34
// "steam vapor going to the letdown condenser through the failed opened
// pressure relief valve CV22"
// specs/pwr_sim/chapter_4/page_049.txt:30-31
func TestEnvelope01_CV22RoutingToLetdownCondenser(t *testing.T) {
	p := MakePlant()
	require.NoError(t, p.GetControlMap()["ReactorBuilding/Faults/PORVStuckOpen"].Set("1"))
	require.NoError(t, p.GetControlMap()["ReactorBuilding/ReliefRoutingToLetdownCondenser"].Set("1"))
	prtBefore := p.ReactorBuilding.PressurizedReliefTank.Level.Float()
	for i := 0; i < 600; i++ {
		p.Step(time.Second)
		p.Update()
	}
	require.Greater(t, p.ReactorBuilding.LetdownCondenserLevel.Float(), 0.0,
		"condenser must fill when routed there")
	assert.Equal(t, prtBefore, p.ReactorBuilding.PressurizedReliefTank.Level.Float(),
		"PRT must not fill when routed to condenser")
	assert.False(t, p.ReactorBuilding.PRTRuptureDiscBurst,
		"no PRT burst when routed to condenser")
}

// G4 (Slice 4): mid-loop uncovered with pumps running.
//
// "Reactor coolant pumps are run only as needed"
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_228.txt:63-68
func TestEnvelopeMidLoop_UncoversCoreWithPumpsRunning(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.MidLoop = true
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = true
	}
	for i := 0; i < 10000; i++ {
		p.Step(time.Second)
		p.Update()
	}
	levelFrac := 1.0 - p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()/RCSDrainableInventoryGal
	assert.InDelta(t, MidLoopLevelFrac, levelFrac, 0.02,
		"level must settle at MidLoopLevelFrac, got %v", levelFrac)
	assert.Greater(t, p.ReactorBuilding.ReactorPressureVessel.UncoveryFrac, 0.0,
		"core must be uncovered at mid-loop")
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		assert.True(t, pu.Running, "pumps must keep running at mid-loop")
	}
	p.ReactorBuilding.RefuelingOperations.MidLoop = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	for i := 0; i < 60; i++ {
		p.Step(time.Second)
		p.Update()
	}
	pzWaterBefore := p.ReactorBuilding.Pressurizer.Water.Float()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	for i := 0; i < 60; i++ {
		p.Step(time.Second)
		p.Update()
	}
	assert.Greater(t, p.ReactorBuilding.Pressurizer.Water.Float(), pzWaterBefore,
		"charging must refill pressurizer water")
	assert.Greater(t, p.ReactorBuilding.ReactorPressureVessel.BreakDischargedNonCreditable.Float(), 0.0,
		"non-creditable floor must remain")
}
