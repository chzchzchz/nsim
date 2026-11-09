package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tier-C covered-DNB unit witnesses (plans/p73_1.txt). The ΔT
// fault line is an external driver, re-pinned before every
// Step: flow-gated inlet/outlet operators otherwise walk the
// seeded core ΔT back to reference inside the horizon. ΔI is
// held at zero because |ΔI| alone cannot cross DNBRFireLimit.
// The live plant trajectory cannot reach DNBR < 1 inside the
// witness band (50 °F/hr envelope; feasibility section), so no
// claim is made about live-coolant timing.

const dnbTestDTF = 140.0

// Seed a covered, full-power vessel with the DNB fault line in
// place. Rod insertion is solved for zero reactivity so the
// feedback does not trim power away from full rating.
func seedCoveredDNB(t *testing.T) ReactorPressureVessel {
	t.Helper()
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceInletF + dnbTestDTF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	require.Less(t, dnbrMargin(&r), DNBRFireLimit,
		"precondition: seeded ΔT must cross DNBRFireLimit")
	return r
}

// Re-pin the Tier-C fault line on the vessel carried into Step.
func repinDNB(rv *ReactorPressureVessel) {
	rv.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	rv.Outlet = Temperature{Val: CoreReferenceInletF + dnbTestDTF, Unit: u.Fahrenheit}
}

// Solve the rod insertion that nulls the reactivity balance at
// the seeded outlet temperature, so fission power stays at full
// rating over the witness horizon.
func nullReactivityInsertion(outletF float64) float64 {
	deltaF := min(max(outletF, DopplerClampLowF), DopplerClampHighF) - float64(CoreDopplerReferenceTempF)
	feedback := (deltaF*CoreDopplerBasePcmPerDegF)/1000.0 + deltaF*netTempCoef(CVCSInitialBoron)/1000.0
	return -feedback / (float64(CoreRodWorthABCD) * 100)
}

func TestRPV_CoveredDNBBurnsOutWithinBand(t *testing.T) {
	t.Parallel()
	r := seedCoveredDNB(t)
	r.ControlRods.Insertion = nullReactivityInsertion(r.Outlet.Float())
	require.InDelta(t, 0.0, float64(r.ControlRods.Insertion)*float64(CoreRodWorthABCD)*100+
		((r.Outlet.Float()-float64(CoreDopplerReferenceTempF))*CoreDopplerBasePcmPerDegF)/1000.0+
		((r.Outlet.Float()-float64(CoreDopplerReferenceTempF))*netTempCoef(CVCSInitialBoron))/1000.0,
		1e-9, "precondition: reactivity is nulled at the seed point")

	const dt = time.Second
	var tCross, tFull time.Duration
	for i := 1; i <= 600; i++ {
		repinDNB(&r)
		r = r.Step(dt, RCPRatedSpeed, 0, 0)
		require.InDelta(t, CoreReferenceInletF+dnbTestDTF, r.Outlet.Float(), 1.0,
			"precondition: re-pinned ΔT holds the fault line")
		if tCross == 0 && r.CoreDamageFrac > 0 {
			tCross = time.Duration(i) * dt
		}
		if tFull == 0 && r.CoreDamageFrac >= 1 {
			tFull = time.Duration(i) * dt
			break
		}
	}
	require.NotZero(t, tCross, "covered burnout must cross into damage")
	require.NotZero(t, tFull, "covered burnout must reach the 50.46 cap")
	assert.GreaterOrEqual(t, tCross, 25*time.Second, "no faster than the derived bound")
	assert.LessOrEqual(t, tFull, 10*time.Minute, "within the testability band, got %v", tFull)
	assert.Equal(t, 1.0, r.CoreDamageFrac)
	assert.Equal(t, float32(1), r.Damage)
	assert.False(t, r.VesselFailed, "covered DNB does not latch the vessel")
	assert.Zero(t, r.OxidizedFrac, "no steam blanket: no oxidation advance")
	assert.Zero(t, r.UncoveryFrac, "this witness is covered throughout")
}

