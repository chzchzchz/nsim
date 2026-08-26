package core

import (
	"fmt"
	"strconv"

	"github.com/chzchzchz/nsim/bridge"
)

// addReactorBuildingControls adds Control entries keyed by the namespace managed by this file to m.
func addReactorBuildingControls(p *Plant, m bridge.ControlMap) {

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
				p.ReactorBuilding.ReactorCoolantPumps.AnyRunning())
		},
		// Aggregate master switch: the write fans out to all four
		// loop pumps; individual machines are driven through the
		// ReactorCoolantPumps/A..D controls (sequential start,
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:35-37).
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(v)
			return nil
		},
	}
	for _, loop := range []string{"A", "B", "C", "D"} {
		m["ReactorBuilding/ReactorCoolantPumps/"+loop+"/Running"] = &bridge.Control{
			Part:  "ReactorBuilding/ReactorCoolantPumps",
			Label: loop + "/Running",
			Kind:  "bool",
			Get: func(loop string) func() string {
				return func() string {
					return bridge.BoolToStr(
						p.ReactorBuilding.ReactorCoolantPumps.ByName(loop).Running)
				}
			}(loop),
			Set: func(loop string) func(string) error {
				return func(s string) error {
					v, err := bridge.StrToBool(s)
					if err != nil {
						return err
					}
					p.ReactorBuilding.ReactorCoolantPumps.ByName(loop).Running = v
					return nil
				}
			}(loop),
		}
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


	// Accumulator isolation valves (chapter 16 planned-cooldown
	// lineup). Normally open so the tanks stay discharge-ready;
	// the operator shuts them before RCS pressure falls to the
	// discharge precharge during a coordinated cooldown.
	// "Before reaching the pressure when the safety injection
	// accumulators would discharge, the accumulator isolation
	// valves are closed."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:111-113
	m["ReactorBuilding/SafetyInjectionSystem/AccumulatorIsolation/Shut"] = &bridge.Control{
		Part:  "ReactorBuilding/SafetyInjectionSystem/AccumulatorIsolation",
		Label: "Shut",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.SafetyInjectionSystem.Accumulator.IsolationShut)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.SafetyInjectionSystem.Accumulator.IsolationShut = v
			return nil
		},
	}


	// Simultaneous cold- and hot-leg injection realignment. The
	// operator performs this control-board switchover about 24
	// hours into cold-leg recirculation for long-term core
	// cooling and boron-buildup prevention.
	// "Approximately 24 hours after the switchover to cold-leg
	//  recirculation, simultaneous flow to the RCS cold legs and
	//  hot legs is established [...]. Switchover to this phase is
	//  performed manually from the control board."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:122-138
	m["ReactorBuilding/SafetyInjectionSystem/HotLegInjection"] = &bridge.Control{
		Part:  "ReactorBuilding/SafetyInjectionSystem",
		Label: "HotLegInjection",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.SafetyInjectionSystem.HotLegInjection)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.SafetyInjectionSystem.HotLegInjection = v
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


	// Steam generator main steam isolation valve (chapter 15):
	// The MSIV isolates the SG secondary side from the
	// turbine and steam dump on a steam-line-break signal
	// or operator command.
	// "ASME Code safety valves and power operated relief and
	// atmospheric dump valves are located on each steam
	// line upstream of the stop valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_215.txt:8-10
	// Steam generator wet-layup fill valve (chapter 16 shutdown):
	// tops the shell steam side to the vents once the reactor
	// coolant reaches layup temperature and the header is at vent
	// pressure.
	// "When the reactor coolant reaches 200°F (93°C), the steam
	// sides of the steam generators are filled to the vents for wet
	// layup."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:144-145
	m["ReactorBuilding/SteamGenerator/WetLayupFillValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/WetLayupFillValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.WetLayupFillValve.Open)
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
			p.ReactorBuilding.SteamGenerator.WetLayupFillValve.Open = v
			return nil
		},
	}

	// Steamline vent (chapter 16 startup). The operator holds the
	// vents open during low-temperature heatup and closes them once
	// the reactor coolant reaches 210-220°F.
	// "When the reactor coolant temperature increases to about
	//  210-220°F (90-105°C), the steamline vents are closed [...]."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
	m["ReactorBuilding/SteamGenerator/SteamlineVent/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/SteamlineVent",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.SteamlineVent.Open)
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
			p.ReactorBuilding.SteamGenerator.SteamlineVent.Open = v
			return nil
		},
	}

	// SG shell drain valve (chapter 16 startup): pulls SG secondary
	// inventory down to the normal operating range at the same point
	// of the sequence.
	// "...and the secondary side of each steam generator is drained
	//  to the normal operating range."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:118-122
	m["ReactorBuilding/SteamGenerator/DrainValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/DrainValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.DrainValve.Open)
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
			p.ReactorBuilding.SteamGenerator.DrainValve.Open = v
			return nil
		},
	}


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


	// Main steam line break malfunction (pwr_sim exercise 4.19).
	// Open is the normalized break size upstream of the MSIV; 1
	// inserts the full "100 % main steam header break".
	//   "insert the malfunction '100 % main steam header break'"
	//   specs/pwr_sim/chapter_4/page_053.txt:14-16
	m["ReactorBuilding/SteamGenerator/MainSteamLineBreak/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/SteamGenerator/MainSteamLineBreak",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.SteamGenerator.MainSteamLineBreak.Open)
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
			p.ReactorBuilding.SteamGenerator.MainSteamLineBreak.Open = v
			return nil
		},
	}

	// Hot-leg LOCA malfunction (pwr_sim exercise 4.20). Open is
	// the normalized break size at the RPV outlet nozzle; 1
	// inserts the full "RC hot leg #1 LOCA break".
	//   "select the malfunction 'RC hot leg #1 LOCA break'"
	//   specs/pwr_sim/chapter_4/page_053.txt:55-56
	m["ReactorBuilding/ReactorPressureVessel/HotLegBreak/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/HotLegBreak",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.ReactorPressureVessel.HotLegBreak.Open)
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
			p.ReactorBuilding.ReactorPressureVessel.HotLegBreak.Open = v
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

	// Automatic makeup engage switch: a VCT low-level signal
	// initiates full makeup until the tank is restored to its
	// design band.
	// "Low-level signals from the volume control tank initiate
	// reactor makeup control or flow from the refueling water
	// storage tank as a backup."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
	m["ReactorBuilding/ReactorMakeupControlValve/Auto"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorMakeupControlValve",
		Label: "Auto",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorMakeupControlValve.Auto)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorMakeupControlValve.Auto = v
			return nil
		},
	}

	// Loss-of-offsite-power declaration (chapter 16 emergency
	// response): "If conditions occur which result in a loss of
	// normal electrical power to a safeguards electrical bus,
	// backup emergency diesel generators are automatically started
	// and necessary equipment will be automatically sequenced on
	// the buses to safely cool down the primary plant if required."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-109
	// The flag is the model's name for the state of the offsite
	// supply: "The system auxiliary power is supplied from
	// independent and electrically isolated buses of the off-site
	// main distribution grid."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:45-47
	m["ReactorBuilding/PlantElectricalSystems/OffsitePowerAvailable"] = &bridge.Control{
		Part:  "ReactorBuilding/PlantElectricalSystems",
		Label: "OffsitePowerAvailable",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.PlantElectricalSystems.OffsitePowerAvailable)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.PlantElectricalSystems.OffsitePowerAvailable = v
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


	// Automatic pressurizer pressure regulation (chapter 16 trip
	// response): "Pressurizer pressure will recover as pressurizer
	// heaters automatically energize." While armed, the heaters cycle
	// inside PressurizerAutoHeaterBand of the heater target and the
	// HeatersOn latch becomes a manual override.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:95-98
	m["ReactorBuilding/Pressurizer/HeaterAutoControl"] = &bridge.Control{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "HeaterAutoControl",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.Pressurizer.HeaterAutoControl)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.Pressurizer.HeaterAutoControl = v
			return nil
		},
	}


	// Operator-selected pressurizer pressure for the heater
	// regulation to hold: staged writes walk heaters and spray along
	// the temperature-pressure program instead of leaving only the
	// model floor and the no-load target reachable.
	// "operation of pressurizer heaters and spray maintains the
	// system pressure in accordance with the applicable
	// temperature-pressure curves."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:142-145
	m["ReactorBuilding/Pressurizer/PressureSetpoint"] = &bridge.Control{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "PressureSetpoint",
		Kind:  "psia",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.Pressurizer.PressureSetpoint)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("psia", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.ReactorBuilding.Pressurizer.PressureSetpoint = Pressure(v)
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

	// Stuck-open PORV failure latch (malfunction injection, secondary
	// training source): while latched the relief valve ignores its
	// auto-reclose and operator close writes; only the block valve
	// terminates the inventory loss.
	// specs/pwr_sim/chapter_4/page_049.txt:26-31
	m["ReactorBuilding/Pressurizer/PORVStuckOpen"] = &bridge.Control{
		Part:  "ReactorBuilding/Pressurizer",
		Label: "PORVStuckOpen",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.Pressurizer.PORVStuckOpen)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.Pressurizer.PORVStuckOpen = v
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
	// function auto-trips the safety valve when pressure exceeds
	// PressurizerSafetySetpoint.
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

	// PRT cold-water spray (chapter 3): "Cold water can be
	// sprayed into the pressurizer relief tank to increase the
	// heat sink capacity."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:35-37
	m["ReactorBuilding/PRTSprayValve/Open"] = &bridge.Control{
		Part:  "ReactorBuilding/PRTSprayValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.ReactorBuilding.PRTSprayValve.Open)
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
			p.ReactorBuilding.PRTSprayValve.Open = v
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
	// The write latches the RPS manual-trip input so the trip
	// declares through normal voting; RPV.Trip is owned by the
	// RPS (Plant.Step propagates ReactorTripped into it each
	// step) and is kept here only for direct-state compatibility.
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
			// Plant.Step overwrites RPV.Trip from the RPS output
			// every tick, so an operator/command-file trip must
			// enter through the RPS manual-trip input or it latches
			// nothing: "The RPS provides for manual initiation of
			// reactor trip by operator action in the control room."
			// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:8-9
			if v {
				p.ReactorBuilding.ReactorProtectionSystem.ManualTrip = true
			}
			return nil
		},
	}


	// Post-trip reset of the RPS trip latch and the ESFAS output
	// latches. The step function re-declares the trip while trip
	// channels still satisfy 2/4 voting, so reset cannot defeat an
	// active trip condition:
	//   "The trip breakers are arranged to provide a 2/4 configuration."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:132-133
	//   Return to service proceeds from hot standby with the rods
	//   withdrawn back to criticality.
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:24-26
	m["ReactorBuilding/ReactorProtectionSystem/Reset"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorProtectionSystem",
		Label: "Reset",
		Kind:  "enum",
		Enum:  []string{"Reset"},
		Get:   func() string { return "0" },
		Set: func(_ string) error {
			p.ReactorBuilding.ReactorProtectionSystem.Reset()
			return nil
		},
	}

	// Containment ventilation circulation, routing the atmosphere
	// to eliminate high-hydrogen pockets after an accident: "A
	// containment ventilation system that circulates the containment
	// atmosphere to remove pockets of high hydrogen concentration
	// after an accident."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:15-17
	m["ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Ventilation"] = &bridge.Control{
		Part:  "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression",
		Label: "Ventilation",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ContainmentBuilding.
					HydrogenDetectionSuppression.VentilationActive)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ContainmentBuilding.
				HydrogenDetectionSuppression.VentilationActive = v
			return nil
		},
	}

	// High-hydrogen alarm acknowledgement; the annunciator clears on
	// the operator's action once suppression has restored margin.
	// "Turned off by operator acknowledgement."
	// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
	m["ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/AlarmReset"] = &bridge.Control{
		Part:  "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression",
		Label: "AlarmReset",
		Kind:  "enum",
		Enum:  []string{"Reset"},
		Get:   func() string { return "0" },
		Set: func(_ string) error {
			p.ReactorBuilding.ContainmentBuilding.
				HydrogenDetectionSuppression.HydrogenAlarm = false
			return nil
		},
	}


	// Operator block on the SI initiation signals for a planned
	// cooldown, armed before process variables reach the automatic
	// actuation setpoints.
	// "Prior to reaching the automatic safety injection setpoints
	// during cooldown and depressurization, the safety injection
	// initiation signals are blocked."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:108-110
	m["ReactorBuilding/ReactorProtectionSystem/SafetyInjectionBlock"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorProtectionSystem",
		Label: "SafetyInjectionBlock",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionBlocked)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorProtectionSystem.SafetyInjectionBlocked = v
			return nil
		},
	}

	// Operator block on the steamline-isolation low-steamline
	// trigger, armed with the SI block for planned cooldowns: an
	// intentional atmospheric-dump cooldown legitimately carries
	// the header through the shared setpoint. Containment-pressure
	// and manual triggers stay live while blocked.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:107-110
	m["ReactorBuilding/ReactorProtectionSystem/SteamlineIsolationBlock"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorProtectionSystem",
		Label: "SteamlineIsolationBlock",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationBlocked)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorProtectionSystem.SteamlineIsolationBlocked = v
			return nil
		},
	}

	// Channel-under-test selector: isolates one monitored input
	// for calibration/testing so an injected stimulus on that
	// channel cannot actuate protections while the crew verifies
	// the instrumentation.
	// "The protection system normally uses 2/4 logic but reverts
	// to a safe 2/3 logic when part of the system is being tested
	// or is involved in maintenance."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
	m["ReactorBuilding/ReactorProtectionSystem/ChannelTest"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorProtectionSystem",
		Label: "ChannelTest",
		Kind:  "enum",
		Get: func() string {
			return p.ReactorBuilding.ReactorProtectionSystem.ChannelTest.name()
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("enum", s)
			if err != nil {
				return err
			}
			switch validated {
			case "Steamline":
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest = RPSSteamlineChannelInTest
			case "SGLevel":
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest = RPSSGLevelChannelInTest
			case "Tcold":
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest = RPSTcoldChannelInTest
			case "Containment":
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest = RPSContainmentChannelInTest
			case "Flow":
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest = RPSFlowChannelInTest
			case "None":
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest = RPSNoChannelInTest
			default:
				return fmt.Errorf("invalid channel test %q", validated)
			}
			return nil
		},
	}


	// Manual safety-injection pushbutton. Table 7-1 lists "Manual
	// safety injection" among the Safety Injection triggers,
	// separate from the manual reactor trip; it also carries the
	// manual entries of the Steamline Isolation and Phase A
	// containment-isolation lists, for which no dedicated
	// pushbuttons are modeled.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-21
	m["ReactorBuilding/ReactorProtectionSystem/ManualSI"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorProtectionSystem",
		Label: "ManualSI",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorProtectionSystem.ManualSI)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorProtectionSystem.ManualSI = v
			return nil
		},
	}


	// Control rod insertion (chapter 7):
	// "The control rods are then manually withdrawn to achieve criticality"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:155-156
	// The Set path writes the operator's demand to
	// DemandInsertion; ReactorBuilding.Step rate-limits the
	// actual transition from Insertion toward DemandInsertion
	// at RodInsertionSpeed (plans/x/fission_controls.md).
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "Insertion",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion)
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
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.DemandInsertion = v
			return nil
		},
	}

	// Dropped-bank failure latch (malfunction injection, secondary
	// training source): while latched one rod bank sits jammed fully
	// into the core at RodDroppedBankInsertion, and the dropped-rod
	// signal raises the automatic load runback.
	// specs/pwr_sim/chapter_4/page_049.txt:61-67
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_207.txt:26-29
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/DroppedBank"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "DroppedBank",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.DroppedBank)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.DroppedBank = v
			return nil
		},
	}


	// Automatic rod control engage switch. With it latched the
	// controller integrates the Tavg program error into
	// DemandInsertion each step; the manual demand path above is
	// then owned by the controller.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/AutoControl"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "AutoControl",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.AutoControl = v
			return nil
		},
	}


	// Rod insertion limit enforcement. The limit caps how far the
	// banks may sink while operating so a trip retains its required
	// shutdown margin; ControlRods.Step clamps DemandInsertion to it
	// while latched (the scram path supersedes).
	//   "Control rods must be pulled above a certain height, the rod
	//    insertion limit, so that in the event a trip occurs the full
	//    insertion of the rods will add at least a prescribed amount
	//    of negative reactivity."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:30-34
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/InsertionLimit"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "InsertionLimit",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.InsertionLimit)
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
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.InsertionLimit = v
			return nil
		},
	}

	m["ReactorBuilding/ReactorPressureVessel/ControlRods/EnforceInsertionLimit"] = &bridge.Control{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "EnforceInsertionLimit",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.EnforceInsertionLimit)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.ReactorBuilding.ReactorPressureVessel.ControlRods.EnforceInsertionLimit = v
			return nil
		},
	}


	// CVCS boron concentration setpoint. The operator writes the
	// desired RCS boron concentration in ppm; CVCS.Step latches
	// tracking toward it and ramps the concentration through the
	// BTRS boration/dilution flow paths until it arrives in the
	// tolerance band, then releases chemistry to volume control.
	// The read-back is the measured concentration so the operator
	// watches the ramp close on the target.
	//   "A boron dilution is started to bring the boron
	//   concentration to the estimated critical concentration."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:36-38
	//   "The operating mode (boration or dilution) and the
	//   magnitude of the boron concentration change required
	//   is determined by comparison of the measured Tc
	//   (cold leg temperature) with the programmed (function
	//   of power) Tc."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:58-62
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
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronTargetConcentration = v
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronSetpointTracking = true
			return nil
			},
		}

		// Dispatcher load reference for this unit in MW against
		// the 1000 MW machine rating. Plant.Step normalizes it into
		// Turbine.LoadDemandRef, which the electro-hydraulic control
		// ramps Setpoint toward while the unit is synchronized; the
		// operator panel writes the same reference directly:
		//   "...comparing turbine speed and first-stage pressure with
		//    reference values established by the dispatcher or
		//    initiated at the operator's panel."
		//    specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:36-41
		m["Electrical/Grid/LoadDemand"] = &bridge.Control{
			Part:  "Electrical/Grid",
			Label: "LoadDemand",
			Kind:  "mw",
			Get: func() string {
				return fmt.Sprintf("%v",
					p.ReactorBuilding.Grid.LoadDemand)
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

		// Refueling-cavity fill and drain commands. Flooding
		// begins once the vessel head is unseated for refueling:
		//   "Reactor cavity is filled with borated water to the
		//    vessel flange."
		//   specs/the_westinghouse_pressurized_water_react/
		//   chapter_13/page_185.txt:53-54
		// Draining accompanies head reseating:
		//   "When the head is about 1 foot above the vessel
		//    flange, the refueling cavity is completely drained
		//    and the flange surface is cleaned."
		//   specs/the_westinghouse_pressurized_water_react/
		//   chapter_13/page_186.txt:113-116
		m["ReactorBuilding/RefuelingOperations/CavityFlooding"] = &bridge.Control{
			Part:  "ReactorBuilding/RefuelingOperations",
			Label: "CavityFlooding",
			Kind:  "bool",
			Get: func() string {
				return bridge.BoolToStr(
					p.ReactorBuilding.RefuelingOperations.CavityFlooding)
			},
			Set: func(s string) error {
				v, err := bridge.StrToBool(s)
				if err != nil {
					return err
				}
				p.ReactorBuilding.RefuelingOperations.CavityFlooding = v
				if v {
					p.ReactorBuilding.RefuelingOperations.CavityDraining = false
				}
				return nil
			},
		}
		m["ReactorBuilding/RefuelingOperations/CavityDraining"] = &bridge.Control{
			Part:  "ReactorBuilding/RefuelingOperations",
			Label: "CavityDraining",
			Kind:  "bool",
			Get: func() string {
				return bridge.BoolToStr(
					p.ReactorBuilding.RefuelingOperations.CavityDraining)
			},
			Set: func(s string) error {
				v, err := bridge.StrToBool(s)
				if err != nil {
					return err
				}
				p.ReactorBuilding.RefuelingOperations.CavityDraining = v
				if v {
					p.ReactorBuilding.RefuelingOperations.CavityFlooding = false
				}
				return nil
			},
		}

	}

// addReactorBuildingSensors adds Sensor entries keyed by the namespace managed by this file to m.
func addReactorBuildingSensors(p *Plant, m bridge.SensorMap) {

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
		// Backing field stores psia; badge now matches the unit.
		Kind:  "psia",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psia",
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
		// Backing field stores psia; badge now matches the unit.
		Kind:  "psia",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psia",
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

	// Continuous hydrogen concentration readout: "A hydrogen
	// detection system that can detect and report in a continuous
	// manner the hydrogen content in the containment atmosphere."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:7-9
	m["ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression",
		Label: "HydrogenPercent",
		Kind:  "percent",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("percent",
				p.ReactorBuilding.ContainmentBuilding.
					HydrogenDetectionSuppression.HydrogenVolumePercent)
		},
	}

	// High-hydrogen alarm: "Alarms at proper setpoints are provided."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:10-11
	m["ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/Alarm"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression",
		Label: "Alarm",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.ContainmentBuilding.
					HydrogenDetectionSuppression.HydrogenAlarm)
		},
	}

	// Recombiner status: "Hydrogen recombiners that burn the free
	// hydrogen. The hydrogen/oxygen combination results in water."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:21-22
	m["ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/RecombinerRunning"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression",
		Label: "RecombinerRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.ContainmentBuilding.
					HydrogenDetectionSuppression.RecombinerActive)
		},
	}

	// Ventilation circulation status: "A containment ventilation
	// system that circulates the containment atmosphere to remove
	// pockets of high hydrogen concentration after an accident."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_118.txt:15-17
	m["ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/VentilationRunning"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression",
		Label: "VentilationRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.ContainmentBuilding.
					HydrogenDetectionSuppression.VentilationActive)
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
				float64(p.ReactorBuilding.ReactorCoolantPumps.Velocity()))
		},
	}
	for _, loop := range []string{"A", "B", "C", "D"} {
		m["ReactorBuilding/ReactorCoolantPumps/"+loop+"/Speed"] = &bridge.Sensor{
			Part:  "ReactorBuilding/ReactorCoolantPumps",
			Label: loop + "/Speed",
			Kind:  "rpm",
			Get: func(loop string) func() (string, error) {
				return func() (string, error) {
					return bridge.FormatSensorValue("rpm",
						float64(p.ReactorBuilding.ReactorCoolantPumps.ByName(loop).Velocity))
				}
			}(loop),
		}
	}


	// Reactor pressure vessel pressure:
	// Operating pressure 2250 psia (155 bar); design pressure 2500 psia.
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_065.txt:45
	m["ReactorBuilding/ReactorPressureVessel/Pressure"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "Pressure",
		// Backing field stores psia; badge now matches the unit.
		Kind:  "psia",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psia",
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
	// Steam generator wet-layup fill valve position:
	// read-back of the vent-fill path used at the end of cooldown.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:144-145
	m["ReactorBuilding/SteamGenerator/WetLayupFillValve/Open"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SteamGenerator/WetLayupFillValve",
		Label: "Open",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.SteamGenerator.WetLayupFillValve.Open)
		},
	}


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


	// Control-rod actual insertion (chapter 2 RCC operation):
	// "Rod cluster control (RCC) assemblies are used for reactor
	// startup or shutdown, to follow load changes, and to
	// control small transient changes in reactivity."
	// specs/the_westinghouse_pressurized_water_react/chapter_2/page_035.txt:90-93
	// Insertion=0 means fully withdrawn (normal operation);
	// Insertion=1 means fully inserted.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_135.txt:43
	// "rod position [...] provides operational reactivity control"
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/Insertion"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "Insertion",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion)
		},
	}


	// Bank position indications decompose the lumped rod path into
	// the shutdown banks and the four control banks under the
	// standard overlap program, so bank sequencing and overlap are
	// observable during rod withdrawal.
	//   "The shutdown banks of the reactor control rods are
	//   withdrawn. [...] The control rod banks are then manually
	//   withdrawn to achieve criticality while observing proper
	//   bank overlap, rod position indications, and nuclear
	//   instrumentation."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:25-26,55-58
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/ShutdownBank"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "ShutdownBank",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.ShutdownBankInsertion())
		},
	}
	for _, bank := range []string{"A", "B", "C", "D"} {
		m["ReactorBuilding/ReactorPressureVessel/ControlRods/ControlBank"+bank] = &bridge.Sensor{
			Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
			Label: "ControlBank" + bank,
			Kind:  "normalized",
			Get: func(bank string) func() (string, error) {
				idx := map[string]int{"A": 0, "B": 1, "C": 2, "D": 3}[bank]
				return func() (string, error) {
					banks := p.ReactorBuilding.ReactorPressureVessel.
						ControlRods.ControlBankInsertions()
					return bridge.FormatSensorValue("normalized", banks[idx])
				}
			}(bank),
		}
	}

	// Source-range count rate. The source-range channels use
	// proportional counters whose pulse rate tracks neutron flux;
	// rising counts with no rod motion are the criticality evidence.
	//   "Evidence of criticality is indicated by a positive sustained
	//   startup rate with no rod motion and increasing source range
	//   counts."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:59-63
	m["ReactorBuilding/ReactorPressureVessel/SourceRangeCounts"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "SourceRangeCounts",
		Kind:  "cps",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("cps",
				p.ReactorBuilding.ReactorPressureVessel.SourceRangeCounts)
		},
	}

	// Startup-rate indication in decades per minute.
	//   "Startup rate indication is also provided in the control room."
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_121.txt:19-20
	m["ReactorBuilding/ReactorPressureVessel/StartupRate"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "StartupRate",
		Kind:  "dpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("dpm",
				p.ReactorBuilding.ReactorPressureVessel.StartupRateDPM)
		},
	}


	// Tavg rod-control program output (chapter 16):
	// "programmed function of turbine load"
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:24-27
	m["ReactorBuilding/ReactorPressureVessel/ControlRods/ProgrammedTavg"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel/ControlRods",
		Label: "ProgrammedTavg",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.ReactorBuilding.ReactorPressureVessel.ControlRods.ProgrammedTavg)
		},
	}


	// Axial flux difference (the axial offset observable): top-half
	// power minus bottom-half power over total core power. The
	// operator holds this inside a target band by adjusting boron
	// whenever rod motion approaches the band limits.
	//   "axial offset — A measure of power distribution in the axial
	//    direction [...]"
	//   specs/the_westinghouse_pressurized_water_react/glossary.md:22
	//   "the operator must insure that the indicated axial flux
	//    difference (AFD) is maintained within the limits"
	//   specs/westinghouse_plant_operations/chapter_19/page_012.txt:4-6
	// The offset is signed by definition: rod insertion drives it
	// negative of the unrodded anchor, so the reading needs the
	// signed percent kind.
	m["ReactorBuilding/ReactorPressureVessel/AxialOffset"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "AxialOffset",
		Kind:  "signedpercent",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("signedpercent",
				p.ReactorBuilding.ReactorPressureVessel.AxialOffset)
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
			// Stored gal/s; presented gpm (x60 at the presentation boundary).
			return bridge.FormatSensorValue("gpm",
				p.ReactorBuilding.ReactorMakeupControlValve.Flow.Float() * 60)
		},
	}

	// Makeup initiation status while the automatic mode is
	// engaged: true from the VCT low-level signal until the tank
	// is back inside its design band.
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4
	m["ReactorBuilding/ReactorMakeupControlValve/Initiated"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorMakeupControlValve",
		Label: "Initiated",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.ReactorMakeupControlValve.Initiated)
		},
	}

	// Channel-under-test status: which monitored input is
	// currently isolated for calibration/testing.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
	m["ReactorBuilding/ReactorProtectionSystem/ChannelUnderTest"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorProtectionSystem",
		Label: "ChannelUnderTest",
		Kind:  "enum",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("enum",
				p.ReactorBuilding.ReactorProtectionSystem.ChannelTest.name())
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

	// PRT rupture-disc state. A burst disc is a one-time device:
	// once latched it stays burst until maintenance replaces it,
	// so no runtime reset path exists by design.
	// "A rupture disc vents the tank to the containment if design
	// pressure is exceeded."
	// specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:37-38
	m["ReactorBuilding/PRTRuptureDiscBurst"] = &bridge.Sensor{
		Part:  "ReactorBuilding/PRTRuptureDiscBurst",
		Label: "RuptureDiscBurst",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.ReactorBuilding.PRTRuptureDiscBurst)
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


	// Hot-leg LOCA break discharge. The exercise records the
	// break flow as a rate into Table V.
	// "Record the break flow in Table V."
	// specs/pwr_sim/chapter_4/page_053.txt:64-65
	// Backing field is a per-step volume; presented as gpm by
	// dividing out the tick and scaling to minutes.
	m["ReactorBuilding/ReactorPressureVessel/BreakFlow"] = &bridge.Sensor{
		Part:  "ReactorBuilding/ReactorPressureVessel",
		Label: "BreakFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gpm",
				p.ReactorBuilding.ReactorPressureVessel.BreakFlow.Float() /
					TimeStep.Seconds()*60)
		},
	}

	// HHSI hot-leg injection split. Non-zero once the operator
	// realigns to simultaneous cold- and hot-leg injection.
	// "a substantial portion of the flow is directed to the RCS
	//  hot legs."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:132-133
	m["ReactorBuilding/SafetyInjectionSystem/HHSIHotLegFlow"] = &bridge.Sensor{
		Part:  "ReactorBuilding/SafetyInjectionSystem",
		Label: "HHSIHotLegFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			// Backing field is stored in gal/s; presented gpm.
			return bridge.FormatSensorValue("gpm",
				p.ReactorBuilding.SafetyInjectionSystem.HHSIHotLegFlow.Float() * 60)
		},
	}


	// Grid frequency (chapter 11 / turbine governor reference):
	// "Wide range speed control of the turbine from turning
	// gear to rated speed."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_201.txt:70-72
	m["Electrical/Grid/Frequency"] = &bridge.Sensor{
		Part:  "Electrical/Grid",
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
	m["Electrical/Grid/Voltage"] = &bridge.Sensor{
		Part:  "Electrical/Grid",
		Label: "Voltage",
		Kind:  "volt",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("volt",
				p.ReactorBuilding.Grid.Voltage)
		},
	}

}
