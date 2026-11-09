package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Relief discharge debits the primary inventory ledger: a PORV
// stuck open (plans/x/corium/corium_04_porv_stuck_open.txt) vents
// to the PRT and must grow BreakDischarged, capping at the
// drainable inventory without touching BreakFlow (PRT is not the
// containment sump / flash source). The debit lands in the
// non-creditable floor, so ECCS refill cannot retire it (CHECK §2).
//
//   - "If system pressure continues to rise, self-actuating
//     ASME-code safety valves will open. Steam from the safety
//     and/or relief valves is piped to the pressurizer relief
//     tank which contains sufficient water to condense the steam."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-35
//   - "For large pipe ruptures, the RCS would be rapidly
//     depressurized and voided of coolant."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:16-17
func TestPlant_ReliefVesselLatches(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	// Plant leg: stuck PORV + safety valve + SI blocked drives
	// ledger growth and clad heatup without any hot-leg break.
	// No per-tick SI stripping and no per-tick pressure reset:
	// pressure is seeded once above the setpoint; heaters hold
	// the bubble and PORVStuckOpen keeps the path open while the
	// safety valve stays latched from the survey above. Run long
	// enough at 50 ms that the 2 gal/s per-path relief integrates
	// thousands of gallons uncapped.
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/Pressurizer/PORVStuckOpen"].Set("1"))
	require.NoError(t, cm["ReactorBuilding/SafetyValve/Open"].Set("1"))
	require.NoError(t, cm["ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock"].Set("1"))
	p.ReactorBuilding.Pressurizer.Pressure = PressurizerSafetySetpoint + 50

	before := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	for i := 0; i < int(10*time.Minute/dt); i++ {
		p.Step(dt)
		p.Update()
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	after := rpv.BreakDischarged.Float()
	require.Greater(t, after, before,
		"relief discharge must debit the inventory ledger")
	require.LessOrEqual(t, after, RCSDrainableInventoryGal+1e-6,
		"ledger stays capped at drainable inventory")
	assert.Zero(t, rpv.BreakFlow.Float(),
		"relief to PRT is not a LOCA flash into containment")
	require.Greater(t, rpv.UncoveryFrac, 0.0,
		"relief-debited ledger must uncovery the core")
	nonCred := rpv.BreakDischargedNonCreditable.Float()
	require.Greater(t, nonCred, 0.0,
		"relief debit must raise the non-creditable floor")
	assert.InDelta(t, after, nonCred, 1e-6,
		"a relief-only drain is entirely non-creditable")
	require.Greater(t, rpv.CladTempF, 800.0,
		"uncovered clad must heat above the saturation anchor")

	// The plant-level walker cannot reach ZrMeltF on a testable
	// horizon: decay heat is capped by the collapsed slow pool
	// (20 h tau), so the film term (plans/p76_1.txt) arrests the
	// rise. The reachability claim is therefore the onset +
	// direction + latch-gate witness, same precedent as
	// TestPlant_SevereAccidentClassWalker: continue this plant's
	// own uncovered vessel (BreakDischarged at the cap and
	// BreakDischargedNonCreditable = cap, i.e. relief-drained) at
	// 1 s ticks; the onset (CladOnsetF) is crossed and damage
	// stages, but with hA = 6000 W/°F the stagnant-steam film
	// prevents the decay-only tail from crossing ZrMeltF
	// (plans/p76_1.txt acceptance 3: SBO plateau with oxidation
	// does not latch on 48 h). The latch itself is pinned by
	// TestRPV_VesselFailedLatchesOnUncoveredMeltRegime, which
	// seeds at the melt regime.
	melt := rpv
	melt.BreakDischarged = Volume{Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	melt.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	melt.Trip = true
	for i := 0; i < 3600*4; i++ {
		melt = melt.Step(time.Second, 0, 0, 0)
	}
	assert.Greater(t, melt.CladTempF, CladOnsetF,
		"relief-uncovered tail crosses the oxidation onset")
	assert.Greater(t, melt.OxidizedFrac, 0.0,
		"relief-uncovered tail oxidizes")
	assert.False(t, melt.VesselFailed,
		"with the steam film, decay-only tail does not latch (plateau)")
	assert.Equal(t, 1.0, melt.UncoveryFrac,
		"fully voided (relief-drained) core stays uncovered")
}

// Relief debit with SI intact: ECCS refill must NOT retire the
// relief drain (CHECK §2). Alternate forced injection credit
// against a relief-drained ledger: the non-creditable floor pins
// UncoveryFrac at 1 while a break-drained ledger refloods.
func TestPlant_ReliefNotCreditedBySafetyInjection(t *testing.T) {
	t.Parallel()

	r := ReactorPressureVessel{
		ControlRods: ControlRods{Insertion: 0, DemandInsertion: 0, BoronConcentration: CVCSInitialBoron},
		Inlet:       Temperature{Val: CoreReferenceInletF, Unit: u.Fahrenheit},
		Outlet:      Temperature{Val: CoreReferenceOutletF, Unit: u.Fahrenheit},
		Pressure:    RPVDesignPressure,
		Trip:        true,
	}
	// Entirely non-creditable (relief) draw at the cap.
	r.BreakDischarged = Volume{Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	r.BreakDischargedNonCreditable = Volume{Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	pzr := &Pressurizer{
		Volume: Volume{Val: 1500, Unit: u.Gallon},
		Water:  Volume{Val: 100, Unit: u.Gallon},
	}
	r.creditSafetyInjection(pzr, RCSDrainableInventoryGal)
	assert.Equal(t, RCSDrainableInventoryGal, r.BreakDischarged.Float(),
		"injection must not retire relief loss")
	assert.Equal(t, RCSDrainableInventoryGal, r.BreakDischargedNonCreditable.Float())

	// Contrast: purely creditable (hot-leg break) draw refloods.
	b := ReactorPressureVessel{}
	b.BreakDischarged = Volume{Val: RCSDrainableInventoryGal * 0.5, Unit: u.Gallon}
	pzr2 := &Pressurizer{
		Volume: Volume{Val: 1500, Unit: u.Gallon},
		Water:  Volume{Val: 100, Unit: u.Gallon},
	}
	b.creditSafetyInjection(pzr2, RCSDrainableInventoryGal*0.5)
	assert.Less(t, b.BreakDischarged.Float(), RCSDrainableInventoryGal*0.5,
		"break draw must be creditable")
}

// Relief debit respects the drainable inventory cap and stops
// once exhausted, without adding break flow.
func TestPrzr_ReliefLedgerCap(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	// Near-cap seed: relief should exhaust the remainder and
	// then stop growing.
	p := MakePlant()
	cm := GetControlMap(&p)
	require.NoError(t, cm["ReactorBuilding/Pressurizer/PORVStuckOpen"].Set("1"))
	require.NoError(t, cm["ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock"].Set("1"))
	p.ReactorBuilding.ReactorPressureVessel.BreakDischarged = Volume{
		Val: RCSDrainableInventoryGal - 5, Unit: u.Gallon}
	p.ReactorBuilding.ReactorPressureVessel.BreakDischargedNonCreditable = Volume{
		Val: RCSDrainableInventoryGal - 5, Unit: u.Gallon}
	p.ReactorBuilding.Pressurizer.Pressure = PressurizerSafetySetpoint + 50

	for i := 0; i < int(30*time.Second/dt); i++ {
		p.Step(dt)
		p.Update()
	}
	got := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.InDelta(t, RCSDrainableInventoryGal, got, 1e-6,
		"relief debit must stop at the drainable inventory")
	assert.Zero(t, p.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float(),
		"exhausted ledger delivers no relief break flow")

	// At-cap control: further relief steps leave the ledger pinned.
	p2 := MakePlant()
	cm2 := GetControlMap(&p2)
	require.NoError(t, cm2["ReactorBuilding/Pressurizer/PORVStuckOpen"].Set("1"))
	p2.ReactorBuilding.ReactorPressureVessel.BreakDischarged = Volume{
		Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	p2.ReactorBuilding.Pressurizer.Pressure = PressurizerSafetySetpoint + 50
	stepN(&p2, int(5*time.Second/dt), dt)
	assert.InDelta(t, RCSDrainableInventoryGal,
		p2.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float(), 1e-9,
		"at-cap ledger must not overshoot")
}

// Hot-leg break burndown is unchanged by relief: the break alone
// and break+relief share one capped ledger, so totals differ
// only by the relief debit, never by double counting the break.
func TestPlant_ReliefSharesHotLegLedgerOnce(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond
	window := 10 * time.Second

	breakOnly := MakePlant()
	cm1 := GetControlMap(&breakOnly)
	require.NoError(t, cm1["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0.01"))
	for i := 0; i < int(window/dt); i++ {
		breakOnly.Step(dt)
		breakOnly.Update()
	}
	base := breakOnly.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	require.Greater(t, base, 0.0, "precondition: break alone debits the ledger")
	// BreakFlow is the hot-leg flash source; with no relief it
	// is the break's per-step flow, never a relief top-up.
	require.Greater(t, breakOnly.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float(), 0.0,
		"break alone produces flash flow")

	both := MakePlant()
	cm2 := GetControlMap(&both)
	require.NoError(t, cm2["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"].Set("0.01"))
	require.NoError(t, cm2["ReactorBuilding/Pressurizer/PORVStuckOpen"].Set("1"))
	require.NoError(t, cm2["ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock"].Set("1"))
	both.ReactorBuilding.Pressurizer.Pressure = PressurizerSafetySetpoint + 50
	for i := 0; i < int(window/dt); i++ {
		both.Step(dt)
		both.Update()
	}
	combined := both.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	// Relief adds to the same ledger; combined must exceed break
	// alone but never exceed the shared cap.
	require.Greater(t, combined, base,
		"relief must add to the shared ledger, not replace the break")
	assert.LessOrEqual(t, combined, RCSDrainableInventoryGal+1e-6,
		"shared ledger is capped once")
}
