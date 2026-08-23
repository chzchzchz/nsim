package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// forcePORVOpen gives the pressurizer a consistent bubble whose
// computed pressure sits above the PORV lift setpoint, and matches
// the RPV pressure so the surge-line fluid-graph edges see no
// differential (they would otherwise move phantom volume).
func forcePORVOpen(p *Plant) {
	const bubbleGal = 500.0 // 1500 + 500*2.5 = 2750 psia > PORVTrip
	pz := &p.ReactorBuilding.Pressurizer
	pz.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pz.Water = Volume{Val: 1500 - bubbleGal, Unit: u.Gallon}
	p.ReactorBuilding.ReactorPressureVessel.Pressure = PressurizerBasePressure +
		Pressure(bubbleGal*PressurizerBubbleStiffness)
	p.ReactorBuilding.SafetyValve.Open = 0
}

// steadyPressurizer pins the pressurizer at its 2250 psia operating
// point with matched RPV pressure so surge-line edges stay inert.
func steadyPressurizer(p *Plant) {
	const bubbleGal = 300.0 // 1500 + 300*2.5 = 2250 psia
	pz := &p.ReactorBuilding.Pressurizer
	pz.Volume = Volume{Val: 1500, Unit: u.Gallon}
	pz.Water = Volume{Val: 1500 - bubbleGal, Unit: u.Gallon}
	p.ReactorBuilding.ReactorPressureVessel.Pressure = PressurizerBasePressure +
		Pressure(bubbleGal*PressurizerBubbleStiffness)
	p.ReactorBuilding.SafetyValve.Open = 0
}

// isolateCVCSVolume stops charging and letdown so tests can watch a
// single water path without the charge/letdown coupling interfering.
func isolateCVCSVolume(p *Plant) {
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false
}

// PORV discharge must land in the pressurizer relief tank, drawn
// one-for-one from the pressurizer liquid inventory.
//
// Spec justification:
//   - "Steam from the safety and/or relief valves is piped to the
//     pressurizer relief tank which contains sufficient water to
//     condense the steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:32-34
func TestPRT_AccumulatesPORVDischarge(t *testing.T) {
	p := MakePlant()
	forcePORVOpen(&p)
	isolateCVCSVolume(&p)

	pzzBefore := p.ReactorBuilding.Pressurizer.Water.Float()
	prtBefore := p.ReactorBuilding.PressurizedReliefTank.Level.Float()
	stepN(&p, int(time.Second/TimeStep))

	prtGain := p.ReactorBuilding.PressurizedReliefTank.Level.Float() - prtBefore
	pzzLoss := pzzBefore - p.ReactorBuilding.Pressurizer.Water.Float()
	wantGain := PRTReliefFlow * 1.0
	assert.InDelta(t, wantGain, prtGain, wantGain*0.01,
		"want PRT to collect %v gal of relief flow, gained=%v", wantGain, prtGain)
	assert.GreaterOrEqual(t, pzzLoss, prtGain-1e-9,
		"want pressurizer to lose at least the discharged volume, loss=%v",
		pzzLoss)
}

// Once the PRT is full the rupture disc vents it: further discharge
// stops accumulating and the latch holds.
//
// Spec justification:
//   - "A rupture disc vents the tank to the containment if design
//     pressure is exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:37-38
func TestPRT_RuptureDiscLatchesOnFull(t *testing.T) {
	p := MakePlant()
	forcePORVOpen(&p)
	isolateCVCSVolume(&p)
	p.ReactorBuilding.PressurizedReliefTank.Level =
		Volume{Val: PRTRuptureCapacity, Unit: u.Gallon}

	stepN(&p, int(10*time.Millisecond/TimeStep))
	assert.True(t, p.ReactorBuilding.RuptureDiscRuptured,
		"want rupture disc latched with PRT at capacity")
	assert.InDelta(t, PRTRuptureCapacity,
		p.ReactorBuilding.PressurizedReliefTank.Level.Float(), 1e-9,
		"want PRT level capped at capacity, got %v",
		p.ReactorBuilding.PressurizedReliefTank.Level.Float())
}

