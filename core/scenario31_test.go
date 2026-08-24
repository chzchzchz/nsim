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
// dilution leg, an isolated interval between modes, then a boration leg
// driven by the operator's boron setpoint through the CVCS tracking drive.
//
// Mode directions ("for dilution, a decrease ... for boration, an increase
// in the boric acid concentration"):
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:12-31
// Pump provides motive force during both modes; isolation reconditions beds
// between modes (bed thermal state is GAP-1 in plans/x/scenario/31.txt):
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_081.txt:33-45
func scenario31Plant(t *testing.T) Plant {
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
		Volume{Val: 450, Unit: u.Gallon}
	p.ReactorBuilding.ReactorMakeupControlValve.Auto = true
	p.PlantOperation.Mode = PowerOperation

	// Boron marches linearly in dt, so the widened step preserves the
	// trajectory while cutting step count 100x. Must not run in parallel:
	// overrideTimeStep mutates global simulation state.
	overrideTimeStep(t, 10*time.Millisecond)
	return p
}

func TestScenario31_DilutionLegLowersBoronAtRatedRate(t *testing.T) {
	p := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")

	const leg = 31250 * time.Millisecond // 25 ppm / 0.8 ppm/s
	start := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	stepFor(&p, leg)

	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.InDelta(t, start-CVCSDilutionRate*leg.Seconds(),
		cvcs.BoronConcentration, 0.01,
		"dilution must lower RCS boron at CVCSDilutionRate while the pump runs")
	assert.InDelta(t, leg.Seconds()/BTRSBedLifeSeconds, cvcs.BTRSBedSaturation,
		0.001, "bed saturation must advance with pump-running time")
	assert.InDelta(t, 450-leg.Seconds()*VCTBTRSCirculation,
		cvcs.VCTLevel.Float(), 1.0,
		"VCT must lose only the recirculation draw plus the level "+
			"controller's small residual under the auto lineup")
	assert.Zero(t, p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel.Float(),
		"BTRS cycling must send no liquid to the BRS holdup tanks")
}

func TestScenario31_IsolationIntervalHoldsChemistry(t *testing.T) {
	p := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")
	stepFor(&p, 31250*time.Millisecond)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "VolumeControl")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "0")

	held := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration
	sat := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSBedSaturation
	vct := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
	stepFor(&p, 12500*time.Millisecond)

	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.InDelta(t, held, cvcs.BoronConcentration, 0.001,
		"boron must hold with the BTRS isolated between modes")
	assert.Equal(t, sat, cvcs.BTRSBedSaturation,
		"saturation must freeze without flow through the demineralizers")
	assert.InDelta(t, vct, cvcs.VCTLevel.Float(), 1.0,
		"without the recirculation draw only the level controller's small "+
			"residual moves the VCT, versus another -25 gal of pumping")
}

func TestScenario31_BorationSetpointRoundTripsAndReleases(t *testing.T) {
	p := scenario31Plant(t)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "Dilution")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "1")
	stepFor(&p, 31250*time.Millisecond)
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode", "VolumeControl")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump", "0")

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
		stepFor(&p, time.Second)
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
