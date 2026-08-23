package core

import (
	"time"

	u "github.com/bcicen/go-units"
)


// Step advances the main condenser by TimeStep.
// Two coupled phenomena are modelled:
//
//  1. The condenser is normally held at sub-atmospheric vacuum by
//     the steam-jet air ejectors / vacuum pumps. When those gas-
//     removal devices are off, in-leakage and dissolved gases let
//     the back-pressure climb toward atmospheric. The spec describes
//     the vacuum subsystem:
//     "Under normal operation the air is removed by steam jet air
//     ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
//
//  2. The latent heat of condensation is rejected to the
//     circulating-water (CW) flow. The CW outlet temperature rises
//     by CondenserCWRise at design flow. When the CW pumps are off,
//     the heat sink is lost and Pressure rises aggressively.
//
// Spec justification for the couplings:
//   - "The main condenser condenses the exhaust from the main
//     turbines and the two feedwater pump turbines."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-15
//   - "There is also sufficient surface to condense steam from the
//     steam dump system with an acceptable increase in turbine back
//     pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:16-18
//
// The CW-pump / vacuum-pump operating state is supplied by the
// caller (Plant.Step), since those pumps live on the Outside and
// Electrical structs respectively. The condenser itself is a passive
// heat exchanger with no internal state machine.
func (c *Condenser) Step(cwIn Temperature, vacuumPumpRunning, cwPumpRunning bool) Condenser {
	ret := *c
	dt := TimeStep

	// 1) Steam mass balance. CondensingRate is set by
	// TurbineBuilding.StepWithSteam before this call and represents
	// the per-step steam mass condensed (in gallons). Convert to
	// condensate level on the shell side.
	steamMass := ret.CondensingRate.Float()
	ret.Level = Volume{Val: ret.Level.Float()+steamMass, Unit: u.Gallon}

	// 2) Heat sink: when the CW pump is off, the tube bundle has no
	// flow. Condenser pressure relaxes toward atmospheric at the
	// no-CW degrade rate.
	cwFlowGpm := float64(0)
	if cwPumpRunning {
		cwFlowGpm = CondenserDesignCWFlowGPM
	}
	if cwFlowGpm <= 0 {
		pressure := float64(ret.Pressure)
		pressure += (float64(AtmosphericPressure) - pressure) *
			(dt.Seconds() / CondenserNoCWDegradeTimeConstant)
		if pressure < 0 {
			pressure = 0
		}
		ret.Pressure = Pressure(pressure)
		ret.CWOut = cwIn
		ret.CondensingRate = Volume{Val: 0, Unit: u.Gallon}
		return ret
	}

	// 3) Vacuum-pump effect. With vacuum pumps running and minimal
	// steam load, Pressure tracks toward CondenserDesignVacuum.
	// Incoming steam load pushes the back-pressure upward — the
	// pressure rise is proportional to the ratio of actual load to
	// design full-load steam load, capped at 30% of the atmospheric
	// gap so a saturated condenser can never reach atmospheric.
	target := float64(CondenserDesignVacuum)
	if !vacuumPumpRunning {
		// Vacuum pumps off: degrade toward atmospheric (but slower
		// than the no-CW case since there is still some flow).
		target = float64(AtmosphericPressure)
	}
	if steamMass > 0 {
		loadFactor := steamMass / CondenserDesignSteamLoadGalPer
		if loadFactor > 1.0 {
			loadFactor = 1.0
		}
		target += loadFactor * (float64(AtmosphericPressure) - float64(CondenserDesignVacuum)) * 0.3
	}
	pressure := float64(ret.Pressure)
	pressure += (target - pressure) * (dt.Seconds() / CondenserVacuumTimeConstant)
	if pressure < 0 {
		pressure = 0
	}
	ret.Pressure = Pressure(pressure)

	// 4) CW outlet temperature. The CW absorbs the latent heat of
	// condensation; at design flow the delta-T across the bundle
	// is CondenserCWRise. Below design flow the delta-T scales
	// inversely with the flow ratio.
	cwDelta := CondenserCWRise * (CondenserDesignCWFlowGPM / cwFlowGpm)
	ret.CWOut = Temperature{Val: cwIn.Float()+cwDelta, Unit: u.Fahrenheit}

	// 5) Hotwell level. Accumulates the per-step condensed steam
	// minus the condensate pump's suction flow (proportional to
	// the CW flow proxy). The hotwell feeds the LP heater train.
	// "The condensate pumps take suction from the condenser
	// hotwell [...]."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-42
	pumpSuctionFlow := cwFlowGpm * 0.01 // gal/s at design flow
	netInflow := steamMass - pumpSuctionFlow*dt.Seconds()
	newLevel := ret.HotwellLevel.Float() + netInflow
	if newLevel < 0 {
		newLevel = 0
	}
	ret.HotwellLevel = Volume{Val: newLevel, Unit: u.Gallon}

	ret.CondensingRate = Volume{Val: 0, Unit: u.Gallon}
	return ret
}

