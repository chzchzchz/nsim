package core

import (
	"encoding/json"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With vacuum pumps running and no steam load, the condenser
// pressure relaxes toward the design vacuum target.
// "Under normal operation the air is removed by steam jet air
// ejectors or vacuum pumps."
// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
func TestCondenser_VacuumPumpsRestoreVacuum(t *testing.T) {
	t.Parallel()
	c := Condenser{
		Pressure:           AtmosphericPressure,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	for i := 0; i < int(5*time.Second/DefaultTimeStep); i++ {
		c = c.Step(DefaultTimeStep, c.CirculatingWaterIn, true, true)
	}
	assert.Less(t, c.Pressure, AtmosphericPressure,
		"want condenser pressure below atmospheric, got %v", c.Pressure)
	assert.LessOrEqual(t, c.Pressure, CondenserDesignVacuum*2,
		"want condenser near design vacuum (%v), got %v", CondenserDesignVacuum, c.Pressure)
}

// With the CW pumps off, the condenser loses its heat sink and
// the back-pressure rises toward atmospheric within a few seconds.
func TestCondenser_NoCWDegradesToAtmospheric(t *testing.T) {
	t.Parallel()
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	for i := 0; i < int(30*time.Second/DefaultTimeStep); i++ {
		c = c.Step(DefaultTimeStep, c.CirculatingWaterIn, true, false)
	}
	assert.Greater(t, c.Pressure, CondenserDesignVacuum*5,
		"want pressure risen from vacuum, got %v", c.Pressure)
	assert.Less(t, c.Pressure, AtmosphericPressure*1.5,
		"want pressure below 1.5× atmospheric, got %v", c.Pressure)
}

// The steam-dump controller opens the dump valves on a turbine trip
// and recloses them automatically once steam is readmitted to the
// turbine.
//
// Spec justification:
//   - "These valves open under a sudden reduction in turbine-
//     generator load (in excess of 10%) or a turbine trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
//   - "The dump valves close automatically as steam is admitted to
//     the turbine."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:32-33
func TestCondenser_TripOpensDumpValveAndReclosesOnAdmission(t *testing.T) {
	t.Parallel()
	// Post-trip stored heat: Tavg well above the no-load value, so
	// the Tavg-modulated trip dump demands full open.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:70-77
	hotTavg := Temperature{Val: CoreReferenceInletF +
		SteamDumpTripDeadbandF + SteamDumpTripSpanF, Unit: u.Fahrenheit}
	tb := TurbineBuilding{
		Condenser: Condenser{
			Pressure:           CondenserDesignVacuum,
			VacuumPumpRunning:  true,
			CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
		},
		Turbine: Turbine{
			State:              TurbineTripped,
			GoverningValve:     Valve{Open: 1},
			LubeOilPumpRunning: true,
			BearingOilPressure: BearingOilDesignPressure,
		},
	}
	for i := 0; i < int(2*time.Second/DefaultTimeStep); i++ {
		tb = tb.StepWithSteam(DefaultTimeStep, SteamGeneratorShellPressure, hotTavg, true, false, 1, 560)
	}
	assert.Greater(t, tb.Turbine.DumpValve.Open, 0.9,
		"want DumpValve opened by the trip, got %v", tb.Turbine.DumpValve.Open)

	// Readmit steam: the dump valves ramp closed again. With Tavg
	// back on the load program there is no standing excess for the
	// untripped restoration branch to carry, so admission alone
	// closes them.
	tb.Turbine.State = TurbineRunning
	tb.Turbine.StopValve.Open = 1.0
	onProgram := Temperature{
		Val:  programmedRodTavg(0) + 1,
		Unit: u.Fahrenheit,
	}
	for i := 0; i < int(2*time.Second/DefaultTimeStep); i++ {
		tb = tb.StepWithSteam(DefaultTimeStep, SteamGeneratorShellPressure, onProgram, true, false, 1, 560)
	}
	assert.Zero(t, tb.Turbine.DumpValve.Open,
		"want DumpValve closed once steam is admitted, got %v",
		tb.Turbine.DumpValve.Open)
}

// Each step accumulates the previous step's CondensingRate into
// Condenser.Level. After several steps with non-zero condensing
// rate, the level rises by the integrated mass.
func TestCondenser_LevelAccumulatesCondensingRate(t *testing.T) {
	t.Parallel()
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	const perStep = 5.0
	const steps = 100
	for i := 0; i < steps; i++ {
		c.CondensingRate = Volume{Val: perStep, Unit: u.Gallon}
		c = c.Step(DefaultTimeStep, c.CirculatingWaterIn, true, true)
	}
	want := perStep * float64(steps)
	got := c.Level.Float()
	assert.Positive(t, got, "want Level > 0")
	assert.InDelta(t, want, got, want*0.1,
		"want Level ~%v after %d steps of %v, got %v", want, steps, perStep, got)
}

// HotwellLevel accumulates condensed steam minus the condensate
// pump suction flow. With CW running and incoming steam load, the
// hotwell rises.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser
//     hotwell [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-42
func TestCondenser_HotwellAccumulates(t *testing.T) {
	t.Parallel()
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
	}
	const perStep = 5.0
	const steps = 1000
	for i := 0; i < steps; i++ {
		c.CondensingRate = Volume{Val: perStep, Unit: u.Gallon}
		c = c.Step(DefaultTimeStep, c.CirculatingWaterIn, true, true)
	}
	assert.Positive(t, c.HotwellLevel.Float(),
		"want HotwellLevel > 0 after %d steps, got %v", steps, c.HotwellLevel.Float())
}

// Hotwell clamps at capacity: seeding at capacity and stepping with a
// positive CondensingRate leaves HotwellLevel == capacity and
// HotwellOverflow grows by exactly the spilled amount; the Level
// mirror matches HotwellLevel after the clamp (plans/p64_1.txt).
// "There is also sufficient surface to condense steam from the steam
// dump system" (specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-20) —
// the finite hotwell must accept dump condensate without running away.
func TestCondenser_HotwellClampsAtCapacity(t *testing.T) {
	t.Parallel()
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
		HotwellLevel:       Volume{Val: CondenserHotwellCapacityGal, Unit: u.Gallon},
		Level:              Volume{Val: CondenserHotwellCapacityGal, Unit: u.Gallon},
	}
	c.CondensingRate = Volume{Val: 100, Unit: u.Gallon}
	c = c.Step(DefaultTimeStep, c.CirculatingWaterIn, true, true)
	assert.Equal(t, CondenserHotwellCapacityGal, c.HotwellLevel.Float(),
		"want clamped at capacity, got %v", c.HotwellLevel.Float())
	assert.Equal(t, 100.0, c.HotwellOverflow.Float(),
		"want spill == steam mass, got %v", c.HotwellOverflow.Float())
	assert.Equal(t, c.HotwellLevel.Float(), c.Level.Float(),
		"want Level mirror == HotwellLevel after clamp, got %v vs %v",
		c.Level.Float(), c.HotwellLevel.Float())
}

