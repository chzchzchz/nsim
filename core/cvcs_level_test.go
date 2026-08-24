package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

func autoCVCS() *CVCS {
	return &CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
		AutoLevelControl:    true,
	}
}

// The level program anchors on the spec's two operating points:
// no-load Tavg holds 25 percent and full-load Tavg holds 60
// percent, flat-clamped outside the band.
//
// Spec:
//   "maintain pressurizer level at a programmed value which
//   varies with RCS average coolant temperature."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:36-40
func TestPzrLevelProgramAnchors(t *testing.T) {
	fullLoadTavg := Temperature{
		Val: (CoreReferenceInletF + CoreReferenceOutletF) / 2,
		Unit: u.Fahrenheit,
	}
	cases := []struct {
		name string
		tavg Temperature
		want float64
	}{
		{"no load", Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit}, PzrLevelNoLoadPct},
		{"full load", fullLoadTavg, PzrLevelFullLoadPct},
		{"below band clamps", Temperature{Val: 200, Unit: u.Fahrenheit}, PzrLevelNoLoadPct},
		{"above band clamps", Temperature{Val: 700, Unit: u.Fahrenheit}, PzrLevelFullLoadPct},
	}
	for _, tc := range cases {
		c := autoCVCS().Step(Volume{}, tc.tavg, 0, 0)
		assert.InDelta(t, tc.want, c.ProgrammedLevel, 1e-9,
			"%s: want ProgrammedLevel=%v", tc.name, tc.want)
	}
}

// The controller splits the error-driven net-flow demand between
// charging and letdown, saturating at the design rates.
//
// Spec:
//   "it must maintain the RCS mass inventory, that is, the
//   pressurizer level, by balancing the charging flow to the
//   letdown flow."
//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:9-12
func TestPzrLevelControllerBalancesFlows(t *testing.T) {
	noLoad := Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit}

	low := autoCVCS().Step(Volume{}, noLoad, 0.05, 0)
	assert.InDelta(t, CVCSDesignChargingFlow.Float(),
		low.ChargingFlow.Float(), 1e-9, "low level saturates charging")
	assert.Zero(t, low.LetdownFlow.Float(), "low level closes letdown")

	high := autoCVCS().Step(Volume{}, noLoad, 0.95, 0)
	assert.InDelta(t, CVCSDesignLetdownFlow.Float(),
		high.LetdownFlow.Float(), 1e-9, "high level saturates letdown")
	assert.Zero(t, high.ChargingFlow.Float(), "high level closes charging")

	smallErrPct := 2.0
	mid := autoCVCS().Step(Volume{}, noLoad, (PzrLevelNoLoadPct-smallErrPct)/100, 0)
	// Wiring check: the expected demand recomputes from the same
	// derived gain constant as production, so this pins the
	// proportional law, not the constant's magnitude.
	assert.InDelta(t, PzrLevelCtrlGainGalPerSecPerPct*smallErrPct,
		mid.ChargingFlow.Float(), 1e-9, "proportional demand in linear band")
	assert.Zero(t, mid.LetdownFlow.Float())

	balanced := autoCVCS().Step(Volume{}, noLoad, PzrLevelNoLoadPct/100, 0)
	assert.Zero(t, balanced.ChargingFlow.Float(), "zero error gives no flow")
	assert.Zero(t, balanced.LetdownFlow.Float(), "zero error gives no flow")
}

// The pump and valve latches gate their paths even with automatic
// control engaged, so a lost charging pump is observable as an unmet
// demand rather than phantom injection.
func TestPzrLevelControllerRespectsLatches(t *testing.T) {
	noLoad := Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit}

	c := autoCVCS()
	c.ChargingPumpRunning = false
	out := c.Step(Volume{}, noLoad, 0.05, 0)
	assert.Zero(t, out.ChargingFlow.Float(),
		"no charging without the pump even below program")

	c = autoCVCS()
	c.LetdownValveOpen = false
	out = c.Step(Volume{}, noLoad, 0.95, 0)
	assert.Zero(t, out.LetdownFlow.Float(),
		"no letdown with the valve shut even above program")
}

