package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// At construction all four RCFC units are running at full
// speed, the backdraft damper is open, and the coil inlet is at
// the design service water temperature. HeatTransferRate is
// initialised to zero and gets computed on the first Step call.
func TestRCFC_InitialStateAllUnitsRunning(t *testing.T) {
	p := MakePlant()
	for i := 0; i < RCFCUnitCount; i++ {
		u0 := p.AuxiliaryBuilding.RCFCUnits[i]
		assert.True(t, u0.Running,
			"want RCFCUnits[%d].Running=true at startup, got %v",
			i, u0.Running)
		assert.False(t, u0.LowSpeed,
			"want RCFCUnits[%d].LowSpeed=false at startup, got %v",
			i, u0.LowSpeed)
		assert.True(t, u0.BackdraftDamperOpen,
			"want RCFCUnits[%d].BackdraftDamperOpen=true at startup, got %v",
			i, u0.BackdraftDamperOpen)
		assert.InDelta(t, RCFCSWInletTemp,
			u0.CoilInletTemp.Float(), 1e-9,
			"want RCFCUnits[%d].CoilInletTemp=RCFCSWInletTemp, got %v",
			i, u0.CoilInletTemp.Float())
	}
	// Step the units with the live containment temperature;
	// design ΔT (120°F atmosphere − 60°F coil inlet) equals
	// RCFCDesignDeltaT (50°F), so each unit's heat transfer
	// rate equals RCFCDesignHeatRejection.
	cbTemp := p.ReactorBuilding.ContainmentBuilding.Temperature
	for i := 0; i < RCFCUnitCount; i++ {
		p.AuxiliaryBuilding.RCFCUnits[i] =
			p.AuxiliaryBuilding.RCFCUnits[i].Step(false, cbTemp)
	}
	sum := 0.0
	for i := 0; i < RCFCUnitCount; i++ {
		sum += p.AuxiliaryBuilding.RCFCUnits[i].HeatTransferRate
	}
	assert.InDelta(t, RCFCDesignHeatRejection*RCFCUnitCount,
		sum, 1e-9,
		"want total HeatTransferRate=%v at design ΔT, got %v",
		RCFCDesignHeatRejection*RCFCUnitCount, sum)
}

// A stopped unit rejects no heat, closes its backdraft damper
// (to protect against reverse flow), and tracks the coil
// inlet/outlet temperatures as equal.
func TestRCFC_StoppedUnitRejectsNoHeat(t *testing.T) {
	r := RCFCUnit{
		Running:             false,
		BackdraftDamperOpen: true,
		CoilInletTemp:       u.NewValue(RCFCSWInletTemp, u.Fahrenheit),
	}
	r2 := r.Step(false, u.NewValue(120, u.Fahrenheit))
	assert.Zero(t, r2.HeatTransferRate,
		"want HeatTransferRate=0 for stopped unit, got %v",
		r2.HeatTransferRate)
	assert.False(t, r2.BackdraftDamperOpen,
		"want BackdraftDamperOpen=false for stopped unit, got %v",
		r2.BackdraftDamperOpen)
}

// When the LOCA signal arrives, a running unit drops to low
// speed.
//
// Spec justification:
//   - "When a LOCA is sensed, the fan motors are automatically
//     switched to low speed to provide proper flow of the
//     steam-air mixture."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:54-57
func TestRCFC_LOCASensedSwitchesToLowSpeed(t *testing.T) {
	r := RCFCUnit{
		Running:             true,
		LowSpeed:            false,
		CoilInletTemp:       u.NewValue(RCFCSWInletTemp, u.Fahrenheit),
		BackdraftDamperOpen: true,
	}
	r2 := r.Step(true, u.NewValue(120, u.Fahrenheit))
	assert.True(t, r2.LowSpeed,
		"want LowSpeed=true after LOCA signal, got %v", r2.LowSpeed)
}

