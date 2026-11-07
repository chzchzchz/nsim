package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateCrossPsia is the 1900 psig pressurization gate expressed in
// absolute pounds per square inch.
//
// "Before raising primary system pressure above 1900 psig (132 bar),
// steam generator pressure must be greater than 585 psig (41 bar) or
// a low steamline pressure safety injection will occur."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:150-153
const gateCrossPsia = 1914.7

// newGatePlant seeds a subcritical, rods-in plant below the
// pressurization gate with the CVCS lineup balanced so the pressurizer
// inventory stays put while the heaters drive pressure. The only
// scenario knob is the secondary header pressure.
func newGatePlant(sgPressure Pressure) *Plant {
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Insertion = 1
	rpv.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.ChargingPumpRunning = false
	cvcs.LetdownValveOpen = false
	p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 0
	turb := &p.TurbineBuilding.Turbine
	turb.StopValve.Open = 0
	turb.DumpValve.Open = 0
	sg := &p.ReactorBuilding.SteamGenerator
	sg.Pressure = sgPressure
	pzr := &p.ReactorBuilding.Pressurizer
	pzr.HeatersOn = false
	pzr.SprayOpen = 0
	pzr.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pzr.Water = Volume{Val: 1440, Unit: u.Gallon}
	// Coherent seed: 60 gal of steam space sits at ~1650 psia per
	// the bubble-pressure relation, so the stored pressure matches
	// the lineup instead of the full-power default ceiling.
	pzr.Pressure = Pressure(1650)
	return &p
}

func setGateControls(t *testing.T, p *Plant, values map[string]string) {
	t.Helper()
	controls := p.GetControlMap()
	for path, value := range values {
		require.NoError(t, controls[path].Set(value), "control %s", path)
	}
}

func stepSeconds(p *Plant, seconds int, dt time.Duration) {
	for range int(time.Duration(seconds) * time.Second / dt) {
		p.Step(dt)
		p.Update()
	}
}

// The planned-startup lineup blocks the SI initiation signals and
// isolates the accumulators, so the operator may carry pressurizer
// pressure across the 1900 psig gate with a still-vented secondary:
// the block holds SI off through the crossing and the accumulator
// inventory stays put. The block is an operator lineup device; the
// corpus states only that the signals ARE blocked during planned
// lineups (chapter_16/page_227.txt:108-110), not that any pressure
// strips it, so none is modelled.
//
// "The Safety Injection System is lined up for operation, with the
// initiation signal manually blocked and accumulators isolated."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:11-13
func TestStartupLineup_BlockedCrossingHoldsAcross1900Psig(t *testing.T) {
	t.Parallel()
	p := newGatePlant(Pressure(15))
	setGateControls(t, p, map[string]string{
		"ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock":    "1",
		"ReactorBuilding/SafetyInjectionSystem/AccumulatorIsolation/Shut": "1",
		"ReactorBuilding/Pressurizer/HeatersOn":                           "1",
	})

	levelBefore := p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level.Float()
	stepSeconds(p, 6, DefaultTimeStep)

	assert.GreaterOrEqual(t, float64(p.ReactorBuilding.Pressurizer.Pressure),
		gateCrossPsia,
		"heaters must carry pressurizer pressure across the 1900 psig gate")
	assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionBlocked,
		"the operator block stands through the crossing")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated,
		"the blocked initiation signal must not actuate SI during the crossing")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"a lone low-steamline channel must not trip the reactor")
	assert.False(t, p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Discharging)
	assert.InDelta(t, levelBefore,
		p.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level.Float(), 1e-9,
		"isolated accumulators must hold their inventory")
}

// With the secondary already warmer than the 585 psig procedural limit,
// the same unblocked crossing proceeds without any protective
// actuation: the low-steamline channel never leaves its normal band.
//
// "steam generator pressure must be greater than 585 psig (41 bar)"
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:150-153
func TestStartupGate_WarmSecondaryPermitsUnblockedCrossing(t *testing.T) {
	t.Parallel()
	p := newGatePlant(Pressure(RPSLowSteamlinePressureTrip + 110))
	setGateControls(t, p, map[string]string{
		"ReactorBuilding/Pressurizer/HeatersOn": "1",
	})
	stepSeconds(p, 4, DefaultTimeStep)

	assert.Greater(t, float64(p.ReactorBuilding.SteamGenerator.Pressure),
		float64(RPSLowSteamlinePressureTrip),
		"the trapped warm header must hold above the low-steamline setpoint")
	assert.GreaterOrEqual(t, float64(p.ReactorBuilding.Pressurizer.Pressure),
		gateCrossPsia,
		"heaters must carry pressurizer pressure across the 1900 psig gate")
	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.False(t, rps.SafetyInjectionActuated,
		"a warm secondary must keep the low-steamline channel clear")
	assert.False(t, rps.SteamlineIsolationActuated)
	assert.False(t, rps.ReactorTripped)
}