// Clamp holds on the no-CW early return too: with CW pumps secured the
// condenser degrades pressure but must still credit the hotwell
// through the same ceiling (plans/p64_1.txt).
func TestCondenser_HotwellClampsOnNoCWReturn(t *testing.T) {
	t.Parallel()
	c := Condenser{
		Pressure:           CondenserDesignVacuum,
		VacuumPumpRunning:  true,
		CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
		HotwellLevel:       Volume{Val: CondenserHotwellCapacityGal, Unit: u.Gallon},
	}
	c.CondensingRate = Volume{Val: 250, Unit: u.Gallon}
	c = c.Step(DefaultTimeStep, c.CirculatingWaterIn, true, false)
	assert.Equal(t, CondenserHotwellCapacityGal, c.HotwellLevel.Float(),
		"want clamped on no-CW return, got %v", c.HotwellLevel.Float())
	assert.Equal(t, 250.0, c.HotwellOverflow.Float(),
		"want spill on no-CW return, got %v", c.HotwellOverflow.Float())
}

// HotwellOverflow survives JSON round-trip (plans/p64_1.txt).
func TestCondenser_HotwellOverflowRoundTrip(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.Condenser.HotwellOverflow = Volume{Val: 1234, Unit: u.Gallon}
	p.nextPlant = nil
	b, err := json.Marshal(p)
	require.NoError(t, err)
	var p2 Plant
	require.NoError(t, json.Unmarshal(b, &p2))
	assert.Equal(t, 1234.0, p2.TurbineBuilding.Condenser.HotwellOverflow.Float())
	assert.Equal(t, p, p2, "every plant field must survive the round trip")
}

