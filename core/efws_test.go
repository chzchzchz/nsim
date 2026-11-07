package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// At construction both EFWS pumps in each subsystem are off,
// the primary tank is full at EFWSPrimaryTankCapacity, and the
// EFWS is not active. The auto-start logic has not fired.
func TestEFWS_InitialStateIsIdle(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.False(t, efws.Active,
		"want EFWSActive=false at startup, got %v",
		efws.Active)
	assert.False(t, efws.SubsystemA.MotorPump.Running,
		"want SubsystemA.MotorPump.Running=false at startup, got %v",
		efws.SubsystemA.MotorPump.Running)
	assert.False(t, efws.SubsystemA.TurbinePump.Running,
		"want SubsystemA.TurbinePump.Running=false at startup, got %v",
		efws.SubsystemA.TurbinePump.Running)
	assert.False(t, efws.SubsystemB.MotorPump.Running,
		"want SubsystemB.MotorPump.Running=false at startup, got %v",
		efws.SubsystemB.MotorPump.Running)
	assert.False(t, efws.SubsystemB.TurbinePump.Running,
		"want SubsystemB.TurbinePump.Running=false at startup, got %v",
		efws.SubsystemB.TurbinePump.Running)
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float(),
		efws.SubsystemA.Tank.Level.Float(), 1e-9,
		"want SubsystemA.Tank.Level=EFWSPrimaryTankCapacity at startup, got %v",
		efws.SubsystemA.Tank.Level.Float())
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float(),
		efws.SubsystemB.Tank.Level.Float(), 1e-9,
		"want SubsystemB.Tank.Level=EFWSPrimaryTankCapacity at startup, got %v",
		efws.SubsystemB.Tank.Level.Float())
	assert.Zero(t, p.ReactorBuilding.EFWSFeedwaterFlow.Float(),
		"want EFWSFeedwaterFlow=0 at startup, got %v",
		p.ReactorBuilding.EFWSFeedwaterFlow.Float())
}

// Setting DieselStartSignal = DieselStartSafetyInjection starts
// the motor-driven EFWS pumps in both subsystems, sets EFWS
// Active, and increments the start counter.
//
// Spec justification:
//   - "Safety injection / Motor driven pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:94-98
func TestEFWS_MotorPumpsStartOnSafetyInjection(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection

	stepN(&p, 1, DefaultTimeStep)
	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.True(t, efws.Active,
		"want EFWSActive=true after SI signal, got %v",
		efws.Active)
	assert.True(t, efws.SubsystemA.MotorPump.Running,
		"want SubsystemA.MotorPump.Running=true, got %v",
		efws.SubsystemA.MotorPump.Running)
	assert.True(t, efws.SubsystemB.MotorPump.Running,
		"want SubsystemB.MotorPump.Running=true, got %v",
		efws.SubsystemB.MotorPump.Running)
	assert.GreaterOrEqual(t, efws.MotorPumpStartCount, 2,
		"want MotorPumpStartCount>=2, got %v",
		efws.MotorPumpStartCount)
}

// A single-SG low-low level starts the motor-driven pumps AND
// the turbine-driven pumps. In the simplified model with a
// single SG field, "low-low in any two SGs" is satisfied
// whenever "low-low in any one SG" is, so both pump classes
// start on the same signal.
//
// Spec justification:
//   - Motor pumps: "Low-low level in 2/4 level channels in any
//     one steam generator / Motor driven pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:71-79
//   - Turbine pumps: "Low-low level in 2/4 level channels in any
//     two steam generators / Turbine drive pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:82-92
func TestEFWS_TurbinePumpsStartOnLowLowSGLevel(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water =
		Volume{Val: EFWSLevelLowLow - 100, Unit: u.Gallon}

	stepN(&p, 1, DefaultTimeStep)
	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.True(t, efws.Active,
		"want EFWSActive=true after SG low-low, got %v",
		efws.Active)
	assert.True(t, efws.SubsystemA.MotorPump.Running,
		"want SubsystemA.MotorPump.Running=true, got %v",
		efws.SubsystemA.MotorPump.Running)
	assert.True(t, efws.SubsystemA.TurbinePump.Running,
		"want SubsystemA.TurbinePump.Running=true, got %v",
		efws.SubsystemA.TurbinePump.Running)
	assert.True(t, efws.SubsystemB.MotorPump.Running,
		"want SubsystemB.MotorPump.Running=true, got %v",
		efws.SubsystemB.MotorPump.Running)
	assert.True(t, efws.SubsystemB.TurbinePump.Running,
		"want SubsystemB.TurbinePump.Running=true, got %v",
		efws.SubsystemB.TurbinePump.Running)
}

