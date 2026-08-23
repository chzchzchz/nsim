package core

import (
	"fmt"
	"strconv"

	"github.com/chzchzchz/nsim/bridge"
)

// addTurbineBuildingControls adds Control entries keyed by the namespace managed by this file to m.
func addTurbineBuildingControls(p *Plant, m bridge.ControlMap) {

	// Turbine stop valve (chapter 15 trip system):
	// "Release of this oil, called auto stop oil, by any one of the
	// turbine trip devices [...] causes all steam control valves to close."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:28-34
	m["TurbineBuilding/Turbine/StopValve"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "StopValve",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.Turbine.StopValve.Open > 0)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Turbine.StopValve.Open = 0
			if v {
				p.TurbineBuilding.Turbine.StopValve.Open = 1
			}
			return nil
		},
	}


	// Feedwater regulator valve demand (chapter 15):
	// "Feedwater flow to each steam generator is controlled by
	// a feedwater regulator in each feedwater line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-72
	m["TurbineBuilding/FeedwaterRegulator/ValveDemand"] = &bridge.Control{
		Part:  "TurbineBuilding/FeedwaterRegulator",
		Label: "ValveDemand",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.TurbineBuilding.FeedwaterRegulator.ValveDemand)
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
			p.TurbineBuilding.FeedwaterRegulator.ValveDemand = v
			return nil
		},
	}


	// Turbine load setpoint (chapter 15):
	// "After generator breaker closure, the ability to increase or decrease
	// electrical load at a controlled selectable rate."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:80-82
	m["TurbineBuilding/Turbine/Setpoint"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "Setpoint",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.TurbineBuilding.Turbine.Setpoint)
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
			p.TurbineBuilding.Turbine.Setpoint = v
			return nil
		},
	}


	// Turbine dump valve (chapter 15 steam dump system):
	// "These valves open under a sudden reduction in
	// turbine-generator load (in excess of 10%) or a turbine trip."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_205.txt:9-10
	// "Condenser steam dump valves." (protective feature 8)
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:91
	m["TurbineBuilding/Turbine/DumpValve"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "DumpValve",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.Turbine.DumpValve.Open > 0)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Turbine.DumpValve.Open = 0
			if v {
				p.TurbineBuilding.Turbine.DumpValve.Open = 1
			}
			return nil
		},
	}


	// Turbine load runback (chapter 15):
	// "Automatic load runback initiated by a dropped rod signal"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26
	m["TurbineBuilding/Turbine/LoadRunback"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "LoadRunback",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.Turbine.LoadRunback)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Turbine.LoadRunback = v
			return nil
		},
	}


	// Main feedwater control valve (chapter 15):
	// "A signal from the Feedwater Control System also sets
	// the position of the motor-driven feedwater pump discharge
	// control valve so as to maintain the main feedwater
	// regulators within their control range."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:73-76
	m["TurbineBuilding/MainFeedwaterControlValve/Open"] = &bridge.Control{
		Part:  "TurbineBuilding/MainFeedwaterControlValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.TurbineBuilding.MainFeedwaterControlValve.Open)
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
			p.TurbineBuilding.MainFeedwaterControlValve.Open = v
			return nil
		},
	}

}

// addTurbineBuildingSensors adds Sensor entries keyed by the namespace managed by this file to m.
func addTurbineBuildingSensors(p *Plant, m bridge.SensorMap) {

	// Moisture separator-reheater outlet temperature (chapter 15):
	// "reheat the steam to over 100°F (55°C) superheat"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_214.txt:55
	m["TurbineBuilding/MoistureSeparatorReheater/OutletTemperature"] = &bridge.Sensor{
		Part:  "TurbineBuilding/MoistureSeparatorReheater",
		Label: "OutletTemperature",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.TurbineBuilding.MoistureSeparatorReheater.OutletTemperature.Float())
		},
	}


	// Turbine velocity (chapter 15 design data):
	// "Speed: 1800 rpm"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:22
	m["TurbineBuilding/Turbine/Velocity"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "Velocity",
		Kind:  "rpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("rpm",
				p.TurbineBuilding.Turbine.Velocity)
		},
	}


	// Turbine real power (chapter 15):
	// "Westinghouse supplies nuclear turbinegenerator systems over
	// a load range of 600 megawatts to 1500 megawatts in three-
	// and four-cylinder configurations for any light water
	// reactor."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_197.txt:10-13
	m["TurbineBuilding/Turbine/RealPower"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "RealPower",
		Kind:  "watt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("watt",
				p.TurbineBuilding.Turbine.RealPower)
		},
	}


	// Generator net real power (chapter 15):
	// "The steam by rotating the blades of the turbine elements
	// (cylinders) rotates the shaft of the generator. The
	// generator produces the electric power of the power plant."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_197.txt:21-23
	// DEHM "monitors output power, the power factor, and speed
	// to ensure that the limits of the generator are not
	// exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_203.txt:32-36
	// Design point: 1370 MVA at 0.9 power factor
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:96-97
	m["TurbineBuilding/Generator/RealPower"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Generator",
		Label: "RealPower",
		Kind:  "watt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("watt",
				p.TurbineBuilding.Generator.RealPower)
		},
	}


	// Turbine state (chapter 15):
	m["TurbineBuilding/Turbine/State"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "State",
		Kind:  "string",
		Get: func() (string, error) {
			switch p.TurbineBuilding.Turbine.State {
			case 0:
				return "turning_gear", nil
			case 1:
				return "starting", nil
			case 2:
				return "running", nil
			case 3:
				return "tripped", nil
			default:
				return fmt.Sprintf("%d", p.TurbineBuilding.Turbine.State), nil
			}
		},
	}


	// Main generator output voltage (chapter 15):
	// "Output voltage: 24,000 to 26,000 volts, three-phase, 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	m["TurbineBuilding/Generator/Voltage"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Generator",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.TurbineBuilding.Generator.Voltage)
		},
	}


	// Main generator frequency (chapter 15):
	// "Output voltage: 24,000 to 26,000 volts, three-phase, 60 Hz"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_208.txt:21
	m["TurbineBuilding/Generator/Frequency"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Generator",
		Label: "Frequency",
		Kind:  "hz",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("hz",
				p.TurbineBuilding.Generator.Frequency)
		},
	}

}
