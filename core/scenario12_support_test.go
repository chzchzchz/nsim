package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Anchors for the Antoine saturation fit against independently
// tabulated saturation pressures (spec-stated tier: the coefficients
// are published physics cached at
// specs/wiki/Vapor_pressure_of_water.txt:793-795; the anchor points are
// standard steam-table values, not recomputed from the same formula).
func TestPressurizerSatPressure_SteamTableAnchors(t *testing.T) {
	assert.InDelta(t, 14.7, float64(PressurizerSatPressure(212)), 0.5,
		"saturation at the normal boiling point must be atmospheric")
	assert.InDelta(t, 134.7, float64(PressurizerSatPressure(350)), 4,
		"saturation at 350 F should match the steam tables")
	assert.InDelta(t, 680.0, float64(PressurizerSatPressure(500)), 25,
		"saturation at 500 F should match the steam tables")
	assert.Greater(t, PressurizerSatPressure(558), PressurizerSatPressure(350),
		"the curve must be monotone over the cooldown range")
}

// A planned cooldown depressurizes alongside the RCS: with heaters off,
// spray taps drive pressure toward the saturation floor of the coolant,
// which passes below the RHRS entry pressure once Tavg is under ~430 F.
// Without temperature tracking this gate was unreachable (fixed floors
// at 1400/1500 psia versus a <400 psia entry condition).
//
// "When RCS temperature and pressure are less than 350°F ... and
// approximately 400 psig, respectively, the RHRS is placed in
// operation."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:127-131
func TestPressurizer_DepressurizeTracksSaturationToRHRSGate(t *testing.T) {
	pz := Pressurizer{
		Volume:   Volume{Val: 1500, Unit: u.Gallon},
		Water:    Volume{Val: 1500, Unit: u.Gallon},
		Pressure: PressurizerHeaterTarget,
	}
	// Cool the hot leg from no-load to below the RHRS entry
	// temperature one degree at a time; heaters off, spray open so
	// manual pressure control tracks the falling saturation floor
	// while contraction grows the steam space (page_227.txt:81-96).
	pz.SprayOpen = 1.0
	for tempF := CoreReferenceInletF - 1; tempF >= 349; tempF-- {
		hotLeg := Temperature{Val: float64(tempF), Unit: u.Fahrenheit}
		for range int(200 * time.Millisecond / TimeStep) {
			pz, _ = pz.Step(hotLeg, nil, nil, nil, 1, 0)
		}
	}
	floor := PressurizerSatPressure(349)
	require.Less(t, float64(pz.Pressure), 400.0,
		"cooldown depressurization must cross the RHRS entry pressure")
	assert.InDelta(t, float64(floor), float64(pz.Pressure), 5.0,
		"pressure settles near the saturation floor for the cold coolant")
	assert.Positive(t, pz.SteamBubble.Float(),
		"contraction must have grown a steam space from the water-solid start")
}

// Steam-dump draw in excess of SG heat input cools the primary:
// two identically seeded vessels differ only in secondaryExcess, and
// the dumped vessel must lose outlet temperature at the closed-form
// m*cp rate (wiring-check tolerance: the relaxation term also moves a
// little, so the band is a factor of two around the energy balance,
// not an exact figure).
//
// "Steam dumps are adjusted to start a 50°F/hr cooldown rate."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:73-74
func TestRPV_SecondaryExcessCoolsOutlet(t *testing.T) {
	// Sized so the unclamped m*cp drop stays inside the operational
	// program envelope (50 F/hr x 60 s = 0.83 F).
	excess := Watts(40e6)
	window := int(60 * time.Second / TimeStep)

	// Subcritical core: production is source-floor, so the dump draw
	// carries the full m*cp cooling share (the planned-cooldown
	// regime).
	base := ReactorPressureVessel{Outlet: Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}}
	dump := base
	base.FissionPower = CoreNeutronSourcePower
	dump.FissionPower = CoreNeutronSourcePower
	for range window {
		base = base.Step(RCPRatedSpeed, 0)
		dump = dump.Step(RCPRatedSpeed, excess)
	}

	// Differential readout: the shared relaxation term cancels
	// between the twins, leaving the pure energy-balance offset.
	massKg := RCSMassKg
	wantDropF := float64(excess) / (massKg * SpecificHeatWater) *
		float64(window) * TimeStep.Seconds() * 9.0 / 5.0
	gotDropF := base.Outlet.Float() - dump.Outlet.Float()
	require.Greater(t, gotDropF, 0.0,
		"the dumped twin must sit below its neutralized twin")
	assert.InDelta(t, wantDropF, gotDropF, wantDropF,
		"cooling should track the m*cp energy balance within a factor of two: "+
			"want %v got %v", wantDropF, gotDropF)
}