// While the EFWS is active and pumps are running with their
// discharge valves full open, the primary tank level decreases
// at the delivered rate (the rated pump discharge flow).
//
// Spec justification:
//   - "Each tank contains a quantity of condensate quality
//     water sufficient to allow the plant to be maintained in
//     hot standby for 13 hours then allow a 5-hour cooldown of
//     the plant to 350°F (176.7°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-56
func TestEFWS_PrimaryTankDepletesWhilePumpsRunning(t *testing.T) {
	t.Parallel()
	e := EFWS{
		Active: true,
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
				FlowModulatingValve: Valve{Open: 1},
			},
		},
		SubsystemB: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
				FlowModulatingValve: Valve{Open: 1},
			},
		},
	}
	dt := DefaultTimeStep.Seconds()
	wantLossA := EFWSMotorPumpFlow.Float() * dt
	wantLossB := EFWSMotorPumpFlow.Float() * dt
	e2 := e.Step(DefaultTimeStep, Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}, false)
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float()-wantLossA,
		e2.SubsystemA.Tank.Level.Float(), 1e-9,
		"want SubsystemA.Tank.Level=%v after one step, got %v",
		EFWSPrimaryTankCapacity.Float()-wantLossA,
		e2.SubsystemA.Tank.Level.Float())
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float()-wantLossB,
		e2.SubsystemB.Tank.Level.Float(), 1e-9,
		"want SubsystemB.Tank.Level=%v after one step, got %v",
		EFWSPrimaryTankCapacity.Float()-wantLossB,
		e2.SubsystemB.Tank.Level.Float())
}

// When both the motor and turbine pumps in a subsystem are
// running ready with full-open discharge valves, both contribute
// their rated flow to the delivered stream, so the tank loss is
// the sum of the two contributions.
func TestEFWS_TankDrawSumsBothRunningPumps(t *testing.T) {
	t.Parallel()
	e := EFWS{
		Active: true,
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
				FlowModulatingValve: Valve{Open: 1},
			},
			TurbinePump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
				FlowModulatingValve: Valve{Open: 1},
			},
		},
	}
	dt := DefaultTimeStep.Seconds()
	wantLoss := (EFWSMotorPumpFlow.Float() + EFWSTurbinePumpFlow.Float()) * dt
	e2 := e.Step(DefaultTimeStep, Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}, false)
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float()-wantLoss,
		e2.SubsystemA.Tank.Level.Float(), 1e-9,
		"want SubsystemA.Tank.Level=%v (both pumps deliver), got %v",
		EFWSPrimaryTankCapacity.Float()-wantLoss,
		e2.SubsystemA.Tank.Level.Float())
	assert.InDelta(t, EFWSMotorPumpFlow.Float()+EFWSTurbinePumpFlow.Float(),
		e2.DeliveredFlow.Float(), 1e-9,
		"want DeliveredFlow=%v, got %v",
		EFWSMotorPumpFlow.Float()+EFWSTurbinePumpFlow.Float(),
		e2.DeliveredFlow.Float())
}

