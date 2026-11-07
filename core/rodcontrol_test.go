package core

import (
	"strconv"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/chzchzchz/nsim/bridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reactor power control (RPC) durable assertions.
//
// The demanded power setpoint integrates toward the operator's
// target at an entered rate limited to 0.8 % of full power per
// second; a Tref program converts it to a reference coolant
// temperature; a deadband rod drive trims Tavg onto the reference;
// and the lead/lag select decides which demand the operator drives.
//
// Spec citations live on the declarations in core/parts.go and
// core/bridge.go; the bands here mirror the scenario 13 evolution
// specs/pwr_sim/chapter_3/page_036.txt:35-45.

// rpcReactorLeadPlant extends the shared full-power lineup with the
// scenario 13 start state: mid-cycle boron so part-load plateaus sit
// on the self-stabilizing side of the MTC crossover, and rods seated
// at the critical position for full power.
func rpcReactorLeadPlant() Plant {
	p := scenario10FullPowerPlant()
	p.ReactorBuilding.Pressurizer.HeatersOn = true
	// Main feedwater is in service during power operation: with the
	// corrected secondary mass balance the boiling stock really
	// depletes at the rated evaporation rate, so a multi-minute hold
	// needs the regulator feeding ("Feedwater flow to each steam
	// generator is controlled by a feedwater regulator [...]
	// controlled by steam generator level, steam flow and feedwater
	// flow", specs/the_westinghouse_pressurized_water_react/
	// chapter_15/page_219.txt:68-76).
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = true
	// The automatic boron trim is armed so the walked descent's
	// standing power error rides the slow chemistry actuator
	// (chapter_7/page_129.txt:112-115); without it the bank pins at
	// its withdrawal limit and the park cannot hold.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true
	// 550 ppm parks the shadow integrator exactly at rated: above it
	// the settled power sags below the park the descent math assumes.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = 600
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = 600
	// Rod position stays at the MakePlant rated-consistent default: a
	// hand-tuned insertion went stale when the fresh-plant seed moved
	// to the hot-standby idle lineup, and the mismatch folded the core
	// on the first steps.
	return p
}

// Entering a rate above 0.8 %/s ramps at 0.8 %/s; entering 0.08 %/s
// ramps at exactly that rate; entering zero holds the integrator.
// Pure integrator test - no plant physics involved.
func TestRPC_RateClampOnOperatorEntry(t *testing.T) {
	rpv := &ReactorPressureVessel{}
	turbine := &Turbine{}

	run := func(rate float64, seconds float64) float64 {
		po := PlantOperation{
			RodControlAuto:       true,
			LeadLagMode:          ReactorLead,
			PowerSetpointDemand:  0.90,
			PowerRateDemand:      rate,
			DemandedPowerSetpoint: 0,
		}
		steps := int(seconds / TimeStep.Seconds())
		for range steps {
			po.StepRPC(rpv, turbine, TimeStep)
		}
		return po.DemandedPowerSetpoint
	}

	assert.InDelta(t, 0.008, run(0.05, 1.0), 1e-9,
		"entry above the cap ramps at RodControlMaxRatePerSec")
	assert.InDelta(t, 0.0008, run(0.0008, 1.0), 1e-9,
		"entry of 0.08 %%/s ramps at exactly that rate")
	assert.InDelta(t, 0.8, run(0.05, 100.0), 1e-6,
		"capped ramp stops at the target, not beyond it")
	assert.InDelta(t, 0.1, run(0.0008, 125.0), 1e-6,
		"0.08 %%/s covers 10 points in 125 s")
	assert.InDelta(t, 0, run(0, 10.0), 1e-12,
		"zero entered rate freezes the integrator")
}

// Reactor-lead end-to-end: the operator writes mode, auto, target,
// and rate - four commands, no command ladder. The walked demand
// descends while the armed automatic boron trim carries the standing
// power error (the slow Power Control System actuator relieving the
// fast one, chapter_7/page_129.txt:112-115), the turbine load follows
// the walked target itself (specs/pwr_sim/chapter_2/page_023.txt:64-71),
// and the coupled loop converges onto the plateau through a damped
// two-time-scale transient measured at 50 ms steps: pf bottoms near
// 0.47 around t=460 s, sweeps through the rated cap near t=1000 s as
// the dilution overswing is borated back, and settles inside the band
// by t~1220 s.
func TestRPC_ReactorLeadDescentTo90PercentHoldsPlateau(t *testing.T) {
	// The descent ramps at the entered 0.02 %/s over ten minutes
	// under the demanded-setpoint rate limit
	// (specs/pwr_sim/chapter_2/page_024.txt:3-12), and the
	// integrator scales linearly with dt, so the widened step still
	// lands exactly on target while cutting step count 20x. Must
	// not run in parallel: overrideTimeStep mutates global
	// simulation state.
	overrideTimeStep(t, 50*time.Millisecond)
	p := rpcReactorLeadPlant()

	setControl(t, &p, "PlantOperation/LeadLagMode", "1")
	setControl(t, &p, "PlantOperation/PowerSetpointTarget", "90")
	setControl(t, &p, "PlantOperation/PowerRateDemand", "0.02")

	rpv := &p.ReactorBuilding.ReactorPressureVessel

	// Engage sequence: the integrator first shadows achieved power
	// (auto off), then the operator flips auto on - the bumpless
	// handoff that makes the demanded setpoint start at 100 % so
	// the entered rate covers the 10-point descent.
	//
	// The entered rate respects the plant's Tavg pacing: outlet
	// temperature moves at most 50 F/hr (OpHeatupRateLimit), so a
	// Tref program steeper than ~1.3 %/min outruns the coolant and
	// pins the deadband open. 0.02 %/s = 1.2 %/min sits inside that
	// envelope; the evolution's 0.08 %/s figure remains covered by
	// the integrator rate-clamp test above.
	const (
		shadowSeconds   = 5.0
		descentSeconds  = 530.0 // 0.10 / 0.0002 = 500 s plus margin
		convergeSeconds = 700.0 // measured band entry near t~1220 s total
		holdSeconds     = 120.0 // envelope window after first band entry
	)
	checkEvery := int(200 * time.Millisecond / TimeStep)

	for range int(shadowSeconds / TimeStep.Seconds()) {
		p.Step()
		p.Update()
	}
	// Anchor to the fixture's own achieved power, not a literal
	// rated figure: the 600 ppm lineup parks slightly off the cap,
	// and the bumpless handoff promise is "integrator == measured",
	// not "integrator == 1.0".
	assert.InDelta(t, fissionFraction(rpv.FissionPower),
		p.PlantOperation.DemandedPowerSetpoint, 1e-6,
		"shadow phase parked the integrator at achieved power")
	setControl(t, &p, "PlantOperation/RodControlAuto", "true")

	sampleGuards := func(stage string) {
		assert.False(t, rpv.Trip, "reactor must not trip during %s", stage)
		assert.Greater(t, float64(p.ReactorBuilding.SteamGenerator.Pressure),
			float64(RPSLowSteamlinePressureTrip),
			"SG pressure must stay above the low-steamline trip during %s",
			stage)
	}

	for range int(descentSeconds / TimeStep.Seconds()) {
		p.Step()
		p.Update()
	}
	sampleGuards("descent")

	// Convergence is an underdamped two-time-scale transient (fast
	// rod/thermal loop around slow chemistry), so a fixed-horizon
	// endpoint would be phase-dependent sampling; poll for first band
	// entry instead, then hold the envelope across a following window
	// per the fixed-horizon honesty rule.
	enteredAt := -1
	for i := range int(convergeSeconds / TimeStep.Seconds()) {
		p.Step()
		p.Update()
		if i%checkEvery != 0 {
			continue
		}
		sampleGuards("converge")
		pfNow := fissionFraction(rpv.FissionPower)
		if enteredAt < 0 && absF(pfNow-0.90) <= 0.02 {
			enteredAt = i
		}
	}
	require.GreaterOrEqual(t, enteredAt, 0,
		"fission power must enter the plateau band within the horizon")

	for i := range int(holdSeconds / TimeStep.Seconds()) {
		p.Step()
		p.Update()
		if i%checkEvery == 0 {
			sampleGuards("hold")
			pfHold := fissionFraction(rpv.FissionPower)
			assert.LessOrEqual(t, absF(pfHold-0.90), 0.04,
				"plateau envelope held through the settling window")
		}
	}

	po := &p.PlantOperation
	pf := fissionFraction(rpv.FissionPower)

	assert.InDelta(t, 0.90, po.DemandedPowerSetpoint, 1e-9,
		"integrator reached the entered target")
	assert.InDelta(t, 0.90, pf, 0.02,
		"fission power settled within the plateau band")
	assert.LessOrEqual(t, absF(rpv.Outlet.Float()-po.referenceTempF()),
		RodControlDeadbandF,
		"outlet temperature held inside the Tref deadband")
	assert.False(t, rpv.Trip)

	turbine := &p.TurbineBuilding.Turbine
	assert.InDelta(t, po.DemandedPowerSetpoint, turbine.Setpoint, 1e-9,
		"reactor lead: turbine load setpoint tracks the walked demand")
	assert.InDelta(t, 0.90, float64(turbine.RealPower)/float64(TurbineRatedkW),
		0.05, "generator settled near 0.9 x rated electrical")
	// Thermal output (fission + decay pools) left the rated cap
	// and sits near the 0.90 plateau: fission ~0.9 x 0.935 x rated
	// plus the decay pools seeded at full power.
	total := float64(rpv.TotalThermalPower())
	assert.Less(t, total, float64(CoreRatedThermalPower),
		"thermal output unpinned from the rated cap")
	assert.GreaterOrEqual(t, total/float64(CoreRatedThermalPower), 0.85,
		"thermal output landed near the 0.90 plateau")

	// The maneuver's slow reactivity compensation lives in chemistry:
	// the trim diluted into the transient and the bank stayed out of
	// the buried regime throughout (rods keep reserve capability,
	// chapter_16/page_231.txt:115-118).
	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.Less(t, cvcs.BoronConcentration, 600.0,
		"automatic trim diluted against the walked deficit")
	assert.LessOrEqual(t, rpv.Insertion, CVCSAutoBoronHighInsertionFrac,
		"rod bank never settled buried past its maneuvering window")
	assert.Zero(t, turbine.DumpValve.Open,
		"reactor leads the steam drop, so no rejection dump")
}

// Zero-value RPC state (turbine lead, auto off) leaves rod demand
// untouched: every pre-RPC behavior and scenario artifact survives.
func TestRPC_DefaultStateLeavesRodsManual(t *testing.T) {
	rpv := &ReactorPressureVessel{}
	rpv.ControlRods.DemandInsertion = 0.42
	turbine := &Turbine{}
	turbine.Setpoint = 0.77

	po := PlantOperation{}
	po.StepRPC(rpv, turbine, TimeStep)

	assert.Equal(t, 0.42, rpv.ControlRods.DemandInsertion,
		"manual rod authority untouched with auto off")
	assert.Equal(t, 0.77, turbine.Setpoint,
		"manual turbine setpoint untouched in turbine lead")
	assert.InDelta(t, 0, po.DemandedPowerSetpoint, 1e-12,
		"integrator shadows measured power while disengaged")
}

// Engaging auto mid-run introduces no reactivity step: the
// integrator initialized from measured power sits exactly where the
// Tref program wants the outlet, so the first engaged tick leaves
// the rods alone. The deadband direction checks then pin regions B
// and D.
func TestRPC_BumplessEngagementAndDeadbandDirection(t *testing.T) {
	newRPV := func() *ReactorPressureVessel {
		rpv := &ReactorPressureVessel{
			FissionPower: Watts(0.9 * float64(CoreFissionPowerAtRated)),
		}
		rpv.Outlet = Temperature{
			Val: 558 + 63*0.9, Unit: u.Fahrenheit,
		}
		return rpv
	}

	programTemp := 558.0 + 63*0.9

	// Disengaged shadow: demand follows measured power, nothing
	// else moves.
	rpv := newRPV()
	rpv.ControlRods.DemandInsertion = 0.30
	turbine := &Turbine{}
	po := PlantOperation{}
	po.StepRPC(rpv, turbine, TimeStep)
	assert.InDelta(t, 0.9, po.DemandedPowerSetpoint, 1e-12)

	// Engage at the matching target: first tick is a no-op.
	po.RodControlAuto = true
	po.LeadLagMode = ReactorLead
	po.PowerSetpointDemand = 0.90
	po.PowerRateDemand = 0.0008
	before := rpv.ControlRods.DemandInsertion
	po.StepRPC(rpv, turbine, TimeStep)
	assert.Equal(t, before, rpv.ControlRods.DemandInsertion,
		"engaging auto at matched state moves no rods")
	assert.InDelta(t, 0.9, po.DemandedPowerSetpoint, 1e-12,
		"bumpless: integrator started from measured power")

	// Region B: raising the target walks Tref above Tavg, and
	// past the deadband the drive withdraws rods.
	po.PowerSetpointDemand = 0.95
	for range 22000 {
		po.StepRPC(rpv, turbine, TimeStep)
	}
	assert.Less(t, rpv.ControlRods.DemandInsertion, before,
		"Tavg below Tref beyond the deadband withdraws rods")

	// Region D: Tavg above Tref beyond the deadband inserts.
	rpv = newRPV()
	rpv.Outlet = Temperature{Val: programTemp + 10, Unit: u.Fahrenheit}
	rpv.ControlRods.DemandInsertion = 0.30
	po = PlantOperation{
		RodControlAuto:        true,
		LeadLagMode:           ReactorLead,
		PowerSetpointDemand:   0.90,
		DemandedPowerSetpoint: 0.90,
	}
	po.StepRPC(rpv, turbine, TimeStep)
	assert.Greater(t, rpv.ControlRods.DemandInsertion, 0.30,
		"Tavg above Tref beyond the deadband inserts rods")
}

// The four controls and three sensors resolve through the bridge
// maps; enum values round-trip through ConvertControlValue.
func TestRPC_BridgeControlsAndSensorsRoundTrip(t *testing.T) {
	p := MakePlant()
	cm := p.GetControlMap()
	for _, path := range []string{
		"PlantOperation/LeadLagMode",
		"PlantOperation/RodControlAuto",
		"PlantOperation/PowerSetpointTarget",
		"PlantOperation/PowerRateDemand",
	} {
		require.Contains(t, cm, path)
	}
	sm := p.GetSensorMap()
	for _, path := range []string{
		"PlantOperation/DemandedPowerSetpoint",
		"PlantOperation/Tref",
		"PlantOperation/PowerError",
	} {
		require.Contains(t, sm, path)
	}

	for _, raw := range []string{"0", "1"} {
		v, err := bridge.ConvertControlValue("enum", raw)
		require.NoError(t, err)
		require.NoError(t, cm["PlantOperation/LeadLagMode"].Set(v))
		assert.Equal(t, raw, cm["PlantOperation/LeadLagMode"].Get())
	}
	assert.Equal(t, ReactorLead, p.PlantOperation.LeadLagMode)

	require.NoError(t, cm["PlantOperation/PowerSetpointTarget"].Set("90"))
	assert.InDelta(t, 0.90, p.PlantOperation.PowerSetpointDemand, 1e-12)
	assert.Equal(t, "90", cm["PlantOperation/PowerSetpointTarget"].Get())

	require.NoError(t, cm["PlantOperation/PowerRateDemand"].Set("0.08"))
	assert.InDelta(t, 0.0008, p.PlantOperation.PowerRateDemand, 1e-15)
	assert.Equal(t, "0.08", cm["PlantOperation/PowerRateDemand"].Get())

	require.NoError(t, cm["PlantOperation/RodControlAuto"].Set("true"))
	assert.True(t, p.PlantOperation.RodControlAuto)

	val, err := sm["PlantOperation/Tref"].Get()
	require.NoError(t, err)
	tref, err := strconv.ParseFloat(val, 64)
	require.NoError(t, err)
	assert.InDelta(t, CoreReferenceInletF, tref, 1e-9,
		"Tref reads the no-load anchor at zero demanded setpoint")

	val, err = sm["PlantOperation/DemandedPowerSetpoint"].Get()
	require.NoError(t, err)
	dsp, err := strconv.ParseFloat(val, 64)
	require.NoError(t, err)
	assert.InDelta(t, 0, dsp, 1e-12)

	val, err = sm["PlantOperation/PowerError"].Get()
	require.NoError(t, err)
	perr, err := strconv.ParseFloat(val, 64)
	require.NoError(t, err)
	// Wiring check: expected recomputed from the control's own
	// convention (demanded minus achieved, the comparison page_135
	// keys the boron mode on), independently of the closure's
	// subtraction order, so the sign is pinned too.
	wantErr := p.PlantOperation.boronDemandPower(
		p.TurbineBuilding.Turbine.Setpoint) -
		fissionFraction(p.ReactorBuilding.ReactorPressureVessel.FissionPower)
	assert.InDelta(t, wantErr, perr, 5e-5,
		"power error reads demanded minus achieved power")
}

// The displayed PowerError must carry the sign the auto-boron tier
// acts on: commanding house load against rated fission - turbine lead,
// so the demand source is the setpoint itself - reads negative, the
// boration direction. specs/the_westinghouse_pressurized_water_react/
// chapter_7/page_135.txt:58-62.
func TestPowerErrorSensor_SignMatchesControlConvention(t *testing.T) {
	p := fullPowerLineup()
	p.PlantOperation.LeadLagMode = TurbineLead
	p.TurbineBuilding.Turbine.Setpoint = 0.05
	sm := p.GetSensorMap()

	val, err := sm["PlantOperation/PowerError"].Get()
	require.NoError(t, err)
	perr, err := strconv.ParseFloat(val, 64)
	require.NoError(t, err)

	want := p.PlantOperation.boronDemandPower(0.05) -
		fissionFraction(p.ReactorBuilding.ReactorPressureVessel.FissionPower)
	assert.Less(t, want, -0.5, "fixture must command a real shortfall")
	assert.InDelta(t, want, perr, 1e-9,
		"displayed error must equal demanded minus achieved, got %v", perr)
}

func absF(v float64) float64 { return max(v, -v) }

// The LeadLagMode selector is registered with Kind "enum" whose words
// are "TurbineLead"/"ReactorLead", so the Set path must accept its own
// declared vocabulary (and keep accepting the numeric form the
// RPC tests use). Regression: the word forms errored out, which made
// the mode unreachable from the web console and command files.
func TestBridge_LeadLagModeAcceptsEnumWords(t *testing.T) {
	p := MakePlant()
	cm := GetControlMap(&p)
	c := cm["PlantOperation/LeadLagMode"]
	require.NotNil(t, c, "want LeadLagMode control registered")

	require.NoError(t, c.Set("ReactorLead"))
	assert.Equal(t, ReactorLead, p.PlantOperation.LeadLagMode,
		"the declared enum word must select reactor lead")
	require.NoError(t, c.Set("TurbineLead"))
	assert.Equal(t, TurbineLead, p.PlantOperation.LeadLagMode,
		"the declared enum word must select turbine lead")
	require.NoError(t, c.Set("1"))
	assert.Equal(t, ReactorLead, p.PlantOperation.LeadLagMode,
		"the numeric form stays accepted")
}

// The part-load secondary line must settle on the Tavg program the
// rod control, CVCS level, and steam-dump programs all share: with
// fission pinned at a load fraction and the pumps at rated speed, the
// converged (inlet+outlet)/2 sits on programmedRodTavg. Regression:
// blending the cold leg toward the hot leg collapsed the loop ΔT
// quadratically with load and parked Tavg up to tens of F above the
// program away from full power.
func TestRPV_PartLoadLineTracksTavgProgram(t *testing.T) {
	for _, load := range []float64{0.35, 0.5, 0.7, 1.0} {
		var rpv ReactorPressureVessel
		p := Watts(load * float64(CoreRatedThermalPower))
		rpv.SeedSteadyStatePower(p)
		fissionW := rpv.FissionPower
		rpv.Inlet = Temperature{Val: RCSNoLoadTavgF, Unit: u.Fahrenheit}
		rpv.Outlet = Temperature{Val: RCSNoLoadTavgF + load*
			(CoreReferenceOutletF-RCSNoLoadTavgF), Unit: u.Fahrenheit}
		for i := 0; i < int(900*time.Second/TimeStep); i++ {
			rpv = rpv.Step(RCPRatedSpeed, 0)
			// Pin the neutron population: this test exercises the
			// thermal program, not the kinetics.
			rpv.FissionPower = fissionW
		}
		tavg := (rpv.Inlet.Float() + rpv.Outlet.Float()) / 2
		assert.InDelta(t, programmedRodTavg(load), tavg, 2.0,
			"load %.2f: settled Tavg must sit on the shared program",
			load)
	}
}

// The measured-Tavg input carries real authority in the outer loop: a
// hot atmosphere trims the power target down (insertion), a cold one
// trims it up (withdrawal), within the trim's bounded span.
// "The needed control rod movement is based on the reactor coolant
//  temperature needed to meet the turbine power demand [...] and the
//  measured reactor coolant temperature."
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-76
func TestRodControl_MeasuredTavgTrimsPowerTarget(t *testing.T) {
	newRods := func() ControlRods {
		return ControlRods{
			DemandInsertion:       0.4,
			programPrimed:         true,
			programTargetFraction: 0.5,
		}
	}
	tavgAt := func(offset float64) Temperature {
		return Temperature{Val: programmedRodTavg(0.5) + offset,
			Unit: u.Fahrenheit}
	}
	steps := int(30 * time.Second / TimeStep)
	runCase := func(offset float64) float64 {
		c := newRods()
		for i := 0; i < steps; i++ {
			c.applyAutoControl(tavgAt(offset), 0.5, 0.5, 0)
		}
		return c.DemandInsertion
	}
	hot := runCase(+RodCtrlTavgTrimSpanF)
	cold := runCase(-RodCtrlTavgTrimSpanF)

	assert.Greater(t, hot, 0.4,
		"a hot residual must drive insertion")
	assert.Less(t, cold, 0.4,
		"a cold residual must drive withdrawal")
	// Rate honesty: neither direction may outrun the demand slew cap
	// over the simulated window (trim acts through the same chain).
	assert.LessOrEqual(t, absF(hot-0.4),
		RodCtrlMaxDemandSlewPerSec*30.0+1e-9,
		"hot trim slew stays inside the demand rate cap")
}

// The DemandedPowerSetpoint panel must show the demand in both lead
// modes: under turbine lead that is the turbine's own setpoint, not
// the shadowed integrator (which equals achieved power and would hide
// any commanded change). specs/the_westinghouse_pressurized_water_react/
// chapter_7/page_130.txt:69-71.
func TestDemandedPowerSetpointSensor_ReadsTurbineDemandInTurbineLead(t *testing.T) {
	p := fullPowerLineup()
	p.PlantOperation.LeadLagMode = TurbineLead
	p.TurbineBuilding.Turbine.Setpoint = 0.05
	sm := p.GetSensorMap()

	val, err := sm["PlantOperation/DemandedPowerSetpoint"].Get()
	require.NoError(t, err)
	demand, err := strconv.ParseFloat(val, 64)
	require.NoError(t, err)
	assert.InDelta(t, 0.05, demand, 1e-9,
		"turbine lead must display the turbine demand, got %v", demand)
}
