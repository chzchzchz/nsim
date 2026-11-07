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
	t.Parallel()
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection

	stepN(&p, int(10*time.Second/DefaultTimeStep), DefaultTimeStep)
	assert.Equal(t, DieselRunning, p.AuxiliaryBuilding.DieselGenerator.State, "want DieselRunning via SI")

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartNone
	// Poll for the shutdown transition with a cap instead of
	// stepping a fixed window past it.
	shutdown := false
	for i := 0; i < int(30*time.Second/DefaultTimeStep); i++ {
		p.Step(DefaultTimeStep)
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
	t.Parallel()
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, int(10*time.Second/DefaultTimeStep), DefaultTimeStep)

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
	t.Parallel()
	p := MakePlant()
	p.Outside.ServiceWaterSystem.PumpRunning = true
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()

	require.False(t, p.Electrical.ESFBusA.Energized, "precondition: ESFBusA should start unenergized")

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, int(10*time.Second/DefaultTimeStep), DefaultTimeStep)

	assert.True(t, p.Electrical.ESFBusA.Energized,
		"want ESFBusA.energized=true after 10 s, got false (voltage=%v)", p.Electrical.ESFBusA.Voltage)
	assert.GreaterOrEqual(t, p.Electrical.ESFBusA.Voltage, DieselRatedVoltage*0.99,
		"want ESFBusA.voltage ~= %v, got %v", DieselRatedVoltage, p.Electrical.ESFBusA.Voltage)
	assert.True(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.DriveReady,
		"want SubsystemA.MotorPump.DriveReady=true after ESFBusA energized, got false")
}

// With the ESF bus de-energized the EFWS motor pump preserves
// the operator's Running intent but DriveReady=false gates flow to
// zero until the diesel energizes the bus.
func TestDieselAFWPumpGatedByDiesel(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.PlantElectricalSystems.OffsitePowerAvailable = false
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Running = true

	stepN(&p, 1, DefaultTimeStep)
	assert.True(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Running,
		"want MotorPump.Running preserved (operator intent) when bus not ready")
	assert.False(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.DriveReady,
		"want MotorPump.DriveReady false before diesel start")

	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, int(10*time.Second/DefaultTimeStep), DefaultTimeStep)
	assert.True(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.DriveReady,
		"want MotorPump.DriveReady true after diesel energizes ESFBusA")
	assert.True(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Running,
		"want MotorPump.Running preserved once DriveReady")
}

// The turbine-driven EFWS pump's DriveReady follows the SG steam
// header pressure. With SG pressure above the threshold the
// turbine pump is ready and the admission valve opens; once the
// SG pressure collapses (e.g. after a reactor trip boils the
// inventory down) DriveReady goes false and the valve closes.
//
// Spec justification:
//   - "The steam supply line for each turbine driven pump is connected
//     to the main steam line from a steam generator."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:42-47
func TestTurbineAFWPumpGatedBySGVesselPressure(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.Running = true

	require.False(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.DriveReady,
		"want SubsystemA.TurbinePump.DriveReady false before SG builds pressure")

	p.ReactorBuilding.SteamGenerator.Pressure = SteamGeneratorShellPressure
	stepN(&p, int(100*time.Millisecond/DefaultTimeStep), DefaultTimeStep)
	assert.Greater(t, p.ReactorBuilding.SteamGenerator.Pressure, SteamGeneratorShellPressure/100,
		"want SG pressure > threshold after SG pressure set")
	assert.True(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.DriveReady,
		"want SubsystemA.TurbinePump.DriveReady true while SG has pressure, got false (sg.P=%v)",
		p.ReactorBuilding.SteamGenerator.Pressure)
	assert.Equal(t, 1.0, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.SteamAdmissionValve.Open,
		"want SteamAdmissionValve open while SG has pressure")

	p.ReactorBuilding.SteamGenerator.Pressure = 0
	stepN(&p, 1, DefaultTimeStep)
	assert.False(t, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.DriveReady,
		"want SubsystemA.TurbinePump.DriveReady false after SG pressure collapses, got true (sg.P=%v)",
		p.ReactorBuilding.SteamGenerator.Pressure)
	assert.Equal(t, 0.0, p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.SteamAdmissionValve.Open,
		"want SteamAdmissionValve closed after SG pressure collapses")
}
