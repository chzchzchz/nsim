package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At construction all three essential service water pumps are
// running (collapsed to a single boolean), the lumped flow is
// the design maximum, and the inlet temperature equals the
// river source temperature.
func TestSWS_InitialStateHasAllPumpsRunning(t *testing.T) {
	p := MakePlant()
	sws := p.Outside.ServiceWaterSystem
	assert.True(t, sws.PumpRunning,
		"want PumpRunning=true at startup, got %v", sws.PumpRunning)
	assert.InDelta(t, ServiceWaterDesignFlow, sws.Flow, 1e-9,
		"want Flow=ServiceWaterDesignFlow at startup, got %v",
		sws.Flow)
	assert.InDelta(t, p.Outside.RiverTemp.Float(),
		sws.InletTemp.Float(), 1e-9,
		"want InletTemp=RiverTemp at startup, got %v",
		sws.InletTemp.Float())
	assert.False(t, sws.NonessentialIsolated,
		"want NonessentialIsolated=false at startup, got %v",
		sws.NonessentialIsolated)
}

// A stopped SWS has zero flow and tracks the outlet at the
// inlet.
func TestSWS_StoppedPumpsHaveZeroFlow(t *testing.T) {
	s := ServiceWaterSystem{
		PumpRunning: true,
		Flow:        ServiceWaterDesignFlow,
	}
	s.PumpRunning = false
	s2 := s.Step(DefaultTimeStep, Temperature{Val: 70, Unit: u.Fahrenheit}, Watts(1e6))
	assert.Zero(t, s2.Flow,
		"want Flow=0 with pumps stopped, got %v", s2.Flow)
	assert.InDelta(t, s2.InletTemp.Float(), s2.OutletTemp.Float(), 1e-9,
		"want OutletTemp=InletTemp with pumps stopped, got %v",
		s2.OutletTemp.Float())
}

// While pumps are running, the outlet temperature rises above
// the inlet by the heat load divided by the per-second water
// mass flow.
func TestSWS_OutletTempRisesWithHeatLoad(t *testing.T) {
	s := ServiceWaterSystem{
		PumpRunning: true,
		DriveReady:  true,
		Flow:        ServiceWaterDesignFlow,
	}
	riverT := Temperature{Val: 70, Unit: u.Fahrenheit}
	s2 := s.Step(DefaultTimeStep, riverT, Watts(0))
	assert.InDelta(t, s2.InletTemp.Float(), s2.OutletTemp.Float(), 1e-9,
		"want OutletTemp=InletTemp with no heat load, got %v",
		s2.OutletTemp.Float())
	// Apply 1e7 W; expect a measurable rise above inlet.
	hot := s.Step(DefaultTimeStep, riverT, Watts(1e7))
	assert.Greater(t, hot.OutletTemp.Float(), hot.InletTemp.Float(),
		"want OutletTemp > InletTemp with heat load, got %v vs %v",
		hot.OutletTemp.Float(), hot.InletTemp.Float())
}

// At construction the SFPCS has its two normal pumps running,
// header pressure in the normal range, standby off, and
// purification flow at the design 100 gpm.
func TestSFPCS_InitialStateHasTwoPumpsRunning(t *testing.T) {
	p := MakePlant()
	sf := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	assert.True(t, sf.LoopAPumpRunning && sf.LoopBPumpRunning,
		"want both duty loops running at startup")
	assert.False(t, sf.StandbyRunning,
		"want StandbyRunning=false at startup, got %v",
		sf.StandbyRunning)
	assert.InDelta(t, SFPCPressureNormalMin,
		sf.HeaderPressure, 1e-9,
		"want HeaderPressure=SFPCPressureNormalMin at startup, got %v",
		sf.HeaderPressure)
	assert.InDelta(t, SFPCPurificationFlow,
		sf.PurificationFlow, 1e-9,
		"want PurificationFlow=SFPCPurificationFlow at startup, got %v",
		sf.PurificationFlow)
}

