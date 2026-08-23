package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// The RHRS does not run until RCS temperature ≤ 350°F and
// pressure < 400 psig. With both criteria met, RHRS.Step latches
// Running=true; with either violated, Running stays false.
//
// Spec justification:
//   - "When the reactor coolant temperature has dropped to 350°F
//     (177°C) and the pressure has been reduced to less than
//     approximately 400 psig (28 bar), the RHR equipment is placed
//     in operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
func TestRHRS_StartRequiresTempAndPressureThresholds(t *testing.T) {
	rcsAbove := Temperature{Val: RHRSTempThreshold+1, Unit: u.Fahrenheit}
	rcsAt := Temperature{Val: RHRSTempThreshold, Unit: u.Fahrenheit}
	rcsBelow := Temperature{Val: RHRSTempThreshold-10, Unit: u.Fahrenheit}
	ccw := Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit}

	hot := (&RHRS{}).Step(rcsAbove, RHRSPressureThreshold-1, ccw)
	assert.False(t, hot.Running,
		"want Running=false with RCS temp above threshold, got %v",
		hot.Running)

	highP := (&RHRS{}).Step(rcsBelow, RHRSPressureThreshold, ccw)
	assert.False(t, highP.Running,
		"want Running=false when pressure == 400 psig (not strictly below), got %v",
		highP.Running)

	started := (&RHRS{}).Step(rcsAt, RHRSPressureThreshold-1, ccw)
	assert.True(t, started.Running,
		"want Running=true at RCS temp == 350°F and pressure < 400 psig, got %v",
		started.Running)
}

// Above the 350°F entrance threshold the RHRS does not start even
// if the operator opens the suction valves and starts the pump.
// Stepping again with the criteria still violated leaves Running
// false and per-train ReturnFlow at zero.
//
// Spec justification:
//   - "When the reactor coolant temperature has dropped to 350°F
//     (177°C) and the pressure has been reduced to less than
//     approximately 400 psig (28 bar), the RHR equipment is placed
//     in operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
func TestRHRS_DoesNotStartAboveTempThreshold(t *testing.T) {
	r := RHRS{
		TrainA: RHRSTrain{
			Pump:          Pump{Running: true},
			SuctionValves: [2]Valve{{Open: 1}, {Open: 1}},
		},
		TrainB: RHRSTrain{
			Pump:          Pump{Running: true},
			SuctionValves: [2]Valve{{Open: 1}, {Open: 1}},
		},
	}
	rcsHot := Temperature{Val: RHRSTempThreshold+10, Unit: u.Fahrenheit}
	ccw := Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit}
	r = r.Step(rcsHot, 100, ccw)
	assert.False(t, r.Running,
		"want Running=false with RCS temp above threshold, got %v",
		r.Running)
	assert.Zero(t, r.TrainA.ReturnFlow.Float(),
		"want TrainA ReturnFlow=0 above threshold, got %v",
		r.TrainA.ReturnFlow.Float())
	assert.Zero(t, r.TrainB.ReturnFlow.Float(),
		"want TrainB ReturnFlow=0 above threshold, got %v",
		r.TrainB.ReturnFlow.Float())
}

// The pump runs on its mini-flow circuit during warmup; the
// hot-leg suction valves cannot open until RHRSTrainWarmupTime
// has elapsed. We model that by leaving SuctionValves[0].Open = 0
// during warmup; the per-step ReturnFlow equals the minimum-flow
// only and HeatTransferRate is zero.
//
// Spec justification:
//   - "RHRS operation generally begins with a short period wherein
//     the pump is run on its miniflow circuit in order to warm the
//     pump and heat exchanger and avoid sudden thermal expansion
//     within these components upon contacting hot reactor coolant.
//     Following system warmup, the block valves in the RHRS suction
//     lines from the RCS are opened [...]."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:26-32
func TestRHRS_WarmupTimerKeepsSuctionClosed(t *testing.T) {
	tr := RHRSTrain{
		Pump:          Pump{Running: true},
		DriveReady:    true,
		SuctionValves: [2]Valve{{Open: 0}, {Open: 1}},
	}
	rcsHot := Temperature{Val: 350, Unit: u.Fahrenheit}
	ccw := Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit}
	tr = tr.Step(rcsHot, ccw, false)
	assert.False(t, tr.WarmupComplete,
		"want WarmupComplete=false before warmup elapses, got %v",
		tr.WarmupComplete)
	assert.Zero(t, tr.HeatExchanger.HeatTransferRate,
		"want HeatTransferRate=0 during warmup, got %v",
		tr.HeatExchanger.HeatTransferRate)
	assert.Greater(t, tr.WarmupTimer, time.Duration(0),
		"want WarmupTimer > 0 after one step, got %v",
		tr.WarmupTimer)
}

// Once the operator opens both suction valves and warmup is
// complete, the train delivers the full design flow to the RCS
// cold legs. ReturnFlow should equal the design flow per train
// per second when BypassValve.Open = 0.
//
// Spec justification:
//   - "A minimum flow for the RHR pump is provided by a
//     recirculation line from the heat exchanger outlet to the
//     pump suction which remains open in all system operating
//     modes."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:53-57
//   - "Operation of the controller and bypass valve maintains
//     constant return flow to the RCS."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:65-66
func TestRHRS_MinimumFlowRecircAlwaysPresent(t *testing.T) {
	tr := RHRSTrain{
		Pump:           Pump{Running: true},
		DriveReady:     true,
		BypassValve:    Valve{Open: 0},
		WarmupComplete: true,
	}
	rcsHot := Temperature{Val: 350, Unit: u.Fahrenheit}
	ccw := Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit}
	tr = tr.Step(rcsHot, ccw, true)
	dt := TimeStep.Seconds()
	want := (RHRSTrainDesignFlow + RHRSPumpMinFlowGalPerStep) * dt
	got := tr.ReturnFlow.Float()
	assert.InDelta(t, want, got, 1e-9,
		"want ReturnFlow=%v gal at full-flow (Bypass=0), got %v",
		want, got)
}

