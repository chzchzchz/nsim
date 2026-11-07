package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// dumpRig builds an isolated running-turbine building for driving the
// dump controller directly: header below the pressure-mode arm point,
// cold leg clear of the low-Tcold dump block, MSIV open.
func dumpRig(t *testing.T, setpoint float64) *TurbineBuilding {
	t.Helper()
	p := MakePlant()
	tb := p.TurbineBuilding
	tb.Turbine.State = TurbineRunning
	tb.Turbine.Velocity = TurbineRatedSpeed
	tb.Turbine.Setpoint = setpoint
	return &tb
}

func rpvTavgF(p *Plant) float64 {
	return (p.ReactorBuilding.ReactorPressureVessel.Inlet.Float() +
		p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()) / 2
}

func runDumpRig(tb *TurbineBuilding, tavg Temperature, seconds int, dt time.Duration) {
	// Above the restoration floor fraction but below the
	// pressure-mode arm point, so any opening is restoration duty
	// alone.
	sgPressure := Pressure(SteamGeneratorShellPressure * 0.99)
	for i := 0; i < int(time.Duration(seconds)*time.Second/dt); i++ {
		*tb = tb.StepWithSteam(dt, sgPressure, tavg, true, false, 1.0, 600)
	}
}

// The deep-band restoration term opens the bypass when Tavg rides
// above its load-programmed value ("Steam dump is controlled by
// signals from the RCS (essentially reactor coolant average
// temperature)",
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:70-72)
// and holds the valve shut when Tavg sits on the program: identical
// rigs differing only in the Tavg sample must diverge. The header sits
// below the pressure-mode arm point, so any opening here is restoration
// simulation state.
func TestSteamDump_RestorationTermTracksProgramExcess(t *testing.T) {
	t.Parallel()
	testDt := 10 * time.Millisecond

	hot := dumpRig(t, 0.05)
	onProgram := dumpRig(t, 0.05)
	programTavg := programmedRodTavg(0.05)

	runDumpRig(hot, Temperature{Val: programTavg + 14, Unit: u.Fahrenheit}, 20, testDt)
	runDumpRig(onProgram, Temperature{Val: programTavg, Unit: u.Fahrenheit}, 20, testDt)

	assert.Greater(t, hot.Turbine.DumpValve.Open, 0.02,
		"a Tavg excess above the program must engage the bypass, got %v",
		hot.Turbine.DumpValve.Open)
	assert.LessOrEqual(t, onProgram.Turbine.DumpValve.Open, 1e-6,
		"an on-program Tavg must hold the bypass shut, got %v",
		onProgram.Turbine.DumpValve.Open)
}

// At power the program anchor equals settled Tavg, so the restoration
// term is self-disabling there: no phantom heat-sink draw may compete
// with the governor.
func TestSteamDump_AtPowerOnProgramHoldsBypassShut(t *testing.T) {
	t.Parallel()
	testDt := 10 * time.Millisecond

	rig := dumpRig(t, 1.0)
	runDumpRig(rig, Temperature{Val: programmedRodTavg(1.0), Unit: u.Fahrenheit}, 20, testDt)

	assert.LessOrEqual(t, rig.Turbine.DumpValve.Open, 1e-6,
		"an on-program at-power lineup must hold the bypass shut, got %v",
		rig.Turbine.DumpValve.Open)
}

