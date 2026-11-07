package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// With the generator breaker closed and the generator producing
// rated voltage, the UAT is energised and outputs the design
// 6600 V plant distribution bus voltage.
//
// Spec justification:
//   - "the unit auxiliary power is transformed from the output
//     of the plant main generator through the unit auxiliary
//     transformer to the plant electrical distribution voltage."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:45-48
//   - "transformed to the plant electrical distribution bus
//     voltage, normally 6600 volts"
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:42-44
func TestStationTransformer_UATEnergizedOnBreakerClose(t *testing.T) {
	s := (&StationTransformer{}).Step(DefaultTimeStep,
		float64(GeneratorRatedVoltage),
		GridRatedVoltage,
		true,
	)
	assert.True(t, s.UATEnergized,
		"want UATEnergized with breaker closed + generator voltage, got false")
	assert.InDelta(t, StationRatedBusVoltage, s.UATOutputVoltage, 1e-9,
		"want UATOutputVoltage=6600 V, got %v", s.UATOutputVoltage)
	assert.Equal(t, "UAT", s.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=UAT, got %q", s.HouseLoadSuppliedFrom)
}

// With the breaker open, UAT de-energises regardless of grid
// voltage. SAT picks up house load as long as grid voltage is
// available.
//
// Spec justification:
//   - "automatic transfer of supplies is provided on sensing
//     loss of power"
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:58-59
//   - "house loads automatically transferred to the offsite
//     sources of power"
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64-66
func TestStationTransformer_BreakerOpenTransfersToSAT(t *testing.T) {
	s := (&StationTransformer{}).Step(DefaultTimeStep,
		0,
		GridRatedVoltage,
		false,
	)
	assert.False(t, s.UATEnergized,
		"want UAT not energised with breaker open, got true")
	assert.Zero(t, s.UATOutputVoltage,
		"want UATOutputVoltage=0, got %v", s.UATOutputVoltage)
	assert.True(t, s.SATEnergized,
		"want SATEnergized with grid voltage, got false")
	assert.InDelta(t, StationRatedBusVoltage, s.SATOutputVoltage, 1e-9,
		"want SATOutputVoltage=6600 V, got %v", s.SATOutputVoltage)
	assert.Equal(t, "SAT", s.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=SAT after breaker open, got %q",
		s.HouseLoadSuppliedFrom)
}

// With both generator and grid unavailable, neither transformer
// is energised and house load cannot be supplied (black-out).
func TestStationTransformer_BothSourcesDownYieldsBlackOut(t *testing.T) {
	s := (&StationTransformer{}).Step(DefaultTimeStep, 0, 0, false)
	assert.False(t, s.UATEnergized,
		"want UAT not energised with no generator, got true")
	assert.False(t, s.SATEnergized,
		"want SAT not energised with no grid, got true")
	assert.Equal(t, "", s.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom='' in black-out, got %q",
		s.HouseLoadSuppliedFrom)
}

// HouseLoad.Step computes TotalMW as FixedMW + VariableMW * load
// fraction when the generator is online; FixedMW alone when
// offline.
//
// Spec justification:
//   - "supply power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
func TestHouseLoad_VariableScalesWithGeneratorFraction(t *testing.T) {
	h := HouseLoad{
		FixedMW:    HouseLoadFixed,
		VariableMW: HouseLoadFixed * HouseLoadVariableFraction,
	}
	offline := h.Step(DefaultTimeStep, false, 0)
	assert.InDelta(t, HouseLoadFixed, offline.TotalMW, 1e-9,
		"want TotalMW=FixedMW=%v offline, got %v",
		HouseLoadFixed, offline.TotalMW)

	halfLoad := h.Step(DefaultTimeStep, true, 0.5)
	want := HouseLoadFixed + HouseLoadFixed*HouseLoadVariableFraction*0.5
	assert.InDelta(t, want, halfLoad.TotalMW, 1e-9,
		"want TotalMW=%v at 0.5 load, got %v", want, halfLoad.TotalMW)

	fullLoad := h.Step(DefaultTimeStep, true, 1.0)
	wantFull := HouseLoadFixed + HouseLoadFixed*HouseLoadVariableFraction
	assert.InDelta(t, wantFull, fullLoad.TotalMW, 1e-9,
		"want TotalMW=%v at full load, got %v", wantFull, fullLoad.TotalMW)
}

// End-to-end: when MakePlant is initialised, the grid is at 60 Hz
// / 24 kV. After one Plant.Step (with the turbine offline but the
// grid available), the SAT is energised and supplies house load.
func TestPlant_GridAndStationDefaultsMatchSpec(t *testing.T) {
	p := MakePlant()
	g := p.ReactorBuilding.Grid
	assert.InDelta(t, 60.0, g.Frequency, 1e-9,
		"want Grid.Frequency=60 Hz, got %v", g.Frequency)
	assert.InDelta(t, GridRatedVoltage, g.Voltage, 1e-9,
		"want Grid.Voltage=%v V, got %v", GridRatedVoltage, g.Voltage)

	// Run one step so the station transformer / house-load
	// wiring runs through its step function.
	stepN(&p, 1, DefaultTimeStep)
	s := p.ReactorBuilding.StationTransformer
	assert.False(t, s.UATEnergized,
		"want UAT not energised at startup (breaker open), got true")
	assert.True(t, s.SATEnergized,
		"want SAT energised with grid available, got false")
	assert.Equal(t, "SAT", s.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=SAT at startup, got %q",
		s.HouseLoadSuppliedFrom)
}
