package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stepFor(p *Plant, d time.Duration) {
	for i := 0; i < int(d/TimeStep); i++ {
		p.Step()
		p.Update()
	}
}

func setControl(t *testing.T, p *Plant, path, value string) {
	t.Helper()
	cm := GetControlMap(p)
	c, ok := cm[path]
	require.True(t, ok, "control %s not registered", path)
	require.NoError(t, c.Set(value))
}

// Scenario 4 (secondary side spin-up): pulling condenser vacuum via
// the operator control drives back-pressure to design vacuum before
// any steam is admitted.
//
// "When the reactor coolant temperature exceeds 400F, steam is drawn
// from the steam generators to warm up the secondary plant, develop a
// vacuum in the main condenser, and roll the turbine."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:171-177
//
// "Under normal operation the air is removed by steam jet air
// ejectors or vacuum pumps."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:21-22
func TestScenario4_VacuumPumpControlEstablishesVacuum(t *testing.T) {
	p := MakePlant()
	// E1 vacuum establishment happens with the reactor offline: no
	// fission or decay heat means no steam production, so the dump
	// pressure mode never arms and the condenser sees no load while
	// the pumps pull down.
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0
	p.TurbineBuilding.Condenser.VacuumPumpRunning = false
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	// E1 requires vacuum established BEFORE any steam admission; a
	// steam load holds the back-pressure up by design
	// (condenser.go load-factor term), so secure admission first.
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	setControl(t, &p, "TurbineBuilding/Condenser/VacuumPump", "1")
	stepFor(&p, 5*time.Second)

	assert.True(t, p.TurbineBuilding.Condenser.VacuumPumpRunning)
	// Physics-forced: established vacuum sits below atmospheric and
	// below the low-vacuum trip plate.
	assert.Less(t, float64(p.TurbineBuilding.Condenser.Pressure), float64(AtmosphericPressure))
	assert.Less(t, float64(p.TurbineBuilding.Condenser.Pressure), float64(TurbineLowVacuumTripPsia))
	// Scenario acceptance band (plans/scenario/4.txt E1); the 3 psia
	// figure encodes the modeled 5 s vacuum recovery, not spec text.
	assert.LessOrEqual(t, float64(p.TurbineBuilding.Condenser.Pressure), 3.0,
		"want vacuum established below the 3 psia acceptance band, got %v",
		p.TurbineBuilding.Condenser.Pressure)
}

// Rolling the turbine into an evacuated condenser must not latch the
// low-vacuum trip.
//
// "Low vacuum trip"
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:26
func TestScenario4_RollingWithVacuumHoldsBackpressure(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = false
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	setControl(t, &p, "TurbineBuilding/Condenser/VacuumPump", "1")
	stepFor(&p, 5*time.Second)

	setControl(t, &p, "TurbineBuilding/Turbine/SpeedDemand", "0.05")
	setControl(t, &p, "TurbineBuilding/Turbine/StopValve", "1")
	stepFor(&p, 10*time.Second)

	// Fixed-horizon read is valid: TripByVacuum latches while stepping
	// and clears only on an operator reset, so an end-state false
	// proves no crossing happened anywhere in the window.
	assert.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State)
	assert.False(t, p.TurbineBuilding.Turbine.TripByVacuum,
		"rolling against an evacuated condenser must not trip on low vacuum")
	assert.Greater(t, float64(p.TurbineBuilding.Turbine.Velocity), 0.0)
}

// Admitting steam without the vacuum lineup latches the low-vacuum
// trip: the condenser sits at atmospheric back-pressure, above the
// trip plate.
func TestScenario4_AdmittingSteamWithoutVacuumTrips(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = false
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0

	setControl(t, &p, "TurbineBuilding/Turbine/SpeedDemand", "0.05")
	setControl(t, &p, "TurbineBuilding/Turbine/StopValve", "1")
	stepFor(&p, 10*time.Second)

	assert.True(t, p.TurbineBuilding.Turbine.TripByVacuum)
	assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State)
}

// Opening the steamline vent draws steam off the SG header and the
// drawn volume lands in the condenser hotwell. Tier 3 (invented
// convention): the capacity fraction is an assumption, so assertions
// are directional against a wiring-derived floor, not exact values.
//
// "When the reactor coolant temperature increases to about 210-220°F
// (90-105°C), the steamline vents are closed [...]."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
func TestScenario4_SteamlineVentDrawsSteamToHotwell(t *testing.T) {
	p := MakePlant()
	sgBefore := p.ReactorBuilding.SteamGenerator.Water.Float()
	hotwellBefore := p.TurbineBuilding.Condenser.HotwellLevel.Float()

	setControl(t, &p, "ReactorBuilding/SteamGenerator/SteamlineVent/Open", "1")
	stepFor(&p, 20*time.Second)

	ventGalPerSec := SteamlineVentCapacityFraction *
		SteamGeneratorFullLoadSteamFlow.Float() / SecondsPerHour
	wantMin := ventGalPerSec * 15 // tolerate header-pressure ramp-in
	gotHotwell := p.TurbineBuilding.Condenser.HotwellLevel.Float() - hotwellBefore
	assert.GreaterOrEqual(t, gotHotwell, wantMin,
		"want >= %v gal of vented steam condensed to the hotwell, got %v",
		wantMin, gotHotwell)
	assert.Less(t, p.ReactorBuilding.SteamGenerator.Water.Float(), sgBefore,
		"vented draw must debit SG shell inventory")
}

