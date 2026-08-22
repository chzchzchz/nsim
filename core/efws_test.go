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
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection

	stepN(&p, 1)
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
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Water =
		u.NewValue(EFWSLevelLowLow-100, u.Gallon)

	stepN(&p, 1)
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

// While the EFWS is active and pumps are running the primary
// tank level decreases at the rated pump discharge rate.
//
// Spec justification:
//   - "Each tank contains a quantity of condensate quality
//     water sufficient to allow the plant to be maintained in
//     hot standby for 13 hours then allow a 5-hour cooldown of
//     the plant to 350°F (176.7°C)."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:52-56
func TestEFWS_PrimaryTankDepletesWhilePumpsRunning(t *testing.T) {
	e := EFWS{
		Active: true,
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:      EFWSPrimaryTankCapacity,
				Capacity:   EFWSPrimaryTankCapacity,
				IsPrimary:  true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: (func() EFWSPump { var p EFWSPump; p.Running = true; return p }() ),
		},
		SubsystemB: EFWSSubsystem{
			Tank: EFWSTank{
				Level:      EFWSPrimaryTankCapacity,
				Capacity:   EFWSPrimaryTankCapacity,
				IsPrimary:  true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: (func() EFWSPump { var p EFWSPump; p.Running = true; return p }() ),
		},
	}
	dt := TimeStep.Seconds()
	wantLossA := EFWSMotorPumpFlow.Float() * dt
	wantLossB := EFWSMotorPumpFlow.Float() * dt
	e2 := e.Step(u.NewValue(EFWSSecureTemp+10, u.Fahrenheit), false)
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
// running, the turbine-pump flow dominates tank depletion
// (the larger of the two rated flows wins). At 200 gpm
// (turbine) vs 150 gpm (motor), the depletion rate equals
// EFWSTurbinePumpFlow.
func TestEFWS_TurbinePumpDominatesTankDepletionWhenBothRunning(t *testing.T) {
	e := EFWS{
		Active: true,
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:      EFWSPrimaryTankCapacity,
				Capacity:   EFWSPrimaryTankCapacity,
				IsPrimary:  true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
	}
	dt := TimeStep.Seconds()
	wantLoss := EFWSTurbinePumpFlow.Float() * dt
	e2 := e.Step(u.NewValue(EFWSSecureTemp+10, u.Fahrenheit), false)
	assert.InDelta(t, EFWSPrimaryTankCapacity.Float()-wantLoss,
		e2.SubsystemA.Tank.Level.Float(), 1e-9,
		"want SubsystemA.Tank.Level=%v (turbine dominates), got %v",
		EFWSPrimaryTankCapacity.Float()-wantLoss,
		e2.SubsystemA.Tank.Level.Float())
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
	e := EFWS{
		Active: true,
		SubsystemA: EFWSSubsystem{
			Tank: EFWSTank{
				Level:      EFWSPrimaryTankCapacity,
				Capacity:   EFWSPrimaryTankCapacity,
				IsPrimary:  true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
		SubsystemB: EFWSSubsystem{
			Tank: EFWSTank{
				Level:      EFWSPrimaryTankCapacity,
				Capacity:   EFWSPrimaryTankCapacity,
				IsPrimary:  true,
				Temperature: EFWSPrimaryTankMaxTemp,
			},
			MotorPump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
			TurbinePump: EFWSPump{AFWPump: AFWPump{Pump: Pump{Running: true}}},
		},
	}
	e2 := e.Step(
		u.NewValue(EFWSSecureTemp, u.Fahrenheit),
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
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	tankA := EFWSPrimaryTankCapacity.Float()
	tankB := EFWSPrimaryTankCapacity.Float()
	stepN(&p, int(time.Second/TimeStep))

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
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure

	stepN(&p, 1)
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