// When the RCS hot-leg temperature has been reduced to
// EFWSSecureTemp AND the RHRS has taken over, the EFWS is
// secured and all pump Running flags are cleared.
//
// Spec justification:
//   - "the reactor coolant hot leg temperature is reduced to
//     350°F (176.7°C) and the RHRS has been activated, the EFWS
//     is secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
//   - "the EFWS continues to supply feedwater to the steam
//     generators throughout the cooldown until the primary
//     system hot leg temperature is reduced to 350°F (176.7°C).
//     At this temperature, the Residual Heat Removal System
//     (RHRS) is activated and the EFWS is secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:91-96
func TestEFWS_SecuredWhenHotLegBelow350FAndRHRSActive(t *testing.T) {
	t.Parallel()
	e := EFWS{
		Active: true,
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump:   EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
		SubsystemB: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump:   EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
	}
	e2 := e.Step(DefaultTimeStep,
		Temperature{Val: EFWSSecureTemp, Unit: u.Fahrenheit},
		true,
	)
	assert.False(t, e2.Active,
		"want EFWSActive=false after secured condition, got %v",
		e2.Active)
	assert.False(t, e2.SubsystemA.MotorPump.Running,
		"want SubsystemA.MotorPump.Running=false after secured, got %v",
		e2.SubsystemA.MotorPump.Running)
	assert.False(t, e2.SubsystemA.TurbinePump.Running,
		"want SubsystemA.TurbinePump.Running=false after secured, got %v",
		e2.SubsystemA.TurbinePump.Running)
	assert.False(t, e2.SubsystemB.MotorPump.Running,
		"want SubsystemB.MotorPump.Running=false after secured, got %v",
		e2.SubsystemB.MotorPump.Running)
	assert.False(t, e2.SubsystemB.TurbinePump.Running,
		"want SubsystemB.TurbinePump.Running=false after secured, got %v",
		e2.SubsystemB.TurbinePump.Running)
}

// The secured condition names the system, not a start mode: pumps
// started manually through the bridge controls (Active stays the
// auto-start latch) must secure exactly like autostarted ones.
//
// Spec justification:
//   - "the reactor coolant hot leg temperature is reduced to
//     350°F (176.7°C) and the RHRS has been activated, the EFWS
//     is secured."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:101-104
func TestEFWS_SecuresManuallyStartedPumpsToo(t *testing.T) {
	t.Parallel()
	e := EFWS{
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump:   EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
		SubsystemB: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump:   EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
	}
	assert.False(t, e.Active,
		"precondition: manually started pumps leave Active=false")

	e2 := e.Step(DefaultTimeStep,
		Temperature{Val: EFWSSecureTemp, Unit: u.Fahrenheit},
		true,
	)

	assert.False(t, e2.SubsystemA.MotorPump.Running,
		"manual-started pump must secure at the secure condition")
	assert.False(t, e2.SubsystemA.TurbinePump.Running,
		"manual-started pump must secure at the secure condition")
	assert.False(t, e2.SubsystemB.MotorPump.Running,
		"manual-started pump must secure at the secure condition")
	assert.False(t, e2.SubsystemB.TurbinePump.Running,
		"manual-started pump must secure at the secure condition")
}

