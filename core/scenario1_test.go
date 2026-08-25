package core

import (
	"math"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 1 (pre-critical lineup and checks) replayed in-process from
// plans/x/scenario1_precritical: the cold-shutdown start state plus the
// manual charging/letdown duty cycle, asserting the Phase 0 expectation
// bands E1-E7 recorded there.
//
// Spec justification:
//   - "Verify electrical power to startup systems, instrumentation
//     calibrated and functioning, fluid systems valved in; fill CVCS
//     at RCS boron concentration with volume control tank in normal
//     range; place Component Cooling Water System in service."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:19-116
//   - "it must maintain the RCS mass inventory, that is,
//     the pressurizer level, by balancing the charging
//     flow to the letdown flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:10-12
func newScenario1Plant(t *testing.T) Plant {
	t.Helper()
	p := MakePlant()

	// Cold-shutdown start state per plans/x/scenario1_precritical/
	// start.json: water-solid pressurizer at 140 F, subcritical core
	// on inserted rods holding neutron-source fission power with
	// day-old decay heat ("~0.4% level about a day after shutdown",
	// specs/wiki/Decay_heat.txt).
	p.ReactorBuilding.Pressurizer.Water =
		Volume{Val: p.ReactorBuilding.Pressurizer.Volume.Float(), Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.SteamBubble = Volume{}
	p.ReactorBuilding.Pressurizer.Temperature =
		Temperature{Val: 140, Unit: u.Fahrenheit}
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Inlet = Temperature{Val: 140, Unit: u.Fahrenheit}
	rpv.Outlet = Temperature{Val: 140, Unit: u.Fahrenheit}
	rpv.Pressure = 415
	rpv.Insertion = 1.0
	rpv.DemandInsertion = 1.0
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = Watts(0.004 * float64(CoreRatedThermalPower))
	return p
}

func TestScenario1_PrecriticalLineupReplay(t *testing.T) {
	p := newScenario1Plant(t)

	require.True(t, p.Electrical.ESFBusA.BreakerClosed,
		"lineup starts with both ESF bus breakers closed")
	require.True(t, p.Electrical.ESFBusB.BreakerClosed)
	require.Equal(t, ColdShutdown, p.PlantOperation.Mode)

	cm := GetControlMap(&p)
	set := func(key, val string) {
		c := cm[key]
		require.NotNil(t, c, "want control registered: %s", key)
		require.NoError(t, c.Set(val))
	}
	// The cold depressurized lineup sits below the automatic SI
	// setpoints, so its initiation signals are blocked for the
	// evolution.
	// "Prior to reaching the automatic safety injection setpoints
	// during cooldown and depressurization, the safety injection
	// initiation signals are blocked."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	set("ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock", "true")
	set("AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning", "1")
	set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")

	const (
		windowMs       = 20000
		dutyCycleMs    = 10000
		letdownOpenMs  = 4000
		flowBand       = 1.0
		vctNormalFloor = 400.0
		waterFloorGal  = 1300.0
	)

	letdownOpen := true
	ccwsLatchedAt := -1
	busesHeld := true
	chargingHeld := true
	letdownHeld := true
	vctHeld := true
	diverted := false
	minWater := p.ReactorBuilding.Pressurizer.Water.Float()
	for ms := 0; ms < windowMs; ms++ {
		// Duty cycle from plans/x/scenario1_precritical/commands.txt:
		// letdown open the first 4 s of each 10 s cycle balances the
		// continuous 30 gal/s charge against 75 gal/s design letdown
		// (30 - 75*0.4 = 0).
		open := ms%dutyCycleMs < letdownOpenMs
		if open != letdownOpen {
			letdownOpen = open
			if open {
				set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")
			} else {
				set("AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "0")
			}
		}
		stepN(&p, 1)

		if !p.Electrical.ESFBusA.Energized || !p.Electrical.ESFBusB.Energized {
			busesHeld = false
		}
		cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
		if cvcs.ChargingFlow.Float() < CVCSDesignChargingFlow.Float()-flowBand ||
			cvcs.ChargingFlow.Float() > CVCSDesignChargingFlow.Float()+flowBand {
			chargingHeld = false
		}
		if letdownOpen {
			// The orifice station passes design flow only at full
			// RCS pressure; below it the achievable flow follows the
			// square root of the available differential.
			// "Very little letdown flow is possible through the normal
			// letdown flow path at low primary system pressures due to
			// the low differential pressure across the CVCS letdown
			// orifices."
			// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:135-139
			want := CVCSDesignLetdownFlow.Float() * math.Sqrt(
				min(float64(p.ReactorBuilding.Pressurizer.Pressure)/CVCSLetdownDesignPsia, 1))
			got := cvcs.LetdownFlow.Float()
			if got < want-flowBand || got > want+flowBand {
				letdownHeld = false
			}
		}
		vct := cvcs.VCTLevel.Float()
		if vct < vctNormalFloor || vct > VCTHighLevel {
			vctHeld = false
		}
		diverted = diverted || cvcs.Diverting
		if ccwsLatchedAt < 0 && p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning {
			ccwsLatchedAt = ms
		}
		minWater = min(minWater, p.ReactorBuilding.Pressurizer.Water.Float())
	}

	// E1: both ESF buses energized at every sample.
	assert.True(t, busesHeld, "both ESF buses must stay energized through the run")
	// E2: CCWS pump auto-latches once the bus power is verified and
	// the inlet sits at its steady-state setpoint.
	assert.GreaterOrEqual(t, ccwsLatchedAt, 0, "want CCWS pump latched")
	assert.LessOrEqual(t, ccwsLatchedAt, 2000, "want CCWS pump latched within 2 s")
	assert.InDelta(t, CCWSInletSteadyStateTemp,
		p.AuxiliaryBuilding.ComponentCoolingWaterSystem.InletTemp.Float(), 2,
		"want CCWS inlet at steady-state setpoint at end")
	// E3/E4: design flows whenever the lineup is open.
	assert.True(t, chargingHeld, "charging flow must hold design while lined up")
	assert.True(t, letdownHeld,
		"letdown flow must track the pressure-scaled design while open")
	// E5: checks neither borate nor dilute.
	assert.InDelta(t, CoreRatedCriticalBoronPpm,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration, 2,
		"want boron concentration unchanged during checks (the "+
			"construction seed is the rated-critical chemistry)")
	assert.Equal(t, CVCSVolumeControl,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode,
		"want no boration/dilution mode entered")
	assert.False(t, diverted, "want balanced cycle to avoid high-level diversion")
	// E6: the pressurizer stays completely filled under the balance.
	assert.GreaterOrEqual(t, minWater, waterFloorGal,
		"want pressurizer essentially full through every cycle")
	assert.GreaterOrEqual(t, p.ReactorBuilding.Pressurizer.Water.Float(), 1490.0,
		"want pressurizer refilled water solid at end")
	assert.LessOrEqual(t, p.ReactorBuilding.Pressurizer.SteamBubble.Float(), 10.0,
		"want no meaningful steam bubble under balanced inventory")
	// E7: the VCT holds its normal operating range throughout.
	assert.True(t, vctHeld, "VCT level must stay inside its normal range")
	assert.InDelta(t, VCTDesignLevel,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float(), 12,
		"want VCT back near design level over whole duty cycles")
	// The evolution must not disturb the shutdown condition.
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"want no spurious reactor trip during lineup")
	assert.Equal(t, ColdShutdown, p.PlantOperation.Mode,
		"want the plant to remain in cold shutdown")
}
