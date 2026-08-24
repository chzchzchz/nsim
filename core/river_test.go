package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// River-to-containment heat-sink chain (plans/x/river.md): the
// river boundary condition must reach the containment cooling
// coils through the service water system, and loss of the
// ultimate heat sink must be visible in the reactor building.
//
// Spec justification:
//   - Shared intake: "serves one circulating water pump and one
//     service water pump."
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:37-38
//   - "Cooling water is supplied by the essential service water
//     system."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:86-87

// The SWS inlet temperature tracks the river every step.
func TestRiverFeedsServiceWaterInlet(t *testing.T) {
	p := MakePlant()
	p.Outside.RiverTemp = Temperature{Val: 88, Unit: u.Fahrenheit}
	stepN(&p, 1)
	assert.InDelta(t, 88.0,
		p.Outside.ServiceWaterSystem.InletTemp.Float(), 1e-9,
		"want SWS InletTemp=88 after one step, got %v",
		p.Outside.ServiceWaterSystem.InletTemp.Float())
}

// The RCFC coil inlet follows the river with a one-step lag
// (the coils read the prior-step service water state).
func TestRiverFeedsRCFCCoilInlet(t *testing.T) {
	p := MakePlant()
	p.Outside.RiverTemp = Temperature{Val: 88, Unit: u.Fahrenheit}
	stepN(&p, 2)
	for i := 0; i < RCFCUnitCount; i++ {
		assert.InDelta(t, 88.0,
			p.AuxiliaryBuilding.RCFCUnits[i].CoilInletTemp.Float(), 1e-9,
			"want RCFCUnits[%d].CoilInletTemp=88 after two steps, got %v",
			i, p.AuxiliaryBuilding.RCFCUnits[i].CoilInletTemp.Float())
	}
}

// With the service water pumps stopped the RCFC coils run dry:
// no unit rejects heat and the coil outlet tracks the inlet.
func TestLossOfServiceWaterKillsRCFCCooling(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = false
	stepN(&p, 3)
	assert.Zero(t, p.Outside.ServiceWaterSystem.Flow,
		"want SWS Flow=0 with pumps stopped, got %v",
		p.Outside.ServiceWaterSystem.Flow)
	for i := 0; i < RCFCUnitCount; i++ {
		unit := p.AuxiliaryBuilding.RCFCUnits[i]
		assert.Zero(t, unit.HeatTransferRate,
			"want RCFCUnits[%d].HeatTransferRate=0 without SW flow, got %v",
			i, unit.HeatTransferRate)
		assert.InDelta(t, unit.CoilInletTemp.Float(),
			unit.CoilOutletTemp.Float(), 1e-9,
			"want RCFCUnits[%d] CoilOutletTemp=CoilInletTemp with dry coils, got %v",
			i, unit.CoilOutletTemp.Float())
	}
}

// A hot river lifts the CCW inlet target above the 105°F design
// value (target = supply + approach); at the nominal 75°F river
// the loop holds the spec value.
func TestHotRiverLiftsCCWInletTarget(t *testing.T) {
	hot := MakePlant()
	hot.Outside.RiverTemp = Temperature{Val: 100, Unit: u.Fahrenheit}
	stepN(&hot, 2)
	ccws := hot.AuxiliaryBuilding.ComponentCoolingWaterSystem
	require.True(t, ccws.PumpRunning,
		"precondition: CCWS auto-starts once an ESF bus is energised")
	assert.Greater(t, ccws.InletTemp.Float(), CCWSInletSteadyStateTemp,
		"want CCW InletTemp > 105 with a 100°F river, got %v",
		ccws.InletTemp.Float())

	nominal := MakePlant()
	stepN(&nominal, 2)
	nominalCCWS := nominal.AuxiliaryBuilding.ComponentCoolingWaterSystem
	assert.InDelta(t, CCWSInletSteadyStateTemp,
		nominalCCWS.InletTemp.Float(), 1e-9,
		"want CCW InletTemp=105 at nominal river, got %v",
		nominalCCWS.InletTemp.Float())
}

// Without service water flow the CCW loop soaks toward the
// cooldown maximum and rejects no duty across its HX.
func TestLossOfServiceWaterSoaksCCWLoop(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = false
	stepN(&p, 2)
	ccws := p.AuxiliaryBuilding.ComponentCoolingWaterSystem
	require.True(t, ccws.PumpRunning,
		"precondition: CCWS auto-starts once an ESF bus is energised")
	assert.Greater(t, ccws.InletTemp.Float(), CCWSInletSteadyStateTemp,
		"want CCW InletTemp soaking above 105 without SW flow, got %v",
		ccws.InletTemp.Float())
	assert.Zero(t, ccws.HXHeatDuty,
		"want HXHeatDuty=0 without SW flow, got %v", ccws.HXHeatDuty)
}

