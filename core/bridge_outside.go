package core

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/chzchzchz/nsim/bridge"
)

// addOutsideControls adds Control entries keyed by the namespace managed by this file to m.
func addOutsideControls(p *Plant, m bridge.ControlMap) {

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

	// Offsite source output breakers (chapter 16): each safeguards
	// train normally feeds from its own offsite source, and opening
	// both strips the grid supply so the diesels auto-start.
	//   "If conditions occur which result in a loss of normal
	//    electrical power to a safeguards electrical bus, backup
	//    emergency diesel generators are automatically started"
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:103-106
	for _, src := range []struct {
		key  string
		part string
		ref  **PowerGenerator
	}{
		{"Electrical/OffsiteSourceA/BreakerClosed",
			"Electrical/OffsiteSourceA", &p.Electrical.OffsiteA},
		{"Electrical/OffsiteSourceB/BreakerClosed",
			"Electrical/OffsiteSourceB", &p.Electrical.OffsiteB},
	} {
		src := src
		m[src.key] = &bridge.Control{
			Part:  src.part,
			Label: "BreakerClosed",
			Kind:  "bool",
			Get: func() string {
				return bridge.BoolToStr((*src.ref).BreakerClosed)
			},
			Set: func(s string) error {
				v, err := bridge.StrToBool(s)
				if err != nil {
					return err
				}
				(*src.ref).BreakerClosed = v
				return nil
			},
		}
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

	// Cooling tower makeup valve (chapter 14 tech manual):
	// "This inventory loss, combined with losses due to
	// evaporation and drift, requires two cooling tower makeup
	// pumps [...]"
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:12-17
	m["Outside/CoolingTower/MakeupValve/Open"] = &bridge.Control{
		Part:  "Outside/CoolingTower",
		Label: "MakeupValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.Outside.CoolingTower.MakeupValve.Open)
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
			p.Outside.CoolingTower.MakeupValve.Open = v
			return nil
		},
	}

	// Cooling tower continuous blowdown valve.
	// "Due to system impurities, a continuous blowdown is
	// performed on the circulating water system."
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:12-17
	m["Outside/CoolingTower/BlowdownValve/Open"] = &bridge.Control{
		Part:  "Outside/CoolingTower",
		Label: "BlowdownValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v", p.Outside.CoolingTower.BlowdownValve.Open)
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
			p.Outside.CoolingTower.BlowdownValve.Open = v
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

	// Auxiliary feedwater pumps lift CST inventory into the steam
	// generators when the main feedwater system cannot hold level.
	//   - Diverse drives: "one steam-driven and one diesel-engine-driven
	//     auxiliary feedwater pump (each of which is safety related)"
	//     specs/westinghouse_tech_manual/chapter_5/page_0527.txt:30-36
	//   - Manual operation: "Auxiliary feedwater flow is manually
	//     increased to maintain a steady value of steam generator
	//     water level."
	//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:8-10
	for _, afw := range []struct {
		id   string
		pump *AFWPump
	}{
		{"Outside/DieselAFWPump", &p.Outside.DieselAFWPump},
		{"Outside/MotorAFWPump", &p.Outside.MotorAFWPump},
		{"Outside/TurbineAFWPump", &p.Outside.TurbineAFWPump},
	} {
		pump := afw.pump
		m[afw.id+"/Running"] = &bridge.Control{
			Part:  afw.id,
			Label: "Running",
			Kind:  "bool",
			Get: func() string {
				return bridge.BoolToStr(pump.Running)
			},
			Set: func(s string) error {
				v, err := bridge.StrToBool(s)
				if err != nil {
					return err
				}
				pump.Running = v
				return nil
			},
		}
		// AFW edge valves (anchors plan): CST suction and service-water
		// backup valves per AFW pump; state on parts.go:1585-1586.
		m[afw.id+"/CSTValve/Open"] = &bridge.Control{
			Part:  afw.id + "/CSTValve",
			Label: "Open",
			Kind:  "normalized",
			Get: func() string {
				return fmt.Sprintf("%v", pump.CSTValve.Open)
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
				pump.CSTValve.Open = v
				return nil
			},
		}
		m[afw.id+"/ServiceWaterValve/Open"] = &bridge.Control{
			Part:  afw.id + "/ServiceWaterValve",
			Label: "Open",
			Kind:  "normalized",
			Get: func() string {
				return fmt.Sprintf("%v", pump.ServiceWaterValve.Open)
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
				pump.ServiceWaterValve.Open = v
				return nil
			},
		}
	}

}

