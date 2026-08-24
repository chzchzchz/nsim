package core

import (
	"testing"
	"time"

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
	// A cold, water-solid lineup runs with the heaters standing:
	// pressure follows the coolant saturation instead of the
	// hot-standby heater target.
	p.ReactorBuilding.Pressurizer.HeatersOn = false

	cm := GetControlMap(&p)
	set := func(key, val string) {
		c := cm[key]
		require.NotNil(t, c, "want control registered: %s", key)
		require.NoError(t, c.Set(val))
	}
	// A cold, depressurized lineup sits below the automatic SI
	// setpoints by construction, so its initiation signals are
	// blocked for the lineup.
	// "Prior to reaching the automatic safety injection setpoints
	// during cooldown and depressurization, the safety injection
	// initiation signals are blocked."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	set("ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock", "true")
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
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float(), 12,
		"want VCT to return near its starting level over whole duty cycles: "+
			"the unclamped tank is the inventory ledger for the balance. "+
			"The bound carries the sqrt(dP) letdown sag while the idle path "+
			"decays pressurizer pressure toward saturation "+
			"(page_227.txt:135-139): throughput is ~1200 gal per run")
	assert.GreaterOrEqual(t, minPZW, 1300.0,
		"want pressurizer essentially full through every cycle")
	assert.InDelta(t, 1500.0, p.ReactorBuilding.Pressurizer.Water.Float(), 1.0,
		"want pressurizer refilled water solid at end (surge-line "+
			"contraction now tracks the cooling Tavg program)")
	assert.Less(t, p.ReactorBuilding.Pressurizer.SteamBubble.Float(), 1.0,
		"want no meaningful steam bubble under balanced inventory")
	assert.InDelta(t, CVCSInitialBoron,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration, 2,
		"want boron concentration unchanged during checks")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"want no spurious reactor trip during lineup")
}

// With automatic makeup engaged, a VCT low-level signal initiates
// full makeup flow and the tank recovers into its design band, at
// which point the initiation clears; the manual twin never
// initiates and keeps draining.
//
// "Low-level signals from the volume control tank initiate reactor
// makeup control or flow from the refueling water storage tank as a
// backup."
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
func TestMakeup_AutoInitiatesOnVCTLowLevel(t *testing.T) {
	// Level drift, flow delivery, and the initiation latch are
	// linear in dt; crossing detection shifts by at most one tick
	// against 30 s windows and band assertions built for slack. Must
	// not run in parallel: overrideTimeStep mutates global simulation
	// state.
	overrideTimeStep(t, 10*time.Millisecond)
	run := func(auto string) (initiated, cleared bool, minVCT, maxVCT, endVCT float64) {
		p := MakePlant()
		cm := GetControlMap(&p)
		require.NoError(t,
			cm["AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning"].Set("1"))
		require.NoError(t,
			cm["AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen"].Set("0"))
		require.NoError(t,
			cm["ReactorBuilding/ReactorMakeupControlValve/Auto"].Set(auto))
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel =
			Volume{Val: VCTLowLevel + 10, Unit: u.Gallon}
		for range int(30 * time.Second / TimeStep) {
			stepN(&p, 1)
			mv := p.ReactorBuilding.ReactorMakeupControlValve
			if mv.Initiated {
				initiated = true
				assert.Greater(t, mv.Flow.Float(), 0.0,
					"an initiated makeup signal must deliver flow")
			} else if initiated {
				cleared = true
			}
			vct := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
			minVCT = min(minVCT, vct)
			maxVCT = max(maxVCT, vct)
			endVCT = vct
		}
		return initiated, cleared, minVCT, maxVCT, endVCT
	}

	autoInitiated, autoCleared, autoMin, autoMax, autoEnd := run("1")
	assert.True(t, autoInitiated,
		"the low-level signal must initiate automatic makeup")
	assert.LessOrEqual(t, autoMin, VCTLowLevel+1.0,
		"fixture must cross the low-level trigger")
	assert.True(t, autoCleared,
		"initiation must clear once the tank is restored")
	assert.GreaterOrEqual(t, autoMax, VCTDesignLevel-1.0,
		"automatic makeup must restore the design band: got %v", autoMax)
	assert.GreaterOrEqual(t, autoEnd, VCTLowLevel-1.0,
		"under a sustained charging draw the tank holds inside its "+
			"band instead of emptying: got %v", autoEnd)

	manualInitiated, _, _, _, manualEnd := run("0")
	assert.False(t, manualInitiated,
		"manual makeup never latches initiation")
	assert.Less(t, manualEnd, VCTLowLevel,
		"without makeup the manual twin keeps draining below the trigger")
}