// Manual lineup (AutoLevelControl zero value) keeps the latch-driven
// design flows from before the controller existed.
func TestCVCSManualLineupUnchangedByLevelControl(t *testing.T) {
	hot := Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	c := &CVCS{
		VCTLevel:            Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
	}
	out := c.Step(Volume{}, hot, 0.99, 0)
	assert.InDelta(t, CVCSDesignChargingFlow.Float(),
		out.ChargingFlow.Float(), 1e-9)
	assert.InDelta(t, CVCSDesignLetdownFlow.Float(),
		out.LetdownFlow.Float(), 1e-9)
}

// End-to-end: a pressurizer sitting above its programmed level drains
// back toward the program through the full plant step, with the
// surplus landing in the VCT/BRS path.
//
// Spec:
//   "The pressurizer level control system adjusts charging flow
//   from the CVCS to maintain pressurizer level at a programmed
//   value which varies with RCS average coolant temperature."
//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:36-40
func TestPzrAutoControlHoldsPlantLevel(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Water = Volume{
		Val:  0.8 * p.ReactorBuilding.Pressurizer.Volume.Float(),
		Unit: u.Gallon,
	}
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = true
	// The automatic controller modulates the letdown path, so its
	// manual valve latch must line up with the program.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	// Neutralize the makeup edge: the ledger below assumes letdown
	// is the only source feeding the VCT/BRS path. Heaters sit at
	// their target so the idle-cooldown path stays idle instead of
	// condensing bubble into the liquid inventory mid-run.
	p.ReactorBuilding.ReactorMakeupControlValve.Open = 0
	p.ReactorBuilding.Pressurizer.HeatersOn = true

	startWater := p.ReactorBuilding.Pressurizer.Water.Float()
	startVCT := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
	startHoldup := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel.Float()
	stepN(&p, int(30*time.Second/TimeStep))

	water := p.ReactorBuilding.Pressurizer.Water.Float()
	fraction := 100 * water / p.ReactorBuilding.Pressurizer.Volume.Float()
	assert.Less(t, fraction, 62.0,
		"want level drained back near the 60 percent program, got %v", fraction)
	assert.Less(t, water, startWater, "want net inventory removed")
	assert.InDelta(t, PzrLevelFullLoadPct,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ProgrammedLevel, 1e-6,
		"rated midpoint temperatures hold the full-load anchor")

	// Volume ledger: with charging parked at zero above program,
	// everything let down out of the pressurizer must land in the
	// VCT or, past the high-level divert, the BRS holdup tanks.
	// The zero is a crossover bound: at the program crossing the
	// proportional demand passes through float-dust positives.
	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.InDelta(t, 0, cvcs.ChargingFlow.Float(), 1e-6,
		"ledger assumes no charging")
	pzrLoss := startWater - water
	vctGain := cvcs.VCTLevel.Float() - startVCT
	holdupGain := cvcs.BRSHoldupLevel.Float() - startHoldup
	assert.InDelta(t, pzrLoss, vctGain+holdupGain, 1e-6*pzrLoss,
		"letdown volume must conserve into VCT+BRS: loss=%v vct=%v holdup=%v",
		pzrLoss, vctGain, holdupGain)
}

// Losing the charging pump with level below program leaves the demand
// unmet: ChargingFlow stays zero instead of phantom injection holding
// the program.
func TestPzrAutoControlChargingPumpLossObservable(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Water = Volume{
		Val:  0.4 * p.ReactorBuilding.Pressurizer.Volume.Float(),
		Unit: u.Gallon,
	}
	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.AutoLevelControl = true
	cvcs.ChargingPumpRunning = false

	stepN(&p, 1)

	assert.Zero(t, cvcs.ChargingFlow.Float(),
		"no charging without the pump despite level below program")
}