// Opening the atmospheric dump valve must spend SG secondary
// inventory; an identical plant with the valve closed keeps its
// water.
//
// Spec justification:
//   - "During plant cooldown, steam is dumped to the condensers to
//     provide a heat sink rather than to atmosphere through the
//     atmospheric dump valves. This conserves condensate in the
//     system."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:33-37
func TestSG_AtmosphericDumpDebitsInventory(t *testing.T) {
	open := MakePlant()
	shut := MakePlant()
	for _, p := range []*Plant{&open, &shut} {
		p.ReactorBuilding.SteamGenerator.Water =
			Volume{Val: 60000, Unit: u.Gallon}
		p.ReactorBuilding.SteamGenerator.Volume =
			Volume{Val: 80000, Unit: u.Gallon}
		p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
		p.TurbineBuilding.MainFeedwaterPump.Running = false
		p.TurbineBuilding.CondensatePump.Running = false
		p.TurbineBuilding.HeaterDrainPump.Running = false
		p.TurbineBuilding.MakeupValve.Open = 0
	}
	open.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open = 1.0

	steps := int(time.Second / TimeStep)
	stepN(&open, steps)
	stepN(&shut, steps)

	// Compare the two plants so shared secondary-side dynamics
	// cancel and only the atmospheric dump differs.
	wantLoss := AtmosphericDumpFraction *
		SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour * 1.0
	diff := shut.ReactorBuilding.SteamGenerator.Water.Float() -
		open.ReactorBuilding.SteamGenerator.Water.Float()
	assert.InDelta(t, wantLoss, diff, wantLoss*0.01,
		"want open-valve SG to lose ~%v gal more than the closed plant, diff=%v",
		wantLoss, diff)
}

// With the VCT above its high-level setpoint letdown continues at
// design rate but accumulates in the BRS holdup tanks; once the VCT
// falls back to its design band the divert clears and letdown
// returns to the VCT again.
//
// Spec justification:
//   - "If the volume control tank is full, a high-water level signal
//     diverts letdown flow to the Boron Recycle"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:19-21
func TestCVCS_LetdownDivertsToHoldupWhenVCTFull(t *testing.T) {
	p := MakePlant()
	isolateCVCSVolume(&p)
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel =
		Volume{Val: VCTHighLevel + 10, Unit: u.Gallon}

	stepN(&p, int(time.Second/TimeStep))
	cv := p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.True(t, cv.Diverting, "want divert latched with VCT above high level")
	assert.InDelta(t, CVCSDesignLetdownFlow.Float()*1.0,
		cv.BRSHoldupLevel.Float(), 1e-6,
		"want holdup to collect 1 s of letdown, got %v", cv.BRSHoldupLevel)
	assert.False(t, p.ReactorBuilding.RuptureDiscRuptured)

	// Charging drains the tank back into its design band (~7 s at
	// 30 gal/s numeric rate); poll until the divert clears.
	for i := 0; i < int(20*time.Second/TimeStep); i++ {
		stepN(&p, 1)
		if !p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Diverting {
			break
		}
	}
	cv = p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.False(t, cv.Diverting,
		"want divert cleared once VCT returns to design band")
	assert.LessOrEqual(t, cv.VCTLevel.Float(), VCTDesignLevel+1e-6,
		"want VCT back at or below design band when divert clears, got %v",
		cv.VCTLevel)
}

