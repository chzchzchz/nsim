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

	// Reactor coolant pump running (chapter 4 startup / cooldown):
	// "One reactor coolant pump is kept running ... until the cooling
	// temperature is below 160°F (71°C)."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:37-40
	m["ReactorBuilding/ReactorCoolantPump/Running"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorCoolantPump",
		Label: "Running",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorCoolantPump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorCoolantPump.Running = v
			return nil
		},
	}

	// Pressurizer spray valve (chapter 7 pressure control):
	// "The pressurizer pressure control system ... spray ..."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
	m["ReactorBuilding/Pressurizer/SprayOpen"] = &bridge.Control{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "SprayOpen",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.ReactorBuilding.Pressurizer.SprayOpen)
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
			p.ReactorBuilding.Pressurizer.SprayOpen = v
			return nil
		},
	}

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

	// Reactor makeup control valve (chapter 4):
	// "Because of the influx of makeup water from the reactor
	// makeup control system [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
	m["ReactorBuilding/ReactorMakeupControlValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorMakeupControlValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.ReactorMakeupControlValve.Open)
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
			p.ReactorBuilding.ReactorMakeupControlValve.Open = v
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

	// Pressurizer heaters (chapter 16 startup):
	// "All pressurizer heaters are energized to start the
	// heatup of the pressurizer necessary to form the
	// steam bubble."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-16
	m["ReactorBuilding/Pressurizer/HeatersOn"] = &bridge.Control{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "HeatersOn",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.Pressurizer.HeatersOn)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.Pressurizer.HeatersOn = v
			return nil
		},
	}

	// Reactor trip (chapter 7 protection system):
	// "Manual trip signal – Redundant control
	// board pushbuttons are provided."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
	m["ReactorBuilding/ReactorPressureVessel/Trip"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "Trip",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorPressureVessel.Trip)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorPressureVessel.Trip = v
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

	// Control rod position (chapter 7):
	// "The control rods are then manually withdrawn to achieve criticality"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:155-156
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/Position"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "Position",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position)
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
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position = v
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

	// Diesel generator start signal (chapter 11):
	// "controls for the governor, voltage regulator, generator
	// breaker, and synchronization for each diesel generator are
	// located on physically separated portions of the main control
	// board."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:2-4
	// Cause taxonomy in core/parts.go:1069-1077:
	//   0 = None, 1 = Manual, 2 = SafetyInjection,
	//   3 = LossOfOffsitePower, 4 = Test.
	// "The onsite emergency diesel generators are started."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:18-22
	// "In the event of total loss of auxiliary power from off-site
	// sources, auxiliary power required for safe shutdown will be
	// supplied from physically and electrically independent diesel
	// generators located on the site."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:28-36
	m["Outside/DieselGenerator/StartSignal"] = &bridge.Control{
		Part:  "Outside/DieselGenerator",
		Label: "StartSignal",
		Kind:  "enum",
		Get: func() string {
			return fmt.Sprintf("%d",
				p.Outside.DieselGenerator.StartSignal)
		},
		Set: func(s string) error {
			switch s {
			case "0":
				p.Outside.DieselGenerator.StartSignal = DieselStartNone
			case "1":
				p.Outside.DieselGenerator.StartSignal = DieselStartManual
			case "2":
				p.Outside.DieselGenerator.StartSignal = DieselStartSafetyInjection
			case "3":
				p.Outside.DieselGenerator.StartSignal = DieselStartLossOfOffsitePower
			case "4":
				p.Outside.DieselGenerator.StartSignal = DieselStartTest
			default:
				return fmt.Errorf("invalid StartSignal %q (want 0..4)", s)
			}
			return nil
		},
	}

	// Diesel generator load fraction (chapter 11):
	// "Loads are automatically sequenced on the diesel generator
	// buses over a short period of time to preclude the possibility
	// of overload trip due to simultaneous starting loads."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:137-140
	// The plant state machine multiplies fuel burn and generator
	// real power by this fraction
	// (core/diesel.go:114,173).
	m["Outside/DieselGenerator/RunningFraction"] = &bridge.Control{
		Part:  "Outside/DieselGenerator",
		Label: "RunningFraction",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.Outside.DieselGenerator.RunningFraction)
		},
		Set: func(s string) error {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			if f < 0 || f > 1 {
				return fmt.Errorf("normalized %v out of range [0,1]", f)
			}
			p.Outside.DieselGenerator.RunningFraction = float32(f)
			return nil
		},
	}

	// Diesel generator breaker (chapter 11):
	// "controls for the governor, voltage regulator, generator
	// breaker, and synchronization for each diesel generator are
	// located on physically separated portions of the main control
	// board."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:2-4
	// Without BreakerClosed the diesel cannot feed the ESF bus:
	// core/electrical.go:70 requires src.Voltage >= pickup AND
	// src.BreakerClosed.
	m["Outside/DieselGenerator/Generator/BreakerClosed"] = &bridge.Control{
		Part:  "Outside/DieselGenerator/Generator",
		Label: "BreakerClosed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.Outside.DieselGenerator.Generator.BreakerClosed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.Outside.DieselGenerator.Generator.BreakerClosed = v
			return nil
		},
	}

	// Engineered Safety Features bus A breaker (chapter 11):
	// "Each power source, diesel generator and offsite, up to the
	// point of connection to the engineered safety features system
	// power buses, is physically and electrically independent."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:29-36
	// "The engineered safety features buses are normally energized
	// from system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:81-82
	// Bus pickup (core/electrical.go) requires this breaker closed
	// before a source can deliver voltage to the train.
	m["Electrical/ESFBusA/BreakerClosed"] = &bridge.Control{
		Part:  "Electrical/ESFBusA",
		Label: "BreakerClosed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(p.Electrical.ESFBusA.BreakerClosed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.Electrical.ESFBusA.BreakerClosed = v
			return nil
		},
	}

	// Engineered Safety Features bus B breaker (chapter 11):
	// Same justification as ESF bus A above.
	m["Electrical/ESFBusB/BreakerClosed"] = &bridge.Control{
		Part:  "Electrical/ESFBusB",
		Label: "BreakerClosed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(p.Electrical.ESFBusB.BreakerClosed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.Electrical.ESFBusB.BreakerClosed = v
			return nil
		},
	}

	return m
}

