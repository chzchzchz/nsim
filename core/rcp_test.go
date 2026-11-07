package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRCP_Startup verifies the exponential startup ramp brings the
// RCP from zero to ≥ 90 % of rated speed within ~5 s (the
// RCPStartupTimeConstant is 2 s, so 5 s is ≈ 2.5 τ).
//
// Spec justification:
//   - "A flywheel in the motor increases the total rotating
//     inertia, allowing the pump to coastdown slowly after
//     deenergizing."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_061.txt:152-158
//   - "Operating speed 1189 rpm"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:175-177
func TestRCP_Startup(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	stepN(&p, int(5*time.Second/DefaultTimeStep), DefaultTimeStep)
	assert.GreaterOrEqual(t, float64(p.ReactorBuilding.ReactorCoolantPump.Velocity),
		0.9*float64(RCPRatedSpeed),
		"want loop-average RCP velocity >= 0.9 * RCPRatedSpeed after 5 s startup, got %v",
		p.ReactorBuilding.ReactorCoolantPump.Velocity)
}

// TestRCP_SequentialStartStagesFlow verifies that starting the loop
// pumps one at a time stages the loop-average flow: N of four rotors
// at rated put the lumped telemetry near N/4 of rated speed. This is
// what makes the sequential-start evolution observable.
//
// Spec justification:
//   - "Reactor coolant pumps are started sequentially and pump heat
//     is used to raise the RCS temperature."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37
//   - "a maximum of two out of four reactor coolant pumps (RCPs)
//     may be secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-108
func TestRCP_SequentialStartStagesFlow(t *testing.T) {
	t.Parallel()
	// simulation state (same discipline as TestRCP_Coastdown).
	testDt := 10 * time.Millisecond
	p := MakePlant()
	// Cold start: the MakePlant full-power seed spins the rotors at
	// rated speed, so secure them to the stopped state this
	// sequential-start evolution begins from.
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = false
		pu.Velocity = 0
	}
	for stage := 1; stage <= 4; stage++ {
		p.ReactorBuilding.ReactorCoolantPumps.All()[stage-1].Running = true
		want := float64(stage) / 4 * float64(RCPRatedSpeed)
		// Poll to the staged plateau: the exponential ramp passes
		// 95 percent of its asymptote after ~3 tau (6 s sim).
		reached := false
		for i := 0; i < int(12*time.Second/testDt) && !reached; i++ {
			p.Step(testDt)
			p.Update()
			reached = float64(p.ReactorBuilding.ReactorCoolantPump.Velocity) >=
				0.95*want
		}
		require.Truef(t, reached,
			"stage %d: want loop-average speed >= %.0f rpm, got %.0f",
			stage, 0.95*want, p.ReactorBuilding.ReactorCoolantPump.Velocity)
		assert.LessOrEqualf(t,
			float64(p.ReactorBuilding.ReactorCoolantPump.Velocity), want*1.001,
			"stage %d: the mean must sit on the N/4 plateau", stage)
	}
	assert.Equal(t, 4, p.ReactorBuilding.ReactorCoolantPumps.RunningCount(),
		"all four loop pumps carry start commands after the sequence")
}

// The RCP DriveReady latch must survive the ReactorBuilding step
// assignment inside Plant.Step so the sensor tracks the ESF bus
// state that feeds the pump motors.
//
// Spec justification:
//   - "All of the engineered safety system loads are fed from two
//     6600-volt engineered safety features buses."
//   - specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
func TestPlant_RCPDriveReadyFollowsESFBreaker(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	stepN(&p, 10, DefaultTimeStep)
	require.False(t, p.ReactorBuilding.ReactorCoolantPump.DriveReady,
		"precondition: ESF breakers open, want DriveReady=false")
	require.False(t, p.ReactorBuilding.ReactorCoolantPumps.LoopA.DriveReady,
		"precondition: the DriveReady latch feeds every loop pump")

	p.Electrical.ESFBusA.BreakerClosed = true
	p.Electrical.ESFBusB.BreakerClosed = true
	stepN(&p, 10, DefaultTimeStep)
	assert.True(t, p.ReactorBuilding.ReactorCoolantPump.DriveReady,
		"want DriveReady=true with the ESF buses energized")
}

