package core

import (
	"strconv"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SGTR tests (gap G2, plans/p60_1.txt): the tube rupture leaks across
// the tube-sheet differential, terminates on depressurization below
// the shell pressure, and drives the steam-line radiation monitor
// into the auto-isolate latch.
//
// "Tube Sheet (thick plate separating primary/secondary at bottom)"
// specs/the_westinghouse_pressurized_water_react/imgs/023.md:4

func sgtrFixture() (SteamGenerator, *ReactorPressureVessel, *Pressurizer) {
	sg := SteamGenerator{
		Water:    Volume{Val: 40000, Unit: u.Gallon},
		Volume:   Volume{Val: 80000, Unit: u.Gallon},
		Pressure: SteamGeneratorShellPressure,
		HeatInput: SteamGeneratorFullLoadHeatInput,
	}
	rpv := &ReactorPressureVessel{breakRemaining: RCSDrainableInventoryGal}
	pzr := &Pressurizer{
		Water:       Volume{Val: 1000, Unit: u.Gallon},
		Volume:      Volume{Val: 1500, Unit: u.Gallon},
		SteamBubble: Volume{Val: 500, Unit: u.Gallon},
		Pressure:    PressurizerHeaterTarget,
	}
	return sg, rpv, pzr
}

func TestSteamGenerator_TubeRuptureFlowsWithDifferential(t *testing.T) {
	t.Parallel()
	// Wiring check (CHECK §8): the 150 gpm design rate is an invented
	// modeling convention (derived, not spec-quoted); this test pins
	// the plumbing — flow scales with Open and with the differential —
	// not the magnitude itself.
	sg, rpv, pzr := sgtrFixture()
	sg.SGTubeRupture.Open = 1
	out := sg.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv, pzr)
	gpm := out.SGTubeRuptureFlow.Float() / DefaultTimeStep.Seconds() * 60
	assert.InDelta(t, SGTubeRuptureGalPerSec*60, gpm, 1.0,
		"full-open rupture at design differential must present the design rate, got %v", gpm)

	sg2, rpv2, pzr2 := sgtrFixture()
	sg2.SGTubeRupture.Open = 0.5
	half := sg2.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv2, pzr2)
	gpmHalf := half.SGTubeRuptureFlow.Float() / DefaultTimeStep.Seconds() * 60
	assert.InDelta(t, 0.5*gpm, gpmHalf, 0.5,
		"half-open rupture must present half the full-open rate, got %v", gpmHalf)

	// Ledger closure: the per-step flow equals the BreakDischarged
	// increase, and (net of the simultaneous boil-off debit, which
	// is the only other water change at zero offtake) the shell
	// water gain.
	assert.InDelta(t, out.SGTubeRuptureFlow.Float(), rpv.BreakDischarged.Float(), 1e-9,
		"BreakDischarged increase must equal this step's leak flow")
	boil := float64(SteamGeneratorFullLoadHeatInput) /
		float64(LatentHeatOfSteamization) / 1000.0 * GallonsPerCubicMeter
	grossGain := out.Water.Float() - sg.Water.Float() + boil*DefaultTimeStep.Seconds()
	assert.InDelta(t, out.SGTubeRuptureFlow.Float(), grossGain, 1e-9,
		"shell water gross gain (before boil-off) must equal the leaked gallons")

	// Multi-step closure over a sustained run: integrating
	// SGTubeRuptureFlow over the run equals the BreakDischarged
	// increase exactly (acceptance criterion 4, run form).
	sg3, rpv3, pzr3 := sgtrFixture()
	sg3.SGTubeRupture.Open = 1
	var cur = sg3
	var integrated float64
	for i := 0; i < int(30*time.Second/DefaultTimeStep); i++ {
		cur = cur.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv3, pzr3)
		integrated += cur.SGTubeRuptureFlow.Float()
	}
	assert.InDelta(t, rpv3.BreakDischarged.Float(), integrated, 1e-9,
		"integrated leak over the run must equal the BreakDischarged increase")

	// Intact tubes: no flow, no debit.
	sg4, rpv4, pzr4 := sgtrFixture()
	closed := sg4.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv4, pzr4)
	assert.Zero(t, closed.SGTubeRuptureFlow.Float())
	assert.Zero(t, rpv4.BreakDischarged.Float())
}

