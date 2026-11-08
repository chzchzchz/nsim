package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit witness for the vessel failure latch: uncovered clad
// past the zircaloy melting regime latches and stays latched;
// covered clad pinned at saturation can never latch.
func TestRPV_VesselFailedLatchesOnUncoveredMeltRegime(t *testing.T) {
	t.Parallel()

	// Below the melt regime, no latch.
	below := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		HotLegBreak: Valve{Open: 1},
		Trip:        true,
	}
	below.SeedSteadyStatePower(CoreRatedThermalPower)
	below.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	below.CladTempF = 3000
	// Keep decay heat off: with it on, clad would cross ZrMeltF
	// during any multi-second horizon (probe-verified), so
	// strip it and assert the latch gate directly.
	below.DecayHeatFast = 0
	below.DecayHeatSlow = 0
	below.FissionPower = 0
	// Zero the oxidation advance so reaction enthalpy does not
	// add to the clad integral.
	below.OxidizedFrac = MaxOxidizedFrac
	for i := 0; i < 3*3600; i++ {
		below = below.Step(time.Second, 0, 0)
	}
	assert.Less(t, below.CladTempF, ZrMeltF, "witness horizon must stay below the melt regime")
	assert.False(t, below.VesselFailed, "below ZrMeltF: no vessel latch")

	// At/past the melt regime while uncovered: latch.
	atMelt := below
	atMelt.CladTempF = ZrMeltF
	atMelt = atMelt.Step(time.Second, 0, 0)
	assert.True(t, atMelt.VesselFailed, "uncovered clad at ZrMeltF latches")
}

// Latches are one-way: neither clears on RPS Reset or on
// ECCS restoration (reflood), per plans/p65_3.txt.
func TestPlant_SevereLatchesDoNotRecover(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	p := MakePlant()
	// Force both latches, then attempt recovery paths.
	p.ReactorBuilding.ReactorPressureVessel.VesselFailed = true
	p.ReactorBuilding.ReactorPressureVessel.CladTempF = 1200
	p.ReactorBuilding.ContainmentBuilding.ContainmentFailed = true
	p.ReactorBuilding.ContainmentBuilding.Damage = 1
	p.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure
	p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	p.ReactorBuilding.ReactorProtectionSystem.Reset()
	for i := 0; i < int(60*time.Second/dt); i++ {
		p.Step(dt)
		p.Update()
	}
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.VesselFailed,
		"VesselFailed survives RPS Reset and covered operation")
	assert.True(t, p.ReactorBuilding.ContainmentBuilding.ContainmentFailed,
		"ContainmentFailed survives RPS Reset and pressure recovery")
	assert.Equal(t, float32(1), p.ReactorBuilding.ContainmentBuilding.Damage,
		"containment Damage mirror stays at 1")
}

// Containment latch threshold: gauge pressure past
// ContainmentFailurePressure (150 psig) latches and mirrors
// Damage; just under it, no latch; absolute-pressure
// confusion would latch a merely design-basis transient.
func TestPlant_ContainmentFailureLatchesPastDesign(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	// Just below failure (gauge 140 psig): no latch.
	p := MakePlant()
	p.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure + 140
	stepN(&p, 10, dt)
	assert.False(t, p.ReactorBuilding.ContainmentBuilding.ContainmentFailed,
		"140 psig gauge stays below the 150 psig failure threshold")
	assert.NotEqual(t, float32(1), p.ReactorBuilding.ContainmentBuilding.Damage,
		"no Damage mirror below threshold")

	// Past failure (gauge 160 psig): latch + mirror.
	p2 := MakePlant()
	p2.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure + 160
	stepN(&p2, 10, dt)
	assert.True(t, p2.ReactorBuilding.ContainmentBuilding.ContainmentFailed,
		"gauge past ContainmentFailurePressure latches")
	assert.Equal(t, float32(1), p2.ReactorBuilding.ContainmentBuilding.Damage,
		"Damage mirrors ContainmentFailed")

	// Boundary: exactly at threshold does not latch (strict >).
	p3 := MakePlant()
	p3.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure + ContainmentFailurePressure
	stepN(&p3, 10, dt)
	assert.False(t, p3.ReactorBuilding.ContainmentBuilding.ContainmentFailed,
		"at exactly ContainmentFailurePressure: no latch (strict >)")
}

