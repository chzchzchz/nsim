package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At full HP exhaust quality (0.90) and full reheat steam flow, the
// MSR reheats the steam by approximately MSRReheatTarget over the
// inlet temperature.
//
// Spec justification:
//   - "reheat the steam to over 100°F (55°C) superheat"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:54-55
func TestMSR_FullQualityReachesReheatTarget(t *testing.T) {
	inlet := u.NewValue(560.0, u.Fahrenheit)
	reheatFlow := u.NewValue(FeedwaterDesignFlow.Float(), u.Gallon)
	m := (&MoistureSeparatorReheater{}).Step(
		SteamGeneratorShellPressure,
		HPExhaustQualityFullLoad,
		inlet,
		reheatFlow,
		SteamGeneratorShellPressure*0.98,
	)

	rise := m.OutletTemperature.Float() - inlet.Float()
	assert.GreaterOrEqual(t, rise, MSRReheatTarget*0.99,
		"want outlet rise >= %v°F at full quality/flow, got %v",
		MSRReheatTarget, rise)
}

// At low HP exhaust quality (more moisture), the MSR drains
// proportionally more liquid. The drain flow scales with the
// liquid fraction.
//
// Spec justification:
//   - "the water is removed and drained to the feedwater system"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:62-63
func TestMSR_LowQualityIncreasesMoistureDrain(t *testing.T) {
	reheatFlow := u.NewValue(FeedwaterDesignFlow.Float(), u.Gallon)
	dry := (&MoistureSeparatorReheater{}).Step(
		SteamGeneratorShellPressure,
		HPExhaustQualityFullLoad,
		u.NewValue(560.0, u.Fahrenheit),
		reheatFlow,
		SteamGeneratorShellPressure*0.98,
	)
	wet := (&MoistureSeparatorReheater{}).Step(
		SteamGeneratorShellPressure,
		HPExhaustQualityLowLoad,
		u.NewValue(560.0, u.Fahrenheit),
		reheatFlow,
		SteamGeneratorShellPressure*0.98,
	)

	assert.Greater(t, wet.DrainFlow.Float(), dry.DrainFlow.Float(),
		"want wet-step drain (%v) > dry-step drain (%v)",
		wet.DrainFlow.Float(), dry.DrainFlow.Float())
}

// At fixed inlet conditions, scaling reheat steam flow scales the
// outlet temperature rise within the linear range (below the
// design extraction point).
func TestMSR_ReheatScalesWithReheatSteamFlow(t *testing.T) {
	inlet := u.NewValue(560.0, u.Fahrenheit)
	// Pick flows below the design extraction point so we stay in
	// the linear region of the model (the model caps at the
	// design point).
	designReheat := FeedwaterDesignFlow.Float() * MSRDesignExtractionFraction
	lowFlow := u.NewValue(designReheat*0.25, u.Gallon)
	highFlow := u.NewValue(designReheat*0.75, u.Gallon)

	mLow := (&MoistureSeparatorReheater{}).Step(
		SteamGeneratorShellPressure, HPExhaustQualityFullLoad,
		inlet, lowFlow, SteamGeneratorShellPressure*0.98,
	)
	mHigh := (&MoistureSeparatorReheater{}).Step(
		SteamGeneratorShellPressure, HPExhaustQualityFullLoad,
		inlet, highFlow, SteamGeneratorShellPressure*0.98,
	)

	lowRise := mLow.OutletTemperature.Float() - inlet.Float()
	highRise := mHigh.OutletTemperature.Float() - inlet.Float()
	assert.Greater(t, highRise, lowRise,
		"want high-flow rise (%v) > low-flow rise (%v)", highRise, lowRise)
	assert.InDelta(t, lowRise, highRise/3, highRise*0.1,
		"want low-flow rise ≈ high-flow rise / 3, got %v vs %v", lowRise, highRise)
}

// ExtractionSteamFlow rises with turbine load fraction. With no
// load there is no extraction steam; at rated load the extraction
// flow is the design fraction of FeedwaterDesignFlow.
func TestTurbineExtractionSteamFlow_ScalesWithLoad(t *testing.T) {
	sgP := SteamGeneratorShellPressure
	condP := Pressure(0)
	stopOpen := 1.0

	tr := (&Turbine{}).Step(sgP, condP, false, &stopOpen)
	// First step: rotor spinning up, partial load.
	zeroLoadFlow := tr.ExtractionSteamFlow.Float()

	// Pin rated load by direct construction of a Running turbine.
	rated := Turbine{
		State:     TurbineRunning,
		Velocity:  TurbineRatedSpeed,
		RealPower: TurbineRatedkW,
	}
	for i := 0; i < 1000; i++ {
		rated = rated.Step(sgP, condP, false, &stopOpen)
	}
	ratedFlow := rated.ExtractionSteamFlow.Float()

	assert.Less(t, zeroLoadFlow, ratedFlow,
		"want rated-load extraction flow (%v) > first-step extraction flow (%v)",
		ratedFlow, zeroLoadFlow)
	assert.InDelta(t,
		FeedwaterDesignFlow.Float(),
		ratedFlow,
		FeedwaterDesignFlow.Float()*0.01,
		"want rated-load extraction ≈ design feedwater flow, got %v", ratedFlow)
}

// End-to-end: with the reactor running at half power the turbine
// reaches partial load, the MSR reheats the HP exhaust, and the LP
// turbine inlet sees a steam temperature above the MSR inlet by at
// least 50°F.
//
// Spec justification:
//   - "reheated steam goes to the low-pressure turbines and to the
//     two main feedwater pump turbines."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:81-83
func TestMSR_HalfLoadPowerGivesSuperheatedLPInlet(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower

	stepN(&p, int(5*time.Second/TimeStep))
	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"turbine should be running at power")

	// The MSR inlet temperature is a linear surrogate of the HP
	// exhaust pressure (sgPressure * 0.90). Reconstruct it the
	// same way turbine.go does, then verify the OutletTemperature
	// sits above it.
	sgP := p.ReactorBuilding.SteamGenerator.Pressure
	msrIn := float64(sgP)*0.90*5.5 + 80.0
	rise := p.TurbineBuilding.MoistureSeparatorReheater.OutletTemperature.Float() - msrIn
	assert.GreaterOrEqual(t, rise, 50.0,
		"want LP inlet superheat ≥ 50°F at half-load steady state, got %v (msrIn=%v, sgP=%v)",
		rise, msrIn, sgP)
}

// MSR drain flow accumulates into the condenser hotwell across
// steps when the turbine is running.
//
// Spec justification:
//   - "where the water is removed and drained to the feedwater
//     system"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:62-63
func TestMSR_DrainFlowsToHotwell(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	hotwellBefore := p.TurbineBuilding.Condenser.HotwellLevel.Float()

	stepN(&p, int(2*time.Second/TimeStep))
	hotwellAfter := p.TurbineBuilding.Condenser.HotwellLevel.Float()
	assert.Greater(t, hotwellAfter, hotwellBefore,
		"want hotwell to rise from MSR drain accumulation, before=%v after=%v",
		hotwellBefore, hotwellAfter)
}