// The same draw against a producing core is load-following absorbed by
// the secondary: the deficit's share of production weights it to near
// zero, so at-power anchors stay put.
func TestRPV_SecondaryExcessSuppressedAtPower(t *testing.T) {
	excess := Watts(200e6)
	atPower := ReactorPressureVessel{
		Outlet: Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
	}
	atPower.SeedSteadyStatePower(CoreRatedThermalPower)
	for range int(60 * time.Second / TimeStep) {
		atPower = atPower.Step(RCPRatedSpeed, excess)
	}
	assert.InDelta(t, CoreReferenceOutletF, atPower.Outlet.Float(), 1.0,
		"at power the draw must not walk the anchor: got %v",
		atPower.Outlet.Float())
}

// The operator SI block arms during planned cooldowns before the
// low-steamline actuation setpoint is crossed; it gates only the SI
// ESFAS output, not reactor trip.
//
// "Prior to reaching the automatic safety injection setpoints during
// cooldown and depressurization, the safety injection initiation
// signals are blocked."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
func TestRPS_SafetyInjectionBlockGatesActuation(t *testing.T) {
	lowSteamline := RPSLowSteamlinePressureTrip - 10

	blocked := ReactorProtectionSystem{SafetyInjectionBlocked: true}
	blocked = blocked.Step(lowSteamline, PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0, 1, 0,
		false)
	assert.False(t, blocked.SafetyInjectionActuated,
		"a blocked SI signal must not actuate below the setpoint")
	assert.False(t, blocked.ReactorTripped,
		"one channel below voting threshold must not trip the reactor")

	open := ReactorProtectionSystem{}
	open = open.Step(lowSteamline, PressurizerHeaterTarget, 50, CoreReferenceInletF, 0, 0, 1, 0,
		false)
	assert.True(t, open.SafetyInjectionActuated,
		"without the block the same state must actuate SI")
}

// Shut accumulator isolation valves hold the tanks through the
// discharge precharge during a planned cooldown.
//
// "Before reaching the pressure when the safety injection accumulators
// would discharge, the accumulator isolation valves are closed."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:111-113
func TestSIS_AccumulatorIsolationShutBlocksDischarge(t *testing.T) {
	belowPrecharge := float64(SISAccumulatorPrechargePressure) - 1

	shut := SIS{Accumulator: Accumulator{
		Level:         SISAccumulatorPerTankCapacity,
		Capacity:      SISAccumulatorPerTankCapacity,
		IsolationShut: true,
	}}
	shut = shut.Step(belowPrecharge)
	assert.False(t, shut.Accumulator.Discharging,
		"isolated tanks must not discharge below precharge")
	assert.Equal(t, SISAccumulatorPerTankCapacity.Float(),
		shut.Accumulator.Level.Float(),
		"isolated tank inventory must stay constant")

	open := SIS{Accumulator: Accumulator{
		Level:    SISAccumulatorPerTankCapacity,
		Capacity: SISAccumulatorPerTankCapacity,
	}}
	open = open.Step(belowPrecharge)
	assert.True(t, open.Accumulator.Discharging,
		"open isolation must discharge below precharge")
	assert.Less(t, open.Accumulator.Level.Float(),
		SISAccumulatorPerTankCapacity.Float())
}