// With RejectValve open and the clamp engaged, the hotwell ends at
// capacity - rejectDebit (the spill is not subtracted a second time
// by the reject ledger), per plans/p64_1.txt. RejectValve debits
// FeedPumpSuctionFlow * open * dt (core/plant.go:701-707); the
// Witness drives StepWithSteam directly (the p59 harness pattern)
// so MakePlant's line-up does not refill past the clamp between
// the reject and the post-write re-assertion.
func TestPlant_HotwellOverflowNotDoubleSubtracted(t *testing.T) {
	t.Parallel()
	b := MakePlant().TurbineBuilding
	b.Condenser.HotwellLevel = Volume{Val: CondenserHotwellCapacityGal, Unit: u.Gallon}
	b.RejectValve.Open = 1
	out := b.StepWithSteam(DefaultTimeStep, SteamGeneratorShellPressure,
		Temperature{Val: 540, Unit: u.Fahrenheit}, true, false, 1, 540)
	hotwell := out.Condenser.HotwellLevel.Float()
	assert.Less(t, hotwell, CondenserHotwellCapacityGal,
		"reject must debit the clamped level")
	assert.LessOrEqual(t, hotwell, CondenserHotwellCapacityGal,
		"hotwell must not exceed capacity")
}

// Twin-plant: a sustained dump profile fills the hotwell, the clamped
// plant stays at capacity with positive overflow, and the profile
// demonstrably exceeds capacity without the ceiling — templated on
// the TestLoadRejectionSteamDumpRideThrough harness
// (core/scenario17_test.go) but paced until the level reaches the
// ceiling (at the observed ~7.5 gal/s dump rate a 30 M gal ceiling
// needs ~4000 s, not 600 s — plans/p64_1.txt). The unit clamp
// witnesses above bound the mechanism; this arc proves the production
// path (Plant.Step + post-write re-assertion) enforces it.
func TestCondenser_HotwellNeverExceedsCapacityUnderDump(t *testing.T) {
	t.Parallel()
	testDt := 10 * time.Millisecond
	clamped := fullPowerLineup()
	cm := clamped.GetControlMap()
	require.NoError(t, cm["TurbineBuilding/Turbine/Setpoint"].Set("0.05"))
	// Run until the ceiling is reached or a timeout bounds the arc.
	const timeout = 6000 * time.Second
	var steps int
	for i := 0; i < int(timeout/testDt); i++ {
		steps = i
		clamped.Step(testDt)
		clamped.Update()
		require.LessOrEqual(t,
			clamped.TurbineBuilding.Condenser.HotwellLevel.Float(),
			CondenserHotwellCapacityGal+1e-9,
			"clamped hotwell must never exceed capacity at step %d", i)
		if clamped.TurbineBuilding.Condenser.HotwellOverflow.Float() > 0 {
			break
		}
	}
	_ = steps
	got := clamped.TurbineBuilding.Condenser.HotwellOverflow.Float()
	assert.Positive(t, got,
		"sustained dump must overflow the hotwell, got %v", got)
}