// When the SFPCS header pressure drops below the standby
// start setpoint (50 psig), the standby pump auto-starts.
//
// Spec justification:
//   - "the third pump automatically starts at a header pressure
//     of 50 psig (4.5 bar)"
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:47-48
func TestSFPCS_StandbyStartsBelow50psig(t *testing.T) {
	s := SpentFuelPoolCoolingSystem{
		LoopAPumpRunning: true,
		LoopBPumpRunning: true,
		DriveReady:       true,
		HeaderPressure:   SFPCStandbyStartPressure - 1.0,
	}
	s2 := s.Step(DefaultTimeStep, true)
	assert.True(t, s2.StandbyRunning,
		"want StandbyRunning=true below 50 psig, got %v",
		s2.StandbyRunning)
}

// Header pressure at or above 50 psig leaves the standby off.
func TestSFPCS_StandbyOffAbove50psig(t *testing.T) {
	s := SpentFuelPoolCoolingSystem{
		LoopAPumpRunning: true,
		LoopBPumpRunning: true,
		DriveReady:       true,
		HeaderPressure:   SFPCStandbyStartPressure + 1.0,
	}
	s2 := s.Step(DefaultTimeStep, true)
	assert.False(t, s2.StandbyRunning,
		"want StandbyRunning=false above 50 psig, got %v",
		s2.StandbyRunning)
}

// A stopped SFPCS has no purification flow and the standby off.
func TestSFPCS_StoppedPumpsZeroPurificationFlow(t *testing.T) {
	s := SpentFuelPoolCoolingSystem{
		PurificationFlow: SFPCPurificationFlow,
	}
	s2 := s.Step(DefaultTimeStep, true)
	assert.Zero(t, s2.PurificationFlow,
		"want PurificationFlow=0 with pumps stopped, got %v",
		s2.PurificationFlow)
	assert.False(t, s2.StandbyRunning,
		"want StandbyRunning=false with pumps stopped, got %v",
		s2.StandbyRunning)
}

// End-to-end via Plant.Step: SWS and SFPCS run at construction;
// the SWS outlet picks up the RCFC heat sink.
func TestSWS_EndToEndSteadyState(t *testing.T) {
	p := MakePlant()
	stepN(&p, int(15*time.Second/DefaultTimeStep), DefaultTimeStep)
	sws := p.Outside.ServiceWaterSystem
	sf := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
	assert.True(t, sws.PumpRunning,
		"want SWS PumpRunning=true after stepping, got %v",
		sws.PumpRunning)
	assert.InDelta(t, ServiceWaterDesignFlow, sws.Flow, 1e-9,
		"want SWS Flow=ServiceWaterDesignFlow, got %v", sws.Flow)
	assert.True(t, sf.LoopAPumpRunning && sf.LoopBPumpRunning,
		"want both SFPCS duty loops running after stepping")
	assert.InDelta(t, SFPCPurificationFlow,
		sf.PurificationFlow, 1e-9,
		"want SFPCS PurificationFlow=SFPCPurificationFlow, got %v",
		sf.PurificationFlow)
}

// SI / loss-of-offsite-power signals energise the ESF bus and the
// 6600V SWS pump motors with it; the simplified model latches
// PumpRunning so the operator doesn't have to start the pumps by
// hand during an accident. Manual / Test starts must NOT auto-start
// the SWS (those are operator-initiated tests, not safety signals).
//
// Spec justification:
//   - SWS pumps fed from the 6600V ESF bus:
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
//   - SI starts diesel + HHSI; SWS follows the same bus path:
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
func TestSWS_PumpAutoStartsOnSafetySignal(t *testing.T) {
	for _, cause := range []DieselStartCause{
		DieselStartSafetyInjection,
		DieselStartLossOfOffsitePower,
	} {
		p := MakePlant()
		require.True(t, p.Outside.ServiceWaterSystem.PumpRunning,
			"precondition: SWS pumps running at construction")

		// Operator stops the SWS pump, then SI signal arrives.
		p.Outside.ServiceWaterSystem.PumpRunning = false
		p.AuxiliaryBuilding.DieselGenerator.StartSignal = cause
		stepN(&p, 1, DefaultTimeStep)
		assert.True(t, p.Outside.ServiceWaterSystem.PumpRunning,
			"want SWS PumpRunning auto-latched on %v, got false",
			cause)
	}

	// Manual / Test starts must NOT auto-start the SWS pump.
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = false
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, 1, DefaultTimeStep)
	assert.False(t, p.Outside.ServiceWaterSystem.PumpRunning,
		"want SWS PumpRunning remain false on DieselStartManual, got true")
}
