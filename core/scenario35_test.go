package core

import (
	"encoding/json"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 35 exercises boron recycle effluent reclamation: a backup
// makeup-and-bleed campaign diverts CVCS letdown into the BRS holdup
// tanks, the reclaim train evaporates that effluent into 4-weight-
// percent boric acid plus clean distillate, and the storage-tank
// radiation monitor auto-diverts contaminated distillate back to the
// feed demineralizer.
//
// Effluent generation ("If the volume control tank is full, a
// high-water level signal diverts letdown flow to the Boron Recycle"):
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:19-21
// Reclaim train and products:
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:1-7
// Radiation auto-divert:
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:20-30
// Duty figure:
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:71-74
func scenario35Plant(t *testing.T) Plant {
	t.Helper()
	p := MakePlant()
	overrideTimeStep(t, 10*time.Millisecond)
	return p
}

func TestScenario35_MakeupAndBleedCampaignFillsHoldupThroughDivert(t *testing.T) {
	p := scenario35Plant(t)
	setControl(t, &p,
		"ReactorBuilding/ReactorMakeupControlValve/Open", "1")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning", "1")
	setControl(t, &p,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")

	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	require.False(t, cvcs.Diverting, "campaign must start undiverted")

	stepFor(&p, 200*time.Second)
	cvcs = &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	require.True(t, cvcs.Diverting,
		"the VCT must cross its high level and latch the divert")
	assert.GreaterOrEqual(t, cvcs.VCTLevel.Float(), VCTHighLevel,
		"the divert rides on a sustained VCT high-level condition")
	assert.Greater(t, cvcs.BRSHoldupLevel.Float(), 0.0,
		"diverted letdown must land in the holdup tanks")

	holdupBefore := cvcs.BRSHoldupLevel.Float()
	stepFor(&p, 60*time.Second)
	cvcs = &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	growth := cvcs.BRSHoldupLevel.Float() - holdupBefore
	assert.InDelta(t, CVCSDesignLetdownFlow.Float()*60.0, growth, 3.0*60.0,
		"while diverting, holdup must grow at the design letdown rate")
}

func TestScenario35_ReclaimTrainSplitsFeedIntoProducts(t *testing.T) {
	p := scenario35Plant(t)
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel =
		Volume{Val: 2500, Unit: u.Gallon}
	setControl(t, &p,
		"AuxiliaryBuilding/BoronRecycleSystem/FeedPumpRunning", "1")

	const run = 3 * time.Minute // 1800 gal at the duty rate
	stepFor(&p, run)

	brs := p.AuxiliaryBuilding.BoronRecycleSystem
	cvcs := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	processed := BRSSEvaporatorDutyGPM * run.Seconds()
	distillateFrac := 1 - CoreRatedCriticalBoronPpm/BRSSProductWeightPercentPpm

	assert.InDelta(t, BRSSEvaporatorDutyGPM, brs.FeedFlow, 0.2,
		"the train must process at the average evaporator duty")
	// Physics-forced conservation: every gallon drawn must leave as
	// one product or the other; the per-product split below is a
	// wiring check recomputed from production constants.
	assert.InDelta(t, processed,
		brs.DistillateDelivered.Float()+brs.BoricAcidProduced.Float(), 0.1,
		"drawn holdup must be fully accounted for by the two products")
	assert.InDelta(t, 2500-processed, cvcs.BRSHoldupLevel.Float(), 0.05,
		"processing must consume exactly the feed volume from the holdup tanks")
	assert.InDelta(t, processed*distillateFrac,
		brs.DistillateDelivered.Float(), 0.05,
		"clean distillate must follow the 4 wt% mass balance to the tank")
	assert.InDelta(t, processed*(1-distillateFrac),
		brs.BoricAcidProduced.Float(), 0.05,
		"the remainder must leave as 4 wt% boric acid concentrate")
	assert.Zero(t, brs.Recirculating.Float(),
		"a clean monitor must recirculate nothing")
	assert.False(t, brs.DistillateDiverted)
}

func TestScenario35_CarryoverTripsAutoDivertAndReleaseResumesDelivery(t *testing.T) {
	p := scenario35Plant(t)
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel =
		Volume{Val: 1500, Unit: u.Gallon}
	setControl(t, &p,
		"AuxiliaryBuilding/BoronRecycleSystem/FeedPumpRunning", "1")
	stepFor(&p, 2*time.Second)

	brs := &p.AuxiliaryBuilding.BoronRecycleSystem
	require.False(t, brs.DistillateDiverted)

	setControl(t, &p,
		"AuxiliaryBuilding/BoronRecycleSystem/Carryover", "1")
	latchSec := 0
	for ; latchSec < 180 && !brs.DistillateDiverted; latchSec++ {
		stepFor(&p, time.Second)
	}
	require.True(t, brs.DistillateDiverted,
		"the monitor must reach its high signal and latch the auto-divert")
	assert.LessOrEqual(t, latchSec, 120,
		"the high signal must arrive within the evolution window")
	divertedDelivered := brs.DistillateDelivered.Float()

	stepFor(&p, 60*time.Second)
	brs = &p.AuxiliaryBuilding.BoronRecycleSystem
	assert.InDelta(t, divertedDelivered, brs.DistillateDelivered.Float(), 1e-9,
		"diverted distillate must not reach the storage tank")
	assert.Greater(t, brs.Recirculating.Float(), 0.0,
		"returned distillate must pool inside the train for reprocessing")

	setControl(t, &p,
		"AuxiliaryBuilding/BoronRecycleSystem/Carryover", "0")

	releaseSec := 0
	for ; releaseSec < 180 && brs.DistillateDiverted; releaseSec++ {
		stepFor(&p, time.Second)
	}
	require.False(t, brs.DistillateDiverted,
		"a cleared carryover must decay the monitor and release the divert")
	assert.LessOrEqual(t, releaseSec, 150,
		"the release must land inside the recovery window")

	resumed := brs.DistillateDelivered.Float()
	stepFor(&p, 30*time.Second)
	brs = &p.AuxiliaryBuilding.BoronRecycleSystem
	assert.Greater(t, brs.DistillateDelivered.Float(), resumed,
		"delivery must resume after release")
	assert.Greater(t,
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel.Float(),
		0.0,
		"the released train must keep drawing holdup inventory for delivery")
}

func TestScenario35_StarvedTrainDrawsRecirculatedInventoryFirst(t *testing.T) {
	brs := BoronRecycleSystem{
		FeedPumpRunning: true,
		Recirculating:   Volume{Val: 50, Unit: u.Gallon},
	}
	out := brs.Step(0)

	draw := BRSSEvaporatorDutyGPM * TimeStep.Seconds()
	distillateFrac := 1 - CoreRatedCriticalBoronPpm/BRSSProductWeightPercentPpm
	assert.InDelta(t, BRSSEvaporatorDutyGPM, out.FeedFlow, 1e-9,
		"an empty holdup tank must not stop a train holding recycled distillate")
	assert.Zero(t, out.HoldupDrawn.Float(),
		"recirculated inventory feeds the draw before new holdup")
	assert.InDelta(t, draw*distillateFrac, out.DistillateDelivered.Float(), 1e-9,
		"released distillate goes straight back to the storage tank")
}

// The feed-flow sensor must read delivered behavior, not intent: a
// train whose available inventory covers less than one step's draw
// reports the throttled rate it actually achieved.
func TestScenario35_StarvingTrainReportsThrottledFeedFlow(t *testing.T) {
	brs := BoronRecycleSystem{
		FeedPumpRunning: true,
		Recirculating:   Volume{Val: 0.002, Unit: u.Gallon},
	}
	out := brs.Step(0)

	fullDraw := BRSSEvaporatorDutyGPM * TimeStep.Seconds()
	require.Less(t, 0.002, fullDraw,
		"fixture must starve the train below one step's draw")
	assert.InDelta(t, 0.002/TimeStep.Seconds(), out.FeedFlow, 1e-9,
		"FeedFlow must report the rate actually processed")
	assert.Less(t, out.FeedFlow, BRSSEvaporatorDutyGPM,
		"a starving train must not read its duty figure")
	assert.Zero(t, out.HoldupDrawn.Float())
}

// The storage-tank monitor sits in the distillate line, so a latched
// carryover cannot drive its signal while no distillate is being
// produced; the trip becomes reachable only once the train runs.
func TestScenario35_MonitorNeedsProductionToReachItsTrip(t *testing.T) {
	idle := BoronRecycleSystem{Carryover: true}
	for i := 0; i < 400; i++ {
		idle = idle.Step(500)
	}
	assert.Zero(t, idle.DistillateRadiation,
		"carryover without production must not climb the monitor")
	assert.False(t, idle.DistillateDiverted)

	running := BoronRecycleSystem{
		FeedPumpRunning: true,
		Carryover:       true,
	}
	latched := false
	for i := 0; i < 200000 && !latched; i++ {
		running = running.Step(500)
		latched = running.DistillateDiverted
	}
	assert.True(t, latched,
		"production flow must carry the monitor to its high signal")
}

// While the auto-divert holds, returned distillate loops through the
// train and the only inventory leaving is 4 wt% concentrate: the
// holdup draw collapses to the concentrate rate.
func TestScenario35_DivertedTrainConsumesOnlyConcentrate(t *testing.T) {
	s := BoronRecycleSystem{
		FeedPumpRunning:     true,
		Carryover:           true,
		DistillateRadiation: BRSSRadiationTripFrac,
		DistillateDiverted:  true,
		Recirculating:       Volume{Val: 50, Unit: u.Gallon},
	}
	out := s.Step(100)

	concentrate := BRSSEvaporatorDutyGPM * TimeStep.Seconds() *
		CoreRatedCriticalBoronPpm / BRSSProductWeightPercentPpm
	assert.Zero(t, out.HoldupDrawn.Float(),
		"a pooled loop covers the whole draw before holdup is touched")
	assert.InDelta(t, 50-concentrate, out.Recirculating.Float(), 1e-9,
		"the loop shrinks only by the product leaving as boric acid")
	assert.Equal(t, s.DistillateDelivered, out.DistillateDelivered,
		"diverted distillate must not reach the storage tank")
}

// The new subsystem serializes through the plain-struct path with no
// method promotion or field collisions: every latched bool, numeric,
// and unit-bearing volume survives marshal/decode identically
// (CHECK_SKILL section 1).
func TestScenario35_StateRoundTripsThroughJSON(t *testing.T) {
	p := MakePlant()
	seed := &p.AuxiliaryBuilding.BoronRecycleSystem
	seed.FeedPumpRunning = true
	seed.Carryover = true
	seed.DistillateRadiation = 0.97
	seed.DistillateDiverted = true
	seed.Recirculating = Volume{Val: 12.5, Unit: u.Gallon}
	seed.BoricAcidProduced = Volume{Val: 3.25, Unit: u.Gallon}
	seed.DistillateDelivered = Volume{Val: 224.75, Unit: u.Gallon}

	data, err := json.Marshal(&p)
	require.NoError(t, err)
	for _, key := range []string{
		"\"FeedPumpRunning\":true", "\"Carryover\":true",
		"\"DistillateRadiation\":0.97", "\"DistillateDiverted\":true",
		"\"Recirculating\"", "\"BoricAcidProduced\"",
		"\"DistillateDelivered\"",
	} {
		assert.Contains(t, string(data), key, "missing key %s", key)
	}

	var back Plant
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, p.AuxiliaryBuilding.BoronRecycleSystem,
		back.AuxiliaryBuilding.BoronRecycleSystem,
		"decode must reproduce the seeded state field for field")
}