// Taking a protection channel to test removes its measurement from
// trip voting and ESFAS triggering, so an injected stimulus on that
// channel alone cannot actuate protections; with all channels in
// service the same state actuates.
//
// "The protection system normally uses 2/4 logic but reverts to a
// safe 2/3 logic when part of the system is being tested or is
// involved in maintenance."
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
func TestRPS_ChannelTestSuppressesTestedChannel(t *testing.T) {
	lowSteamline := RPSLowSteamlinePressureTrip - 10

	tested := ReactorProtectionSystem{ChannelTest: RPSSteamlineChannelInTest}
	tested = tested.Step(lowSteamline, PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0)
	assert.Equal(t, 0, tested.TripSignalCount,
		"the isolated channel contributes no trip vote")
	assert.False(t, tested.SafetyInjectionActuated,
		"no SI actuation while the steamline channel is under test")
	assert.False(t, tested.SteamlineIsolationActuated,
		"steamline isolation stays clear during its own channel test")

	inService := ReactorProtectionSystem{}
	inService = inService.Step(lowSteamline, PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0)
	assert.True(t, inService.SafetyInjectionActuated,
		"with channels in service the same state actuates SI")

	highContainment := RPSHighContainmentPressureTrip + 10
	sprayTested := ReactorProtectionSystem{
		ChannelTest: RPSContainmentChannelInTest}
	sprayTested = sprayTested.Step(SteamGeneratorShellPressure, PressurizerHeaterTarget, 50,
		CoreReferenceInletF, highContainment, 0)

	assert.False(t, sprayTested.ContainmentSprayActuated,
		"containment spray stays clear during its channel test")
	assert.False(t, sprayTested.ContainmentIsolationPhaseBActuated,
		"Phase B isolation stays clear during the containment test")
}

// The scenario-1 gap-closure controls round-trip through the bridge.
func TestBridge_Scenario1GapControlsRoundTrip(t *testing.T) {
	p := MakePlant()
	cm := GetControlMap(&p)

	auto := cm["ReactorBuilding/ReactorMakeupControlValve/Auto"]
	require.NotNil(t, auto, "want Auto control registered")
	require.NoError(t, auto.Set("1"))
	assert.Equal(t, "1", auto.Get())

	channel := cm["ReactorBuilding/ReactorProtectionSystem/ChannelTest"]
	require.NotNil(t, channel, "want ChannelTest control registered")
	require.NoError(t, channel.Set("Tcold"))
	assert.Equal(t, "Tcold", channel.Get())
	require.Error(t, channel.Set("Bogus"),
		"unknown enum values must be rejected")
	assert.Equal(t, "Tcold", channel.Get(),
		"a rejected set leaves the selection untouched")
	require.NoError(t, channel.Set("None"))
	assert.Equal(t, "None", channel.Get())

	sm := GetSensorMap(&p)
	for _, id := range []string{
		"ReactorBuilding/ReactorMakeupControlValve/Initiated",
		"ReactorBuilding/ReactorProtectionSystem/ChannelUnderTest",
	} {
		s, ok := sm[id]
		require.Truef(t, ok, "want sensor %q registered", id)
		out, err := s.Get()
		require.NoErrorf(t, err, "Get %q", id)
		assert.NotEmptyf(t, out, "want non-empty value from sensor %q", id)
	}
}
