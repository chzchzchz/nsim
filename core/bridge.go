package core

import (
	"fmt"
	"strconv"

	"github.com/chzchzchz/nsim/bridge"
)

// GetControlMap returns a bridge.ControlMap populated with controls for the
// given plant. Each closure captures p and dereferences live state on every
// call so the map survives Plant.Step() and Plant.Update() cycles without
// pinning stale nextPlant addresses.
func GetControlMap(p *Plant) bridge.ControlMap {
	m := make(bridge.ControlMap)
	addPlantOperationControls(p, m)
	addReactorBuildingControls(p, m)
	addTurbineBuildingControls(p, m)
	addAuxiliaryBuildingControls(p, m)
	addOutsideControls(p, m)
	return m
}

// addPlantOperationControls registers the operator's plant-level mode
// selection. Mode changes are operator actions during startups and
// shutdowns (placing the unit in hot standby, entering cold shutdown),
// so the mode must be drivable at runtime, not only through a plant
// JSON edit.
//
// Spec justification:
//   - Mode definitions and the cold/hot distinction:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86
//   - Hot standby as an operator-placed condition:
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:24-29
func addPlantOperationControls(p *Plant, m bridge.ControlMap) {
	m["PlantOperation/Mode"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "Mode",
		Kind:  "enum",
		Enum: []string{
			"ColdShutdown",
			"HotStandby",
			"PowerOperation",
		},
		Get: func() string {
			return fmt.Sprintf("%d", p.PlantOperation.Mode)
		},
		Set: func(s string) error {
			switch s {
			case "0":
				p.PlantOperation.Mode = ColdShutdown
			case "1":
				p.PlantOperation.Mode = HotStandby
			case "2":
				p.PlantOperation.Mode = PowerOperation
			default:
				return fmt.Errorf("invalid Mode %q (want 0..2)", s)
			}
			return nil
		},
	}

	// Lead/lag plant mode select (chapter 2.8 RPC screen):
	//   "MODE will indicate whether the reactor is under TURBINE
	//    LEADING to REACTOR LEADING control, this status can also
	//    be changed here."
	//   specs/pwr_sim/chapter_2/page_023.txt:27-28
	m["PlantOperation/LeadLagMode"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "LeadLagMode",
		Kind:  "enum",
		Enum:  []string{"TurbineLead", "ReactorLead"},
		Get: func() string {
			return fmt.Sprintf("%d", p.PlantOperation.LeadLagMode)
		},
		Set: func(s string) error {
			switch s {
			case "0", "TurbineLead":
				p.PlantOperation.LeadLagMode = TurbineLead
			case "1", "ReactorLead":
				p.PlantOperation.LeadLagMode = ReactorLead
			default:
				return fmt.Errorf(
					"invalid LeadLagMode %q (want 0..1)", s)
			}
			return nil
		},
	}

	// RPC auto rod control engage (chapter 2.8): the RPC screen
	// shows "auto/manual mode (changeable by user)" for the gray
	// rod drive.
	//   specs/pwr_sim/chapter_2/page_024.txt:23-24
	m["PlantOperation/RodControlAuto"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "RodControlAuto",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(p.PlantOperation.RodControlAuto)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.PlantOperation.RodControlAuto = v
			return nil
		},
	}

	// Stepback-failure malfunction insert: models the training
	// malfunction that impairs the plant control system's power
	// reduction, leaving the reactor shutdown system as the only
	// remaining protection.
	//   "This malfunction event impairs the first line of protective
	//    action initiated by the reactor control system ... However,
	//    the reactor shutdown system (SDS) is always poised to act"
	//   specs/pwr_sim/chapter_4/page_051.txt:5-9
	m["PlantOperation/StepbackFailed"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "StepbackFailed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(p.PlantOperation.StepbackFailed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.PlantOperation.StepbackFailed = v
			return nil
		},
	}

	// Reactor power setpoint target (chapter 3 pop-up). The operator
	// enters percent — "Enter 'reactor power SP target' = 90 %"
	// specs/pwr_sim/chapter_3/page_036.txt:39-40 — while storage and
	// the RPC integrator work in fractions of rated power, so the
	// boundary converts once in each direction.
	m["PlantOperation/PowerSetpointTarget"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "PowerSetpointTarget",
		Kind:  "percent",
		Get: func() string {
			return strconv.FormatFloat(p.PlantOperation.PowerSetpointDemand*100, 'f', -1, 64)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("percent", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.PlantOperation.PowerSetpointDemand = v / 100
			return nil
		},
	}

	// Reactor power ramp rate (chapter 3 pop-up), entered as percent
	// of rated power per second — "enter 'power rate' = 0.08 %/sec"
	// specs/pwr_sim/chapter_3/page_036.txt:39-40 — limited by the
	// maximum rate of 0.8 % of full power per second
	// specs/pwr_sim/chapter_2/page_024.txt:3-5 (RodControlMaxRatePerSec
	// stores that cap as a fraction). Storage stays fraction-per-second;
	// without the ÷100 here the documented entry reads as ten times the
	// cap and the clamp silently hides it.
	m["PlantOperation/PowerRateDemand"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "PowerRateDemand",
		Kind:  "pctpersec",
		Get: func() string {
			return strconv.FormatFloat(p.PlantOperation.PowerRateDemand*100, 'f', -1, 64)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("pctpersec", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.PlantOperation.PowerRateDemand = v / 100
			return nil
		},
	}

	// Dispatch permissive: the operator's grant that lets remote
	// dispatch demands reach the turbine load reference.
	// "to act as a permissive network so that unsafe demands from
	// the computer [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:10-12
	m["PlantOperation/LoadFollow"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "LoadFollow",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(p.PlantOperation.LoadFollowActive)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.PlantOperation.LoadFollowActive = v
			return nil
		},
	}
}