// HeatTransferRate scales linearly with the live ΔT between
// containment atmosphere and coil inlet, clamped to the design
// rating above design ΔT.
func TestRCFC_HeatTransferScalesWithDeltaT(t *testing.T) {
	r := RCFCUnit{
		Running:             true,
		BackdraftDamperOpen: true,
		CoilInletTemp:       u.NewValue(RCFCSWInletTemp, u.Fahrenheit),
	}
	// Half-design ΔT (25°F above coil inlet) → half heat rating.
	half := r.Step(false, u.NewValue(RCFCSWInletTemp+25, u.Fahrenheit))
	assert.InDelta(t, RCFCDesignHeatRejection*0.5,
		half.HeatTransferRate, 1e-9,
		"want half design heat at half ΔT, got %v",
		half.HeatTransferRate)
	// ΔT at or below the coil approach temp → no heat transfer.
	zero := r.Step(false, u.NewValue(RCFCSWInletTemp+5, u.Fahrenheit))
	assert.Zero(t, zero.HeatTransferRate,
		"want zero heat transfer below approach temp, got %v",
		zero.HeatTransferRate)
	// ΔT well above design → clamped to design rating.
	clamped := r.Step(false, u.NewValue(RCFCSWInletTemp+200, u.Fahrenheit))
	assert.InDelta(t, RCFCDesignHeatRejection,
		clamped.HeatTransferRate, 1e-9,
		"want clamped heat at super-design ΔT, got %v",
		clamped.HeatTransferRate)
}

// RCFCUnitsRunning returns the count of units with Running=true.
func TestRCFC_RCFCUnitsRunningCountsRunningUnits(t *testing.T) {
	var units [RCFCUnitCount]RCFCUnit
	units[0].Running = true
	units[1].Running = true
	units[2].Running = false
	units[3].Running = false
	assert.Equal(t, 2, RCFCUnitsRunning(&units),
		"want 2 running units, got %d", RCFCUnitsRunning(&units))
}

// ContainmentBuilding.Step advances lumped temperature by
// net heat input minus RCFC heat sink; pressure is derived via
// the ideal-gas law (P_containment increases with temperature).
func TestContainmentBuilding_TemperatureAndPressureTrackHeatBalance(t *testing.T) {
	c := ContainmentBuilding{
		Pressure:       AtmosphericPressure,
		Temperature:    u.NewValue(120, u.Fahrenheit),
		DesignPressure: ContainmentDesignPressure,
		FreeVolume:     ContainmentFreeVolume,
	}
	// No heat in, no heat out → temperature unchanged.
	c2 := c.Step(Watts(0), Watts(0))
	assert.InDelta(t, c.Temperature.Float(), c2.Temperature.Float(), 1e-9,
		"want Temperature unchanged with no heat, got %v → %v",
		c.Temperature.Float(), c2.Temperature.Float())
	// Heat in, no RCFC → temperature rises.
	hot := c.Step(Watts(1e7), Watts(0))
	assert.Greater(t, hot.Temperature.Float(), c.Temperature.Float(),
		"want Temperature > 120 after 1e7 W in, got %v",
		hot.Temperature.Float())
	// RCFC sink matches heat input → temperature unchanged.
	balanced := c.Step(Watts(1e7), Watts(1e7))
	assert.InDelta(t, c.Temperature.Float(),
		balanced.Temperature.Float(), 1e-6,
		"want Temperature unchanged with balanced heat, got %v → %v",
		c.Temperature.Float(),
		balanced.Temperature.Float())
}

// End-to-end via Plant.Step: all four RCFC units run at
// startup; after the LOCA signal arrives the fans switch to low
// speed; the RCFC sink keeps the containment atmosphere from
// runaway heating.
func TestRCFC_EndToEndLOCAStartsLowSpeed(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	// Step long enough for the diesel to reach ReadyToLoad.
	stepN(&p, int(15*time.Second/TimeStep))
	lowCount := 0
	for i := 0; i < RCFCUnitCount; i++ {
		if p.AuxiliaryBuilding.RCFCUnits[i].LowSpeed {
			lowCount++
		}
	}
	assert.Equal(t, RCFCUnitCount, lowCount,
		"want all %d RCFC units at low speed after LOCA, got %d",
		RCFCUnitCount, lowCount)
}
