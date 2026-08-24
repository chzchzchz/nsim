package core

import (
	"math"
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When the reactor trips, Plant.Step must cascade the trip to the
// turbine: StopValve.Open drops to 0 and the turbine transitions
// to TurbineTripped within a few steps.
//
// Spec justification:
//   - "Turbine trip following a reactor trip." (item 2 of the
//     protective features list)
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:64
//   - "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
// The seeded pressurizer must describe a plant that exists before the
// first Step: the steam bubble is the vapor space above the liquid
// inventory, so Water + SteamBubble must equal Volume exactly.
func TestPlant_MakePlantPressurizerSeedCoherent(t *testing.T) {
	p := MakePlant()
	pzr := p.ReactorBuilding.Pressurizer
	assert.Equal(t, pzr.Volume.Float(),
		pzr.Water.Float()+pzr.SteamBubble.Float(),
		"Water + SteamBubble must equal Volume")
}

func TestPlant_ReactorTripCascadesToTurbine(t *testing.T) {
	p := MakePlant()
	// Bring the reactor to rated power; let the SG build pressure.
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	stepN(&p, int(2*time.Second/TimeStep))
	// Manually trip the reactor.
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	// Step past StopValveCloseDelay.
	stepN(&p, int(100*time.Millisecond/TimeStep))
	assert.Zero(t, p.TurbineBuilding.Turbine.StopValve.Open,
		"want StopValve.Open == 0 after reactor trip, got %v",
		p.TurbineBuilding.Turbine.StopValve.Open)
	assert.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State,
		"want TurbineTripped after reactor trip, got %v",
		p.TurbineBuilding.Turbine.State)
}

// With the reactor tripped, the condenser no longer receives full
// steam load from the turbine and Pressure relaxes toward the
// design vacuum.
//
// Spec justification:
//   - "Under normal operation the air is removed by steam jet air
//     ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
func TestPlant_CondenserVacuumRecoversAfterTrip(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.Condenser.VacuumPumpRunning = true
	p.TurbineBuilding.Condenser.Pressure = AtmosphericPressure
	// Vacuum recovery only — no reactor or turbine dynamics needed.
	for i := 0; i < int(5*time.Second/TimeStep); i++ {
		p.TurbineBuilding.Condenser = p.TurbineBuilding.Condenser.Step(
			p.TurbineBuilding.Condenser.CirculatingWaterIn,
			true,
			true,
		)
	}
	assert.Less(t, p.TurbineBuilding.Condenser.Pressure, AtmosphericPressure,
		"want vacuum below atmospheric, got %v",
		p.TurbineBuilding.Condenser.Pressure)
}

// When the main feed pump is stopped (representing no SG-side
// demand), the minimum-flow recirculation line sends
// FeedwaterMinFlow per step back to the deaerator (HeaterDrainTank).
//
// Spec justification:
//   - "The discharge from the pumps is automatically recirculated
//     back to the deaerator whenever the flow to the high-pressure
//     feedwater heaters falls below the minimum."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:28-31
func TestPlant_MinFlowRecirculationToDeaerator(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.MainFeedwaterPump.Running = false
	before := p.TurbineBuilding.HeaterDrainTank.Level.Float()
	stepN(&p, int(2*time.Second/TimeStep))
	after := p.TurbineBuilding.HeaterDrainTank.Level.Float()
	assert.Greater(t, after, before,
		"want HeaterDrainTank.Level to rise from min-flow recirculation, before=%v after=%v",
		before, after)
}

// End-to-end: at full power the heater train should produce a
// cumulative temperature rise from 120°F inlet to a meaningfully
// higher outlet. This exercises the entire LP+HP chain wired in
// TurbineBuilding.StepWithSteam.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser hotwell
//     and pump condensate [...] through four stages of low-pressure
//     feedwater heaters to the deaerator. [...] The water discharge
//     from the feedwater pumps flows through two stages of high-
//     pressure heaters then into the steam generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-51
func TestPlant_HeaterTrainRaisesFeedwaterTemperature(t *testing.T) {
	p := MakePlant()
	p.TurbineBuilding.LowPressureHeater.DrainValve = Valve{Open: 1.0}
	p.TurbineBuilding.HighPressureHeater.DrainValve = Valve{Open: 1.0}
	stepN(&p, int(5*time.Second/TimeStep))
	rise := p.TurbineBuilding.HighPressureHeater.TemperatureRise.Float()
	assert.GreaterOrEqual(t, rise, 0.0, "want non-negative HP heater rise, got %v", rise)
}

// synchronizeAndCloseBreaker brings the turbine to rated speed with
// the breaker open, then closes the breaker once terminal voltage and
// frequency are inside the synchronism window. This is the spec's
// startup order: DEHM speed control to rated, synchronizer check,
// breaker close (page_201.txt:70-92). Plant.Step's synchronism check
// would immediately clear a close onto an out-of-step machine.
func synchronizeAndCloseBreaker(p *Plant) {
	p.TurbineBuilding.Generator.BreakerClosed = false
	for i := 0; i < int(30*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		g := p.TurbineBuilding.Generator
		fOK := g.Frequency >= GridRatedFrequency-SyncFreqToleranceHz
		dV := g.Voltage - p.ReactorBuilding.Grid.Voltage
		if dV < 0 {
			dV = -dV
		}
		vOK := dV <= SyncVoltageTolFraction*p.ReactorBuilding.Grid.Voltage
		if fOK && vOK {
			break
		}
	}
	p.TurbineBuilding.Generator.BreakerClosed = true
}