// GetSensorMap returns a bridge.SensorMap populated with sensors for the
// given plant. Each Get closure calls bridge.FormatSensorValue according to
// Kind.
func GetSensorMap(p *Plant) bridge.SensorMap {
	m := make(bridge.SensorMap)

	// Reactor coolant outlet temperature (chapter 3 design data):
	// "Reactor coolant inlet temperature: 621°F (327°C);
	// outlet: 558°F (292°C)"
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60-61
	m["ReactorBuilding/ReactorPressureVessel/Outlet"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "Outlet",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.ReactorBuilding.ReactorPressureVessel.Outlet.Float())
		},
	}

	// Steam generator pressure:
	m["ReactorBuilding/SteamGenerator/Pressure"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator",
		Label: "Pressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.ReactorBuilding.SteamGenerator.Pressure)
		},
	}

	// Pressurizer pressure (chapter 16):
	// "operation of pressurizer heaters and spray maintains the system
	// pressure in accordance with the applicable temperature-pressure
	// curves."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:42-43
	m["ReactorBuilding/Pressurizer/Pressure"] = &bridge.Sensor{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "Pressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.ReactorBuilding.Pressurizer.Pressure)
		},
	}

	// Steam generator water inventory:
	m["ReactorBuilding/SteamGenerator/Water"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator",
		Label: "Water",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.ReactorBuilding.SteamGenerator.Water.Float())
		},
	}

	// Containment atmosphere temperature (chapter 6):
	// "As this water comes in contact with the steam in the
	// containment atmosphere, the steam is condensed"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:56-57
	m["ReactorBuilding/ContainmentBuilding/Temperature"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ContainmentBuilding",
		Label: "Temperature",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.ReactorBuilding.ContainmentBuilding.Temperature.Float())
		},
	}

	// Diesel generator coolant temperature (chapter 11):
	// "Each diesel engine's fuel oil system, air starting
	// system, and the generator output and excitation systems
	// are equipped with instrumentation to monitor all
	// important parameters"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:99-101
	m["Outside/DieselGenerator/CoolantTemp"] = &bridge.Sensor{
		Part:  "Outside/DieselGenerator",
		Label: "CoolantTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.Outside.DieselGenerator.CoolantTemp.Float())
		},
	}

	// Component cooling water system outlet temperature (chapter 5):
	// "The component cooling water inlet temperature to various
	// components during normal cooldown is permitted to increase
	// to 120°F (49°C)"
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-110
	m["ReactorBuilding/ComponentCoolingWaterSystem/OutletTemp"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ComponentCoolingWaterSystem",
		Label: "OutletTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.ReactorBuilding.ComponentCoolingWaterSystem.OutletTemp.Float())
		},
	}

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

	// Reactor coolant pump running (chapter 4 startup / cooldown):
	// "One reactor coolant pump is kept running ... until the cooling
	// temperature is below 160°F (71°C)."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:37-40
	m["ReactorBuilding/ReactorCoolantPump/Running"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorCoolantPump",
		Label: "Running",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.ReactorCoolantPump.Running)
		},
	}

	// Reactor coolant inlet temperature (chapter 3 design data):
	// "Reactor coolant inlet temperature: 621°F (327°C);
	// outlet: 558°F (292°C)"
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:60
	m["ReactorBuilding/ReactorPressureVessel/Inlet"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "Inlet",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.ReactorBuilding.ReactorPressureVessel.Inlet.Float())
		},
	}

	// Reactor pressure vessel pressure:
	// Operating pressure 2250 psia (155 bar); design pressure 2500 psia.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:45
	m["ReactorBuilding/ReactorPressureVessel/Pressure"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "Pressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.ReactorBuilding.ReactorPressureVessel.Pressure)
		},
	}

	// Steam generator total volume (chapter 3 design data):
	// Table 3.4-1: "Overall height: 67 ft, 8 in."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:25-44
	m["ReactorBuilding/SteamGenerator/Volume"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator",
		Label: "Volume",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.ReactorBuilding.SteamGenerator.Volume.Float())
		},
	}

	// Reactor coolant boron concentration (chapter 7):
	// "Control of the reactor coolant's boron concentration
	// involves both the Boron Thermal Regeneration System (BTRS)
	// and the Reactor Water Makeup System."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:37-38
	m["ReactorBuilding/ReactorPressureVessel/BoronConcentration"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "BoronConcentration",
		Kind:  "ppm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("ppm",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration)
		},
	}

	// Pressurizer water inventory (chapter 16 startup):
	// "Drain pressurizer to no-load operating level [...] lower
	// pressurizer level to the normal range of 20 to 80 percent"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:22-23
	m["ReactorBuilding/Pressurizer/Water"] = &bridge.Sensor{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "Water",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.ReactorBuilding.Pressurizer.Water.Float())
		},
	}

	// Containment pressure (chapter 5 safety injection / chapter 7 RPS):
	// "High containment pressure as reported by two out of four signals."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:97
	m["ReactorBuilding/ContainmentPressure"] = &bridge.Sensor{
		Part:  "ReactorBuilding",
		Label: "ContainmentPressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.ReactorBuilding.ContainmentPressure)
		},
	}

	// Diesel generator ready-to-load status (chapter 11):
	// "designed to reach rated speed and be ready to
	// begin accepting load within 10 seconds"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:48-49
	m["Outside/DieselGenerator/ReadyToLoad"] = &bridge.Sensor{
		Part:  "Outside/DieselGenerator",
		Label: "ReadyToLoad",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.Outside.DieselGenerator.ReadyToLoad)
		},
	}

	// Diesel fuel oil tank level (chapter 11):
	// "Each diesel generator is provided with a fuel oil day tank
	// and a fuel storage tank. The onsite supply of fuel for each
	// diesel generator is sufficient to allow continuous rated load
	// operation for a minimum of seven days."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:118-122
	m["Outside/DieselGenerator/Fuel"] = &bridge.Sensor{
		Part:  "Outside/DieselGenerator",
		Label: "Fuel",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.Outside.DieselGenerator.Fuel.Level.Float())
		},
	}

	// Containment spray header isolation valves "A" (chapter 6):
	// "This signal starts the spray pumps, opens the spray header
	// isolation valves "A""
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:59-60
	m["ReactorBuilding/ContainmentSpraySystem/SprayHeaderOpen"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ContainmentSpraySystem",
		Label: "SprayHeaderOpen",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.ContainmentSpraySystem.SprayHeaderOpen)
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

	// Engineered Safety Features bus A voltage (chapter 11):
	// "All of the engineered safety system loads are fed from two
	// 6600-volt engineered safety features buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
	m["Electrical/ESFBusA/Voltage"] = &bridge.Sensor{
		Part:  "Electrical/ESFBusA",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.Electrical.ESFBusA.Voltage)
		},
	}

	// Engineered Safety Features bus A energized status (chapter 11):
	// "The engineered safety features buses are normally energized
	// from system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:81-82
	m["Electrical/ESFBusA/Energized"] = &bridge.Sensor{
		Part:  "Electrical/ESFBusA",
		Label: "Energized",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.Electrical.ESFBusA.Energized)
		},
	}

	// Engineered Safety Features bus B voltage (chapter 11):
	// "All of the engineered safety system loads are fed from two
	// 6600-volt engineered safety features buses."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:72-74
	m["Electrical/ESFBusB/Voltage"] = &bridge.Sensor{
		Part:  "Electrical/ESFBusB",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.Electrical.ESFBusB.Voltage)
		},
	}

	// Engineered Safety Features bus B energized status (chapter 11):
	// "The engineered safety features buses are normally energized
	// from system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:81-82
	m["Electrical/ESFBusB/Energized"] = &bridge.Sensor{
		Part:  "Electrical/ESFBusB",
		Label: "Energized",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.Electrical.ESFBusB.Energized)
		},
	}

	// Diesel generator output voltage (chapter 11):
	// "The diesel generators are provided with static-type brushless
	// excitation and field flashing for rapid voltage buildup."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:46-47
	m["Outside/DieselGenerator/Generator/Voltage"] = &bridge.Sensor{
		Part:  "Outside/DieselGenerator/Generator",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.Outside.DieselGenerator.Generator.Voltage)
		},
	}

	// Diesel generator output frequency (chapter 11):
	// "60 Hz output which feeds its corresponding"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_168.txt:15
	m["Outside/DieselGenerator/Generator/Frequency"] = &bridge.Sensor{
		Part:  "Outside/DieselGenerator/Generator",
		Label: "Frequency",
		Kind:  "hz",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("hz",
				p.Outside.DieselGenerator.Generator.Frequency)
		},
	}

	// Diesel generator output real power (chapter 11):
	// "Each diesel engine's fuel oil system, air starting
	// system, and the generator output and excitation systems
	// are equipped with instrumentation to monitor all
	// important parameters and to annunciate abnormal
	// conditions."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:97-101
	// "capable of carrying rated load within 30 seconds after
	// receipt of a start signal"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:133-136
	m["Outside/DieselGenerator/Generator/RealPower"] = &bridge.Sensor{
		Part:  "Outside/DieselGenerator/Generator",
		Label: "RealPower",
		Kind:  "watt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("watt",
				p.Outside.DieselGenerator.Generator.RealPower)
		},
	}

	// Grid frequency (chapter 11 / turbine governor reference):
	// "Wide range speed control of the turbine from turning
	// gear to rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
	m["ReactorBuilding/Grid/Frequency"] = &bridge.Sensor{
		Part:  "ReactorBuilding/Grid",
		Label: "Frequency",
		Kind:  "hz",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("hz",
				p.ReactorBuilding.Grid.Frequency)
		},
	}

	// Off-site grid voltage (chapter 16):
	// "If offsite power is not available, the long coast"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64
	m["ReactorBuilding/Grid/Voltage"] = &bridge.Sensor{
		Part:  "ReactorBuilding/Grid",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.ReactorBuilding.Grid.Voltage)
		},
	}

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