// The SWS outlet rise reflects the total essential load: RCFC
// heat rejection plus the component cooling HX duty.
//
// Spec justification:
//   - "Component cooling heat exchangers"
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:81
func TestSWSTotalHeatLoadIncludesCCWDuty(t *testing.T) {
	p := MakePlant()
	stepN(&p, 2)
	rcfcSum := 0.0
	for i := 0; i < RCFCUnitCount; i++ {
		rcfcSum += p.AuxiliaryBuilding.RCFCUnits[i].HeatTransferRate
	}
	duty := float64(p.AuxiliaryBuilding.ComponentCoolingWaterSystem.HXHeatDuty)
	require.Greater(t, duty, 0.0,
		"want CCW HXHeatDuty > 0 in steady state, got %v", duty)

	massKgPerS := ServiceWaterDesignFlow * KilogramsPerGPM
	wantRise := (rcfcSum + duty) / (massKgPerS * SpecificHeatWater) * 9.0 / 5.0
	sws := p.Outside.ServiceWaterSystem
	gotRise := sws.OutletTemp.Float() - sws.InletTemp.Float()
	assert.InDelta(t, wantRise, gotRise, 1e-9,
		"want SWS outlet rise=%v from RCFC+CCW loads, got %v",
		wantRise, gotRise)
}

// CSS spray condensation removes enthalpy from the containment
// atmosphere: with the spray system forced on, the containment
// temperature falls below an otherwise identical no-spray plant.
// Comparative form because the RCFC units already cool the
// atmosphere from the 120°F startup value on their own.
//
// Spec justification:
//   - "the steam is condensed and falls to the containment floor
//     as drops of a borated sodium hydroxide solution."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_116.txt:4-7
func TestContainment_SprayCondensationCoolsAtmosphere(t *testing.T) {
	steps := 5 * int(time.Second/TimeStep)

	base := MakePlant()
	stepN(&base, steps)
	baseT := base.ReactorBuilding.ContainmentBuilding.Temperature.Float()

	sprayed := MakePlant()
	css := &sprayed.AuxiliaryBuilding.ContainmentSpraySystem
	css.Running = true
	css.DriveReady = true
	css.SprayHeaderOpen = true
	stepN(&sprayed, steps)
	sprayT := sprayed.ReactorBuilding.ContainmentBuilding.Temperature.Float()

	assert.Less(t, sprayT, baseT,
		"want sprayed containment %v cooler than baseline %v",
		sprayT, baseT)
}

// The diesel jacket cooler joins the SWS load list once the engine
// runs with cooling flow: the SWS outlet rise reflects RCFC + CCW
// HX + jacket duty together.
//
// Spec justification:
//   - "Diesel generator coolers"
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:89
func TestSWS_HeatLoadIncludesDieselJacketDuty(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal =
		DieselStartSafetyInjection
	stepN(&p, int(15*time.Second/TimeStep))

	dg := p.AuxiliaryBuilding.DieselGenerator
	require.True(t, dg.ReadyToLoad,
		"precondition: diesel ready after 15 s on SI signal")
	require.Greater(t, float64(dg.JacketHeatDuty), 0.0,
		"want jacket duty > 0 while running with SW flow, got %v",
		dg.JacketHeatDuty)

	rcfcSum := 0.0
	for i := 0; i < RCFCUnitCount; i++ {
		rcfcSum += p.AuxiliaryBuilding.RCFCUnits[i].HeatTransferRate
	}
	duty := float64(p.AuxiliaryBuilding.ComponentCoolingWaterSystem.HXHeatDuty)
	massKgPerS := ServiceWaterDesignFlow * KilogramsPerGPM
	wantRise := (rcfcSum + duty + float64(dg.JacketHeatDuty)) /
		(massKgPerS * SpecificHeatWater) * 9.0 / 5.0
	sws := p.Outside.ServiceWaterSystem
	gotRise := sws.OutletTemp.Float() - sws.InletTemp.Float()
	assert.InDelta(t, wantRise, gotRise, 1e-9,
		"want SWS outlet rise=%v from all essential loads, got %v",
		wantRise, gotRise)

	// With no service water flow the dry cooler hands nothing to
	// the SWS header. A manual start is used because an SI signal
	// would auto-re-latch the SWS pumps (see
	// TestSWS_PumpAutoStartsOnSafetySignal), making the dry-cooler
	// state unreachable through that path.
	dry := MakePlant()
	dry.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	dry.AuxiliaryBuilding.DieselGenerator.StartSignal =
		DieselStartManual
	dry.Outside.ServiceWaterSystem.PumpRunning = false
	stepN(&dry, int(15*time.Second/TimeStep))
	assert.Zero(t,
		dry.AuxiliaryBuilding.DieselGenerator.JacketHeatDuty,
		"want jacket duty=0 with dry cooler, got %v",
		dry.AuxiliaryBuilding.DieselGenerator.JacketHeatDuty)
}