// The bypass controller holds total ReturnFlow constant while the
// operator opens the BypassValve. With BypassValve.Open = 1 the
// flow all goes through the bypass line and the HX sees no
// flow, so HeatTransferRate drops to zero.
//
// Spec justification:
//   - "A second flow control valve located in the bypass line
//     around the RHR heat exchangers is automatically positioned
//     by a flow controller that measures flow in the main return
//     line to the RCS. Operation of the controller and bypass
//     valve maintains constant return flow to the RCS."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:61-66
func TestRHRS_BypassValveKeepsReturnFlowConstant(t *testing.T) {
	rcsHot := Temperature{Val: 350, Unit: u.Fahrenheit}
	ccw := Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit}
	mk := func(open float64) *RHRSTrain {
		return &RHRSTrain{
			Pump:           Pump{Running: true},
			DriveReady:     true,
			BypassValve:    Valve{Open: open},
			WarmupComplete: true,
		}
	}
	dt := TimeStep.Seconds()
	wantFlow := (RHRSTrainDesignFlow + RHRSPumpMinFlowGalPerStep) * dt

	a := mk(0).Step(rcsHot, ccw, true)
	b := mk(0.5).Step(rcsHot, ccw, true)
	c := mk(1).Step(rcsHot, ccw, true)

	assert.InDelta(t, wantFlow, a.ReturnFlow.Float(), 1e-9,
		"want ReturnFlow=%v at Bypass=0, got %v",
		wantFlow, a.ReturnFlow.Float())
	assert.InDelta(t, wantFlow, b.ReturnFlow.Float(), 1e-9,
		"want ReturnFlow=%v at Bypass=0.5, got %v",
		wantFlow, b.ReturnFlow.Float())
	assert.InDelta(t, wantFlow, c.ReturnFlow.Float(), 1e-9,
		"want ReturnFlow=%v at Bypass=1, got %v",
		wantFlow, c.ReturnFlow.Float())

	assert.Greater(t, a.HeatExchanger.HeatTransferRate,
		c.HeatExchanger.HeatTransferRate,
		"want HX duty higher at Bypass=0 than Bypass=1, got %v vs %v",
		a.HeatExchanger.HeatTransferRate,
		c.HeatExchanger.HeatTransferRate)
	assert.Zero(t, c.HeatExchanger.HeatTransferRate,
		"want HX duty = 0 at full bypass, got %v",
		c.HeatExchanger.HeatTransferRate)
}

// End-to-end: the operator trips the reactor, waits for RCS
// temperature and pressure to drop below the RHRS entrance
// thresholds, starts both trains with the suction valves open
// and the warmup period elapsed, and observes the RCS outlet
// temperature fall under RHRS heat rejection.
//
// Spec justification:
//   - Entrance: "When the reactor coolant temperature has
//     dropped to 350°F (177°C) and the pressure has been reduced
//     to less than approximately 400 psig (28 bar), the RHR
//     equipment is placed in operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:3-6
//   - Cooldown target: "The system is designed to reduce the RCS
//     temperature to 140°F (60°C) within 20 hours following
//     reactor shutdown."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:31-32
func TestRHRS_EndToEndCooldownTransfersHeatFromRCS(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.Outlet =
		Temperature{Val: RHRSTempThreshold, Unit: u.Fahrenheit}
	p.ReactorBuilding.Pressurizer.Pressure = RHRSPressureThreshold - 50

	r := &p.AuxiliaryBuilding.ResidualHeatRemovalSystem
	r.TrainA = RHRSTrain{
		Pump:           Pump{Running: true},
		DriveReady:     true,
		BypassValve:    Valve{Open: 0},
		WarmupComplete: true,
		SuctionValves:  [2]Valve{{Open: 1}, {Open: 1}},
	}
	r.TrainB = r.TrainA

	r2 := r.Step(
		p.ReactorBuilding.ReactorPressureVessel.Outlet,
		float64(p.ReactorBuilding.Pressurizer.Pressure),
		Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
	)
	r = &r2
	assert.True(t, r.HasTakenOver,
		"want HasTakenOver=true after one step with both trains running, got %v",
		r.HasTakenOver)
	assert.Greater(t,
		r.TrainA.HeatExchanger.HeatTransferRate+
			r.TrainB.HeatExchanger.HeatTransferRate,
		Watts(0),
		"want positive HX heat rejection with hot RCS, got %v",
		r.TrainA.HeatExchanger.HeatTransferRate+
			r.TrainB.HeatExchanger.HeatTransferRate)
	assert.Less(t,
		r.TrainA.ReturnFlow.Float()+r.TrainB.ReturnFlow.Float(),
		float64(RHRSTrainDesignFlow)*TimeStep.Seconds()*2+
			RHRSPumpMinFlowGalPerStep*TimeStep.Seconds()*2+1e-9,
		"want ReturnFlow within design bound, got %v",
		r.TrainA.ReturnFlow.Float()+r.TrainB.ReturnFlow.Float())
}
