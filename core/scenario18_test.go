package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 18 reactor trip response supervision: pressing the manual trip
// pushbutton must latch the RPS trip so all rods fully insert, the turbine
// trips off the line, house loads transfer to offsite power, and no
// engineered safeguards actuate on the plain trip.
//
//   - "Whenever a reactor trip occurs, the reactor trip breakers open and
//     all control and shutdown rods are fully inserted. The turbine and
//     turbine-generator will trip off the line with house loads
//     automatically transferred to the offsite sources of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:62-66
//   - Manual trip initiation:
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
//   - A reactor trip avoids unnecessary ESFAS actuation; Table 7-1's manual
//     entries are separate manual-ESFAS pushbuttons, not the reactor-trip
//     button.
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:11-13
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-21
func TestPlant_TripControlLatchesRPSAndDropsRods(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	stepN(&p, int(2*time.Second/TimeStep))
	require.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"precondition: plant must be untripped before the pushbutton")

	cm := GetControlMap(&p)
	require.NoError(t,
		cm["ReactorBuilding/ReactorPressureVessel/Trip"].Set("true"))
	stepN(&p, int(2*time.Second/TimeStep))

	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.True(t, rps.ReactorTripped,
		"the pushbutton must latch the RPS trip")
	assert.True(t, p.ReactorBuilding.ReactorProtectionSystem.ManualTrip)
	assert.InDelta(t, 1.0,
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion, 1e-9,
		"a trip forces full rod insertion")
	assert.Zero(t, p.TurbineBuilding.Turbine.StopValve.Open)
	assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State)
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed)
	assert.Equal(t, "SAT",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"house loads transfer to the offsite source")

	assert.False(t, rps.SafetyInjectionActuated,
		"a plain manual trip must not actuate SI")
	assert.False(t, rps.SteamlineIsolationActuated)
	assert.False(t, rps.ContainmentIsolationPhaseAActuated)
	assert.False(t, rps.ContainmentSprayActuated)
	assert.False(t, rps.FeedwaterIsolationActuated)
}

// While the turbine sits tripped, the condenser steam dump is controlled by
// the RCS average temperature: it opens for the stored heat, modulates down
// as Tavg approaches the no-load value, and recloses once Tavg is restored,
// instead of holding wide open and dragging the header below protective
// setpoints.
//
//   - "Steam dump is controlled by signals from the RCS (essentially reactor
//     coolant average temperature), and is reduced as rapidly as the reactor
//     control system reduces core power [...]; the total quantity of steam
//     dumped is minimized."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:70-77
//   - "The steam dump system will actuate if necessary and restore Tavg to
//     the no-load value."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:95-96
func TestTurbineBuilding_TripDumpModulatesOnTavgError(t *testing.T) {
	t.Parallel()
	newTripped := func() TurbineBuilding {
		return TurbineBuilding{
			Condenser: Condenser{
				Pressure:           CondenserDesignVacuum,
				VacuumPumpRunning:  true,
				CirculatingWaterIn: Temperature{Val: CondenserDesignCWIn, Unit: u.Fahrenheit},
			},
			Turbine: Turbine{
				State:              TurbineTripped,
				GoverningValve:     Valve{Open: 1},
				LubeOilPumpRunning: true,
				BearingOilPressure: BearingOilDesignPressure,
			},
		}
	}
	tavgAt := func(excessF float64) Temperature {
		return Temperature{Val: CoreReferenceInletF + excessF, Unit: u.Fahrenheit}
	}

	hot := newTripped()
	for i := 0; i < int(1*time.Second/TimeStep); i++ {
		hot = hot.StepWithSteam(SteamGeneratorShellPressure,
			tavgAt(SteamDumpTripDeadbandF+SteamDumpTripSpanF),
			true, true)
	}
	require.Greater(t, hot.Turbine.DumpValve.Open, 0.9,
		"the initial stored-heat error demands a wide-open dump")

	// With only a little headroom above the dump floor, the residual
	// Tavg excess throttles to that fraction of the slope instead of
	// overdrafting the header.
	floorF := SteamDumpTripHeaderFloorFraction * float64(SteamGeneratorShellPressure)
	lowHeader := Pressure(floorF + 0.15*
		(SteamDumpPressureModeMargin*float64(SteamGeneratorShellPressure) - floorF))
	midExcess := SteamDumpTripSpanF * 0.1
	mid := newTripped()
	for i := 0; i < int(2*time.Second/TimeStep); i++ {
		mid = mid.StepWithSteam(lowHeader,
			tavgAt(midExcess),
			true, true)
	}
	assert.Greater(t, mid.Turbine.DumpValve.Open, 0.05,
		"residual excess keeps some dump demand")
	assert.Less(t, mid.Turbine.DumpValve.Open, 0.2,
		"the dump modulates down with the shrinking excess")

	restored := mid
	for i := 0; i < int(2*time.Second/TimeStep); i++ {
		restored = restored.StepWithSteam(lowHeader,
			tavgAt(0),
			true, true)
	}
	assert.Zero(t, restored.Turbine.DumpValve.Open,
		"Tavg back at no load recloses the dump")
}