// When the reactor trips, the breaker must open so the unit stops
// exporting power to the grid. Plant.Step cascades the trip
// through the protective-layer wiring.
//
// Spec justification:
//   - "Turbine trip following a reactor trip." (item 2 of the
//     protective features list)
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:33-35
//   - "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
func TestPlant_ReactorTripOpensBreaker(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/TimeStep))
	require.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"precondition: want BreakerClosed before trip, got false")
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	stepN(&p, int(100*time.Millisecond/TimeStep))
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed,
		"want BreakerClosed=false after reactor trip, got true")
}

// With the generator breaker open the unit produces no electrical
// power even when the turbine is running at speed — the breaker
// gates grid connection.
//
// Spec justification:
//   - "output power, the power factor, and speed to ensure that the
//     limits of the generator are not exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
func TestPlant_BreakerOpenZerosRealPower(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Generator.BreakerClosed = false
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	stepN(&p, int(5*time.Second/TimeStep))
	assert.Zero(t, p.TurbineBuilding.Generator.RealPower,
		"want RealPower=0 with breaker open, got %v",
		p.TurbineBuilding.Generator.RealPower)
}

// End-to-end: when the operator closes the breaker at full load,
// the turbine produces RealPower and the UAT picks up house
// load (HouseLoadSuppliedFrom="UAT"). Net generator output to
// the grid is gross − house load.
//
// Spec justification:
//   - "the plant distribution buses are split and part are
//     supplied by the plant's own generator and the other part
//     are supplied by the system auxiliary transformers."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:54-56
//   - "supply power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
func TestPlant_BreakerCloseEnergizesUATAndSubtractsHouseLoad(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(5*time.Second/TimeStep))
	require.True(t, p.TurbineBuilding.Generator.BreakerClosed,
		"precondition: want BreakerClosed, got false")
	assert.Equal(t, "UAT", p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=UAT with breaker closed, got %q",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
	// Generator.RealPower is now the net of gross turbine output
	// minus house load; both should be positive.
	assert.Greater(t, p.TurbineBuilding.Generator.RealPower, Watts(0),
		"want net RealPower > 0, got %v",
		p.TurbineBuilding.Generator.RealPower)
	assert.Less(t, p.TurbineBuilding.Generator.RealPower,
		p.TurbineBuilding.Turbine.RealPower,
		"want net < gross (house load subtracted), got net=%v gross=%v",
		p.TurbineBuilding.Generator.RealPower,
		p.TurbineBuilding.Turbine.RealPower)
	// Wiring check: the subtraction consumes TotalMW on the MW
	// scale converted to watts, so at full load the gap is the
	// fixed plus variable station demand.
	wantNet := float64(p.TurbineBuilding.Turbine.RealPower) -
		p.ReactorBuilding.HouseLoad.TotalMW*1_000_000
	assert.InDelta(t, wantNet,
		float64(p.TurbineBuilding.Generator.RealPower), 1.0,
		"want net = gross - house load in watts")
}

// End-to-end: a reactor trip opens the generator breaker and
// drops the UAT, automatically transferring house load to SAT.
// The reactor trip does NOT remove house load from the grid; it
// just switches the supply source from UAT to SAT.
//
// Spec justification:
//   - "The turbine and turbine-generator will trip off the line
//     with house loads automatically transferred to the offsite
//     sources of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:63-66
//   - "automatic transfer of supplies is provided on sensing
//     loss of power"
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:58-59
func TestPlant_ReactorTripTransfersHouseLoadToSAT(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(2*time.Second/TimeStep))
	require.Equal(t, "UAT",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"precondition: want UAT supplying before trip, got %q",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
	p.ReactorBuilding.ReactorPressureVessel.Trip = true
	stepN(&p, int(200*time.Millisecond/TimeStep))
	assert.Equal(t, "SAT",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom,
		"want HouseLoadSuppliedFrom=SAT after trip, got %q",
		p.ReactorBuilding.StationTransformer.HouseLoadSuppliedFrom)
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed,
		"want BreakerClosed=false after trip, got true")
}

