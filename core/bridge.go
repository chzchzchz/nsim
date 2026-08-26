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

	// Reactor power setpoint target (chapter 3 pop-up):
	//   "Enter 'reactor power SP target' = 90 %"
	//   specs/pwr_sim/chapter_3/page_036.txt:37-38
	m["PlantOperation/PowerSetpointTarget"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "PowerSetpointTarget",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.PlantOperation.PowerSetpointDemand)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("normalized", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.PlantOperation.PowerSetpointDemand = v
			return nil
		},
	}

	// Reactor power ramp rate (chapter 3 pop-up), entered as a
	// fraction of rated power per second; the integrator clamps
	// it to RodControlMaxRatePerSec (0.008 = 0.8 %/s).
	//   "limited by the maximum rate of 0.8 % of full power per
	//    second"
	//   specs/pwr_sim/chapter_2/page_024.txt:3-5
	m["PlantOperation/PowerRateDemand"] = &bridge.Control{
		Part:  "PlantOperation",
		Label: "PowerRateDemand",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.PlantOperation.PowerRateDemand)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("normalized", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.PlantOperation.PowerRateDemand = v
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

	// Dispatcher load demand in MW against the 1000 MW machine
	// rating; the wiring normalizes it to the Setpoint fraction at
	// the model boundary (core/plant.go). Reaches the turbine load
	// reference only while the dispatch permissive above is engaged.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:44-47
	m["Electrical/Grid/LoadDemand"] = &bridge.Control{
		Part:  "Electrical/Grid",
		Label: "LoadDemand",
		Kind:  "mw",
		Get: func() string {
			return fmt.Sprintf("%v", p.ReactorBuilding.Grid.LoadDemand)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("mw", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.ReactorBuilding.Grid.LoadDemand = v
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
			return bridge.FormatSensorValue("normalized",
				p.PlantOperation.DemandedPowerSetpoint)
		},
	}

	m["PlantOperation/Tref"] = &bridge.Sensor{
		Part:  "PlantOperation",
		Label: "Tref",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.PlantOperation.referenceTempF())
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
			powerError := fissionFraction(
				p.ReactorBuilding.ReactorPressureVessel.FissionPower) -
				p.PlantOperation.boronDemandPower(
					p.TurbineBuilding.Turbine.Setpoint)
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