// Wet layup fill tops shell inventory toward the vents once the
// header is at vent pressure, and refuses a live header.
//
// "When the reactor coolant reaches 200°F (93°C), the steam sides of
// the steam generators are filled to the vents for wet layup."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:144-145
func TestSteamGenerator_WetLayupFillTopsToVents(t *testing.T) {
	live := SteamGenerator{
		Volume: Volume{Val: 100000, Unit: u.Gallon},
		Water:  Volume{Val: 30000, Unit: u.Gallon},
	}
	live.Pressure = Pressure(SGWetLayupMaxHeaderPsia + 100)
	live.WetLayupFillValve = Valve{Open: 1}
	for range int(60 * time.Second / TimeStep) {
		live = live.Step(0)
	}
	assert.Equal(t, 30000.0, live.Water.Float(),
		"a live header must not accept layup fill")

	vented := SteamGenerator{
		Volume: Volume{Val: 100000, Unit: u.Gallon},
		Water:  Volume{Val: 30000, Unit: u.Gallon},
	}
	vented.Pressure = Pressure(5)
	vented.WetLayupFillValve = Valve{Open: 1}
	for range int(60 * time.Second / TimeStep) {
		vented = vented.Step(0)
	}
	wantGain := SGWetLayupFillGalPerSec * 60.0
	assert.InDelta(t, 30000+wantGain, vented.Water.Float(), wantGain*0.01,
		"fill should add the design rate while below the vents")

	for range int(3500 * time.Second / TimeStep) {
		vented = vented.Step(0)
	}
	assert.Equal(t, 100000.0, vented.Water.Float(),
		"fill clamps at the vents (shell full)")
}

// The manual charging throttle scales design flow in non-auto mode;
// the auto level controller is unaffected by it.
//
// "The charging pump flow and low-pressure letdown valve are
// controlled manually during plant startup and cooldown."
// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
func TestCVCS_ChargingFlowDemandThrottlesManualFlow(t *testing.T) {
	hotTavg := Temperature{Val: 571, Unit: u.Fahrenheit}
	c := CVCS{ChargingPumpRunning: true}
	c = c.Step(Volume{}, hotTavg, 0.6, PressurizerHeaterTarget)
	require.Equal(t, CVCSDesignChargingFlow.Float(), c.ChargingFlow.Float(),
		"default demand keeps full design flow")

	half := CVCS{ChargingPumpRunning: true, ChargingFlowDemand: 0.5}
	half = half.Step(Volume{}, hotTavg, 0.6, PressurizerHeaterTarget)
	assert.InDelta(t, CVCSDesignChargingFlow.Float()*0.5,
		half.ChargingFlow.Float(), 1e-9)

	full := CVCS{ChargingPumpRunning: true, ChargingFlowDemand: 2}
	full = full.Step(Volume{}, hotTavg, 0.6, PressurizerHeaterTarget)
	assert.InDelta(t, CVCSDesignChargingFlow.Float(),
		full.ChargingFlow.Float(), 1e-9,
		"demand clamps at design flow")
}

// Auxiliary spray is throttled charging flow, so the valve delivers
// nothing with the charging pump secured and its drive follows the
// valve otherwise.
//
// "Charging flow is throttled to control pressurizer cooldown rate and
// raise pressurizer level using auxiliary spray since normal spray is
// not available when reactor coolant pumps have been secured."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104
func TestCVCS_AuxSprayDriveRequiresChargingPump(t *testing.T) {
	secured := CVCS{AuxSprayValveOpen: 1}
	secured = secured.Step(Volume{}, Temperature{}, 0.5, PressurizerHeaterTarget)
	assert.Zero(t, secured.AuxSprayDrive,
		"no charging pump means no auxiliary spray flow")

	supplying := CVCS{AuxSprayValveOpen: 1, ChargingPumpRunning: true}
	supplying = supplying.Step(Volume{}, Temperature{}, 0.5,
		PressurizerHeaterTarget)
	assert.InDelta(t, 1.0, supplying.AuxSprayDrive, 1e-9,
		"open valve on a running pump delivers full drive")
}