// Condenser models the main condenser (chapter 15.3). The shell-
// side steam condenses on the tube bundle; the latent heat is
// rejected to the circulating water (CW) flowing inside the tubes.
//
// Spec justification for the field set:
//   - Pressure — the condenser operates at sub-atmospheric vacuum
//     under normal conditions: "Under normal operation the air is
//     removed by steam jet air ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
//   - Level — condensate accumulation; the condensate pumps (out of
//     scope for p9) eventually drain it back to the CST.
//     "There is also sufficient surface to condense steam from the
//     steam dump system with an acceptable increase in turbine back
//     pressure."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:14-18
//   - HotwellLevel — the level in the condenser hotwell from which
//     the condensate pumps take suction. The spec describes "the
//     closed-loop type with deaeration accomplished in the condenser
//     hotwell".
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:38-40
//   - VacuumPumpRunning — operator-controlled flag set from the
//     steam-jet air ejector / vacuum pump subsystem.
//   - CirculatingWaterIn — temperature of the CW water entering the
//     condenser tubes. Set once per step from outside.go's
//     CirculatingWaterPump state.
//   - CWOut — informational outlet temperature of the CW (delta-T
//     across the tube bundle).
//   - CondensingRate — per-step steam mass condensed in gallons; set
//     by TurbineBuilding.StepWithSteam from the turbine's exhaust
//     mass flow, consumed by the condenser step.
type Condenser struct {
	Part
	Pressure
	Level              Volume
	HotwellLevel       Volume
	VacuumPumpRunning  bool
	CirculatingWaterIn Temperature
	CWOut              Temperature
	CondensingRate     Volume
}
// AtmosphericPressure is the standard sea-level pressure the
// condenser back-pressure relaxes toward when the vacuum pumps and
// circulating-water pumps are unavailable. Not stated explicitly in
// the Westinghouse PWR spec but used to bound the no-CW condenser
// degradation.
const AtmosphericPressure Pressure = 14.7
// Condenser and main-steam system constants (plans/p9_1.txt).
//
// Spec justification:
//   - CondenserDesignVacuum — "Under normal operation the air is
//     removed by steam jet air ejectors or vacuum pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
//     The number below approximates a well-operated condenser with
//     a modest air in-leakage.
//   - CondenserDesignCWIn — design circulating-water inlet
//     temperature of 75°F (24°C), matching the RiverTemp default
//     used for the Service Water system (core/plant.go).
//   - CondenserCWRise — design temperature rise of the CW across
//     the tube bundle, sized to give a sensible heat-rejection
//     value. The spec does not state a value.
//   - CondenserDesignCWFlowGPM — per-condenser CW flow sized for a
//     large 4-loop plant (essential CW pump rating "approximately
//     20,000 gpm").
//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:39-43
//   - CondenserDesignSteamLoadGalPerStep — full-load steam
//     volumetric flow per step, derived from
//     SteamGeneratorFullLoadSteamFlow × 4 loops / steps-per-second.
//     The per-SG spec value is 3,813,000 lb/hr (480 kg/s).
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:49-56
//   - SteamDumpOpenThreshold — condenser pressure above which the
//     dump valves open automatically to relieve inertial heat. Not
//     stated in the spec; sized above design vacuum but well below
//     atmospheric.
//   - StopValveCloseDelay — hydraulic time for the auto-stop-oil
//     system to dump fluid pressure and close all stop valves
//     after a trip signal. The spec describes mechanical-hydraulic
//     dumping without a stated delay.
//     "Release of this oil, called auto stop oil, by any one of
//     the turbine trip devices [...] causes all steam control
//     valves to close."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
//   - CondenserVacuumTimeConstant / CondenserNoCWDegradeTimeConstant
//     — sized for plausible ~2 s vacuum recovery and a few seconds
//     of no-CW degradation; not stated in the spec.
const (
	CondenserDesignVacuum            Pressure = 1.5
	CondenserDesignCWIn              float64  = 75
	CondenserCWRise                  float64  = 15
	CondenserDesignCWFlowGPM         float64  = 20000
	CondenserDesignSteamLoadGalPer   float64  = 100
	SteamDumpOpenThreshold           Pressure = 5.0
	StopValveCloseDelay                       = 50 * time.Millisecond
	CondenserVacuumTimeConstant      float64  = 2.0
	CondenserNoCWDegradeTimeConstant float64  = 5.0

	// Circulating Water System design constants
	// (plans/p31_1.txt).
	//
	// Spec justification:
	//   - CW pumps: "vertical, mixed flow, volute type
	//     circulating water pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:56-58
	//   - CW source diversity: "river, lake, ocean, or cooling
	//     tower."
	//     specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:28-30
	//   - Condenser: "single-pass deaerating type with
	//     straight-through tubes."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-20
	//   - Vacuum removal: "steam jet air ejectors or vacuum
	//     pumps."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:21-22

	// CirculatingWaterPumpCount is the number of CW pumps
	// installed per unit. The spec describes "a typical system
	// design will provide six pumps, three each for essential
	// and nonessential service" but the main-condenser path
	// uses one CW pump per half-condenser (two pumps total
	// per unit for the CWS condenser function). The simplified
	// model uses two.
	// "Circulating water for each main condenser is supplied
	// by vertical, mixed flow, volute type circulating water
	// pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:56-58
	CirculatingWaterPumpCount = 2

	// CirculatingWaterPumpFlowGPM is the design flow rate per
	// CW pump (gal/min). The spec does not give a numeric value;
	// sized at 550,000 gpm per pump so two pumps deliver ~1.1
	// million gpm total, which is consistent with a 4-loop
	// Westinghouse CWS for ~3,800,000 lb/hr main-steam flow.
	CirculatingWaterPumpFlowGPM float64 = 550_000

	// CirculatingWaterDesignFlowGPM is the total CWS design
	// flow when both pumps are in service (gal/min).
	CirculatingWaterDesignFlowGPM =
		CirculatingWaterPumpFlowGPM * CirculatingWaterPumpCount

	// CirculatingWaterSourceTempF is the assumed constant
	// intake temperature for river / lake / ocean sources
	// (°F). The spec does not state a value; 70 °F is typical
	// for temperate climates.
	CirculatingWaterSourceTempF = 70.0

	// CondenserDesignCWRiseF is the design temperature rise of
	// the CW across the condenser tube bundle at full
	// condensing duty (°F). The spec does not state a number;
	// 18 °F is sized to reject full condenser duty at the
	// design CW flow.
	CondenserDesignCWRiseF = 18.0

	// CoolingTowerApproachF is the assumed temperature
	// difference between the tower cold-water basin and the
	// ambient wet-bulb (°F) when a mechanical-draft cooling
	// tower is the CW source. The spec does not state a value;
	// 5 °F is typical for modern mechanical-draft towers.
	CoolingTowerApproachF = 5.0
)