func TestSteamGenerator_TubeRuptureStopsBelowShell(t *testing.T) {
	t.Parallel()
	sg, rpv, pzr := sgtrFixture()
	sg.SGTubeRupture.Open = 1
	// RCS at the shell pressure: differential is zero, flow stops.
	atShell := sg.Step(DefaultTimeStep, 0, SteamGeneratorShellPressure, rpv, pzr)
	assert.Zero(t, atShell.SGTubeRuptureFlow.Float(),
		"leak must stop at zero differential")
	// RCS below the shell: also zero (termination).
	below := sg.Step(DefaultTimeStep, 0, SteamGeneratorShellPressure-100, rpv, pzr)
	assert.Zero(t, below.SGTubeRuptureFlow.Float(),
		"leak must stop below shell pressure")

	// Termination after a live leak: seed the monitor hot, stop the
	// flow, and the latch must release once the signal decays.
	live, rpv2, pzr2 := sgtrFixture()
	live.SGTubeRupture.Open = 1
	live.SteamlineRadiation = SGTubeRuptureMonitorTripFrac + 0.04
	live.SteamlineIsolated = true
	dead := live.Step(DefaultTimeStep, 0, SteamGeneratorShellPressure, rpv2, pzr2)
	assert.Zero(t, dead.SGTubeRuptureFlow.Float())
	// One step of decay is not enough to release; run it out.
	cur := dead
	for i := 0; i < int(300*time.Second/DefaultTimeStep) && cur.SteamlineIsolated; i++ {
		cur = cur.Step(DefaultTimeStep, 0, SteamGeneratorShellPressure, rpv2, pzr2)
	}
	assert.False(t, cur.SteamlineIsolated,
		"latch must release after depressurization decays the monitor")
	assert.Less(t, cur.SteamlineRadiation, SGTubeRuptureMonitorReleaseFrac)
}

func TestSteamGenerator_RadiationMonitorIsolatesMSIV(t *testing.T) {
	t.Parallel()
	sg, rpv, pzr := sgtrFixture()
	sg.SGTubeRupture.Open = 1
	// Full-open leak at the design differential, stepped at the
	// production tick. With tau 20 s, the signal crosses the 0.95
	// trip fraction within ~3 tau (60 s) of simulated time.
	var cur = sg
	var isolatedAt time.Duration
	const tick = DefaultTimeStep
	steps := int(90 * time.Second / tick)
	for i := 0; i < steps; i++ {
		cur = cur.Step(tick, 0, PressurizerHeaterTarget, rpv, pzr)
		if cur.SteamlineIsolated {
			isolatedAt = time.Duration(i+1) * tick
			break
		}
	}
	require.NotZero(t, isolatedAt, "monitor must trip under a full-open leak")
	assert.LessOrEqual(t, isolatedAt, 60*time.Second,
		"latch must trip within ~3 tau of the leak opening, took %v", isolatedAt)
	assert.GreaterOrEqual(t, cur.SteamlineRadiation, SGTubeRuptureMonitorTripFrac)

	// Plant-level: the latched SG closes the MSIV and secures main
	// feedwater through the existing fwi path.
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.SteamlineIsolated = true
	// Keep the monkey signal high so applyRadiationMonitors does not
	// release the latch (release needs signal < ReleaseFrac with no flow).
	p.ReactorBuilding.SteamGenerator.SteamlineRadiation = 1.0
	p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = 1
	p.Step(DefaultTimeStep)
	p.Update()
	require.True(t, p.ReactorBuilding.SteamGenerator.SteamlineIsolated,
		"precondition: latch must survive the step")
	assert.Equal(t, 0.0,
		p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open,
		"latched SGTR isolate must close the MSIV")
}

// TestScenario_sgtuberupture exercises the catalog entry end to end:
// the scripted insert drives detection through the auto-isolate latch,
// and termination follows when RCS pressure is walked below the shell
// pressure. Short by design: the full cooldown takes tens of minutes
// simulated; here the depressurization leg is seeded directly and only
// the detection→isolation→termination chain is walked.
func TestScenario_sgtuberupture(t *testing.T) {
	t.Parallel()
	s, ok := ScenarioByID("sg-tube-rupture")
	require.True(t, ok)
	p := MakePlant()
	s.Apply(&p)

	// Fire the scripted insert through the bridge grammar, exactly as
	// the production runner does.
	script := s.Script
	cm := GetControlMap(&p)
	ApplyScript(&script, int64(scenarioTriggerDelayMs)*int64(time.Millisecond), cm)
	require.Equal(t, 1.0, p.ReactorBuilding.SteamGenerator.SGTubeRupture.Open,
		"script must insert the rupture via the registered control")

	// Step through the detection window: the 20 s tau monitor must
	// trip and latch the auto-isolate well within 60 s of simulated
	// time, closing the MSIV.
	latched := false
	for i := 0; i < int(70*time.Second/DefaultTimeStep); i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		if p.ReactorBuilding.SteamGenerator.SteamlineIsolated {
			latched = true
			break
		}
	}
	require.True(t, latched, "steam-line monitor must latch auto-isolate")
	assert.Equal(t, 0.0, p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open,
		"latched isolate must close the MSIV")

	// Depressurize: force RCS pressure below the shell pressure and
	// verify the leak terminates and the scenario ends done.
	p.ReactorBuilding.Pressurizer.Pressure = SteamGeneratorShellPressure - 100
	terminated := false
	for i := 0; i < int(10*time.Second/DefaultTimeStep); i++ {
		p.Step(DefaultTimeStep)
		p.Update()
		if p.ReactorBuilding.SteamGenerator.SGTubeRuptureFlow.Float() == 0 {
			terminated = true
			break
		}
		// Hold the depressurized RCS: heaters would otherwise pull
		// pressure back up above the shell line.
		p.ReactorBuilding.Pressurizer.Pressure = SteamGeneratorShellPressure - 100
	}
	require.True(t, terminated, "leak must terminate once RCS sits below shell pressure")
	res := s.Check(&p)
	assert.True(t, res.Done, "scenario must report done after termination: %s", res.Detail)
}