// With the reactor coolant pumps secured (sprayDrive == 0) normal spray
// must do nothing; RCP-driven spray depressurizes only with flow behind
// it.
//
// "To provide spray flow a RCP must be operating in one of the two loops
// with spray flow connections."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:96-98
func TestPressurizer_NormalSprayNeedsRCPFlow(t *testing.T) {
	hotLeg := Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	steps := int(time.Second / TimeStep)

	secured := Pressurizer{
		Volume: Volume{Val: 1500, Unit: u.Gallon}, SprayOpen: 1,
		Pressure: PressurizerHeaterTarget,
	}
	for range steps {
		secured, _ = secured.Step(hotLeg, nil, nil, nil, 0, 0)
	}
	assert.Equal(t, float64(PressurizerHeaterTarget),
		float64(secured.Pressure),
		"normal spray without an operating RCP must not move pressure")

	running := secured
	for range steps {
		running, _ = running.Step(hotLeg, nil, nil, nil, 1, 0)
	}
	assert.Less(t, float64(running.Pressure),
		float64(PressurizerHeaterTarget),
		"the same spray tap with RCP flow must depressurize")
}

// Auxiliary spray holds pressure control with the pumps fully secured:
// it walks a steam-bubble pressurizer down to the coolant saturation
// floor at the slow auxiliary rate while the idle path stays out of the
// way.
//
// "Pressurizer pressure is also controlled by auxiliary spray while a
// steam bubble exists in the pressurizer."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:104-106
func TestPressurizer_AuxSprayControlsPressureWithRCPsSecured(t *testing.T) {
	hotLeg := Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	pz := Pressurizer{
		Volume:   Volume{Val: 1500, Unit: u.Gallon},
		Water:    Volume{Val: 900, Unit: u.Gallon},
		Pressure: PressurizerHeaterTarget,
	}
	window := int(10 * time.Second / TimeStep)
	for range window {
		pz, _ = pz.Step(hotLeg, nil, nil, nil, 0, 1)
	}
	floor := float64(PressurizerSatPressure(CoreReferenceOutletF))
	want := float64(PressurizerHeaterTarget) -
		float64(PressurizerAuxSprayRate)*float64(window)*TimeStep.Seconds()
	got := float64(pz.Pressure)
	assert.InDelta(t, max(want, floor), got, max(want, floor)*0.02,
		"auxiliary spray should track its own rate toward the saturation "+
			"floor: want ~%v got %v", max(want, floor), got)
}

// Letdown orifice flow collapses as the primary is depressurized:
// full design flow at design pressure, half at a quarter of it, and
// nearly nothing near atmospheric.
//
// "Very little letdown flow is possible through the normal letdown flow
// path at low primary system pressures due to the low differential
// pressure across the CVCS letdown orifices."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:135-139
func TestCVCS_LetdownFallsWithLowRcsPressure(t *testing.T) {
	open := func(pzrPressure Pressure) CVCS {
		c := CVCS{LetdownValveOpen: true}
		return c.Step(Volume{}, Temperature{}, 0.5, pzrPressure)
	}
	design := open(Pressure(CVCSLetdownDesignPsia))
	assert.InDelta(t, CVCSDesignLetdownFlow.Float(),
		design.LetdownFlow.Float(), 1e-9, "design pressure passes design flow")

	half := open(Pressure(CVCSLetdownDesignPsia / 4))
	assert.InDelta(t, CVCSDesignLetdownFlow.Float()*0.5,
		half.LetdownFlow.Float(), 1e-9,
		"sqrt(dP) scaling halves flow at a quarter pressure")

	cold := open(30)
	assert.Less(t, cold.LetdownFlow.Float(), CVCSDesignLetdownFlow.Float()*0.15,
		"near-atmospheric letdown is nearly starved: got %v",
		cold.LetdownFlow.Float())
}