// addOutsideSensors adds Sensor entries keyed by the namespace managed by this file to m.
func addOutsideSensors(p *Plant, m bridge.SensorMap) {

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

	// CW pump upstream-power readiness. The CW pump motors ride on
	// the 6600 V plant distribution bus fed by UAT or SAT (see
	// plans/x/uat_sat_gating.md).
	m["Outside/CirculatingWaterPump/DriveReady"] = &bridge.Sensor{
		Part:  "Outside/CirculatingWaterPump",
		Label: "DriveReady",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.Outside.CirculatingWaterPump.DriveReady)
		},
	}

	// Auxiliary feedwater pumps only deliver against their latched
	// drive: diesel-fed bus for the diesel pump, ESF bus A for the
	// motor pump, and an SG steam header for the turbine pump (see
	// addOutsideControls for the spec citations).
	for _, afw := range []struct {
		id   string
		pump *AFWPump
	}{
		{"Outside/DieselAFWPump", &p.Outside.DieselAFWPump},
		{"Outside/MotorAFWPump", &p.Outside.MotorAFWPump},
		{"Outside/TurbineAFWPump", &p.Outside.TurbineAFWPump},
	} {
		pump := afw.pump
		m[afw.id+"/DriveReady"] = &bridge.Sensor{
			Part:  afw.id,
			Label: "DriveReady",
			Kind:  "bool",
			Get: func() (string, error) {
				return bridge.FormatSensorValue("bool", pump.DriveReady)
			},
		}
	}

	// Cooling-tower basin level. The CW pumps draw suction from
	// this basin when the tower is the configured CW source;
	// below CoolingTowerMinSuctionGal they lose suction.
	// "The cooled water falls into the cooling tower basin [...]
	// The cooling tower basin has a capacity of 5.2 x 106 gallons."
	// specs/westinghouse_tech_manual/chapter_14/page_1044.txt:37-39
	m["Outside/CoolingTower/Basin/Level"] = &bridge.Sensor{
		Part:  "Outside/CoolingTower",
		Label: "BasinLevel",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.Outside.CoolingTower.Basin.Level.Float())
		},
	}

	// Cold-basin temperature: the temperature of the water the
	// CW pumps return to the condenser in tower-source mode.
	m["Outside/CoolingTower/ColdBasinTemp"] = &bridge.Sensor{
		Part:  "Outside/CoolingTower",
		Label: "ColdBasinTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.Outside.CoolingTower.ColdBasinTemp.Float())
		},
	}

	// Evaporation loss into the tower (drift folded in).
	m["Outside/CoolingTower/EvaporationFlow"] = &bridge.Sensor{
		Part:  "Outside/CoolingTower",
		Label: "EvaporationFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			// Stored gal/s; presented gpm (x60 at the presentation boundary).
			return bridge.FormatSensorValue("gpm",
				p.Outside.CoolingTower.EvaporationFlow.Float()*60)
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
				Severity     string  `json:"severity"`
				Active       bool    `json:"active"`
				Acknowledged bool    `json:"acknowledged"`
				TimeS        float64 `json:"time_s"`
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
					Severity:     sev,
					Active:       a.Active,
					Acknowledged: a.Acknowledged,
					TimeS:        a.Timestamp.Seconds(),
				}
			}
			b, err := json.Marshal(out)
			if err != nil {
				return "", err
			}
			return string(b), nil
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
	// single lumped Flow value already stored in gpm
	// (ServiceWaterPumpFlow is a per-pump gpm figure), so this
	// sensor needs no unit conversion.
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
	// tracks Outside.RiverTemp each step (shared intake house
	// with the circulating water pumps).
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

	// Backing field FlowGPM is stored in true gpm; no conversion.
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

	// Environmental contamination sensors (plans/p74_1.txt
	// Wiring): instantaneous monitor readings and cumulative
	// released activity. Physics output, not operator levers.
	// The on-line gas monitor at the plant vent
	// (chapter_10/page_162.txt:65-67) and the liquid monitor
	// (chapter_10/page_162.txt:76-82) feed the operator alert
	// (page_0003.txt:8).
	m["Outside/PlantVentActivity"] = &bridge.Sensor{
		Part:  "Outside",
		Label: "PlantVentActivity",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.Outside.PlantVentActivityBq)
		},
	}
	m["Outside/LiquidMonitor"] = &bridge.Sensor{
		Part:  "Outside",
		Label: "LiquidMonitor",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.Outside.LiquidMonitorBq)
		},
	}
	m["Outside/GasReleasedCumulative"] = &bridge.Sensor{
		Part:  "Outside",
		Label: "GasReleasedCumulative",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.Outside.GasReleasedBq)
		},
	}
	m["Outside/LiquidReleasedCumulative"] = &bridge.Sensor{
		Part:  "Outside",
		Label: "LiquidReleasedCumulative",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.Outside.LiquidReleasedBq)
		},
	}
	m["Outside/GasHoldup"] = &bridge.Sensor{
		Part:  "Outside",
		Label: "GasHoldup",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.Outside.GasHoldupActivity)
		},
	}
	m["Outside/LiquidHoldup"] = &bridge.Sensor{
		Part:  "Outside",
		Label: "LiquidHoldup",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.Outside.LiquidHoldupActivity)
		},
	}

}
