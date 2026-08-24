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
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPump.Running = true
	p.ReactorBuilding.ReactorCoolantPump.Velocity = 0
	stepN(&p, int(5*time.Second/TimeStep))
	assert.GreaterOrEqual(t, float64(p.ReactorBuilding.ReactorCoolantPump.Velocity),
		0.9*float64(RCPRatedSpeed),
		"want RCP velocity >= 0.9 * RCPRatedSpeed after 5 s startup, got %v",
		p.ReactorBuilding.ReactorCoolantPump.Velocity)
}


// The RCP DriveReady latch must survive the ReactorBuilding step
// assignment inside Plant.Step so the sensor tracks the ESF bus
// state that feeds the pump motors.
//
// Spec justification:
//   - "All of the engineered safety system loads are fed from two
//      6600-volt engineered safety features buses."
//   - specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
func TestPlant_RCPDriveReadyFollowsESFBreaker(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.BreakerClosed = false
	p.Electrical.ESFBusB.BreakerClosed = false
	stepN(&p, 10)
	require.False(t, p.ReactorBuilding.ReactorCoolantPump.DriveReady,
		"precondition: ESF breakers open, want DriveReady=false")

	p.Electrical.ESFBusA.BreakerClosed = true
	p.Electrical.ESFBusB.BreakerClosed = true
	stepN(&p, 10)
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
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPump.Running = true
	// Poll to rated speed with a cap; the ramp is monotone so the
	// exit comes as soon as the precondition holds.
	reached := false
	for i := 0; i < int(20*time.Second/TimeStep); i++ {
		p.Step()
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
	p.ReactorBuilding.ReactorCoolantPump.Running = false
	// The flywheel coastdown is long by design (chapter 16 relies on
	// it for flow during the power decay transient), so poll for the
	// sub-1% crossing instead of stepping a fixed window past it.
	const maxCoast = 50 * time.Second
	crossed := false
	for i := 0; i < int(maxCoast/TimeStep); i++ {
		p.Step()
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
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPump.Running = true
	p.ReactorBuilding.ReactorCoolantPump.Velocity = RCPRatedSpeed / 2
	p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput = 0
	stepN(&p, 1)
	got := float64(p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput)
	want := float64(RCPRatedHeatInput) * 0.25
	assert.InDelta(t, want, got, want*0.05,
		"want PumpHeatInput ~= %v (RCPRatedHeatInput/4) at half speed, got %v",
		want, got)
}

// TestRCP_HeatInputOff verifies the per-step PumpHeatInput is zero
// when the pump is stopped (velocity = 0).
func TestRCP_HeatInputOff(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorCoolantPump.Running = false
	p.ReactorBuilding.ReactorCoolantPump.Velocity = 0
	p.ReactorBuilding.ReactorPressureVessel.PumpHeatInput = 0
	stepN(&p, 1)
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
		simulateFluidFlow(g, TimeStep)
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
	p := &Pump{Velocity: 1189, Running: true}
	got := pumpLabel(p)
	assert.Contains(t, got, "1189",
		"want pumpLabel to report RPM, got %q", got)
}
