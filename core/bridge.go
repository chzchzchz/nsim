package core

import (
	"encoding/json"
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

	// RHR train-A pump manual stop / start. The operator stops
	// the RHR pumps when plant startup begins; during cooldown
	// one train runs on miniflow until RCS is below 350°F.
	// "The RHR pumps are stopped when the plant startup
	// begins."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_085.txt:46-48
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Running = v
			return nil
		},
	}
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/PumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Running = v
			return nil
		},
	}

	// RHRS Train A hot-leg suction isolation valve pair.
	// Two series motor-operated valves on the RCS hot-leg
	// suction line; both must be open for the train to take
	// suction from the RCS. The bridge exposes the first
	// valve (Valve 1 of 2) — closing it disables the train.
	// "series isolation valves providing the isolation [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_084.txt:18
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/SuctionValve1/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA",
		Label: "SuctionValve1",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.SuctionValves[0].Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.SuctionValves[0].Open = v
			return nil
		},
	}

	// RHRS Train A hot-leg suction isolation valve pair
	// (second valve).
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/SuctionValve2/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA",
		Label: "SuctionValve2",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.SuctionValves[1].Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.SuctionValves[1].Open = v
			return nil
		},
	}

	// RHRS Train B hot-leg suction isolation valve pair
	// (first valve).
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/SuctionValve1/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB",
		Label: "SuctionValve1",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.SuctionValves[0].Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.SuctionValves[0].Open = v
			return nil
		},
	}

	// RHRS Train B hot-leg suction isolation valve pair
	// (second valve).
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/SuctionValve2/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB",
		Label: "SuctionValve2",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.SuctionValves[1].Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.SuctionValves[1].Open = v
			return nil
		},
	}

	// RHRS Train A bypass flow-control valve (around the RHR
	// heat exchanger). Open=1 means full bypass (no heat
	// rejection); Open=0 means full flow through the HX.
	// "A second flow control valve [...] bypass valve [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:58-66
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/BypassValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA",
		Label: "BypassValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.BypassValve.Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.BypassValve.Open = v
			return nil
		},
	}

	// RHRS Train B bypass flow-control valve.
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/BypassValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB",
		Label: "BypassValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.BypassValve.Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.BypassValve.Open = v
			return nil
		},
	}

	// EFWS motor-driven pump manual control. Auto-starts on
	// SI / low-low SG level; the operator manually stops it
	// when main feedwater is restored and manually starts it
	// for plant cooldown.
	// "Normal feedwater flow to the steam generators by the
	// main feedwater system is resumed and the emergency
	// feedwater pumps are manually stopped."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:88-91
	// "the pumps are manually started and the flowrates are
	// manually controlled by positioning the flow modulating
	// valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA",
		Label: "MotorPumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPumpRunning = v
			return nil
		},
	}
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "MotorPumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPumpRunning = v
			return nil
		},
	}
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePumpRunning = v
			return nil
		},
	}
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePumpRunning = v
			return nil
		},
	}

	// HHSI pump manual start. Manual starting of the HHSI
	// pumps from the main control board backs up the CVCS
	// charging pumps for RCS boration.
	// "Manual starting of the HHSI pumps will result in the
	// injection of borated water as RCS pressure is decreased
	// below the shutoff head of the pump."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:159-164
	m["ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning"] = &bridge.Control{
		Part:  "ReactorBuilding/SafetyInjectionSystem",
		Label: "HHSIPumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning = v
			return nil
		},
	}

	// Circulating water pump operator control. The operator
	// varies the number of CW pumps in service from the main
	// control room.
	// "Controls, interlocks, and indicators in the main
	// control room are provided to enable the operator to
	// modulate the circulating water pump discharge valves and
	// to vary the number of pumps in service."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:66-70
	m["Outside/CirculatingWaterSystem/PumpRunning"] = &bridge.Control{
		Part:  "Outside/CirculatingWaterSystem",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.Outside.CirculatingWaterPump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.Outside.CirculatingWaterPump.Running = v
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

	// Steam generator main steam isolation valve (chapter 15):
	// The MSIV isolates the SG secondary side from the
	// turbine and steam dump on a steam-line-break signal
	// or operator command.
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam
	// line upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	m["ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/MainSteamIsolationValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open)
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
			p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open = v
			return nil
		},
	}

	// Steam generator atmospheric dump valve (chapter 15).
	// The atmospheric dump valve (also called "steam dump to
	// atmosphere") is the operator handle for dumping excess
	// SG steam when the turbine is unavailable (load
	// rejection, turbine trip).
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam
	// line upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	m["ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/AtmosphericDumpValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open)
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
			p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open = v
			return nil
		},
	}

	// Steam generator safety valve (chapter 15).
	// Self-actuating ASME-Code spring-loaded safety valve;
	// the bridge exposes it as a control for completeness
	// (the Step function will also auto-trip it on
	// overpressure).
	// "Safety valves in each steam generator outlet main
	// steam line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:42-45
	m["ReactorBuilding/SteamGenerator/SafetyValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/SafetyValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.SafetyValve.Open)
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
			p.ReactorBuilding.SteamGenerator.SafetyValve.Open = v
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

	// Pressurizer power-operated relief valve (chapter 3
	// pressure-limiting train):
	// "Reductions in plant electrical load with resultant
	// pressure increases beyond the pressure-limiting
	// capability of the pressurizer spray system cause the
	// air-operated relief valves to open. These valves are
	// automatically opened at a pressure below system design
	// pressure. They can also be opened manually from a control
	// console in the control room."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:23-30
	m["ReactorBuilding/PORV/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/PORV",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.ReactorBuilding.PORV.Open)
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
			p.ReactorBuilding.PORV.Open = v
			return nil
		},
	}

	// Pressurizer ASME-code safety valves (chapter 3):
	// "If system pressure continues to rise, self-actuating
	// ASME-code safety valves will open. Steam from the
	// safety and/or relief valves is piped to the pressurizer
	// relief tank."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-34
	// These are spring-loaded and self-actuating; the bridge
	// exposes the aggregate state for visibility. The Step
	// function auto-trips the safety valve when pressure
	// exceeds PressurizerPORVTrip.
	m["ReactorBuilding/SafetyValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SafetyValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.ReactorBuilding.SafetyValve.Open)
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
			p.ReactorBuilding.SafetyValve.Open = v
			return nil
		},
	}

	// Reactor coolant loop block valve (chapter 3):
	// "Reactor coolant loop isolation valves may be closed
	// to isolate a loop."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_071.txt:145
	m["ReactorBuilding/BlockValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/BlockValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.ReactorBuilding.BlockValve.Open)
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
			p.ReactorBuilding.BlockValve.Open = v
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
	// The Set path writes the operator's demand to
	// DemandPosition; ReactorBuilding.Step rate-limits the
	// actual transition from Position toward DemandPosition
	// at RodInsertionSpeed (plans/x/fission_controls.md).
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
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandPosition = v
			return nil
		},
	}

	// CVCS boron concentration setpoint. The operator writes
	// the desired RCS boron concentration in ppm directly; this
	// feeds the reactivity model on the next step. The CVCS
	// rate dynamics (boration/dilution) still run in their
	// respective modes; the setpoint is the operator-visible
	// handle on the slow shim path.
	//   "The operating mode (boration or dilution) and the
	//   magnitude of the boron concentration change required
	//   is determined by comparison of the measured Tc
	//   (cold leg temperature) with the programmed (function
	//   of power) Tc."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
	//   "Operational Reactivity Control — Reactivity control is
	//   provided by the neutron absorbing control rods and a
	//   soluble chemical neutron absorber (boric acid) in the
	//   reactor coolant."
	//   specs/the_westinghouse_pressurized_water_react/chapter_2/page_036.txt:114
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/BoronSetpoint"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "BoronSetpoint",
		Kind:  "ppm",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("ppm", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			// Write to CVCS so the next step's propagation
			// (CVCS.BoronConcentration → RPV.ControlRods.BoronConcentration)
			// does not overwrite the operator's demand.
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronConcentration = v
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration = v
			return nil
		},
	}

	// CVCS charging-pump running latch. The CVCS Step reads
	// ChargingPumpRunning to decide ChargingFlow — this is the
	// operator-visible handle.
	// "The charging pump flow and low-pressure letdown valve
	// are controlled manually during plant startup and
	// cooldown."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingPumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "ChargingPumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = v
			return nil
		},
	}

	// CVCS letdown valve-open latch. The CVCS Step reads
	// LetdownValveOpen to decide LetdownFlow (in conjunction
	// with the operating mode — CVCSLetdown forces it closed).
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownValveOpen"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "LetdownValveOpen",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = v
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
	// Cause taxonomy in core/parts.go:1071-1077:
	//   0 = None, 1 = Manual, 2 = SafetyInjection,
	//   3 = LossOfOffsitePower, 4 = Test.
	// "The onsite emergency diesel generators are started."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:18-22
	// "In the event of total loss of auxiliary power from off-site
	// sources, auxiliary power required for safe shutdown will be
	// supplied from physically and electrically independent diesel
	// generators located on the site."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:28-36
	// Enum advertises the human-readable labels so the bridge
	// renders a dropdown instead of a text input. The indices
	// match the iota order at core/parts.go:1071-1077.
	m["AuxiliaryBuilding/DieselGenerator/StartSignal"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/DieselGenerator",
		Label: "StartSignal",
		Kind:  "enum",
		Enum: []string{
			"None",
			"Manual",
			"Safety Injection",
			"Loss of Offsite Power",
			"Test",
		},
		Get: func() string {
			return fmt.Sprintf("%d",
				p.AuxiliaryBuilding.DieselGenerator.StartSignal)
		},
		Set: func(s string) error {
			switch s {
			case "0":
				p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartNone
			case "1":
				p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
			case "2":
				p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
			case "3":
				p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartLossOfOffsitePower
			case "4":
				p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartTest
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
	m["AuxiliaryBuilding/DieselGenerator/RunningFraction"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/DieselGenerator",
		Label: "RunningFraction",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.DieselGenerator.RunningFraction)
		},
		Set: func(s string) error {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			if f < 0 || f > 1 {
				return fmt.Errorf("normalized %v out of range [0,1]", f)
			}
			p.AuxiliaryBuilding.DieselGenerator.RunningFraction = float32(f)
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
	m["AuxiliaryBuilding/DieselGenerator/Generator/BreakerClosed"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/DieselGenerator/Generator",
		Label: "BreakerClosed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.DieselGenerator.Generator.BreakerClosed = v
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

	// Service Water pump run command (chapter 12):
	// "The Service Water System supplies all the cooling water
	// for the plant, including that required for emergency
	// shutdown. Consequently, high reliability and redundancy
	// is required."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
	// "The three essential service water pumps for each unit
	// are typically rated at approximately 20,000 gpm
	// (4500 m3/hr)."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	// PumpRunning is the lumped "at least one essential SW
	// pump running" flag. With this set true, the simplified
	// model drives Flow = ServiceWaterDesignFlow and cools the
	// diesel jacket water (see Plant.Step -> AuxiliaryBuilding.Step).
	m["Outside/ServiceWaterSystem/PumpRunning"] = &bridge.Control{
		Part:  "Outside/ServiceWaterSystem",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.Outside.ServiceWaterSystem.PumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.Outside.ServiceWaterSystem.PumpRunning = v
			return nil
		},
	}

	// Service Water nonessential-header isolation (chapter 12):
	// "The nonessential equipment coolers are fed from
	// separate branch headers that can be isolated from the
	// main headers under accident conditions to conserve
	// water."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:23-26
	// Operator flips the isolation valves during accident
	// conditions; the simplified model records the boolean
	// directly.
	m["Outside/ServiceWaterSystem/NonessentialIsolated"] = &bridge.Control{
		Part:  "Outside/ServiceWaterSystem",
		Label: "NonessentialIsolated",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.Outside.ServiceWaterSystem.NonessentialIsolated)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.Outside.ServiceWaterSystem.NonessentialIsolated = v
			return nil
		},
	}

	// Alarm reset (chapter 8 annunciator):
	// "Turned off by operator acknowledgement."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	// The control body is an AlarmID string; the operator
	// (or HMI auto-loop) pushes one ID at a time. There is
	// no read-back value — Get returns empty.
	// AcknowledgeAll is a convenience control that acknowledges
	// every active alarm at once. The control body is unused;
	// Set ignores the value and calls AcknowledgeAll() directly.
	// "Turned off by operator acknowledgement."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	m["Alarms/AcknowledgeAll"] = &bridge.Control{
		Part:  "Plant",
		Label: "AcknowledgeAll",
		Kind:  "enum",
		Enum:  []string{"Acknowledge"},
		Get:   func() string { return "0" },
		Set: func(_ string) error {
			p.ReactorBuilding.PlantControlRoom.AcknowledgeAll()
			return nil
		},
	}

	m["Alarms/Reset"] = &bridge.Control{
		Part:  "Plant",
		Label: "ResetAlarm",
		Kind:  "string",
		Get:   func() string { return "" },
		Set: func(s string) error {
			p.ReactorBuilding.PlantControlRoom.ResetAlarm(AlarmID(s))
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
	m["AuxiliaryBuilding/DieselGenerator/CoolantTemp"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGenerator",
		Label: "CoolantTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.AuxiliaryBuilding.DieselGenerator.CoolantTemp.Float())
		},
	}

	// Component cooling water system outlet temperature (chapter 5):
	// "The component cooling water inlet temperature to various
	// components during normal cooldown is permitted to increase
	// to 120°F (49°C)"
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:108-110
	m["AuxiliaryBuilding/ComponentCoolingWaterSystem/OutletTemp"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ComponentCoolingWaterSystem",
		Label: "OutletTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.AuxiliaryBuilding.ComponentCoolingWaterSystem.OutletTemp.Float())
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

	// Steam generator heat extraction rate (primary-side heat
	// input from the RPV). The SG is the dominant primary
	// energy sink; reducing this is the fastest way to stop a
	// temperature drop once it starts.
	//   "3,813,000 Ib/hr (480 kg/sec)" steam flow per steam
	//   generator — Table 3.4-1.
	//   specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:56
	m["ReactorBuilding/SteamGenerator/HeatInput"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator",
		Label: "HeatInput",
		Kind:  "watt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("watt",
				p.ReactorBuilding.SteamGenerator.HeatInput)
		},
	}

	// RCP shaft speed. The shaft-loss heat enters the primary
	// coolant; reduced flow raises ΔT, providing a secondary
	// primary-temperature lever via the coolant pump.
	//   "Primary temperature is maintained through the
	//   balanced use of reactor coolant pump heat input and a
	//   steam dump system."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-111
	m["ReactorBuilding/ReactorCoolantPump/Speed"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorCoolantPump",
		Label: "Speed",
		Kind:  "rpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("rpm",
				float64(p.ReactorBuilding.ReactorCoolantPump.Velocity))
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

	// Main steam isolation valve (chapter 15 steam system):
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam line
	// upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	// The bridge sensor exposes the actual Open fraction for
	// visibility — the Step function uses it to gate the SG-to-
	// turbine steam path.
	m["ReactorBuilding/SteamGenerator/MainSteamIsolationValve/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator/MainSteamIsolationValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.SteamGenerator.MainSteamIsolationValve.Open)
		},
	}

	// Steam generator safety valve (chapter 15):
	// "Safety valves in each steam generator outlet main
	// steam line."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:42-45
	m["ReactorBuilding/SteamGenerator/SafetyValve/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator/SafetyValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.SteamGenerator.SafetyValve.Open)
		},
	}

	// Steam generator atmospheric dump valve (chapter 15):
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam line
	// upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	m["ReactorBuilding/SteamGenerator/AtmosphericDumpValve/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator/AtmosphericDumpValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open)
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

	// Control-rod actual position (chapter 2 RCC operation):
	// "Rod cluster control (RCC) assemblies are used for reactor
	// startup or shutdown, to follow load changes, and to
	// control small transient changes in reactivity."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:90-93
	// Position=1 means fully withdrawn (normal operation);
	// Position=0 means fully inserted.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:43
	// "rod position [...] provides operational reactivity control"
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/Position"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "Position",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.Position)
		},
	}

	// CVCS charging flow rate (chapter 4 CVCS):
	// "The charging pumps take the coolant from the volume
	// control tank [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
	// "system pressure is maintained by charging and letdown
	// flow control."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:96-97
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "ChargingFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingFlow.Float())
		},
	}

	// CVCS letdown flow rate (chapter 4 CVCS):
	// "The charging pumps take the coolant from the volume
	// control tank [...]" — the letdown line returns RCS
	// coolant back to the VCT through the letdown heat
	// exchanger.
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:79-80
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownFlow"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "LetdownFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownFlow.Float())
		},
	}

	// Reactor makeup control valve flow (chapter 4 CVCS):
	// "Because of the influx of makeup water from the reactor
	// makeup control system [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-6
	m["ReactorBuilding/ReactorMakeupControlValve/Flow"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorMakeupControlValve",
		Label: "Flow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.ReactorBuilding.ReactorMakeupControlValve.Flow.Float())
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

	// Pressurizer PORV position (chapter 3 pressure
	// limiting):
	// "Reductions in plant electrical load with resultant
	// pressure increases beyond the pressure-limiting
	// capability of the pressurizer spray system cause the
	// air-operated relief valves to open."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:23-30
	m["ReactorBuilding/PORV/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/PORV",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.PORV.Open)
		},
	}

	// Pressurizer ASME-code safety valve position
	// (chapter 3):
	// "If system pressure continues to rise, self-actuating
	// ASME-code safety valves will open."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:30-31
	m["ReactorBuilding/SafetyValve/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SafetyValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.SafetyValve.Open)
		},
	}

	// Reactor coolant loop block valve position
	// (chapter 3):
	// "Reactor coolant loop isolation valves may be closed
	// to isolate a loop."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_071.txt:145
	m["ReactorBuilding/BlockValve/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/BlockValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.BlockValve.Open)
		},
	}

	// Pressurizer relief tank level (chapter 3):
	// "Steam from the safety and/or relief valves is piped
	// to the pressurizer relief tank which contains
	// sufficient water to condense the steam."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:33-35
	m["ReactorBuilding/PressurizedReliefTank/Level"] = &bridge.Sensor{
		Part:  "ReactorBuilding/PressurizedReliefTank",
		Label: "Level",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.ReactorBuilding.PressurizedReliefTank.Level.Float())
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
	m["AuxiliaryBuilding/DieselGenerator/ReadyToLoad"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGenerator",
		Label: "ReadyToLoad",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.DieselGenerator.ReadyToLoad)
		},
	}

	// Diesel fuel oil tank level (chapter 11):
	// "Each diesel generator is provided with a fuel oil day tank
	// and a fuel storage tank. The onsite supply of fuel for each
	// diesel generator is sufficient to allow continuous rated load
	// operation for a minimum of seven days."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:118-122
	m["AuxiliaryBuilding/DieselGenerator/Fuel"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGenerator",
		Label: "Fuel",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.AuxiliaryBuilding.DieselGenerator.Fuel.Level.Float())
		},
	}

	// Containment spray header isolation valves "A" (chapter 6):
	// "This signal starts the spray pumps, opens the spray header
	// isolation valves "A""
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:59-60
	m["AuxiliaryBuilding/ContainmentSpraySystem/SprayHeaderOpen"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ContainmentSpraySystem",
		Label: "SprayHeaderOpen",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.ContainmentSpraySystem.SprayHeaderOpen)
		},
	}

	// Containment spray pump running.
	// "Containment spray is initiated automatically in response
	// to a containment high pressure signal. This signal starts
	// the spray pumps [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-60
	m["AuxiliaryBuilding/ContainmentSpraySystem/PumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ContainmentSpraySystem",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.ContainmentSpraySystem.Running)
		},
	}

	// CCWS pump running. One component cooling pump is in
	// service per train during normal operation; a standby pump
	// in each train auto-starts on pump failure.
	// "During normal full power operation, one component
	// cooling safeguards train and the component cooling
	// auxiliary subsystem (each operating with one pump and one
	// heat exchanger) are required to accommodate the heat
	// removal loads."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:71-77
	// "Failure of a pump will not affect the time required for
	// cooldown because a standby pump in each train of the
	// Safeguards subsystem is provided."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_106.txt:1-4
	m["AuxiliaryBuilding/ComponentCoolingWaterSystem/PumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ComponentCoolingWaterSystem",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning)
		},
	}

	// HHSI pump running. Auto-started on safety injection; also
	// manually started from the main control board for
	// RCS boration backup.
	// "The receipt of the safety injection signal initiates
	// the following emergency core cooling actions: [...] The
	// HHSI pumps are started."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:109-118
	// "Manual starting of the HHSI pumps will result in the
	// injection of borated water as RCS pressure is decreased
	// below the shutoff head of the pump."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:159-164
	m["ReactorBuilding/SafetyInjectionSystem/HHSIPumpRunning"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SafetyInjectionSystem",
		Label: "HHSIPumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning)
		},
	}

	// RHR train-A pump running. The RHR pumps are stopped
	// when plant startup begins and run on the miniflow
	// circuit during cooldown.
	// "One reactor coolant pump is kept running to provide for
	// uniform cooldown of all loops until the cooling
	// temperature is below 160°F."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_086.txt:37-40
	// "The RHR pumps are stopped when the plant startup
	// begins."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_085.txt:46-48
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA/PumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainA",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA.Running)
		},
	}
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB/PumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem/TrainB",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainB.Running)
		},
	}

	// EFWS motor-driven pump running. Auto-started on
	// safety-injection signal or low-low SG level; manually
	// stopped when main feedwater is restored.
	// "Signal / Pumps Started [...] Low-low level in 2/4
	// level channels in any one steam generator / Motor
	// driven pumps. [...] Safety injection / Motor driven
	// pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:69-81
	// "Normal feedwater flow to the steam generators by the
	// main feedwater system is resumed and the emergency
	// feedwater pumps are manually stopped."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:88-91
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA",
		Label: "MotorPumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPumpRunning)
		},
	}
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "MotorPumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPumpRunning)
		},
	}
	// EFWS turbine-driven pump running. Auto-started on low-low
	// level in two steam generators.
	// "Low-low level in 2/4 level channels in any two steam
	// generators / Turbine drive pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:82-85
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePumpRunning)
		},
	}
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePumpRunning)
		},
	}

	// SFPCS pump running. Two pumps normal, third standby.
	// "Normal operation requires two pumps with the third pump
	// serving as a standby."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PumpRunning)
		},
	}
	// SFPCS standby pump auto-started at low header pressure.
	// "the third pump automatically starts at a header
	// pressure of 50 psig (4.5 bar)."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:47-48
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/StandbyRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "StandbyRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.StandbyRunning)
		},
	}

	// Circulating water pump running. The operator modulates
	// the CW pump discharge valves and varies the number of
	// pumps in service from the main control room.
	// "Controls, interlocks, and indicators in the main
	// control room are provided to enable the operator to
	// modulate the circulating water pump discharge valves and
	// to vary the number of pumps in service."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:66-70
	m["Outside/CirculatingWaterSystem/PumpRunning"] = &bridge.Sensor{
		Part:  "Outside/CirculatingWaterSystem",
		Label: "PumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.Outside.CirculatingWaterPump.Running)
		},
	}
	// Alarms aggregate the current active alarm conditions
	// across all plant subsystems. Kind "alarm" signals the
	// TypeScript HMI to render structured alarm rows instead
	// of a scalar sensor readout.
	m["Plant/Alarms"] = &bridge.Sensor{
		Part:  "Plant",
		Label: "Alarms",
		Kind:  "alarm",
		Get: func() (string, error) {
			list := p.Alarms()
			type alarmJSON struct {
				ID           string  `json:"id"`
				Description  string  `json:"description"`
				Severity    string  `json:"severity"`
				Active      bool    `json:"active"`
				Acknowledged bool   `json:"acknowledged"`
				TimeS       float64 `json:"time_s"`
			}
			out := make([]alarmJSON, len(list))
			for i, a := range list {
				sev := "info"
				switch a.Severity {
				case SeverityWarning:
					sev = "warning"
				case SeverityTrip:
					sev = "trip"
				}
				out[i] = alarmJSON{
					ID:           string(a.ID),
					Description:  a.Description,
					Severity:    sev,
					Active:      a.Active,
					Acknowledged: a.Acknowledged,
					TimeS:       a.Timestamp.Seconds(),
				}
			}
			b, err := json.Marshal(out)
			if err != nil {
				return "", err
			}
			return string(b), nil
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
	m["AuxiliaryBuilding/DieselGenerator/Generator/Voltage"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGenerator/Generator",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.AuxiliaryBuilding.DieselGenerator.Generator.Voltage)
		},
	}

	// Diesel generator output frequency (chapter 11):
	// "60 Hz output which feeds its corresponding"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_168.txt:15
	m["AuxiliaryBuilding/DieselGenerator/Generator/Frequency"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGenerator/Generator",
		Label: "Frequency",
		Kind:  "hz",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("hz",
				p.AuxiliaryBuilding.DieselGenerator.Generator.Frequency)
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
	m["AuxiliaryBuilding/DieselGenerator/Generator/RealPower"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGenerator/Generator",
		Label: "RealPower",
		Kind:  "watt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("watt",
				p.AuxiliaryBuilding.DieselGenerator.Generator.RealPower)
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

	// Service Water System flow (chapter 12):
	// "The three essential service water pumps for each unit
	// are typically rated at approximately 20,000 gpm
	// (4500 m3/hr)."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:46-48
	// "The discharge of the service water pumps passes
	// through two strainers typically with 1/8-inch (0.3 cm)
	// openings to a common header."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:51-54
	// The simplified model collapses the three pumps into a
	// single lumped Flow value (gpm); we use the bridge
	// "gallon" kind as the closest analog (no "gpm" kind
	// exists in bridge/convert.go).
	m["Outside/ServiceWaterSystem/Flow"] = &bridge.Sensor{
		Part:  "Outside/ServiceWaterSystem",
		Label: "Flow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.Outside.ServiceWaterSystem.Flow)
		},
	}

	// Service Water inlet temperature (chapter 12):
	// "The Service Water System supplies all the cooling water
	// for the plant, including that required for emergency
	// shutdown. Consequently, high reliability and redundancy
	// is required."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-19
	// InletTemp is the river/lake/ocean intake temperature;
	// held constant at ServiceWaterSourceTemp (70°F) by the
	// simplified model.
	m["Outside/ServiceWaterSystem/InletTemp"] = &bridge.Sensor{
		Part:  "Outside/ServiceWaterSystem",
		Label: "InletTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.Outside.ServiceWaterSystem.InletTemp.Float())
		},
	}

	// Service Water outlet temperature (chapter 12):
	// The outlet temperature rises above the inlet as the
	// water picks up heat from the essential equipment
	// coolers it serves.
	// "The Service Water System supplies all the cooling water
	// for the plant, including that required for emergency
	// shutdown."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:16-18
	// "Diesel generator coolers" are typical SWS loads whose
	// heat load drives the outlet rise.
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_169.txt:88-89
	m["Outside/ServiceWaterSystem/OutletTemp"] = &bridge.Sensor{
		Part:  "Outside/ServiceWaterSystem",
		Label: "OutletTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.Outside.ServiceWaterSystem.OutletTemp.Float())
		},
	}

	// Circulating Water System flow (chapter 12):
	// "Circulating water for each main condenser is supplied
	// by vertical, mixed flow, volute type circulating water
	// pumps."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_170.txt:51-53
	// The spec does not quote a numeric gpm for the CWS pumps;
	// the simplified model uses an order-of-magnitude number
	// (550,000 gpm per pump) sized to deliver ~1.1 million
	// gpm for two pumps, consistent with a 4-loop Westinghouse
	// plant. See core/parts.go:1293-1303 for the sizing
	// rationale.
	m["Outside/CirculatingWaterSystem/Flow"] = &bridge.Sensor{
		Part:  "Outside/CirculatingWaterSystem",
		Label: "Flow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.Outside.CirculatingWaterSystem.FlowGPM)
		},
	}

	// Spent Fuel Pool purification flow (chapter 12):
	// "passing approximately 100 gpm (23 m3/hr) of each
	// loop's flow through a filter and demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:68-71
	// PurificationFlow is the per-loop cleanup flow through
	// the filter and demineralizer; the simplified model
	// collapses it into a single value (100 gpm).
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PurificationFlow"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "PurificationFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PurificationFlow)
		},
	}

	// Active alarms (chapter 8 annunciator):
	// "Turned off by operator acknowledgement."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	// The sensor returns the alarm list as a JSON-encoded
	// string; the bridge Kind is "string" because the bridge
	// format layer (bridge/convert.go) does not know how to
	// serialise a slice of structs directly. The HMI side is
	// responsible for parsing the JSON back into objects.
	m["Alarms/Active"] = &bridge.Sensor{
		Part:  "Plant",
		Label: "ActiveAlarms",
		Kind:  "string",
		Get: func() (string, error) {
			list := p.Alarms()
			if list == nil {
				list = []Alarm{}
			}
			b, err := json.Marshal(list)
			if err != nil {
				return "", err
			}
			return string(b), nil
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
