package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// runningMotorPump returns a motor-driven EFWS pump with its
// discharge valve at the operator position and the drive ready, so
// flow is gated only by the valve.
func runningMotorPump() EFWSPump {
	return EFWSPump{AFWPump: AFWPump{
		Pump:       Pump{Running: true},
		DriveReady: true,
	}}
}

// Inside the assumed no-operator-action window after an automatic
// start the discharge valves are full open even with the SG level
// far above the maintenance band.
//
// "it is assumed that no operator action can be taken for 30
// minutes, and for this period it is assumed that these valves
// will be full open."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:36-39
func TestEFWS_ValvesFullOpenInsideActuationWindow(t *testing.T) {
	t.Parallel()
	e := EFWS{
		Active:           true,
		AutoStartElapsed: EFWSFullOpenPeriod - 5*time.Second,
		SubsystemA:       EFWSSubsystem{MotorPump: runningMotorPump()},
		SubsystemB:       EFWSSubsystem{MotorPump: runningMotorPump()},
	}
	e.ApplyModulatingValveLineup(95)
	assert.Equal(t, 1.0, e.SubsystemA.MotorPump.FlowModulatingValve.Open)
	assert.Equal(t, 1.0, e.SubsystemB.MotorPump.FlowModulatingValve.Open)
}

// After the window the valves hold the SG level inside a narrow
// band below the high-level trip: shut above the band, reopened
// below it, previous position standing in between.
//
// "When operator action can be taken (after an assumed 30-minute
// delay), the emergency feedwater flow is adjusted by positioning
// the flow modulating valves to restore and maintain the steam
// generator water levels within the narrow control range."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:29-34
func TestEFWS_ValvesThrottleAfterActuationWindow(t *testing.T) {
	t.Parallel()
	newActive := func(elapsed time.Duration) *EFWS {
		return &EFWS{
			Active:           true,
			AutoStartElapsed: elapsed,
			SubsystemA:       EFWSSubsystem{MotorPump: runningMotorPump()},
			SubsystemB:       EFWSSubsystem{MotorPump: runningMotorPump()},
		}
	}

	shut := newActive(EFWSFullOpenPeriod + time.Minute)
	shut.ApplyModulatingValveLineup(EFWSLevelControlClose)
	assert.Equal(t, 0.0, shut.SubsystemA.MotorPump.FlowModulatingValve.Open,
		"level at the band top must shut the valves")

	held := newActive(EFWSFullOpenPeriod + time.Minute)
	held.SubsystemA.MotorPump.FlowModulatingValve.Open = 1.0
	held.ApplyModulatingValveLineup((EFWSLevelControlClose + EFWSLevelControlReopen) / 2)
	assert.Equal(t, 1.0, held.SubsystemA.MotorPump.FlowModulatingValve.Open,
		"inside the band the previous position must stand")

	reopened := newActive(EFWSFullOpenPeriod + time.Minute)
	reopened.SubsystemA.MotorPump.FlowModulatingValve.Open = 0.0
	reopened.ApplyModulatingValveLineup(EFWSLevelControlReopen - 1)
	assert.Equal(t, 1.0, reopened.SubsystemA.MotorPump.FlowModulatingValve.Open,
		"below the band the valves must reopen")
}

// Manual starts are operated by positioning the modulating valves;
// the actuation lineup must not touch them when the auto-start
// latch never engaged.
//
// "the pumps are manually started and the flowrates are manually
// controlled by positioning the flow modulating valves."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
func TestEFWS_ManualStartLeavesOperatorValvePosition(t *testing.T) {
	t.Parallel()
	e := EFWS{
		Active:     false,
		SubsystemA: EFWSSubsystem{MotorPump: runningMotorPump()},
	}
	e.SubsystemA.MotorPump.FlowModulatingValve.Open = 0.5
	e.ApplyModulatingValveLineup(95)
	assert.Equal(t, 0.5, e.SubsystemA.MotorPump.FlowModulatingValve.Open)
}

// Full-plant regression for the unattended-overfill artifact: an
// auto-started EFWS fills during the full-open window, then the
// post-window lineup stops the fill below the 90 percent high-level
// trip instead of swelling into a spurious reactor trip.
//
// "When operator action can be taken (after an assumed 30-minute
// delay), the emergency feedwater flow is adjusted by positioning
// the flow modulating valves to restore and maintain the steam
// generator water levels within the narrow control range."
// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:29-34
func TestPlant_EFWSWindowStopsFillBeforeHighLevelTrip(t *testing.T) {
	t.Parallel()
	testDt := 50 * time.Millisecond
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.Insertion = 1
	rpv.DemandInsertion = 1
	rpv.FissionPower = CoreNeutronSourcePower
	cvcs := &p.AuxiliaryBuilding.ChemicalVolumeControlSystem
	cvcs.ChargingPumpRunning = false
	cvcs.LetdownValveOpen = false
	sg := &p.ReactorBuilding.SteamGenerator
	sg.MainSteamIsolationValve.Open = 0
	sg.Pressure = SteamGeneratorShellPressure
	// Just under the maintenance band so the remaining two
	// minutes of full-open window carry the level across it.
	sg.Water = Volume{Val: (EFWSLevelControlClose - 0.1) / 100.0 * sg.Volume.Float(), Unit: u.Gallon}
	turb := &p.TurbineBuilding.Turbine
	turb.StopValve.Open = 0
	turb.DumpValve.Open = 0
	efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	efws.Active = true
	efws.AutoStartElapsed = EFWSFullOpenPeriod - 2*time.Minute
	for _, sub := range []*EFWSSubsystem{&efws.SubsystemA, &efws.SubsystemB} {
		sub.MotorPump = runningMotorPump()
		sub.Tank = EFWSTank{
			Level:       EFWSPrimaryTankCapacity,
			Capacity:    EFWSPrimaryTankCapacity,
			IsPrimary:   true,
			Temperature: EFWSPrimaryTankMaxTemp,
		}
	}

	stepN(&p, int(3*time.Minute/testDt), testDt)

	waterPct := 100.0 * p.ReactorBuilding.SteamGenerator.Water.Float() /
		p.ReactorBuilding.SteamGenerator.Volume.Float()
	valveOpen := p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.FlowModulatingValve.Open
	assert.Less(t, waterPct, RPSHighSteamGeneratorLevelTrip,
		"the post-window lineup must stop the fill below the high-level trip")
	assert.GreaterOrEqual(t, waterPct, EFWSLevelControlClose,
		"precondition: the window itself must have filled into the band")
	assert.Equal(t, 0.0, valveOpen)
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"no spurious high-level reactor trip from unattended AFW fill")
}