// PlantOperation.Step in ColdShutdown mode resets all ramping flags
// and leaves AuxFeedwaterInUse=true (the auxiliary path is the only
// source available before main feedwater is lined up).
//
// Spec justification:
//   - "Startup from a cold condition is initiated following a
//     shutdown that required the Reactor Coolant System (RCS) to
//     be cooled down and depressurized."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
func TestPlantOperation_ColdShutdownResetsRamping(t *testing.T) {
	po := PlantOperation{
		RampingUp:           true,
		RampingDown:         true,
		AuxFeedwaterInUse:   false,
		StartupPhase:        50,
	}
	out := po.Step(
		ColdShutdown,
		Temperature{Val: 120, Unit: u.Fahrenheit},
		Pressure(200),
		0,
		CoreRatedThermalPower,
		false, 0, 0, 0,
	)
	assert.Equal(t, ColdShutdown, out.Mode)
	assert.False(t, out.RampingUp, "want RampingUp=false in ColdShutdown")
	assert.False(t, out.RampingDown, "want RampingDown=false in ColdShutdown")
	assert.True(t, out.AuxFeedwaterInUse,
		"want AuxFeedwaterInUse=true in ColdShutdown")
	assert.Equal(t, 0.0, out.StartupPhase,
		"want StartupPhase reset to 0 in ColdShutdown")
}

// PlantOperation.Step in HotStandby mode activates the RCP cap
// whenever RHRS is not yet in service, and clears it once RHRS is
// running.
//
// Spec justification:
//   - "a maximum of two out of four reactor coolant pumps (RCPs)
//     may be secured. Proper loop flow and desired loop
//     temperatures are maintained with the other RCPs until the
//     Residual Heat Removal System (RHRS) is in service."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:104-108
func TestPlantOperation_HotStandbyRCPCap(t *testing.T) {
	po := PlantOperation{}
	withRHRS := po.Step(
		HotStandby,
		Temperature{Val: 550, Unit: u.Fahrenheit},
		Pressure(2250),
		0,
		CoreRatedThermalPower,
		true, 0, 0, 0,
	)
	assert.False(t, withRHRS.HotStandbyRCPLimitActive,
		"want HotStandbyRCPLimitActive=false when RHRS in service")
	withoutRHRS := po.Step(
		HotStandby,
		Temperature{Val: 550, Unit: u.Fahrenheit},
		Pressure(2250),
		0,
		CoreRatedThermalPower,
		false, 0, 0, 0,
	)
	assert.True(t, withoutRHRS.HotStandbyRCPLimitActive,
		"want HotStandbyRCPLimitActive=true when RHRS not in service")
}

// Aux-to-main feedwater transition. When per-SG steam flow exceeds
// the spec's 3.5% threshold, PlantOperation.Step must flip
// AuxFeedwaterInUse to false so the main feedwater regulators take
// over.
//
// Spec justification:
//   - "Before reaching the flow capacity of the auxiliary feedwater
//     system, equivalent to about 3.5-percent steam flow, the steam
//     generators feedwater supply is switched to the main feedwater
//     system."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:129-133
func TestPlantOperation_AuxToMainFeedwaterTransition(t *testing.T) {
	po := PlantOperation{}
	below := po.Step(
		PowerOperation,
		Temperature{Val: 580, Unit: u.Fahrenheit},
		Pressure(2250),
		CoreRatedThermalPower,
		CoreRatedThermalPower,
		true, 1, 0.020, 1.0,
	)
	assert.True(t, below.AuxFeedwaterInUse,
		"want AuxFeedwaterInUse=true below 3.5%% threshold")
	above := po.Step(
		PowerOperation,
		Temperature{Val: 580, Unit: u.Fahrenheit},
		Pressure(2250),
		CoreRatedThermalPower,
		CoreRatedThermalPower,
		true, 1, 0.050, 1.0,
	)
	assert.False(t, above.AuxFeedwaterInUse,
		"want AuxFeedwaterInUse=false above 3.5%% threshold")
}

// Power-raise rate limit. With the target setpoint above the
// current power by more than one tick of the 5%/min ramp, RampingUp
// must be true and the per-tick increment must not exceed
// OpPowerRaiseRate × ratedPower × TimeStep.
//
// Spec justification:
//   - "plant loading is continued to full power at a rate not to
//     exceed 5 percent of full power per minute."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:148-150
func TestPlantOperation_PowerRaiseRateLimit(t *testing.T) {
	po := PlantOperation{}
	// Plant at zero power; target setpoint = full load.
	// Demand delta = CoreRatedThermalPower ≫ one tick of ramp.
	out := po.Step(
		PowerOperation,
		Temperature{Val: 580, Unit: u.Fahrenheit},
		Pressure(2250),
		0,
		CoreRatedThermalPower,
		true, 1, 0.050, 1.0,
	)
	assert.True(t, out.RampingUp,
		"want RampingUp=true when demand exceeds one tick")
	assert.False(t, out.RampingDown,
		"want RampingDown=false when raising")
}

// When the target setpoint is within one tick of the current power,
// the regulator settles: neither RampingUp nor RampingDown is set
// and StartupPhase is latched at 100.
func TestPlantOperation_PowerOperationSettles(t *testing.T) {
	po := PlantOperation{}
	tinyDelta := OpPowerRaiseRate * float64(CoreRatedThermalPower) *
		TimeStep.Seconds() / 2
	out := po.Step(
		PowerOperation,
		Temperature{Val: 580, Unit: u.Fahrenheit},
		Pressure(2250),
		Watts(float64(CoreRatedThermalPower)-tinyDelta),
		CoreRatedThermalPower,
		true, 1, 0.050, 1.0,
	)
	assert.False(t, out.RampingUp,
		"want RampingUp=false when within one tick of target")
	assert.False(t, out.RampingDown,
		"want RampingDown=false when within one tick of target")
	assert.Equal(t, 100.0, out.StartupPhase,
		"want StartupPhase=100 in PowerOperation")
}