// TestRCP_Coastdown verifies the exponential coastdown from rated
// speed decays toward zero over the RCPCoastdownTimeConstant (10 s);
// after 50 s (≈ 5 τ) velocity is < 1 % of rated.
//
// Spec justification:
//   - "A heavy flywheel on the pump motor shaft provides long
//     coastdown times to preclude rapid decreases in core cooling
//     flow during pump trips."
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_020.txt:83-84
func TestRCP_Coastdown(t *testing.T) {
	t.Parallel()
	// The pump lag integrates v -= v*dt/tau with tau = 10 s
	// (RCPCoastdownTimeConstant, sized from the flywheel's "long
	// coastdown" description,
	// specs/the_westinghouse_pressurized_water_react/chapter_1/page_020.txt:83-84).
	// At this tick width dt/tau is 1e-3 per step, shifting the
	// discrete decay's effective tau by about 0.05 percent, orders
	// of magnitude inside the 1-percent-of-rated crossing the test
	// global simulation state.
	testDt := 10 * time.Millisecond
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	// Poll to rated speed with a cap; the ramp is monotone so the
	// exit comes as soon as the precondition holds.
	reached := false
	for i := 0; i < int(20*time.Second/testDt); i++ {
		p.Step(testDt)
		p.Update()
		if float64(p.ReactorBuilding.ReactorCoolantPump.Velocity) >=
			0.99*float64(RCPRatedSpeed) {
			reached = true
			break
		}
	}
	require.True(t, reached,
		"precondition: want RCP at rated speed before coastdown, got %v",
		p.ReactorBuilding.ReactorCoolantPump.Velocity)
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(false)
	// The flywheel coastdown is long by design (chapter 16 relies on
	// it for flow during the power decay transient), so poll for the
	// sub-1% crossing instead of stepping a fixed window past it.
	const maxCoast = 50 * time.Second
	crossed := false
	for i := 0; i < int(maxCoast/testDt); i++ {
		p.Step(testDt)
		p.Update()
		if float64(p.ReactorBuilding.ReactorCoolantPump.Velocity) <
			0.01*float64(RCPRatedSpeed) {
			crossed = true
			break
		}
	}
	assert.True(t, crossed,
		"want RCP velocity < 1%% of rated within %v coastdown, got %v (rated=%v)",
		maxCoast, p.ReactorBuilding.ReactorCoolantPump.Velocity, RCPRatedSpeed)
}

// TestRCP_HeatInputScalesWithSpeed verifies the per-step
// PumpHeatInput follows the ω² relationship: at half rated speed
// it equals ≈ 25 % of RCPRatedHeatInput.
//
// Spec justification:
//   - "Primary temperature is maintained through the balanced use of
//     reactor coolant pump heat input and a steam dump system..."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
func TestRCP_HeatInputScalesWithSpeed(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	// Single-pump lineup: secure the others so only the driven
	// rotor contributes heat.
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Running = false
		pu.Velocity = 0
	}
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.Running = true
	p.ReactorBuilding.ReactorCoolantPumps.LoopA.Velocity = RCPRatedSpeed / 2
	p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput = 0
	stepN(&p, 1, DefaultTimeStep)
	got := float64(p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput)
	want := float64(RCPRatedHeatInput) * 0.25
	assert.InDelta(t, want, got, want*0.05,
		"want PumpHeatInput ~= %v (RCPRatedHeatInput/4) at half speed, got %v",
		want, got)
}

// TestRCP_HeatInputOff verifies the per-step PumpHeatInput is zero
// when the pump is stopped (velocity = 0).
func TestRCP_HeatInputOff(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(false)
	for _, pu := range p.ReactorBuilding.ReactorCoolantPumps.All() {
		pu.Velocity = 0
	}
	p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput = 0
	stepN(&p, 1, DefaultTimeStep)
	assert.Equal(t, 0.0,
		float64(p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput),
		"want PumpHeatInput=0 with velocity=0, got %v",
		p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput)
}

// TestRCP_HeadScalesWithVelocity drives simulateFluidFlow directly
// so the per-edge head contribution can be isolated from the rest
// of Plant.Step dynamics. At full RCP speed the hot-leg edge
// delivers head=pumpHead; at half speed it delivers half that head
// and the resulting per-step flow halves.
//
// Spec justification:
//   - "Design capacity 97,600 gpm [...] Design head 280 ft (85.3 m)"
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_056.txt:124-130
//   - "long coastdown times [...] to preclude rapid decreases in
//     core cooling flow during pump trips."
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_020.txt:83-84
func TestRCP_HeadScalesWithVelocity(t *testing.T) {
	t.Parallel()
	runFlow := func(vel Velocity) float64 {
		from := &Tank{
			Pressure: Pressure(0),
			Level:    Volume{Val: 1e9, Unit: u.Gallon}, // effectively infinite source
			Capacity: Volume{Val: 1e9, Unit: u.Gallon},
		}
		to := &Tank{
			Pressure: Pressure(0),
			Level:    Volume{Val: 0, Unit: u.Gallon},
			Capacity: Volume{Val: 1e9, Unit: u.Gallon},
		}
		pump := &Pump{
			Running:  true,
			Velocity: vel,
		}
		g := &PlantGraph{
			Nodes: map[string]GraphNode{
				"from": {Ref: from},
				"to":   {Ref: to},
			},
			Edges: []GraphEdge{{
				From: "from", To: "to", Pump: pump,
			}},
		}
		simulateFluidFlow(g, DefaultTimeStep)
		return to.Level.Float()
	}
	fullFlow := runFlow(RCPRatedSpeed)
	halfFlow := runFlow(RCPRatedSpeed / 2)
	require.Greater(t, fullFlow, 0.0,
		"precondition: want positive flow at rated speed, got %v", fullFlow)
	ratio := halfFlow / fullFlow
	assert.InDelta(t, 0.5, ratio, 0.01,
		"want half/full flow ratio ~= 0.5, got %v (full=%v half=%v)",
		ratio, fullFlow, halfFlow)
}

// TestPumpLabel_RPM verifies the DOT label reports the running RPM.
func TestPumpLabel_RPM(t *testing.T) {
	t.Parallel()
	p := &Pump{Velocity: 1189, Running: true}
	got := pumpLabel(p)
	assert.Contains(t, got, "1189",
		"want pumpLabel to report RPM, got %q", got)
}
