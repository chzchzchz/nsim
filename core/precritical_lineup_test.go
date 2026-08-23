package core

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario artifact assertions for the pre-critical lineup evolution
// (plans/x/scenario1_precritical). These encode the physics claims as
// fast unit tests instead of replaying the headless run.

// With an ESF bus energised the CCWS pump auto-latches on the first
// step and the inlet temperature holds its steady-state setpoint:
// the lineup places the Component Cooling Water System in service
// without further operator action.
//
// Spec justification:
//   - "The Component Cooling Water System is in service,
//     supplying cooling water to the NSSS systems."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:113-115
func TestCCWS_AutoLatchesOnESFBusPower(t *testing.T) {
	p := MakePlant()
	require.True(t, p.Electrical.ESFBusA.BreakerClosed)
	stepN(&p, 2)
	assert.True(t, p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning,
		"want CCWS pump latched once the ESF bus is energised")
	assert.InDelta(t, CCWSInletSteadyStateTemp,
		p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp.Float(), 2,
		"want CCWS inlet at steady-state setpoint")
}

// A balanced charging/letdown duty cycle holds the pressurizer water
// solid and keeps the volume control tank inside its normal operating
// range without boration or dilution. Charging runs continuously at
// CVCSDesignChargingFlow; letting down at CVCSDesignLetdownFlow for a
// 0.4 fraction of each cycle nets zero inventory change
// (30 - 75*0.4 = 0 gal/s).
//
// Spec justification:
//   - "The RCS and pressurizer are completely filled with borated
//     water and the system pressure is maintained by charging and
//     letdown flow control."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:94-97
//   - "The Chemical and Volume Control System (CVCS) is filled with
//     water at the same boron concentration as the RCS. The volume
//     control tank level is within the normal operating range..."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:101-105
//   - "it must maintain the RCS mass inventory, that is,
//     the pressurizer level, by balancing the charging
//     flow to the letdown flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:10-12
func TestPrecriticalLineup_DutyCycleBalancesChargingLetdown(t *testing.T) {
	p := MakePlant()
	// Cold-shutdown primary conditions per the scenario start state:
	// water-solid pressurizer, subcritical core on inserted rods with
	// neutron-source fission power and day-old decay heat.
	p.ReactorBuilding.Pressurizer.Water =
		Volume{Val: p.ReactorBuilding.Pressurizer.Volume.Float(), Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.SteamBubble =
		Volume{Val: 0, Unit: u.Gallon}
	p.ReactorBuilding.ReactorPressureVessel.Insertion = 1.0
	p.ReactorBuilding.ReactorPressureVessel.DemandInsertion = 1.0
	p.ReactorBuilding.ReactorPressureVessel.FissionPower =
		CoreNeutronSourcePower
	p.ReactorBuilding.ReactorPressureVessel.DecayHeatFast = 0
	// Day-old decay heat: "~0.4% level about a day after shutdown"
	// per the two-pool model narrative (specs/wiki/Decay_heat.txt,
	// core/reactor.go decay-heat constants).
	p.ReactorBuilding.ReactorPressureVessel.DecayHeatSlow = Watts(
		0.004 * float64(CoreRatedThermalPower))

	cm := GetControlMap(&p)
	set := func(key, val string) {
		c := cm[key]
		require.NotNil(t, c, "want control registered: %s", key)
		require.NoError(t, c.Set(val))
	}
	set("AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning", "1")
	set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")

	letdownOpen := true
	set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")
	startVCT := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
	minVCT := startVCT
	maxVCT := minVCT
	minPZW := p.ReactorBuilding.Pressurizer.Water.Float()
	for i := 0; i < 20000; i++ {
		// 10 s cycle: letdown open the first 4 s, shut the last 6 s.
		open := i%10000 < 4000
		if open != letdownOpen {
			letdownOpen = open
			if open {
				set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")
			} else {
				set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "0")
			}
		}
		stepN(&p, 1)
		vct := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
		minVCT = min(minVCT, vct)
		maxVCT = max(maxVCT, vct)
		pzw := p.ReactorBuilding.Pressurizer.Water.Float()
		minPZW = min(minPZW, pzw)
	}
	assert.GreaterOrEqual(t, minVCT, 400.0,
		"want VCT never below normal-range floor")
	assert.LessOrEqual(t, maxVCT, VCTHighLevel,
		"want VCT never above high-level diversion point")
	assert.False(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Diverting,
		"want balanced cycle to avoid high-level diversion")
	assert.InDelta(t, startVCT,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float(), 1e-6,
		"want VCT to return to its starting level over whole duty cycles: "+
			"the unclamped tank is the inventory ledger for the balance")
	assert.GreaterOrEqual(t, minPZW, 1300.0,
		"want pressurizer essentially full through every cycle")
	assert.Equal(t, 1500.0, p.ReactorBuilding.Pressurizer.Water.Float(),
		"want pressurizer refilled water solid at end")
	assert.Zero(t, p.ReactorBuilding.Pressurizer.SteamBubble.Float(),
		"want no steam bubble under balanced inventory")
	assert.InDelta(t, CVCSInitialBoron,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration, 2,
		"want boron concentration unchanged during checks")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"want no spurious reactor trip during lineup")
}