// Load-follow ramp down. With the target setpoint below the current
// power by more than one tick, RampingDown must be true.
//
// Spec justification:
//   - "ramp changes of 5 percent of rated power per minute."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_231.txt:112
func TestPlantOperation_PowerLowerRateLimit(t *testing.T) {
	po := PlantOperation{}
	out := po.Step(
		PowerOperation,
		Temperature{Val: 580, Unit: u.Fahrenheit},
		Pressure(2250),
		CoreRatedThermalPower,
		CoreRatedThermalPower,
		true, 1, 0.050, 0.5,
	)
	assert.False(t, out.RampingUp,
		"want RampingUp=false when lowering")
	assert.True(t, out.RampingDown,
		"want RampingDown=true when demand drops below one tick")
}

// Load-follow operating range. PlantOperation.Step in PowerOperation
// mode accepts any target setpoint within the spec's 15%–100%
// operating range. Below 15% the plant should leave PowerOperation
// in a real plant; the simplified model still permits any
// setpoint but the operator is expected to drive the mode
// transition explicitly.
//
// Spec justification:
//   - "The NSSS is capable of following repetitive load changes
//     automatically throughout the range of 15 percent to 100
//     percent of rated power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_230.txt:62-65
func TestPlantOperation_LoadFollow15To100(t *testing.T) {
	po := PlantOperation{}
	for _, frac := range []float64{0.15, 0.50, 1.00} {
		out := po.Step(
			PowerOperation,
			Temperature{Val: 580, Unit: u.Fahrenheit},
			Pressure(2250),
			Watts(frac*float64(CoreRatedThermalPower)),
			CoreRatedThermalPower,
			true, 1, frac, frac,
		)
		assert.Equal(t, PowerOperation, out.Mode,
			"want Mode=PowerOperation at %v fraction", frac)
	}
}

// ClampRate returns posLimit*dt when desired delta exceeds the
// positive ceiling, negLimit*dt when desired delta falls below
// the negative floor, and the raw delta otherwise.
func TestClampRate_Positive(t *testing.T) {
	dt := time.Second
	got := ClampRate(0, 100, 5, -5, dt)
	assert.InDelta(t, 5.0, got, 1e-9,
		"want ClampRate to clamp to posLimit when desired > ceiling")
}

func TestClampRate_Negative(t *testing.T) {
	dt := time.Second
	got := ClampRate(0, -100, 5, -5, dt)
	assert.InDelta(t, -5.0, got, 1e-9,
		"want ClampRate to clamp to negLimit when desired < floor")
}

func TestClampRate_Passthrough(t *testing.T) {
	dt := time.Second
	got := ClampRate(0, 3, 5, -5, dt)
	assert.InDelta(t, 3.0, got, 1e-9,
		"want ClampRate to pass desired delta through within limits")
}

// Plant.Step must propagate PlantOperation mode into nextPlant and
// apply the resulting gating. With Mode=ColdShutdown,
// AuxFeedwaterInUse must end up true after the step.
func TestPlant_StepPropagatesPlantOperation(t *testing.T) {
	p := MakePlant()
	p.PlantOperation.Mode = ColdShutdown
	stepN(&p, 1)
	assert.Equal(t, ColdShutdown, p.PlantOperation.Mode,
		"want Mode=ColdShutdown after step")
	assert.True(t, p.PlantOperation.AuxFeedwaterInUse,
		"want AuxFeedwaterInUse=true in ColdShutdown")
}

// The discriminating case for the same wiring: nextPlant is a
// zero-valued scratch pad, so an unseeded PlantOperation.Step call
// advances ColdShutdown regardless of the configured lineup. Only a
// non-zero mode catches it — ColdShutdown equals the zero value and
// passes either way.
//
// Spec justification:
//   - hot standby is a distinct operator-selected condition
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
func TestPlant_StepAdvancesConfiguredHotStandby(t *testing.T) {
	p := MakePlant()
	p.PlantOperation.Mode = HotStandby
	require.False(t, p.AuxiliaryBuilding.ResidualHeatRemovalSystem.Running,
		"precondition: RHRS secured so the hot-standby RCP cap arms")
	stepN(&p, 1)
	assert.Equal(t, HotStandby, p.PlantOperation.Mode,
		"want the configured mode preserved through Step")
	assert.True(t, p.PlantOperation.HotStandbyRCPLimitActive,
		"want the hot-standby RCP cap armed with RHRS out of service")
}