// End-to-end via Plant.Step: a safety-injection actuation
// signal starts the EFWS motor-driven pumps, the EFWS feeds
// water to the steam generators, and the primary tank level
// decreases per step.
//
// Spec justification:
//   - "Safety injection / Motor driven pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:94-98
//   - "supply at least the minimum required flow, within one
//     minute of the actuation signal, to at least two effective
//     steam generators"
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:23-26
func TestEFWS_EndToEndSafetyInjectionStartsFeedwaterDelivery(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	tankA := EFWSPrimaryTankCapacity.Float()
	tankB := EFWSPrimaryTankCapacity.Float()
	stepN(&p, int(time.Second/DefaultTimeStep), DefaultTimeStep)

	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.True(t, efws.Active,
		"want EFWSActive=true after SI signal, got %v",
		efws.Active)
	assert.True(t, efws.SubsystemA.MotorPump.Running,
		"want SubsystemA.MotorPump.Running=true, got %v",
		efws.SubsystemA.MotorPump.Running)
	assert.True(t, efws.SubsystemB.MotorPump.Running,
		"want SubsystemB.MotorPump.Running=true, got %v",
		efws.SubsystemB.MotorPump.Running)

	assert.Less(t, efws.SubsystemA.Tank.Level.Float(), tankA,
		"want SubsystemA.Tank.Level < %v after 1 s, got %v",
		tankA, efws.SubsystemA.Tank.Level.Float())
	assert.Less(t, efws.SubsystemB.Tank.Level.Float(), tankB,
		"want SubsystemB.Tank.Level < %v after 1 s, got %v",
		tankB, efws.SubsystemB.Tank.Level.Float())
}

// End-to-end: when SG pressure is present, the turbine-driven
// pump's SteamAdmissionValve opens and its DriveReady is set
// true. This implements the fail-open steam admission valve
// described in the spec.
//
// Spec justification:
//   - "The steam supply line for each turbine driven pump is
//     connected to the main steam line from a steam generator.
//     This line is fitted with a steam admission valve which
//     is a pneumatically operated valve arranged to fail-open
//     on loss of air or electrical power."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
func TestEFWS_TurbineSteamAdmissionValveOpensWithSGPressure(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure

	stepN(&p, 1, DefaultTimeStep)
	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.Equal(t, 1.0,
		float64(efws.SubsystemA.TurbinePump.SteamAdmissionValve.Open),
		"want SubsystemA.TurbinePump.SteamAdmissionValve.Open=1, got %v",
		efws.SubsystemA.TurbinePump.SteamAdmissionValve.Open)
	assert.Equal(t, 1.0,
		float64(efws.SubsystemB.TurbinePump.SteamAdmissionValve.Open),
		"want SubsystemB.TurbinePump.SteamAdmissionValve.Open=1, got %v",
		efws.SubsystemB.TurbinePump.SteamAdmissionValve.Open)
	assert.True(t, efws.SubsystemA.TurbinePump.DriveReady,
		"want SubsystemA.TurbinePump.DriveReady=true, got %v",
		efws.SubsystemA.TurbinePump.DriveReady)
	assert.True(t, efws.SubsystemB.TurbinePump.DriveReady,
		"want SubsystemB.TurbinePump.DriveReady=true, got %v",
		efws.SubsystemB.TurbinePump.DriveReady)
}

// A throttled discharge valve scales the tank drawdown (and the
// reported delivery) by the valve fraction: an operator holding
// the modulating valves at half open uses the tank inventory at
// half the rated rate.
//
// Spec justification:
//   - "the pumps are manually started and the flowrates are
//     manually [controlled by positioning the flow modulating
//     valves.]"
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
func TestEFWS_ThrottledValveScalesTankDrawdown(t *testing.T) {
	t.Parallel()
	e := EFWS{
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
				FlowModulatingValve: Valve{Open: 0.25},
			},
		},
	}
	dt := DefaultTimeStep.Seconds()
	wantLoss := 0.25 * EFWSMotorPumpFlow.Float() * dt
	e2 := e.Step(DefaultTimeStep, Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}, false)
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float()-wantLoss,
		e2.SubsystemA.Tank.Level.Float(), 1e-9,
		"want SubsystemA.Tank.Level=%v after one throttled step, got %v",
		EFWSPrimaryTankCapacity.Float()-wantLoss,
		e2.SubsystemA.Tank.Level.Float())
	assert.InDelta(t, 0.25*EFWSMotorPumpFlow.Float(),
		e2.DeliveredFlow.Float(), 1e-9,
		"want DeliveredFlow=%v, got %v",
		0.25*EFWSMotorPumpFlow.Float(), e2.DeliveredFlow.Float())
}