func TestSteamGenerator_TubeRuptureExhaustsInventory(t *testing.T) {
	t.Parallel()
	// Exhaustion regression (CHECK §2): the cumulative SGTR debit is
	// capped at RCSDrainableInventoryGal, and the flow reaches exactly
	// zero once the ledger is spent, even with the rupture still open.
	sg, rpv, pzr := sgtrFixture()
	sg.SGTubeRupture.Open = 1
	pzr.Water = Volume{Val: RCSDrainableInventoryGal + 1, Unit: u.Gallon}
	pzr.Volume = Volume{Val: RCSDrainableInventoryGal + 2000, Unit: u.Gallon}
	pzr.SteamBubble = Volume{Val: 1000, Unit: u.Gallon}
	var cur = sg
	var total float64
	steps := int(120*time.Second/DefaultTimeStep)
	for i := 0; i < steps; i++ {
		cur = cur.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv, pzr)
		total += cur.SGTubeRuptureFlow.Float()
	}
	assert.LessOrEqual(t, rpv.BreakDischarged.Float(), RCSDrainableInventoryGal+1e-9,
		"cumulative SGTR debit must not exceed the drainable inventory")
	assert.InDelta(t, rpv.BreakDischarged.Float(), total, 1e-9,
		"integrated SGTubeRuptureFlow must equal the BreakDischarged increase")
	// The per-step draw (2.5 gal/s over 120 s = 300 gal) is far below
	// the cap, so exercise exhaustion directly: seat the ledger just
	// under the cap and step until the last gallons are spent.
	sg2, rpv2, pzr2 := sgtrFixture()
	sg2.SGTubeRupture.Open = 1
	rpv2.BreakDischarged = Volume{Val: RCSDrainableInventoryGal - 0.001, Unit: u.Gallon}
	rpv2.breakRemaining = 0.001
	cur2 := sg2.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv2, pzr2)
	assert.InDelta(t, 0.001, cur2.SGTubeRuptureFlow.Float(), 1e-9,
		"final step must be capped at the remaining inventory")
	cur2 = cur2.Step(DefaultTimeStep, 0, PressurizerHeaterTarget, rpv2, pzr2)
	assert.Zero(t, cur2.SGTubeRuptureFlow.Float(),
		"leak must reach exactly zero once the ledger is exhausted")
}

func TestSGTubeRupture_SensorsRegisteredAndRead(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	m := GetSensorMap(&p)
	for _, path := range []string{
		"ReactorBuilding/SteamGenerator/SteamlineRadiation",
		"TurbineBuilding/Condenser/HotwellRadiation",
	} {
		s, ok := m[path]
		require.True(t, ok, "sensor %q must be registered", path)
		out, err := s.Get()
		require.NoError(t, err)
		got, err := strconv.ParseFloat(out, 64)
		require.NoError(t, err, "sensor emitted %q", out)
		assert.Equal(t, 0.0, got, "%s must read 0 on the intact plant", path)
	}

	// With the leak open the steam-line monitor must read > 0.
	p.ReactorBuilding.SteamGenerator.SGTubeRupture.Open = 1
	p.ReactorBuilding.SteamGenerator.SteamlineRadiation = 0.5
	p.TurbineBuilding.Condenser.HotwellRadiation = 0.25
	out, err := m["ReactorBuilding/SteamGenerator/SteamlineRadiation"].Get()
	require.NoError(t, err)
	got, err := strconv.ParseFloat(out, 64)
	require.NoError(t, err)
	assert.Greater(t, got, 0.0, "steam-line monitor must read > 0 with the leak open")
	out, err = m["TurbineBuilding/Condenser/HotwellRadiation"].Get()
	require.NoError(t, err)
	got, err = strconv.ParseFloat(out, 64)
	require.NoError(t, err)
	assert.Greater(t, got, 0.0, "effluent monitor must read > 0 with the leak open")

	// Control round-trips through the bridge grammar.
	cm := GetControlMap(&p)
	ctrl, ok := cm["ReactorBuilding/SteamGenerator/SGTubeRupture/Open"]
	require.True(t, ok, "SGTubeRupture control must be registered")
	require.NoError(t, ctrl.Set("1"))
	assert.Contains(t, ctrl.Get(), "1")
}