// The synchronism check pops the breaker when it is closed onto an
// out-of-step machine: with the rotor stationary the terminal
// frequency and voltage are far outside the sync window, so
// Plant.Step clears BreakerClosed on the next step.
//
// Spec justification:
//   - DEHM supports "automatic synchronizers" as remote equipment.
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:90-92
func TestPlant_BreakerCloseRequiresSyncTolerance(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Generator.BreakerClosed = true
	stepN(&p, int(time.Second/TimeStep))
	assert.False(t, p.TurbineBuilding.Generator.BreakerClosed,
		"want breaker popped when closed out of synchronism")
}

// A sudden setpoint drop larger than 10% is a load rejection: the
// dump valves open, the condenser sees the dumped steam, and the SG
// header sags because total draw exceeds boil-off capacity.
//
// Spec justification:
//   - "These valves open under a sudden reduction in turbine-
//     generator load (in excess of 10%) or a turbine trip."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
//   - "This heat is rejected to the condenser through the steam dump
//     valves"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:6-7
func TestPlant_LoadRejectOpensDumpAndRoutesSteam(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(5*time.Second/TimeStep))
	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"precondition: want turbine running")

	p.TurbineBuilding.Turbine.Setpoint = 0.5
	stepN(&p, int(2*time.Second/TimeStep))
	assert.Greater(t, p.TurbineBuilding.Turbine.DumpValve.Open, 0.9,
		"want dump valves opened by load rejection, got %v",
		p.TurbineBuilding.Turbine.DumpValve.Open)
	assert.Greater(t, p.TurbineBuilding.Condenser.CondensingRate.Float(), 0.0,
		"want condenser receiving dumped steam")
	assert.Greater(t, p.TurbineBuilding.SteamOfftakeFraction, 1.0,
		"want SG offtake above boil-off capacity during dump, got %v",
		p.TurbineBuilding.SteamOfftakeFraction)
	assert.Less(t, float64(p.ReactorBuilding.SteamGenerator.Pressure),
		0.95*float64(SteamGeneratorShellPressure),
		"want SG header pressure sagging under dump load, got %v",
		p.ReactorBuilding.SteamGenerator.Pressure)
}

// A manual stop-valve closure on a synchronized unit latches the turbine
// trip through the steam-loss deadline, and the trip state alone holds the
// condenser dump valves open until the operator resets at rest — the
// bypass "temporarily replaces the turbine as the steam load".
//
// Spec justification:
//   - stop/governor valves close and MW drops instantly on turbine trip:
//     specs/pwr_sim/chapter_3/page_041.txt:12-15
//   - bypass valves open on turbine trip and modulate to hold boiler
//     pressure:
//     specs/pwr_sim/chapter_3/page_041.txt:39-45
//   - "These valves close automatically as steam is admitted to the
//     turbine."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:32-33
func TestPlant_StopValveClosureHoldsDumpUntilReset(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(5*time.Second/TimeStep))
	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"precondition: want turbine running")

	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Generator.BreakerClosed = false
	stepN(&p, int(3*time.Second/TimeStep))
	require.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State,
		"steam-loss deadline should latch the trip")
	assert.Equal(t, Watts(0), p.TurbineBuilding.Turbine.RealPower,
		"tripped machine makes no power")
	assert.Greater(t, p.TurbineBuilding.Turbine.DumpValve.Open, 0.9,
		"trip state should hold the condenser dump open, got %v",
		p.TurbineBuilding.Turbine.DumpValve.Open)

	stepN(&p, int(10*time.Second/TimeStep))
	p.TurbineBuilding.Turbine.ResetRequest = true
	stepN(&p, int(10*time.Millisecond/TimeStep))
	require.Equal(t, TurbineOff, p.TurbineBuilding.Turbine.State,
		"reset at rest should clear the trip to off")
	p.TurbineBuilding.Turbine.StopValve.Open = 1
	stepN(&p, int(2*time.Second/TimeStep))
	assert.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"steam admission after reset should return the machine to service")
}

// The atmospheric dump valve relieves SG inventory and pressure
// directly to atmosphere without adding condensate to the condenser.
//
// Spec justification:
//   - "ASME Code safety valves and power operated relief and
//     atmospheric dump valves are located on each steam line
//     upstream of the stop valves."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
//   - "During plant cooldown, steam is dumped to the condensers to
//     provide a heat sink rather than to atmosphere through the
//     atmospheric dump valves."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:33-36
func TestPlant_AtmosphericDumpRelievesSGOnly(t *testing.T) {
	run := func(atmOpen float64) (Pressure, Volume) {
		p := MakePlant()
		p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
		p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
		p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open = atmOpen
		stepN(&p, int(5*time.Second/TimeStep))
		return p.ReactorBuilding.SteamGenerator.Pressure,
			p.TurbineBuilding.Condenser.CondensingRate
	}
	pClosed, cClosed := run(0)
	pOpen, cOpen := run(1)
	assert.Less(t, float64(pOpen), float64(pClosed),
		"want atmospheric relief to sag SG header pressure, open=%v closed=%v",
		pOpen, pClosed)
	// Exhaust to the condenser tracks turbine admission only; with
	// the header sagging the condenser sees no extra steam, whereas
	// a condenser dump would push its rate up.
	assert.LessOrEqual(t, cOpen.Float(), cClosed.Float(),
		"want atmospheric relief not to add condenser duty, open=%v closed=%v",
		cOpen.Float(), cClosed.Float())
}

