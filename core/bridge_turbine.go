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


	// Turbine governing valves. The DEHM trims them against the
	// active reference, but the operator can position the valve
	// bank manually; cracking them open is also what starts steam
	// flow from the Off state (the DEHM trim only runs while the
	// machine is already rolling):
	//   - "A manual raise-lower backup system to position the
	//      turbine valves in the case of a system contingency."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:95-97
	//   - "The operator may position the valves, when necessary,
	//      through the manual valve controllers."
	//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:114-116
	m["TurbineBuilding/Turbine/GoverningValve"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "GoverningValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.TurbineBuilding.Turbine.GoverningValve.Open)
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
			p.TurbineBuilding.Turbine.GoverningValve.Open = v
			return nil
		},
	}


	// Momentary turbine trip reset. Consumed by the next
	// Turbine.Step; the machine returns to service only once the
	// rotor has decayed to rest. Restoring valve control from the
	// operator's panel after the auto-stop-oil trip latch:
	//   "The operator may position the valves, when necessary,
	//    through the manual valve controllers."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:114-116
	m["TurbineBuilding/Turbine/Reset"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "Reset",
		Kind:  "enum",
		Enum:  []string{"Reset"},
		Get:   func() string { return "0" },
		Set: func(_ string) error {
			p.TurbineBuilding.Turbine.ResetRequest = true
			return nil
		},
	}


	// Feedwater regulator valve demand (chapter 15):
	// "Feedwater flow to each steam generator is controlled by
	// a feedwater regulator in each feedwater line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_219.txt:69-73
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


	// Manual transfer of SG level control: while latched, the
	// regulator holds the operator's ValveDemand instead of the
	// three-element law's. The unload evolution transfers at
	// house-load level.
	// "reactor control and steam generator water level control are
	//  transferred to manual operation"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:156-159
	m["TurbineBuilding/FeedwaterRegulator/ManualControl"] = &bridge.Control{
		Part:  "TurbineBuilding/FeedwaterRegulator",
		Label: "ManualControl",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.FeedwaterRegulator.ManualControl)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.FeedwaterRegulator.ManualControl = v
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


	// Turbine speed demand (chapter 15): pre-breaker wide-range
	// speed control reference as a fraction of rated speed.
	// "(1) Wide range speed control of the turbine from turning gear
	// to rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-76
	m["TurbineBuilding/Turbine/SpeedDemand"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "SpeedDemand",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.TurbineBuilding.Turbine.SpeedDemand)
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
			p.TurbineBuilding.Turbine.SpeedDemand = v
			return nil
		},
	}


	// Turning gear (chapter 15 startup): holds the rotor at a slow
	// roll with no steam admission.
	m["TurbineBuilding/Turbine/TurningGear"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "TurningGear",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(p.TurbineBuilding.Turbine.TurningGear)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Turbine.TurningGear = v
			return nil
		},
	}


	// Motor-driven auxiliary lube-oil pump (chapter 15): the DEHM
	// supervises operation "from checking the bearing oil pump before
	// startup"; loss of the header trips the turbine.
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_203.txt:87-89
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
	m["TurbineBuilding/Turbine/LubeOilPump"] = &bridge.Control{
		Part:  "TurbineBuilding/Turbine",
		Label: "LubeOilPump",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.Turbine.LubeOilPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Turbine.LubeOilPumpRunning = v
			return nil
		},
	}


	// Condenser vacuum pump / steam-jet air ejector (chapter 15):
	// operator handle for developing condenser vacuum before steam
	// is admitted during plant startup.
	// "Under normal operation the air is removed by steam jet air
	//  ejectors or vacuum pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
	m["TurbineBuilding/Condenser/VacuumPump"] = &bridge.Control{
		Part:  "TurbineBuilding/Condenser",
		Label: "VacuumPump",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.Condenser.VacuumPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Condenser.VacuumPumpRunning = v
			return nil
		},
	}


	// Condensate pump (chapter 15): takes suction from the
	// condenser/CST path feeding the heater drain tank ahead of
	// the main feedwater pumps; started with the conventional
	// condensate and feedwater systems during secondary plant
	// startup.
	// "Conventional condensate and feedwater systems startup
	//  procedures would be followed."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:175-177
	m["TurbineBuilding/CondensatePump/Running"] = &bridge.Control{
		Part:  "TurbineBuilding/CondensatePump",
		Label: "Running",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.CondensatePump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.CondensatePump.Running = v
			return nil
		},
	}


	// Main feedwater pump (chapter 16 startup): the SG feedwater
	// supply is switched from the auxiliary feedwater system to
	// this system before AFW flow capacity (~3.5-percent steam
	// flow) is reached.
	// "the steam generators feedwater supply is switched to the
	//  main feedwater system; the auxiliary feedwater system is
	// secured and lined up for safeguards actuation."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:45-50
	// The operator starts the main feedwater pumps to place the
	// main feedwater system in service; the running latch gates
	// the feedwater regulator.
	// "The feedwater booster pumps draw suction from the deaerator
	//  water storage tank [...] and supply the main feedwater
	//  pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:46-50
	m["TurbineBuilding/MainFeedwaterPump/Running"] = &bridge.Control{
		Part:  "TurbineBuilding/MainFeedwaterPump",
		Label: "Running",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.MainFeedwaterPump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.MainFeedwaterPump.Running = v
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

	// Main generator breaker. Closing it parallels the unit on the
	// grid once the synchronizer tolerance is met; Plant.Step pops
	// the breaker again whenever terminal frequency/voltage drift
	// outside the sync band, so an out-of-step close cannot stick.
	//
	// Spec: "The turbine-generator is brought up to synchronous speed
	// and paralleled on the grid."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:52-53
	// "Accelerate the main turbine to 1800 rpm, and then synchronize
	// the generator and connect it to the grid."
	// specs/westinghouse_plant_operations/chapter_19/page_018.txt:12-13
	m["TurbineBuilding/Generator/BreakerClosed"] = &bridge.Control{
		Part:  "TurbineBuilding/Generator",
		Label: "BreakerClosed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.TurbineBuilding.Generator.BreakerClosed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.TurbineBuilding.Generator.BreakerClosed = v
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


	// Governing-valve bank position (chapter 15): DEHM-positioned
	// governing valves throttle admission behind the stop valves.
	// "The control signal is transmitted to the electro-hydraulic
	// actuator on the main steam governing valves and reheat steam
	// intercept valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:41-44
	m["TurbineBuilding/Turbine/GoverningValve"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "GoverningValve",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.TurbineBuilding.Turbine.GoverningValve.Open)
		},
	}


	// First-stage pressure (chapter 15): DEHM controller input.
	// "computes signals comparing turbine speed and first-stage
	// pressure with reference values established by the dispatcher or
	// initiated at the operator's panel."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:37-41
	m["TurbineBuilding/Turbine/FirstStagePressure"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "FirstStagePressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.TurbineBuilding.Turbine.FirstStagePressure)
		},
	}


	// Bearing-oil header pressure (chapter 15): trip input for the
	// low bearing oil pressure device.
	// "Low bearing oil pressure trip"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25
	m["TurbineBuilding/Turbine/BearingOilPressure"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "BearingOilPressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.TurbineBuilding.Turbine.BearingOilPressure)
		},
	}


	// Condenser back-pressure (chapter 15): the vacuum the operator
	// develops before rolling the turbine; low vacuum trips it.
	// "Under normal operation the air is removed by steam jet air
	//  ejectors or vacuum pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:19-22
	m["TurbineBuilding/Condenser/Pressure"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Condenser",
		Label: "Pressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.TurbineBuilding.Condenser.Pressure)
		},
	}


	// Extraction nonreturn valve status (chapter 15): latched shut on
	// any turbine trip.
	// "Extraction line nonreturn valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:83
	m["TurbineBuilding/Turbine/ExtractionNRVClosed"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Turbine",
		Label: "ExtractionNRVClosed",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.BoolToStr(
				p.TurbineBuilding.Turbine.ExtractionNRVClosed), nil
		},
	}


	// Generator MVA-limit alarm (chapter 15): apparent power exceeded
	// the rating and the DEHM clamped output onto the rating circle.
	// "output power, the power factor, and speed to ensure that the
	// limits of the generator are not exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
	m["TurbineBuilding/Generator/MVALimitAlarm"] = &bridge.Sensor{
		Part:  "TurbineBuilding/Generator",
		Label: "MVALimitAlarm",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.BoolToStr(
				p.TurbineBuilding.Generator.GeneratorMVALimitAlarm), nil
		},
	}

}
