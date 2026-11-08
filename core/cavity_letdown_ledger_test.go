package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Refueling-cavity drain debits the primary inventory ledger:
// draining the cavity pool above the head removes primary-side water
// and must grow BreakDischarged (non-creditable) so a held-open drain
// uncovers the core — at RefuelingCavityDrainRate *
// RefuelingCavityDepthGallonsPerFoot / 60 gal/s, capped at
// RCSDrainableInventoryGal, without touching BreakFlow (the liquid
// does not flash to containment).
//
//   - "Spent fuel is handled underwater from the time it leaves
//     the reactor vessel until it is placed in a cask for shipment
//     from the site. The water provides an effective, economical,
//     and transparent radiation shield as well as a reliable medium
//     for decay heat removal."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:14-18
//   - "it forms a pool above the reactor. The cavity is filled to a
//     depth of approximately 24 feet (731 cm) of water when a fuel
//     assembly is being transferred between the reactor and the fuel
//     transfer system."
//     specs/the_westinghouse_pressurized_water_react/chapter_13/page_172.txt:43-46
func TestRefuel_CavityDrainDebitsLedger(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhaseReassembly
	p.ReactorBuilding.RefuelingOperations.CavityFloodDepth = RefuelingCavityMaxDepth
	p.ReactorBuilding.RefuelingOperations.CavityFlooded = true
	p.ReactorBuilding.RefuelingOperations.CavityDraining = true
	// Isolate the cavity leg from any other ledger writers: charging
	// off and letdown closed so netCVCS cannot debit mid-window.
	// (SI need not be blocked: this lineup produces no SI signal,
	// so creditSafetyInjection is never called anyway.)
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false

	before := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	window := 30 * time.Second
	for i := 0; i < int(window/dt); i++ {
		p.Step(dt)
		p.Update()
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	got := rpv.BreakDischarged.Float()
	require.Greater(t, got, before,
		"cavity drain must debit the inventory ledger")
	want := RefuelingCavityDrainRate * window.Seconds() / 60.0 *
		RefuelingCavityDepthGallonsPerFoot
	assert.InDelta(t, before+want, got, 1e-6,
		"cavity drain debits at drain rate * gallons-per-foot")
	assert.InDelta(t, got, rpv.BreakDischargedNonCreditable.Float(), 1e-6,
		"a cavity-only drain is entirely non-creditable")
	assert.Zero(t, rpv.BreakFlow.Float(),
		"cavity drain is liquid transfer, not a LOCA flash")
	assert.Less(t, p.ReactorBuilding.RefuelingOperations.CavityFloodDepth,
		RefuelingCavityMaxDepth,
		"cavity depth must fall while the drain is commanded")
	assert.LessOrEqual(t, got, RCSDrainableInventoryGal+1e-6,
		"ledger stays capped at drainable inventory")
}

// Letdown-without-charging debits the primary inventory ledger: with
// LetdownValveOpen=true and ChargingPumpRunning=false, the CVCS
// net-negative balance drains the RCS to the VCT and must grow
// BreakDischarged at |netCVCS|/dt per tick, reaching the
// non-creditable floor.
//
// "Under steady-state operation, pressurizer level represents a
// balance of injection water flow into the RCS and water letdown
// flow into the Chemical and Volume Control System."
// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:62-65
func TestPlant_LetdownWithoutChargingDebitsLedger(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	// Hold the drain boundary open long enough to integrate a
	// non-trivial volume; each tick's netCVCS lands in the ledger
	// the same tick (the debit sits before the RPV step).
	before := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	window := 30 * time.Second
	for i := 0; i < int(window/dt); i++ {
		p.Step(dt)
		p.Update()
	}
	rpv := p.ReactorBuilding.ReactorPressureVessel
	got := rpv.BreakDischarged.Float()
	require.Greater(t, got, before,
		"letdown-without-charging must debit the inventory ledger")
	require.LessOrEqual(t, got, RCSDrainableInventoryGal,
		"ledger stays capped at drainable inventory")
	nonCred := rpv.BreakDischargedNonCreditable.Float()
	require.Greater(t, nonCred, 0.0,
		"letdown debit must raise the non-creditable floor")
	assert.InDelta(t, got, nonCred, 1e-6,
		"a letdown-only drain is entirely non-creditable")

	// At-cap control: a ledger already at the cap does not grow
	// further from the same lineup.
	p2 := MakePlant()
	p2.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	p2.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p2.ReactorBuilding.ReactorPressureVessel.BreakDischarged = Volume{
		Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	p2.ReactorBuilding.ReactorPressureVessel.BreakDischargedNonCreditable = Volume{
		Val: RCSDrainableInventoryGal, Unit: u.Gallon}
	stepN(&p2, int(5*time.Second/dt), dt)
	assert.InDelta(t, RCSDrainableInventoryGal,
		p2.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float(), 1e-9,
		"at-cap ledger must not overshoot")
}

// CavityDraining outside the reassembly phase does not debit the
// ledger: the state machine only drops depth there, so debiting in
// any other phase would invent gallons the pool never lost (CHECK §2
// exhaustion / class 7). The pool sits full (24 ft) and draining is
// armed, but the phase is preparation — ledger must stay flat.
// Cavity debit is tick-exact against the actual depth drop the
// state machine performs: after each Plant.Step the ledger gain
// equals (depthBefore-depthAfter) * gallons-per-foot for that tick
// — never a rate-times-tick snapshot that can lag or lead the pool.
func TestRefuel_CavityDrainTickExactAgainstDepthDrop(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhaseReassembly
	p.ReactorBuilding.RefuelingOperations.CavityFloodDepth = RefuelingCavityMaxDepth
	p.ReactorBuilding.RefuelingOperations.CavityDraining = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false

	for i := 0; i < 10; i++ {
		depthBefore := p.ReactorBuilding.RefuelingOperations.CavityFloodDepth
		ledgerBefore := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
		p.Step(dt)
		p.Update()
		depthAfter := p.ReactorBuilding.RefuelingOperations.CavityFloodDepth
		ledgerAfter := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
		want := (depthBefore - depthAfter) * RefuelingCavityDepthGallonsPerFoot
		assert.InDelta(t, want, ledgerAfter-ledgerBefore, 1e-9,
			"tick debit must equal the observed depth drop")
	}
}

func TestRefuel_CavityDrainOutsideReassemblyDoesNotDebit(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhasePreparation
	p.ReactorBuilding.RefuelingOperations.CavityFloodDepth = RefuelingCavityMaxDepth
	p.ReactorBuilding.RefuelingOperations.CavityDraining = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false

	before := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	stepN(&p, int(5*time.Second/dt), dt)
	got := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.InDelta(t, before, got, 1e-9,
		"cavity drain outside reassembly must not debit the ledger")
}

// Cavity drain cannot debit more gallons than the pool actually held:
// with depth seeded near empty (0.5 ft) and the drain held open, the
// total debit is bounded by depth x gallons-per-foot, then zero once
// empty — a held-open drain must stop inventing mass (CHECK §2: bound
// cumulative draws against the drained system's own inventory anchor).
func TestRefuel_CavityDrainBoundedByPoolInventory(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhaseReassembly
	p.ReactorBuilding.RefuelingOperations.CavityFloodDepth = 0.5
	p.ReactorBuilding.RefuelingOperations.CavityDraining = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false

	before := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	// Run far past the 30-second empty point (0.5 ft at 1 ft/min).
	stepN(&p, int(120*time.Second/dt), dt)
	got := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	wantCap := 0.5 * RefuelingCavityDepthGallonsPerFoot
	// Exact-value equality carries the bound: a code path that
	// manufactured gallons past the pool's 0.5 ft would land strictly
	// above before+wantCap and fail; LessOrEqual against the same
	// expected value would still pass on phantom mass up to the bound.
	assert.InDelta(t, before+wantCap, got, 1e-6,
		"total cavity debit is bounded by the pool's own inventory")
	// Once empty, stepping further must not grow the ledger.
	mid := got
	stepN(&p, int(30*time.Second/dt), dt)
	after := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.InDelta(t, mid, after, 1e-9,
		"empty cavity must deliver exactly zero further debit")
}

// Letdown debit cannot exceed the pressurizer's usable inventory:
// seed the pressurizer nearly dry and letdown-only; the ledger grows
// at most by the pre-step water, then stops — it never drains a dry
// pressurizer forever (CHECK §2: draw path sized against remaining
// inventory, not capacity).
func TestPlant_LetdownDebitBoundedByPressurizerInventory(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	// Nearly dry: a 10 gal seed bounds any letdown debit.
	p.ReactorBuilding.Pressurizer.Water = Volume{Val: 10, Unit: u.Gallon}

	before := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	stepN(&p, int(60*time.Second/dt), dt)
	got := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.LessOrEqual(t, got, before+10+1e-6,
		"letdown debit is bounded by the pressurizer's usable water")

	// At empty: no further growth over the next window.
	mid := got
	stepN(&p, int(30*time.Second/dt), dt)
	after := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.InDelta(t, mid, after, 1e-9,
		"dry pressurizer must deliver exactly zero further letdown debit")
}

// Cavity drain and letdown share the single drainable-inventory cap:
// once the ledger is at the cap, neither path pushes it past
// RCSDrainableInventoryGal.
func TestPlant_CavityAndLetdownShareLedgerCap(t *testing.T) {
	t.Parallel()
	dt := 50 * time.Millisecond

	p := MakePlant()
	p.ReactorBuilding.RefuelingOperations.Phase = RefuelingPhaseReassembly
	p.ReactorBuilding.RefuelingOperations.CavityFloodDepth = RefuelingCavityMaxDepth
	p.ReactorBuilding.RefuelingOperations.CavityDraining = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.ReactorBuilding.ReactorPressureVessel.BreakDischarged = Volume{
		Val: RCSDrainableInventoryGal - 5, Unit: u.Gallon}
	p.ReactorBuilding.ReactorPressureVessel.BreakDischargedNonCreditable = Volume{
		Val: RCSDrainableInventoryGal - 5, Unit: u.Gallon}

	for i := 0; i < int(10*time.Second/dt); i++ {
		p.Step(dt)
		p.Update()
	}
	got := p.ReactorBuilding.ReactorPressureVessel.BreakDischarged.Float()
	assert.InDelta(t, RCSDrainableInventoryGal, got, 1e-6,
		"cavity + letdown share one capped ledger")
}