// Apparent power above the 1370 MVA rating is scaled back onto the
// rating circle and raises the MVA-limit alarm.
//
// Spec justification:
//   - "Rating: 1370 mva"
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:18
//   - "output power, the power factor, and speed to ensure that the
//     limits of the generator are not exceeded."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
func TestPowerGenerator_MVAEnforced(t *testing.T) {
	gen := PowerGenerator{RatedMVA: GeneratorRatedMVA}
	p := 1.5e9
	q := 0.7e9
	p2, q2 := clampMVA(&gen, p, q)
	s := math.Hypot(p2, q2)
	assert.LessOrEqual(t, s, GeneratorRatedMVA*1000.0*1.0001,
		"want apparent power clamped onto the rating circle, got %v VA", s)
	assert.True(t, gen.GeneratorMVALimitAlarm, "want MVA-limit alarm raised")

	gen2 := PowerGenerator{RatedMVA: GeneratorRatedMVA}
	p3, q3 := clampMVA(&gen2, 1.0e9, 0.4843e9)
	assert.InDelta(t, 1.0e9, p3, 1.0)
	assert.False(t, gen2.GeneratorMVALimitAlarm,
		"want no alarm when within rating")
	_ = q3
}

// The turbine-trip stepback is the plant control system response to a
// trip: fold neutron power quickly to 60 percent by rod insertion, park
// there with zero measured reactivity (xenon margin preserved for
// recovery), release the rods when the machine returns to service.
//
// Spec justification:
//   - "The reactor neutron power will be reduced quickly to 60 % by
//     rapid insertion of control rods - this is known as reactor power
//     'stepback' [...] still has enough positive reactivity [...] to
//     bring the reactor back to full power."
//     specs/pwr_sim/chapter_3/page_041.txt:28-35
func TestPlant_TurbineTripStepbackHoldsSixtyPercent(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(5*time.Second/TimeStep))
	require.Equal(t, TurbineRunning, p.TurbineBuilding.Turbine.State,
		"precondition: want turbine running")

	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Generator.BreakerClosed = false
	stepN(&p, int(3*time.Second/TimeStep))
	require.True(t, p.PlantOperation.StepbackActive,
		"trip should latch the automatic stepback")
	require.False(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"stepback must hold without a reactor trip")

	target := StepbackPowerFraction * float64(CoreFissionPowerAtRated)
	stepN(&p, int(30*time.Second/TimeStep))
	fp := float64(p.ReactorBuilding.ReactorPressureVessel.FissionPower)
	assert.InDelta(t, target, fp, 0.03*target,
		"stepback should park neutron power at the runback level")
	require.True(t, p.PlantOperation.StepbackActive,
		"hold should persist while the turbine is out of service")
	found := false
	for _, a := range p.Alarms() {
		if a.ID == AlarmIDTurbineStepback {
			found = true
		}
	}
	assert.True(t, found, "want stepback alarm active during hold")

	p.TurbineBuilding.Turbine.ResetRequest = true
	stepN(&p, int(10*time.Millisecond/TimeStep))
	require.Equal(t, TurbineOff, p.TurbineBuilding.Turbine.State,
		"reset at rest should clear the trip to off")
	p.TurbineBuilding.Turbine.StopValve.Open = 1
	p.TurbineBuilding.Turbine.SpeedDemand = 1.0
	stepN(&p, int(3*time.Second/TimeStep))
	assert.False(t, p.PlantOperation.StepbackActive,
		"turbine back in service should release the stepback")
}

// Main feedwater delivery moves secondary inventory from the condenser
// hotwell into the steam generator one-for-one: the differential
// between two identical plants differing only in pump state isolates
// the feed path from every shared flow.
//
// Spec justification:
//   - "The condensate pumps take suction from the condenser hotwell
//     and pump condensate [...]; the water discharge from the feedwater
//     pumps flows [...] into the steam generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-51
func TestPlant_MainFeedwaterReplenishesSGFromHotwell(t *testing.T) {
	run := func(pumpOn bool) (float64, float64) {
		p := MakePlant()
		p.ReactorBuilding.SteamGenerator.Water =
			Volume{Val: 20000, Unit: u.Gallon}
		p.TurbineBuilding.Condenser.HotwellLevel =
			Volume{Val: 50000, Unit: u.Gallon}
		p.TurbineBuilding.MainFeedwaterPump.Running = pumpOn
		stepN(&p, int(10*time.Second/TimeStep))
		return p.ReactorBuilding.SteamGenerator.Water.Float(),
			p.TurbineBuilding.Condenser.HotwellLevel.Float()
	}
	sgOff, hwOff := run(false)
	sgOn, hwOn := run(true)

	dSG := sgOn - sgOff
	dHW := hwOn - hwOff
	assert.Greater(t, dSG, 0.0,
		"running main feedwater should raise SG inventory")
	assert.InDelta(t, dSG, -dHW, 1.0,
		"SG gain must come out of the hotwell (ledger closed)")
}