// A running pump without its drive ready delivers nothing and
// draws nothing from the tank.
func TestEFWS_NoDrawdownWithoutDriveReady(t *testing.T) {
	t.Parallel()
	e := EFWS{
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       EFWSPrimaryTankCapacity,
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}},
				FlowModulatingValve: Valve{Open: 1},
			},
		},
	}
	e2 := e.Step(DefaultTimeStep, Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}, false)
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float(),
		e2.SubsystemA.Tank.Level.Float(), 1e-9,
		"tank must not drain without a ready drive, got %v",
		e2.SubsystemA.Tank.Level.Float())
	assert.Zero(t, e2.DeliveredFlow.Float(),
		"no delivery without a ready drive, got %v",
		e2.DeliveredFlow.Float())
}

// A nearly empty tank caps the step's drawdown at its remaining
// inventory: that remainder is credited as delivery, and once
// dry both the tank and the reported flow sit at zero.
func TestEFWS_DryTankStopsDeliveryAtRemainingInventory(t *testing.T) {
	t.Parallel()
	dt := DefaultTimeStep.Seconds()
	remaining := 0.5 * EFWSMotorPumpFlow.Float() * dt
	e := EFWS{
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:       Volume{Val: remaining, Unit: u.Gallon},
				Capacity:    EFWSPrimaryTankCapacity,
				IsPrimary:   true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{
				AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
				FlowModulatingValve: Valve{Open: 1},
			},
		},
	}
	e2 := e.Step(DefaultTimeStep, Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}, false)
	assert.Zero(t, e2.SubsystemA.Tank.Level.Float(),
		"the step should have emptied the tank, got %v",
		e2.SubsystemA.Tank.Level.Float())
	assert.InDelta(t, remaining/dt, e2.DeliveredFlow.Float(), 1e-9,
		"delivery must clamp to the remaining inventory, want %v got %v",
		remaining/dt, e2.DeliveredFlow.Float())

	e3 := e2.Step(DefaultTimeStep, Temperature{Val: EFWSSecureTemp + 10, Unit: u.Fahrenheit}, false)
	assert.Zero(t, e3.DeliveredFlow.Float(),
		"a dry tank delivers nothing, got %v",
		e3.DeliveredFlow.Float())
}

// End-to-end through Plant.Step with a manual half-open lineup:
// the flow reported to the SG side equals the measured tank
// drawdown over the window, so secondary-side mass is conserved
// between the primary tanks and the steam generators even under
// a throttled lineup.
//
// Spec justification:
//   - "the emergency feedwater pumps take suction from the
//     primary emergency feedwater tanks and discharge the water
//     into the main feedwater piping [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:12-17
func TestPlant_EFWSReportedFlowMatchesTankDrawdown(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	efws := &p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	for _, sub := range []*EFWSSubsystem{&efws.SubsystemA, &efws.SubsystemB} {
		sub.MotorPump = EFWSPump{
			AFWPump:             AFWPump{Pump: Pump{Running: true}, DriveReady: true},
			FlowModulatingValve: Valve{Open: 0.5},
		}
	}
	beforeA := efws.SubsystemA.Tank.Level.Float()
	beforeB := efws.SubsystemB.Tank.Level.Float()

	steps := int(time.Second / DefaultTimeStep)
	stepN(&p, steps, DefaultTimeStep)

	after := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	drawn := beforeA + beforeB -
		after.SubsystemA.Tank.Level.Float() -
		after.SubsystemB.Tank.Level.Float()
	integrated := p.ReactorBuilding.EFWSFeedwaterFlow.Float() *
		float64(steps) * DefaultTimeStep.Seconds()
	assert.InDelta(t, drawn, integrated, 1e-6,
		"reported flow must equal tank drawdown: drawn=%v integrated=%v",
		drawn, integrated)
}
