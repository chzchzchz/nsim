package core

import (
	"strconv"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 31 exercises one full BTRS cycle at power operation: a manual
// dilution leg, a recirculation-only conditioning interval, then a boration
// leg driven by the operator's boron setpoint through the CVCS tracking
// drive.
//
// Mode directions ("for dilution, a decrease ... for boration, an increase
// in the boric acid concentration"):
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:12-31
// The recirculation pump provides motive force during both modes, and with
// the loop isolated it reconditions the beds toward the next mode's
// temperature (dilution leaves them at 50 F; boration wants 140 F):
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:33-45
func scenario31Plant(t *testing.T) (Plant, time.Duration) {
	t.Helper()
	p := MakePlant()
	// Power-operation lineup mirrors plans/x/scenario/31/start.json: all
	// RCPs running (chapter_16/page_224.txt:35-37), automatic charging /
	// letdown level control (chapter_4/page_076.txt:139-149), RWST backup
	// makeup armed (chapter_4/page_075.txt:1-4).
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel =
		Volume{Val: 520, Unit: u.Gallon}
	p.ReactorBuilding.ReactorMakeupControlValve.Auto = true
	p.PlantOperation.Mode = PowerOperation

	// Boron marches linearly in dt, so the widened step preserves the
	return p, 10 * time.Millisecond
}

func TestScenario31_DilutionLegLowersBoronAtRatedRate(t *testing.T) {
	t.Parallel()
	p, testDt := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")

	const leg = 31250 * time.Millisecond // 25 ppm / 0.8 ppm/s
	start := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	startVct := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
	stepFor(&p, leg, testDt)

	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.InDelta(t, start-CVCSDilutionRate*leg.Seconds(),
		cvcs.BoronConcentration, 0.01,
		"dilution must lower RCS boron at CVCSDilutionRate while the pump runs")
	assert.InDelta(t, startVct-leg.Seconds()*VCTBTRSCirculation,
		cvcs.VCTLevel.Float(), 1.0,
		"VCT must lose only the recirculation draw plus the level "+
			"controller's small residual under the auto lineup")
	assert.InDelta(t, BTRSColdBedF, cvcs.BTRSBedTemperatureF, 0.01,
		"the first exchange campaign must leave the beds at its own temperature")
	assert.InDelta(t, leg.Seconds()/BTRSBedLifeSeconds, cvcs.BTRSBedSaturation,
		0.001, "bed saturation must advance with pump-running time")
	assert.Zero(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel.Float(),
		"BTRS cycling must send no liquid to the BRS holdup tanks")
}

// With the loop isolated but recirculating, the pump conditions the idle
// beds toward the opposite of the last exchange mode without moving boron:
// "after completion of a dilution mode ... the BTRS recirculation pump is
// used to increase the temperature of the demineralizers to 140°F for the
// subsequent boration mode"
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:41-45
func TestScenario31_IsolationIntervalConditionsWithoutExchanging(t *testing.T) {
	t.Parallel()
	p, testDt := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")
	stepFor(&p, 31250*time.Millisecond, testDt)

	// Pump stays lined up; the operator releases the boron mode so the
	// isolated recirculation doubles as bed reconditioning duty for the
	// next mode (page_081.txt:41-45).
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "VolumeControl")
	held := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	sat := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSBedSaturation
	cold := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSBedTemperatureF
	stepFor(&p, 35000*time.Millisecond, testDt)

	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.InDelta(t, held, cvcs.BoronConcentration, 0.001,
		"conditioning circulation must not change RCS boron")
	assert.Equal(t, sat, cvcs.BTRSBedSaturation,
		"exchange capacity is consumed only by actual boron transfer")
	assert.Greater(t, cvcs.BTRSBedTemperatureF, cold,
		"the idle recirculation must heat the beds toward boration readiness")
	assert.GreaterOrEqual(t, cvcs.BTRSBedTemperatureF,
		BTRSHotBedF-BTRSBedReadyBandF,
		"the scripted isolation must leave the beds boration-ready")
}

func TestScenario31_BorationSetpointRoundTripsAndReleases(t *testing.T) {
	t.Parallel()
	p, testDt := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")
	stepFor(&p, 31250*time.Millisecond, testDt)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "VolumeControl")
	stepFor(&p, 35000*time.Millisecond, testDt)

	// The setpoint drive selects Boration + pump on its own and releases
	// chemistry to volume control inside the arrival tolerance
	// (core/cvcs.go:116-131, 250-269); the makeup-flow integrator analog
	// is chapter_16/page_231.txt:127-133. The target is whatever the
	// lineup started at, not a global constant.
	start := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	setControl(t, &p,
		"ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint",
		strconv.FormatFloat(start, 'f', -1, 64))
	require.True(t,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronSetpointTracking,
		"writing the setpoint must latch the tracking drive")

	tracking := true
	for i := 0; tracking && i < 120; i++ {
		stepFor(&p, time.Second, testDt)
		cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
		tracking = cvcs.BoronSetpointTracking
		if tracking {
			assert.True(t, cvcs.BTRSRecirculationPumpRunning,
				"the drive must keep the pump lined up while tracking")
		}
	}
	require.False(t, tracking,
		"drive must release within two minutes of the round trip")

	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.False(t, tracking, "drive must release after round trip")
	assert.InDelta(t, start, cvcs.BoronConcentration,
		CVCSBoronSetpointTolerancePpm+0.01,
		"round trip must land within the arrival tolerance of the start value")
	assert.Equal(t, CVCSVolumeControl, cvcs.Mode,
		"released chemistry must be back in volume control")
	assert.False(t, cvcs.BTRSRecirculationPumpRunning,
		"release must secure the recirculation pump")
}

// Thermal regeneration is direction-dependent: switching straight from
// dilution to boration on cold beds exchanges nothing until the beds
// reach boration temperature, after which the same running mode picks
// the exchange back up.
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:41-45
func TestScenario31_UnconditionedOppositeModeHoldsUntilHot(t *testing.T) {
	t.Parallel()
	p, testDt := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")
	stepFor(&p, 31250*time.Millisecond, testDt)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Boration")

	stepFor(&p, 5*time.Second, testDt)
	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.Zero(t, cvcs.BorationRate,
		"cold beds must not release boron into the charging stream")
	assert.Greater(t, cvcs.BTRSBedTemperatureF, BTRSColdBedF,
		"the active mode's exchangers must still condition the beds")

	for i := 0; i < 40 && !cvcs.bedAtTarget(CVCSBoration); i++ {
		stepFor(&p, time.Second, testDt)
		cvcs = p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	}
	require.True(t, cvcs.bedAtTarget(CVCSBoration),
		"running the mode must eventually bring the beds to temperature")
	boronBefore := cvcs.BoronConcentration
	stepFor(&p, 5*time.Second, testDt)
	assert.Greater(t,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration,
		boronBefore,
		"once the beds are hot, boration must raise RCS boron again")
}