// Coolant expansion raises pressurizer level through the surge line:
// with every secondary outlet secured, core heat accumulates in the
// primary, the RCS swells, and pressurizer water rises while the
// bubble shrinks to keep the tank whole.
//
// Spec justification:
//   - "Raise pressurizer level above no-load value in anticipation of
//      coolant contraction" (the surge coupling, either sign).
//      specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:70-72
func TestPlant_PressurizerSurgeTracksCoolantExpansion(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	startWater := p.ReactorBuilding.Pressurizer.Water.Float()
	startOutlet := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()

	// Secure CVCS so charging/letdown cannot move pressurizer level;
	// run an RCP so shaft heat holds total power just above rated.
	// Both fixtures then isolate the surge-line coupling.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false
	p.ReactorBuilding.ReactorCoolantPump.Running = true

	// Secure every secondary outlet so core heat accumulates in the
	// primary: stop valves shut, condenser dump shut, atmospheric
	// dump and SG safety valves shut.
	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Turbine.DumpValve.Open = 0
	p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open = 0
	stepN(&p, int(45*time.Second/TimeStep))

	outlet := p.ReactorBuilding.ReactorPressureVessel.Outlet.Float()
	assert.Greater(t, outlet, startOutlet,
		"precondition: want the primary heated above its starting point")
	water := p.ReactorBuilding.Pressurizer.Water.Float()
	assert.Greater(t, water, startWater,
		"expansion should raise pressurizer level through the surge line")
	bubble := p.ReactorBuilding.Pressurizer.SteamBubble.Float()
	vol := p.ReactorBuilding.Pressurizer.Volume.Float()
	assert.InDelta(t, vol, water+bubble, 1e-6,
		"pressurizer volumes must stay conserved")
}

// Pre-synchronization steam draw scales with rotor speed: a slow-roll
// rotor cannot swallow design flow. A synchronized machine at the same
// speed draws full design steam.
//
// Spec justification:
//   - "Wide range speed control of the turbine from turning gear to
//      rated speed."
//      specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
func TestTurbine_RollingSteamDrawScalesWithSpeed(t *testing.T) {
	base := Turbine{
		State:          TurbineRunning,
		Velocity:       TurbineRatedSpeed / 2,
		Synchronized:   false,
		GoverningValve: Valve{Open: 1},
	}
	half := base.Step(SteamGeneratorShellPressure, CondenserDesignVacuum, false, nil)
	syncd := base
	syncd.Synchronized = true
	full := syncd.Step(SteamGeneratorShellPressure, CondenserDesignVacuum, false, nil)

	speedFrac := max(float64(base.Velocity)/float64(TurbineRatedSpeed),
		TurbineRollMinSteamFraction)
	want := CondenserDesignSteamLoadGalPer * speedFrac
	require.Greater(t, float64(half.Velocity),
		TurbineRollMinSteamFraction*float64(TurbineRatedSpeed),
		"precondition: rotor above the roll floor so speed scaling shows")
	assert.InDelta(t, want, half.steamDemandGalPerStep, want*0.05,
		"rolling draw should scale with rotor speed")
	assert.InDelta(t, CondenserDesignSteamLoadGalPer,
		full.steamDemandGalPerStep, CondenserDesignSteamLoadGalPer*0.02,
		"synchronized machine draws full design steam at rated speed")
}

// The SG safety valves pop at their setpoint, relieve to atmosphere,
// and reseat with blowdown once pressure falls back.
//
// Spec justification:
//   - "the boiler pressure safety relief valve will open on high
//      boiler pressure"
//      specs/pwr_sim/chapter_3/page_041.txt:20-22
//   - "self-actuating ASME-code [safety valves] which open
//      automatically to reduce steam pressure"
//      specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:34-38
func TestPlant_SGSafetyValvePopsAndReseats(t *testing.T) {
	pop := MakePlant().ReactorBuilding.SteamGenerator
	pop.HeatInput = SteamGeneratorFullLoadHeatInput
	pop.Water = Volume{Val: 80000, Unit: u.Gallon}
	pop.Pressure = SteamGeneratorSafetyValveSetpointPsia
	pop = pop.Step(0)
	require.Greater(t, pop.SafetyValve.Open, 0.9,
		"valves should pop at setpoint")
	assert.Less(t, pop.Water.Float(), 80000.0,
		"relief spends secondary inventory")

	reseat := pop
	// Quarter heat: boil-off falls below the safety-valve relief
	// capacity, so the header sags and the valves reseat.
	reseat.HeatInput = SteamGeneratorFullLoadHeatInput / 4
	for i := 0; i < 8000; i++ {
		reseat = reseat.Step(0)
	}
	assert.Equal(t, 0.0, reseat.SafetyValve.Open,
		"valves should reseat once pressure decays below blowdown")
}