// Charging must add to and letdown must remove pressurizer liquid
// volume ("pressurizer level represents a balance of injection water
// flow into the RCS and water letdown flow into the Chemical and
// Volume Control System.",
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65).
func TestCVCS_ChargeLetdownMovesPressurizerWater(t *testing.T) {
	chargeOnly := MakePlant()
	isolateCVCSVolume(&chargeOnly)
	steadyPressurizer(&chargeOnly)
	chargeOnly.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = true

	letdownOnly := MakePlant()
	isolateCVCSVolume(&letdownOnly)
	steadyPressurizer(&letdownOnly)
	letdownOnly.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true

	steps := int(time.Second / TimeStep)
	chargeBefore := chargeOnly.ReactorBuilding.Pressurizer.Water.Float()
	letdownBefore := letdownOnly.ReactorBuilding.Pressurizer.Water.Float()
	stepN(&chargeOnly, steps)
	stepN(&letdownOnly, steps)

	// The surge-line graph edges contribute small residual flows,
	// so assert the coupled design-flow movement with margin.
	assert.GreaterOrEqual(t,
		chargeOnly.ReactorBuilding.Pressurizer.Water.Float()-chargeBefore,
		CVCSDesignChargingFlow.Float()*0.5,
		"want charging to raise pressurizer water")
	assert.LessOrEqual(t,
		letdownOnly.ReactorBuilding.Pressurizer.Water.Float()-letdownBefore,
		-CVCSDesignLetdownFlow.Float()*0.5,
		"want letdown to lower pressurizer water")
}

// Reactor makeup draws from the RWST: the VCT gains exactly what the
// RWST loses, and the source clamps at empty.
//
// Spec justification:
//   - "Low-level signals from the volume control tank initiate
//     reactor makeup control or flow from the refueling water storage
//     tank as a backup."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
func TestCVCS_MakeupDepletesRWST(t *testing.T) {
	p := MakePlant()
	isolateCVCSVolume(&p)
	p.ReactorBuilding.ReactorMakeupControlValve.Open = 1.0

	rwstBefore := p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float()
	vctBefore := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()
	stepN(&p, int(time.Second/TimeStep))

	rwstLoss := rwstBefore - p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float()
	vctGain := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float() - vctBefore
	assert.InDelta(t, ReactorMakeupDesignFlow.Float()*1.0, rwstLoss, 1e-6,
		"want RWST to lose 1 s of makeup flow, lost=%v", rwstLoss)
	assert.InDelta(t, rwstLoss, vctGain, 1e-6,
		"want VCT gain to equal RWST loss, rwstLoss=%v vctGain=%v",
		rwstLoss, vctGain)

	// Near-empty RWST clamps the delivered makeup instead of going
	// negative.
	p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel =
		Volume{Val: 0.05, Unit: u.Gallon}
	stepN(&p, int(2*time.Second/TimeStep))
	assert.GreaterOrEqual(t,
		p.ReactorBuilding.SafetyInjectionSystem.RWSTLevel.Float(), 0.0,
		"want RWST to clamp at zero")
}