// The solid-plant pressure control valve bleeds a water-solid primary
// back to the selected pressure through the RHR/CVC interface, then
// stands down once the selected pressure is reached.
//
// "Solid plant primary pressure control utilizes a pressure control
// valve between the RHR and CVC systems which is set to maintain a
// selected pressure while charging makes up for any coolant contraction
// and losses."
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:115-122
func TestRHRS_SolidPlantPressureValveHoldsSelectedPressure(t *testing.T) {
	rhrs := RHRS{Running: true,
		SolidPlantPressureSetpoint: Pressure(400)}
	rhrs.SolidPlantPressureValve.Open = 1
	solid := Pressurizer{
		Volume:   Volume{Val: 1500, Unit: u.Gallon},
		Water:    Volume{Val: 1500, Unit: u.Gallon},
		Pressure: PressurizerHeaterTarget,
	}

	bledGal := 0.0
	for range int(60 * time.Second / TimeStep) {
		bledGal += rhrs.SolidPlantRelieve(&solid).Float()
		if rhrs.SolidPlantReliefFlow.Float() == 0 {
			break
		}
	}
	assert.GreaterOrEqual(t, float64(solid.Pressure), 400.0,
		"the bleed must never undershoot the selected pressure")
	assert.InDelta(t, 400.0, float64(solid.Pressure), 5.0,
		"the bleed should settle on the selected pressure: got %v",
		solid.Pressure)
	assert.InDelta(t, (float64(PressurizerHeaterTarget)-400.0)/
		PressurizerBubbleStiffness, bledGal, 1.0,
		"settling 2250 -> 400 psia takes the stiffness-equivalent volume")

	idle := rhrs.SolidPlantRelieve(&solid)
	assert.Zero(t, idle.Float(),
		"at the setpoint the valve stands down")
}

// The valve has no authority without a running RHR train to force flow
// between the two systems.
func TestRHRS_SolidPlantValveNeedsRHRRunning(t *testing.T) {
	rhrs := RHRS{SolidPlantPressureSetpoint: Pressure(400)}
	rhrs.SolidPlantPressureValve.Open = 1
	solid := Pressurizer{
		Volume:   Volume{Val: 1500, Unit: u.Gallon},
		Water:    Volume{Val: 1500, Unit: u.Gallon},
		Pressure: PressurizerHeaterTarget,
	}
	bled := rhrs.SolidPlantRelieve(&solid)
	assert.Zero(t, bled.Float(),
		"without forced RHR flow the CVC interface carries nothing")
	assert.Equal(t, float64(PressurizerHeaterTarget),
		float64(solid.Pressure), "pressure must be untouched")
}

// The gap-closure controls are operator-facing: they must round-trip
// through the bridge and the auxiliary spray must actually move plant
// pressure with the reactor coolant pumps secured.
func TestBridge_GapClosureControlsRoundTrip(t *testing.T) {
	p := MakePlant()
	m := GetControlMap(&p)

	aux := m["AuxiliaryBuilding/ChemicalVolumeControlSystem/AuxSprayValve/Open"]
	require.NotNil(t, aux, "want aux spray control registered")
	require.NoError(t, aux.Set("0.5"))
	assert.Equal(t, "0.5", aux.Get())

	valve := m["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureValve/Open"]
	require.NotNil(t, valve, "want solid-plant valve control registered")
	require.NoError(t, valve.Set("1"))
	assert.Equal(t, "1", valve.Get())

	setpoint := m["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureSetpoint"]
	require.NotNil(t, setpoint, "want solid-plant setpoint control registered")
	require.NoError(t, setpoint.Set("400"))
	assert.Equal(t, "400", setpoint.Get())
}

func TestPlant_AuxSprayMovesPressureWithRCPsSecured(t *testing.T) {
	run := func(auxOpen string) float64 {
		p := MakePlant()
		cm := GetControlMap(&p)
		require.NoError(t, cm["AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning"].Set("1"))
		require.NoError(t, cm["AuxiliaryBuilding/ChemicalVolumeControlSystem/AuxSprayValve/Open"].Set(auxOpen))
		// Aux-spray pressure control is a heaters-secured evolution:
		// "All pressurizer heaters are turned off and the spray
		// valves are controlled manually [...]"
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:81-84
		p.ReactorBuilding.Pressurizer.HeatersOn = false
		before := p.ReactorBuilding.Pressurizer.Pressure
		stepN(&p, int(5*time.Second/TimeStep))
		return float64(before) - float64(p.ReactorBuilding.Pressurizer.Pressure)
	}
	idleOnly := run("0")
	withAux := run("1")
	assert.Greater(t, withAux, idleOnly+float64(PressurizerAuxSprayRate)*2.5,
		"auxiliary spray must depressurize well beyond the idle drift: "+
			"idle %v aux %v", idleOnly, withAux)
}