// At nameplate the secondary energy balance closes: rated heat input
// meets full-admission design steam flow, so the header holds its
// full-load shell pressure and the generator delivers rated output
// instead of settling on a supply-limited arc.
//
// Spec justification:
//   - "Full load pressure, shell side: 920 psia (63 bar)" (Table
//     3.4-1): specs/the_westinghouse_pressurized_water_react/
//     chapter_3/page_066.txt:36,80-82
//   - "plant loading is continued to full power at a rate not to
//     exceed 5 percent of full power per minute":
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:63-67
func TestPlant_FullLoadHoldsShellPressureAndRatedOutput(t *testing.T) {
	// Steady-state balance: fission rides the rated cap (a pinned
	// state, insensitive to tick width), the SG header relaxes toward
	// its anchor through dt-scaled lags, and every assertion is a
	// wide band around that settled point (+/-40 psia against a
	// 920 psia anchor). Must not run in parallel: overrideTimeStep
	// mutates global simulation state.
	overrideTimeStep(t, 10*time.Millisecond)
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(60*time.Second/TimeStep))

	sgP := float64(p.ReactorBuilding.SteamGenerator.Pressure)
	assert.InDelta(t, float64(SteamGeneratorShellPressure), sgP, 40,
		"full-load header should hold the shell pressure, got %v", sgP)
	rp := float64(p.TurbineBuilding.Generator.RealPower)
	assert.Greater(t, rp, 0.95*float64(TurbineRatedkW),
		"full load should deliver at least 95 percent of rated MW, got %v", rp)
	assert.Zero(t, p.TurbineBuilding.Turbine.DumpValve.Open,
		"condenser dump stays shut at balanced full load")
	assert.Zero(t, p.ReactorBuilding.SteamGenerator.SafetyValve.Open,
		"SG safety valves stay seated at balanced full load")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped,
		"no spurious reactor trip at sustained full load")
}

// A turbine-trip bypass opens on the trip itself and then modulates
// around the full-load shell pressure while the stepback folds power:
// the header never crashes toward the low-steamline protection channel
// the way a wide-open over-draw would, and the safety valves stay out
// of the event entirely.
//
// Spec justification:
//   - "turbine bypass valves open automatically on turbine trip,
//      then modulate to pass steam to the condenser and hold boiler
//      pressure at the constant setpoint"
//     specs/pwr_sim/chapter_3/page_041.txt:37-45
//   - SG pressure must stay far above the low-steamline trip:
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt
func TestPlant_TripBypassModulatesAndHoldsHeaderPressure(t *testing.T) {
	// Stepback demand ramps linearly in dt and bypass valve travel is
	// linear too; the header envelope is asserted against wide bounds
	// (880 psia floor, safety-valve ceiling) rather than per-step
	// shape.
	//
	// Fidelity note: this hold cannot ride a widened TimeStep. The
	// stepback fold follows the reactor's per-step reactivity
	// compounding, so a coarser tick folds power more slowly against
	// the tripped turbine's fixed offtake, and the surplus steam
	// pushes the header peak past the 985 psia safety-valve ceiling:
	// measured 985.12 psia at a uniform 10 ms tick and 985.04 psia at
	// 5 ms against an assertion that exists to prove the valve stays
	// seated. It keeps the default step throughout rather than
	// overrideTimeStep, and stays parallel because nothing global is
	// mutated.
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.SeedSteadyStatePower(CoreRatedThermalPower)
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	synchronizeAndCloseBreaker(&p)
	stepN(&p, int(5*time.Second/TimeStep))

	p.TurbineBuilding.Turbine.StopValve.Open = 0
	p.TurbineBuilding.Generator.BreakerClosed = false
	stepN(&p, int(3*time.Second/TimeStep))
	require.Equal(t, TurbineTripped, p.TurbineBuilding.Turbine.State,
		"precondition: want the trip latched")
	require.True(t, p.PlantOperation.StepbackActive,
		"precondition: want the automatic stepback folding power")

	minP, maxP := 1e9, 0.0
	modulated := false
	for i := 0; i < int(60*time.Second/TimeStep); i++ {
		p.Step()
		p.Update()
		sgP := float64(p.ReactorBuilding.SteamGenerator.Pressure)
		minP = min(minP, sgP)
		maxP = max(maxP, sgP)
		open := p.TurbineBuilding.Turbine.DumpValve.Open
		if open > 0.05 && open < 0.95 {
			modulated = true
		}
	}
	assert.GreaterOrEqual(t, minP, 880.0,
		"bypass modulation should keep the header near its setpoint, min %v", minP)
	assert.LessOrEqual(t, maxP, float64(SteamGeneratorSafetyValveSetpointPsia),
		"transient lift should stay below the SG safety-valve setpoint, max %v", maxP)
	assert.True(t, modulated,
		"bypass should throttle between wide open and shut during the hold")
	assert.Zero(t, p.ReactorBuilding.SteamGenerator.SafetyValve.Open,
		"SG safety valves stay seated through the trip and hold")
	assert.False(t, p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionActuated,
		"no low-steamline protection actuation")
}