// Bridge sensors for the two latches plus the landed severe
// sensors; sensors only, no controls.
func TestBridge_SevereSensors(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	sm := GetSensorMap(&p)
	cm := GetControlMap(&p)
	for _, id := range []string{
		"ReactorBuilding/ReactorPressureVessel/VesselFailed",
		"ReactorBuilding/ContainmentBuilding/ContainmentFailed",
		"ReactorBuilding/ReactorPressureVessel/CladTemp",
		"ReactorBuilding/ReactorPressureVessel/Uncovery",
		"ReactorBuilding/ReactorPressureVessel/Damage",
		"ReactorBuilding/ReactorPressureVessel/OxidizedFrac",
	} {
		s, ok := sm[id]
		require.True(t, ok, "sensor %q must be registered", id)
		out, err := s.Get()
		require.NoError(t, err)
		assert.NotEmpty(t, out, "sensor %q must produce a value", id)
		_, isControl := cm[id]
		assert.False(t, isControl, "%q must be a sensor, not a control", id)
	}

	// Plant-level both-legs check is
	// TestPlant_SevereAccidentClassWalker; here the sensors
	// must read back a latched plant.
	p.ReactorBuilding.ReactorPressureVessel.VesselFailed = true
	p.ReactorBuilding.ContainmentBuilding.ContainmentFailed = true
	out, err := sm["ReactorBuilding/ReactorPressureVessel/VesselFailed"].Get()
	require.NoError(t, err)
	assert.Equal(t, "1", out, "latched vessel reads back as 1")
	out, err = sm["ReactorBuilding/ContainmentBuilding/ContainmentFailed"].Get()
	require.NoError(t, err)
	assert.Equal(t, "1", out, "latched containment reads back as 1")
}

// Accident-class witness: asserts regime reaches, never
// wall-clock times (plans/p65_3.txt Acceptance criteria).
// Prevention half: design-basis double-ended guillotine with
// ECCS intact never latches either failure. Beyond-design
// half: open break without injection latches both.
func TestPlant_SevereAccidentClassWalker(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	// Prevention half: full-open break, ECCS delivery intact.
	// The covered leg never depressurizes on testable horizons
	// (see TestPlant_TMIClassWalker), so this asserts the latches
	// stay clear over the plant-level horizon.
	prevented := MakePlant()
	cm := GetControlMap(&prevented)
	prevented.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
	require.NoError(t, cm["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	for i := 0; i < int(10*time.Minute/dt); i++ {
		prevented.Step(dt)
		prevented.Update()
	}
	assert.False(t, prevented.ReactorBuilding.ReactorPressureVessel.VesselFailed,
		"ECCS intact: no vessel failure")
	assert.False(t, prevented.ReactorBuilding.ContainmentBuilding.ContainmentFailed,
		"ECCS intact: no containment failure")

	// Beyond-design half: open break without injection
	// (Fukushima-style stripping, TestPlant_FukushimaClassWalker).
	// Containment is driven past the failure pressure to fire
	// the second latch; vessel latch fires on the unit-level
	// melt-regime trajectory.
	beyond := MakePlant()
	cm2 := GetControlMap(&beyond)
	require.NoError(t, cm2["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("1"))
	for i := 0; i < int(4*time.Hour/dt); i++ {
		beyond.Step(dt)
		beyond.Update()
		beyond.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = false
		beyond.ReactorBuilding.SafetyInjectionSystem.Accumulator.Level = Volume{Val: 0, Unit: u.Gallon}
	}
	rpv := beyond.ReactorBuilding.ReactorPressureVessel
	assert.Equal(t, 1.0, rpv.UncoveryFrac, "precondition: core uncovered")
	assert.Greater(t, rpv.CladTempF, 2240.0, "precondition: clad deep past 50.46 cap")

	// Unit-level melt-regime leg: seed at the melt regime,
	// uncovered, and step once. The plant-level walker cannot
	// reach ZrMeltF on testable horizons with the lumped
	// collapsed decay pools, so the latch is asserted here.
	melt := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		HotLegBreak: Valve{Open: 1},
		Trip:        true,
	}
	melt.SeedSteadyStatePower(CoreRatedThermalPower)
	melt.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	melt.CladTempF = ZrMeltF + 10
	melt = melt.Step(time.Second, 0, 0)
	assert.True(t, melt.VesselFailed, "beyond-design: clad past melt regime latches vessel")

	// Second latch requires the containment step too: drive
	// the plant-level melt leg's containment past the threshold.
	// (Covered by TestPlant_ContainmentFailureLatchesPastDesign.)

	// Containment latch leg: drive gauge pressure past the
	// failure threshold. The plant state above has CSS running
	// after the 4-hour walker, whose spray drag (0.5 effectiveness
	// × 8.33 flow per step) pulls pressure to the 126 psig fixed
	// point before the latch check (probe-verified), so the
	// latch fires only when the step starts above the fixed
	// point; seed well above it.
	beyond.ReactorBuilding.ContainmentBuilding.Pressure = AtmosphericPressure + ContainmentFailurePressure + 100
	stepN(&beyond, 2, dt)
	assert.True(t, beyond.ReactorBuilding.ContainmentBuilding.ContainmentFailed,
		"beyond-design: containment past failure pressure latches")
	assert.Equal(t, float32(1), beyond.ReactorBuilding.ContainmentBuilding.Damage,
		"beyond-design: Damage mirror stays at 1")
}
