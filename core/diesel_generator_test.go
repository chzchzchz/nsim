package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enoughFuel returns a DieselGenerator with full day-tank fuel so the
// engine can complete a start sequence without tripping on fuel.
func enoughFuel() DieselGenerator {
	return DieselGenerator{
		Fuel: Tank{
			Level:    Volume{Val: 500, Unit: u.Gallon},
			Capacity: Volume{Val: 500, Unit: u.Gallon},
			Part:     Part{Temperature: Temperature{Val: 80, Unit: u.Fahrenheit}},
		},
		CoolantTemp: Temperature{Val: 80, Unit: u.Fahrenheit},
	}
}

// Setting StartSignal to a non-None cause starts the engine without
// operator action (SafetyInjection is the spec's most-cited cause).
// Clearing it back to None after the engine is running eventually returns
// it to Off.
func TestDieselStartSafetyInjection(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection

	stepN(&p, int(10*time.Second/TimeStep))
	assert.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGenerator.State, "want DieselRunning via SI")

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartNone
	// Poll for the shutdown transition with a cap instead of
	// stepping a fixed window past it.
	shutdown := false
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		if p.AuxiliaryBuilding.DieselGenerator.State == DieselOff {
			shutdown = true
			break
		}
	}
	assert.True(t, shutdown, "want DieselOff after clearing signal")
}

// 10 s after the start signal, the generator side has built up to rated
// voltage and frequency, the breaker has closed, and RealPower matches
// rated kW.
func TestDieselGeneratorReachesRatedOutputIn10s(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, int(10*time.Second/TimeStep))

	g := p.AuxiliaryBuilding.DieselGenerator.Generator
	assert.True(t, g.BreakerClosed, "want BreakerClosed after 10 s")
	assert.GreaterOrEqual(t, g.Voltage, DieselRatedVoltage*0.99,
		"want Voltage ~= %v, got %v", DieselRatedVoltage, g.Voltage)
	assert.GreaterOrEqual(t, g.Frequency, DieselRatedFrequency*0.99,
		"want Frequency ~= %v, got %v", DieselRatedFrequency, g.Frequency)
	assert.Equal(t, DieselRatedkW, g.RealPower,
		"want RealPower %v at full load, got %v", DieselRatedkW, g.RealPower)
}

// With offsite power unavailable, the ESF bus is energised by the diesel
// within ~10 s of StartSignal, and the motor-driven AFW pump's
// DriveReady follows the bus.
func TestESFBusEnergizedByDiesel(t *testing.T) {
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()

	require.False(t, p.Electrical.ESFBusA.Energized, "precondition: ESFBusA should start unenergized")

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, int(10*time.Second/TimeStep))

	assert.True(t, p.Electrical.ESFBusA.Energized,
		"want ESFBusA.energized=true after 10 s, got false (voltage=%v)", p.Electrical.ESFBusA.Voltage)
	assert.GreaterOrEqual(t, p.Electrical.ESFBusA.Voltage, DieselRatedVoltage*0.99,
		"want ESFBusA.voltage ~= %v, got %v", DieselRatedVoltage, p.Electrical.ESFBusA.Voltage)
	assert.True(t, p.Outside.MotorAFWPump.DriveReady,
		"want MotorAFWPump.DriveReady=true after ESFBusA energized, got false")
}

// With the diesel not running, the operator's intent (Running=true) is
// preserved on the diesel-driven AFW pump, but DriveReady=false means
// fluid flow (computed by the consumer) is gated to zero.
func TestDieselAFWPumpGatedByDiesel(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.Outside.DieselAFWPump.Running = true // operator request
	p.Outside.DieselAFWPump.DriveSource = DriveDiesel

	stepN(&p, 1)
	assert.True(t, p.Outside.DieselAFWPump.Running,
		"want DieselAFWPump.Running preserved (operator intent) when diesel not ready")
	assert.False(t, p.Outside.DieselAFWPump.DriveReady,
		"want DieselAFWPump.DriveReady false before diesel start")

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, int(10*time.Second/TimeStep))
	assert.True(t, p.Outside.DieselAFWPump.DriveReady,
		"want DieselAFWPump.DriveReady true after diesel ReadyToLoad")
	assert.True(t, p.Outside.DieselAFWPump.Running,
		"want DieselAFWPump.Running preserved once DriveReady")
}

// The turbine-driven AFW pump's DriveReady follows the SG steam
// header pressure. With SG pressure above the trip threshold the
// turbine pump is ready; once the SG pressure collapses (e.g.
// after a reactor trip boils the inventory down) DriveReady goes
// false.
//
// Spec justification:
//   - "The steam supply line for each turbine driven pump is connected
//     to the main steam line from a steam generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
func TestTurbineAFWPumpGatedBySGVesselPressure(t *testing.T) {
	p := MakePlant()
	p.Outside.TurbineAFWPump.Running = true
	p.Outside.TurbineAFWPump.DriveSource = DriveSteam

	require.False(t, p.Outside.TurbineAFWPump.DriveReady,
		"want TurbineAFWPump.DriveReady false before SG builds pressure")

	// Pin SG pressure above the trip threshold (SteamGeneratorShellPressure/100);
	// the per-step SteamGenerator.Step only relaxes Pressure when
	// HeatInput > 0, and we leave the reactor off so the setpoint
	// sticks for the duration of the first leg of the test.
	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	stepN(&p, int(100*time.Millisecond/TimeStep))
	assert.Greater(t, p.ReactorBuilding.SteamGenerator.Pressure, SteamGeneratorShellPressure/100,
		"want SG pressure > 0 after SG pressure set")
	assert.True(t, p.Outside.TurbineAFWPump.DriveReady,
		"want TurbineAFWPump.DriveReady true while SG has pressure, got false (sg.P=%v)",
		p.ReactorBuilding.SteamGenerator.Pressure)

	// Drop the SG pressure below the trip threshold (e.g. after the
	// reactor trips and the SG inventory boils off); DriveReady
	// must follow on the very next step.
	p.ReactorBuilding.SteamGenerator.Pressure = 0
	stepN(&p, 1)
	assert.False(t, p.Outside.TurbineAFWPump.DriveReady,
		"want TurbineAFWPump.DriveReady false after SG pressure collapses, got true (sg.P=%v)",
		p.ReactorBuilding.SteamGenerator.Pressure)
}