// addPlantOperationSensors registers the reactor power control
// observables demanded by the evolution's Table II: actual/demanded
// setpoint, power error, and Tref.
//
// Spec justification:
//   - "the POWER ERROR is also determined from current reactor
//     power minus demanded power setpoint"
//     specs/pwr_sim/chapter_2/page_024.txt:17-18
func addPlantOperationSensors(p *Plant, m bridge.SensorMap) {
	m["PlantOperation/DemandedPowerSetpoint"] = &bridge.Sensor{
		Part:  "PlantOperation",
		Label: "DemandedPowerSetpoint",
		Kind:  "normalized",
		Get: func() (string, error) {
			// Both lead modes through one demand source: the walked
			// integrator under reactor lead, the turbine's setpoint
			// under turbine lead (where the integrator shadows
			// achieved power and would display nothing useful).
			// chapter_7/page_130.txt:69-71.
			return bridge.FormatSensorValue("normalized",
				p.PlantOperation.boronDemandPower(
					p.TurbineBuilding.Turbine.Setpoint))
		},
	}

	m["PlantOperation/Tref"] = &bridge.Sensor{
		Part:  "PlantOperation",
		Label: "Tref",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			// The reference the rod loop actually steers by:
			// evaluated at the shared demand source so the panel
			// matches the steering input in both lead modes.
			return bridge.FormatSensorValue("fahrenheit",
				p.PlantOperation.referenceTempFor(
					p.PlantOperation.boronDemandPower(
						p.TurbineBuilding.Turbine.Setpoint)))
		},
	}

	// Power error is signed, so it bypasses the non-negative
	// numeric kinds and formats directly (same pattern as the
	// turbine state sensor).
	m["PlantOperation/PowerError"] = &bridge.Sensor{
		Part:  "PlantOperation",
		Label: "PowerError",
		Kind:  "string",
		Get: func() (string, error) {
			powerError := p.PlantOperation.boronDemandPower(
				p.TurbineBuilding.Turbine.Setpoint) -
				fissionFraction(
					p.ReactorBuilding.ReactorPressureVessel.FissionPower)
			return fmt.Sprintf("%+.4f", powerError), nil
		},
	}
}

// GetSensorMap returns a bridge.SensorMap populated with sensors for the
// given plant. Each Get closure calls bridge.FormatSensorValue according to
// Kind.
func GetSensorMap(p *Plant) bridge.SensorMap {
	m := make(bridge.SensorMap)
	addPlantOperationSensors(p, m)
	addReactorBuildingSensors(p, m)
	addTurbineBuildingSensors(p, m)
	addAuxiliaryBuildingSensors(p, m)
	addOutsideSensors(p, m)
	return m
}

// GetControlMap returns a populated ControlMap for this plant.
func (p *Plant) GetControlMap() bridge.ControlMap {
	return GetControlMap(p)
}

// GetSensorMap returns a populated SensorMap for this plant.
func (p *Plant) GetSensorMap() bridge.SensorMap {
	return GetSensorMap(p)
}