func TestRPV_DNBDamageDoesNotHealOnRecovery(t *testing.T) {
	t.Parallel()
	r := seedCoveredDNB(t)
	r.ControlRods.Insertion = nullReactivityInsertion(r.Outlet.Float())
	r.CoreDamageFrac = 1
	r.Damage = 1

	r.Inlet = Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit}
	r.Outlet = Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit}
	for i := 0; i < 5; i++ {
		r = r.Step(time.Second, RCPRatedSpeed, 0, 0)
	}
	satF := PressurizerSatTemperatureF(float64(r.Pressure))
	require.InDelta(t, satF, r.CladTempF, 1, "covered leg re-pins to saturation on recovery")
	assert.Equal(t, 1.0, r.CoreDamageFrac, "damage is one-way after DNB recovery")
	assert.Equal(t, float32(1), r.Damage)
	assert.False(t, r.VesselFailed)
}

func TestRPV_DNBDoesNotFireBelowAtPowerPermissive(t *testing.T) {
	t.Parallel()
	r := seedCoveredDNB(t)
	r.FissionPower = CoreNeutronSourcePower
	r.DecayHeatFast = 0
	r.DecayHeatSlow = 0

	for i := 0; i < 10; i++ {
		repinDNB(&r)
		r = r.Step(time.Second, RCPRatedSpeed, 0, 0)
	}
	require.Less(t, dnbrMargin(&r), DNBRFireLimit, "precondition: ΔT holds past the threshold")
	satF := PressurizerSatTemperatureF(float64(r.Pressure))
	assert.Equal(t, satF, r.CladTempF, "clad pinned at saturation below the permissive")
	assert.Zero(t, r.CoreDamageFrac)
}

func TestRPV_UncoveryWinsDNBOverlap(t *testing.T) {
	t.Parallel()
	r := seedCoveredDNB(t)
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.9, Unit: u.Gallon}
	r.ControlRods.Insertion = nullReactivityInsertion(r.Outlet.Float())
	r.CladTempF = ZrMeltF

	repinDNB(&r)
	r = r.Step(time.Second, RCPRatedSpeed, 0, 0)
	assert.Greater(t, r.UncoveryFrac, 0.0, "overlap tick is uncovered")
	assert.Greater(t, r.OxidizedFrac, 0.0, "uncovered leg takes the steam reaction")
	assert.True(t, r.VesselFailed, "uncovered leg still latches the vessel")
	assert.Equal(t, 1.0, r.CoreDamageFrac, "single damage readout, not a double count")
}

func TestRPV_WarmStartZeroFlowNoDNBRAtReference(t *testing.T) {
	t.Parallel()
	// At reference ΔT the margin holds DNBRRefMargin, so true
	// zero flow cannot auto-inflate: flowDivisor is 1.0 at
	// totalFlow == 0, not the degraded-floor clamp.
	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
	}
	r.SeedSteadyStatePower(CoreRatedThermalPower)
	for i := 0; i < 20; i++ {
		r = r.Step(time.Second, 0, 0, 0)
		assert.GreaterOrEqual(t, dnbrMargin(&r), DNBRFireLimit,
			"reference ΔT holds the margin at zero flow")
		assert.Zero(t, r.CoreDamageFrac)
	}
}

func TestHDS_CoveredDNBDNBProducesNoHydrogen(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	rpv := &p.ReactorBuilding.ReactorPressureVessel
	rpv.ControlRods = ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron}
	rpv.SeedSteadyStatePower(CoreRatedThermalPower)
	rpv.ControlRods.Insertion = nullReactivityInsertion(rpv.Outlet.Float())
	rpv.CladTempF = CladOnsetF + 500
	stepN(&p, 20, 20*time.Millisecond)
	assert.Zero(t, rpv.UncoveryFrac, "precondition: the excursion stays covered")
	assert.Zero(t, rpv.OxidizedFrac, "covered excursion oxidizes on neither leg")
	assert.Zero(t,
		p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent,
		"no H2 is derived from a covered DNB excursion")
}