// The shell drain pulls SG inventory toward the normal operating band
// and the drained water is tracked into the condenser hotwell.
// Differential against an identical no-drain plant; tier 3 on rate
// (invented constant), asserted directionally.
func TestScenario4_DrainValvePullsLevelToNormalBand(t *testing.T) {
	baseline := MakePlant()
	setControl(t, &baseline, "TurbineBuilding/MainFeedwaterPump/Running", "0")
	stepFor(&baseline, 60*time.Second)

	drained := MakePlant()
	setControl(t, &drained, "TurbineBuilding/MainFeedwaterPump/Running", "0")
	setControl(t, &drained, "ReactorBuilding/SteamGenerator/DrainValve/Open", "1")
	hotwellBefore := drained.TurbineBuilding.Condenser.HotwellLevel.Float()
	stepFor(&drained, 60*time.Second)

	drainDelta := baseline.ReactorBuilding.SteamGenerator.Water.Float() -
		drained.ReactorBuilding.SteamGenerator.Water.Float()
	assert.GreaterOrEqual(t, drainDelta, 0.8*SGDrainValveGalPerSec*55,
		"drained SG should sit ~%v gal below baseline, delta %v",
		SGDrainValveGalPerSec*55, drainDelta)
	hotwellGain := drained.TurbineBuilding.Condenser.HotwellLevel.Float() -
		hotwellBefore
	assert.GreaterOrEqual(t, hotwellGain, 0.8*SGDrainValveGalPerSec*55,
		"drained water must land in the hotwell, got %v", hotwellGain)
}

// The condensate pump moves hotwell inventory through the LP heater
// train into the deaerator tank; with it secured the hotwell holds.
//
// "The condensate pumps take suction from the condenser hotwell and
// pump condensate [...] through four stages of low-pressure feedwater
// heaters to the deaerator."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-46
// Draw magnitude recomputed from production constants (pump head,
// FeedwaterMinFlow): a wiring check, asserted as a floor plus
// conservation of the transferred volume.
func TestScenario4_CondensatePumpMovesHotwellToDeaerator(t *testing.T) {
	// Quiescent lineup: neutron-source power only, so no turbine
	// exhaust condenses, no extraction feeds the heaters, and the
	// only deaerator inflows are the minimum-flow recirculation
	// (constant while the feed pump is secured) and the condensate
	// pump suction under test.
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.FissionPower = CoreNeutronSourcePower
	rpv.DecayHeatFast = 0
	rpv.DecayHeatSlow = 0
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Condenser.HotwellLevel =
		Volume{Val: 50000, Unit: u.Gallon}
	hotwellBefore := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	hdtBefore := p.TurbineBuilding.HeaterDrainTank.Level.Float()

	setControl(t, &p, "TurbineBuilding/CondensatePump/Running", "1")
	stepFor(&p, 3*time.Second)

	draw := pumpHead * 3
	recirc := FeedwaterMinFlow.Float() * 3
	hotwellDrop := hotwellBefore -
		p.TurbineBuilding.Condenser.HotwellLevel.Float()
	assert.GreaterOrEqual(t, hotwellDrop, 0.9*draw,
		"want pump to draw >= %v gal from the hotwell, drew %v",
		draw, hotwellDrop)
	hdtRise := p.TurbineBuilding.HeaterDrainTank.Level.Float() - hdtBefore
	assert.InDelta(t, draw+recirc, hdtRise, 0.1*(draw+recirc),
		"deaerator gain must be the pump suction plus recirculation")
}

// CVCS letdown must measurably drain pressurizer level against a
// letdown-closed reference run: the level represents the balance of
// charging injection against letdown draw.
//
// "pressurizer level represents a balance of injection water flow
//  into the RCS and water letdown flow into the Chemical and Volume
//  Control System."
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
// Direction is physics-forced (letdown net draw must lower level);
// magnitude recomputed from the CVCS design constants is a wiring
// check, asserted as a floor.
func TestScenario4_LetdownMeasurablyDrainsPressurizer(t *testing.T) {
	closed := MakePlant()
	setControl(t, &closed,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "0")
	stepFor(&closed, 30*time.Second)
	closedWater := closed.ReactorBuilding.Pressurizer.Water.Float()

	open := MakePlant()
	setControl(t, &open,
		"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen", "1")
	require.True(t, open.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen)
	stepFor(&open, 30*time.Second)

	drain := closedWater - open.ReactorBuilding.Pressurizer.Water.Float()
	netGalPerSec := CVCSDesignLetdownFlow.Float() -
		CVCSDesignChargingFlow.Float()
	assert.GreaterOrEqual(t, drain, 0.8*netGalPerSec*25,
		"letdown-open PZR should sit >= %v gal below baseline, delta %v",
		0.8*netGalPerSec*25, drain)
}
