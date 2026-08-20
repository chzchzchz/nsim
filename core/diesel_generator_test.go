package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
)

// enoughFuel returns a DieselGenerator with full day-tank fuel so the
// engine can complete a start sequence without tripping on fuel.
func enoughFuel() DieselGenerator {
	return DieselGenerator{
		Fuel: Tank{
			Level:    u.NewValue(500, u.Gallon),
			Capacity: u.NewValue(500, u.Gallon),
			Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
		},
		CoolantTemp: u.NewValue(80, u.Fahrenheit),
	}
}

// Setting StartSignal to a non-None cause starts the engine without
// operator action (SafetyInjection is the spec's most-cited cause).
// Clearing it back to None after the engine is running eventually returns
// it to Off.
func TestDieselStartSafetyInjection(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = enoughFuel()
	p.Outside.DieselGenerator.StartSignal = DieselStartSafetyInjection

	stepN(&p, int(10*time.Second/TimeStep))
	if p.Outside.DieselGenerator.State != DieselRunning {
		t.Fatalf("want DieselRunning via SI, got %v", p.Outside.DieselGenerator.State)
	}

	p.Outside.DieselGenerator.StartSignal = DieselStartNone
	stepN(&p, int(30*time.Second/TimeStep))
	if p.Outside.DieselGenerator.State != DieselOff {
		t.Fatalf("want DieselOff after clearing signal, got %v", p.Outside.DieselGenerator.State)
	}
}

// 10 s after the start signal, the generator side has built up to rated
// voltage and frequency, the breaker has closed, and RealPower matches
// rated kW.
func TestDieselGeneratorReachesRatedOutputIn10s(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = enoughFuel()
	p.Outside.DieselGenerator.StartSignal = DieselStartManual

	stepN(&p, int(10*time.Second/TimeStep))

	g := p.Outside.DieselGenerator.Generator
	if !g.BreakerClosed {
		t.Fatalf("want BreakerClosed after 10 s, got false")
	}
	if g.Voltage < DieselRatedVoltage*0.99 {
		t.Fatalf("want Voltage ~= %v, got %v", DieselRatedVoltage, g.Voltage)
	}
	if g.Frequency < DieselRatedFrequency*0.99 {
		t.Fatalf("want Frequency ~= %v, got %v", DieselRatedFrequency, g.Frequency)
	}
	if g.RealPower != DieselRatedkW {
		t.Fatalf("want RealPower %v at full load, got %v", DieselRatedkW, g.RealPower)
	}
}

// With offsite power unavailable, the ESF bus is energised by the diesel
// within ~10 s of StartSignal, and the motor-driven AFW pump's
// DriveReady follows the bus.
func TestESFBusEnergizedByDiesel(t *testing.T) {
	p := MakePlant()
	p.ServiceWaterPump.Running = true
	p.Outside.DieselGenerator = enoughFuel()

	if p.Electrical.ESFBusA.Energized {
		t.Fatal("precondition: ESFBusA should start unenergized")
	}

	p.Outside.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, int(10*time.Second/TimeStep))

	if !p.Electrical.ESFBusA.Energized {
		t.Fatalf("want ESFBusA.energized=true after 10 s, got false (voltage=%v)", p.Electrical.ESFBusA.Voltage)
	}
	if p.Electrical.ESFBusA.Voltage < DieselRatedVoltage*0.99 {
		t.Fatalf("want ESFBusA.voltage ~= %v, got %v", DieselRatedVoltage, p.Electrical.ESFBusA.Voltage)
	}
	if !p.Outside.MotorAFWPump.DriveReady {
		t.Fatalf("want MotorAFWPump.DriveReady=true after ESFBusA energized, got false")
	}
}

// With the diesel not running, the operator's intent (Running=true) is
// preserved on the diesel-driven AFW pump, but DriveReady=false means
// fluid flow (computed by the consumer) is gated to zero.
func TestDieselAFWPumpGatedByDiesel(t *testing.T) {
	p := MakePlant()
	p.Outside.DieselGenerator = enoughFuel()
	p.Outside.DieselAFWPump.Running = true // operator request
	p.Outside.DieselAFWPump.DriveSource = DriveDiesel

	stepN(&p, 1)
	if !p.Outside.DieselAFWPump.Running {
		t.Fatal("want DieselAFWPump.Running preserved (operator intent) when diesel not ready")
	}
	if p.Outside.DieselAFWPump.DriveReady {
		t.Fatal("want DieselAFWPump.DriveReady false before diesel start")
	}

	p.Outside.DieselGenerator.StartSignal = DieselStartManual
	stepN(&p, int(10*time.Second/TimeStep))
	if !p.Outside.DieselAFWPump.DriveReady {
		t.Fatal("want DieselAFWPump.DriveReady true after diesel ReadyToLoad")
	}
	if !p.Outside.DieselAFWPump.Running {
		t.Fatal("want DieselAFWPump.Running preserved once DriveReady")
	}
}