// In turbine lead the demanded-minus-achieved tier keys on the
// turbine's own setpoint ("The rod control system automatically
// adjusts the power level of the reactor to match the power demanded
// by the turbine",
// specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:69-71):
// commanding house load against rated fission selects boration through
// the production step path. Under reactor lead the walked integrator
// is the demand; with the operator holding a rated demand the tier
// starts quiet - neither rod position nor axial offset fires it here
// (both seeded mid-band).
func TestAutoBoron_TurbineLeadErrorKeysOnTurbineSetpoint(t *testing.T) {
	t.Parallel()
	newArmed := func(mode LeadLagMode) *Plant {
		p := fullPowerLineup()
		op := &p.PlantOperation
		op.RodControlAuto = true
		op.LeadLagMode = mode
		op.PowerSetpointDemand = 1.0
		// Engaging auto is bumpless only if the integrator starts on
		// the achieved power; a zero seed would read as a shutdown
		// demand until the walk catches up.
		op.DemandedPowerSetpoint = fissionFraction(
			p.ReactorBuilding.ReactorPressureVessel.FissionPower)
		rpv := &p.ReactorBuilding.ReactorPressureVessel
		rpv.ControlRods.Insertion = 0.045
		rpv.ControlRods.DemandInsertion = 0.045
		rpv.AxialOffset = 0
		p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true
		return &p
	}

	turbineLead := newArmed(TurbineLead)
	turbineLead.TurbineBuilding.Turbine.Setpoint = 0.05
	stepN(turbineLead, 20, DefaultTimeStep)

	cvcs := &turbineLead.AuxiliaryBuilding.ChemicalVolumeControlSystem
	assert.Equal(t, CVCSBoration, cvcs.Mode,
		"commanded 0.05 against achieved 1.00 must select boration, got %v", cvcs.Mode)
	assert.Greater(t, cvcs.AutoBoronDrive, 0.0,
		"a standing outside-band error must pace the exchange")

	// Under reactor lead the turbine setpoint must not reach the
	// tier at all - the walked integrator is the demand - so two
	// lineups differing only in that setpoint select the same mode
	// however the plant transients.
	withLoad := newArmed(ReactorLead)
	withoutLoad := newArmed(ReactorLead)
	withoutLoad.TurbineBuilding.Turbine.Setpoint = 0.05
	stepN(withLoad, 20, DefaultTimeStep)
	stepN(withoutLoad, 20, DefaultTimeStep)

	modeWith := withLoad.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode
	modeWithout := withoutLoad.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode
	assert.Equal(t, modeWith, modeWithout,
		"the turbine setpoint must not influence the tier under reactor lead")
}

// The armed full-load -> house-load descent, paced at the spec loading
// cap ("plant loading is continued to full power at a rate not to
// exceed 5 percent of full power per minute",
// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150),
// must complete without protective actuation while fission tracks the
// walked demand and the header holds its ESFAS-safe floor. The first
// attempt at this arc walked 18 percent per minute and tripped; at the
// spec pace the p48 halves track healthy through sp 0.15 (measured max
// deviation 0.043). The hold phase's terminal overshoot is item 18's
// recorded remaining blocker and is deliberately out of this window.
//
// The widened tick stretches the walk; state bands carry assertions.
func TestHouseLoadDescent_PacedWalkTracksProgramWithoutTrip(t *testing.T) {
	t.Parallel()
	testDt := 10 * time.Millisecond
	p := fullPowerLineup()
	p.PlantOperation.RodControlAuto = true
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = true
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = true

	ratePerSec := float64(TurbineLoadRunbackRate)
	walkTicks := int(0.95 / ratePerSec * float64(time.Second) / float64(testDt))
	checkpointEvery := int(30 * time.Second / testDt)

	minSgP := 1e9
	for i := 0; i < walkTicks; i++ {
		p.TurbineBuilding.Turbine.Setpoint =
			max(p.TurbineBuilding.Turbine.Setpoint-
				ratePerSec*testDt.Seconds(), 0.05)
		p.Step(testDt)
		p.Update()
		sp := p.TurbineBuilding.Turbine.Setpoint
		ff := fissionFraction(p.ReactorBuilding.ReactorPressureVessel.FissionPower)
		if i%checkpointEvery == 0 && sp >= 0.15 {
			deviation := ff - sp
			if deviation < 0 {
				deviation = -deviation
			}
			assert.LessOrEqual(t, deviation, 0.08,
				"fission must track the walked demand at sp %.3f, got %.3f", sp, ff)
		}
		sgP := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		minSgP = min(minSgP, sgP)
	}
	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.False(t, rps.ReactorTripped,
		"the spec-paced descent must not trip the reactor")
	assert.False(t, rps.SafetyInjectionActuated,
		"the spec-paced descent must not actuate SI")
	assert.GreaterOrEqual(t, minSgP,
		SteamDumpTripHeaderFloorFraction*float64(SteamGeneratorShellPressure),
		"the header must hold its floor through the walk, got %v psia", minSgP)
}