// A fully depleted BTRS bed halts boron changes even with the
// recirculation pump running; below saturation the bed consumption
// advances each step.
//
// Spec justification:
//   - "This operation continues until the resin beds are fully
//     depleted as indicated by the boron concentration meter."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:11-13
func TestCVCS_BedSaturationHaltsBoration(t *testing.T) {
	spent := (&CVCS{
		Mode:                         CVCSBoration,
		BoronConcentration:           CVCSInitialBoron,
		BTRSRecirculationPumpRunning: true,
		BTRSBedSaturation:            1.0,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.Zero(t, spent.BorationRate,
		"want BorationRate=0 on a saturated bed")
	assert.InDelta(t, CVCSInitialBoron, spent.BoronConcentration, 1e-9,
		"want no boron change on a saturated bed")

	fresh := CVCS{
		Mode:                         CVCSBoration,
		BoronConcentration:           CVCSInitialBoron,
		BTRSRecirculationPumpRunning: true,
	}
	after := fresh.Step(Volume{Val: 0, Unit: u.Gallon})
	assert.Positive(t, after.BTRSBedSaturation,
		"want bed saturation to advance while borating")
	assert.Greater(t, after.BoronConcentration, CVCSInitialBoron,
		"want boron rise on a fresh bed")
}

// LP heater drain shells must cascade their collected volume to the
// condenser hotwell rather than accumulating forever.
//
// Spec justification:
//   - "Drains from the four low-pressure feedwater heaters are
//     cascaded back to the condenser."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:88-90,
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:1-2
func TestTurbineBuilding_LPHeaterDrainsCascadeToHotwell(t *testing.T) {
	p := MakePlant()
	silenceHotwellFlows(&p)
	p.TurbineBuilding.LowPressureHeater.DrainLevel =
		Volume{Val: 123, Unit: u.Gallon}

	hotwellBefore := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	stepN(&p, 1)

	drainAfter := p.TurbineBuilding.LowPressureHeater.DrainLevel.Float()
	hotwellRise := p.TurbineBuilding.Condenser.HotwellLevel.Float() - hotwellBefore
	assert.InDelta(t, 0, drainAfter, 1e-9,
		"want LP drain shell emptied to the condenser, got %v", drainAfter)
	assert.InDelta(t, 123, hotwellRise, 1e-6,
		"want hotwell to rise by the cascaded drain volume, got %v",
		hotwellRise)
}

// silenceHotwellFlows stops the per-step hotwell inflows/outflows
// that are unrelated to the path under test (MSR drains, condensate
// pump take, feed-pump recirculation).
func silenceHotwellFlows(p *Plant) {
	p.TurbineBuilding.Turbine.MoistureSeparatorReheater.DrainFlow =
		Volume{Val: 0, Unit: u.Gallon}
	p.TurbineBuilding.CondensatePump.Running = false
	p.TurbineBuilding.MainFeedwaterPump.Running = false
}

// The heater drain tank never exceeds capacity, and a high level
// opens the dump valve that sends water to the condenser.
//
// Spec justification:
//   - "A dump valve which taps off the cross-connection drains the
//     tanks to condenser A on a high level in either tank."
//     specs/westinghouse_tech_manual/chapter_7/page_0636.txt:36-38
func TestTurbineBuilding_HDTDumpValveCapsLevel(t *testing.T) {
	p := MakePlant()
	silenceHotwellFlows(&p)
	p.TurbineBuilding.HeaterDrainTank.Level =
		Volume{Val: HeaterDrainTankCapacityGal, Unit: u.Gallon}

	hotwellBefore := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	stepN(&p, int(time.Second/TimeStep))

	hdt := p.TurbineBuilding.HeaterDrainTank.Level.Float()
	hotwellRise := p.TurbineBuilding.Condenser.HotwellLevel.Float() - hotwellBefore
	assert.LessOrEqual(t, hdt, HeaterDrainTankCapacityGal,
		"want HDT capped at capacity, got %v", hdt)
	assert.Greater(t, hotwellRise, 0.0,
		"want dump valve to send water to the condenser, rose=%v", hotwellRise)
}

// A running main feedwater pump draws its suction from the heater
// drain tank (the deaerator stand-in).
//
// Spec justification:
//   - "The feedwater booster pumps draw suction from the deaerator
//     water storage tank, which is mounted directly below the
//     deaerator, and supply the main feedwater pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:46-49
func TestTurbineBuilding_FeedPumpDrawsDownHDT(t *testing.T) {
	p := MakePlant()
	silenceHotwellFlows(&p)
	p.TurbineBuilding.HeaterDrainTank.Level =
		Volume{Val: 5000, Unit: u.Gallon}
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	// Match pump head against SG pressure so the HDT->SG graph edge
	// sees no differential and only the modelled suction moves level.
	p.ReactorBuilding.SteamGenerator.Pressure = Pressure(pumpHead)

	stepN(&p, int(time.Second/TimeStep))
	got := p.TurbineBuilding.HeaterDrainTank.Level.Float()
	wantDraw := FeedPumpSuctionFlow * 1.0
	assert.GreaterOrEqual(t, 5000-got, wantDraw,
		"want HDT drawn down by at least the %v gal modelled suction "+
			"(surge-line graph edges may add more), drew=%v", wantDraw, 5000-got)
	assert.Less(t, got, 5000.0,
		"want running feed pump to deplete the HDT, got %v", got)
}
