package core

import (
	"fmt"
	"strconv"

	"github.com/chzchzchz/nsim/bridge"
)

// addAuxiliaryBuildingControls adds Control entries keyed by the namespace managed by this file to m.
func addAuxiliaryBuildingControls(p *Plant, m bridge.ControlMap) {

	// Recirculation switchover: sump isolation valves "C" open,
	// spray-pump suction isolation valves "D" close so the pumps
	// draw from the containment sump; spray additive stops.
	// "The recirculation mode of the CSS will be initiated
	// manually by the operator."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:63-70
	m["AuxiliaryBuilding/ContainmentSpraySystem/SuctionSump"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ContainmentSpraySystem",
		Label: "SuctionSump",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ContainmentSpraySystem.SuctionSump)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ContainmentSpraySystem.SuctionSump = v
			return nil
		},
	}

	// SAT isolation valves "B": with them closed the spray
	// additive tank is isolated and the pumps deliver borated
	// water without sodium hydroxide.
	// "This signal starts the spray pumps, opens the spray header
	// isolation valves "A", and opens the SAT isolation valves
	// "B"."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
	m["AuxiliaryBuilding/ContainmentSpraySystem/SATIsolationValveBOpen"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ContainmentSpraySystem",
		Label: "SATIsolationValveBOpen",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ContainmentSpraySystem.SATIsolationValveBOpen)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ContainmentSpraySystem.SATIsolationValveBOpen = v
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


	// Solid-plant primary pressure control valve between the RHR
	// and CVC systems, with its selected-pressure setpoint. It
	// trims a water-solid primary back to the setpoint while the
	// RHRS provides the forced flow between the two systems.
	// "Solid plant primary pressure control utilizes a pressure
	// control valve between the RHR and CVC systems which is set
	// to maintain a selected pressure while charging makes up for
	// any coolant contraction and losses."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:117-122
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem",
		Label: "SolidPlantPressureValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantPressureValve.Open)
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
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantPressureValve.Open = v
			return nil
		},
	}

	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureSetpoint"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem",
		Label: "SolidPlantPressureSetpoint",
		Kind:  "psig",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantPressureSetpoint)
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("psig", s)
			if err != nil {
				return err
			}
			v, err := strconv.ParseFloat(validated, 64)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantPressureSetpoint = Pressure(v)
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
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Running = v
			return nil
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "MotorPumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPump.Running = v
			return nil
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.Running = v
			return nil
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.Running)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.Running = v
			return nil
		},
	}

	// EFWS discharge flow-modulating valves. After the assumed
	// 30-minute full-open window the flow is adjusted by
	// positioning these valves, and manual starts are operated
	// the same way; the automatic lineup overrides them only
	// while the actuation window is open.
	// "When operator action can be taken (after an assumed
	// 30-minute delay), the emergency feedwater flow is adjusted
	// by positioning the flow modulating valves to restore and
	// maintain the steam generator water levels within the narrow
	// control range."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_100.txt:29-34
	// "the pumps are manually started and the flowrates are
	// manually controlled by positioning the flow modulating
	// valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_102.txt:14-18
	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPump/FlowModulatingValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPump",
		Label: "FlowModulatingValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.FlowModulatingValve.Open)
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
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.FlowModulatingValve.Open = v
			return nil
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePump/FlowModulatingValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePump",
		Label: "FlowModulatingValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.FlowModulatingValve.Open)
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
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.FlowModulatingValve.Open = v
			return nil
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPump/FlowModulatingValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPump",
		Label: "FlowModulatingValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPump.FlowModulatingValve.Open)
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
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPump.FlowModulatingValve.Open = v
			return nil
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePump/FlowModulatingValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePump",
		Label: "FlowModulatingValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.FlowModulatingValve.Open)
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
			p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.FlowModulatingValve.Open = v
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


	// CVCS boron operating mode selector. Boration raises the
	// RCS boron concentration toward the cold shutdown value
	// before cooldown; dilution lowers it on startup.
	// "If the plant is to be taken to the cold shutdown
	//  condition, boration is initiated to bring the boron
	//  concentration to the cold shutdown value."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:50-52
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronMode"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "BoronMode",
		Kind:  "enum",
		Enum:  []string{"VolumeControl", "Boration", "Dilution", "Letdown"},
		Get: func() string {
			switch p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode {
			case CVCSBoration:
				return "Boration"
			case CVCSDilution:
				return "Dilution"
			case CVCSLetdown:
				return "Letdown"
			default:
				return "VolumeControl"
			}
		},
		Set: func(s string) error {
			validated, err := bridge.ConvertControlValue("enum", s)
			if err != nil {
				return err
			}
			switch validated {
			case "Boration":
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSBoration
			case "Dilution":
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSDilution
			case "Letdown":
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSLetdown
			case "VolumeControl":
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSVolumeControl
			}
			return nil
		},
	}


	// BTRS recirculation pump: provides the motive force to
	// route flow from the VCT through the demineralizers during
	// boration and dilution; without it neither mode changes
	// boron concentration.
	// "The BTRS recirculation pump provides the motive force to
	//  route flow from the VCT through the demineralizers
	//  during boration and dilution"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:37-39
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSPump"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "BTRSPump",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = v
			return nil
		},
	}

	// BRS recycle evaporator feed pump: the panel latch that moves
	// holdup-tank effluent through the demineralizers, stripping
	// column, and evaporator shell.
	// "borated liquid is then pumped to the recycle evaporator
	// package by one of the recycle evaporator feed pumps"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:1-3
	m["AuxiliaryBuilding/BoronRecycleSystem/FeedPumpRunning"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "FeedPumpRunning",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.BoronRecycleSystem.FeedPumpRunning)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.BoronRecycleSystem.FeedPumpRunning = v
			return nil
		},
	}

	// Evaporator carryover fault latch: contaminates the distillate so
	// the storage-tank radiation monitor sees a high signal. Modeling
	// device for the protection the scintillation counter provides.
	// "To provide that the reactor makeup water storage tank will not
	// become contaminated, a scintillation counter is located in the
	// line leading into the tank."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:25-28
	m["AuxiliaryBuilding/BoronRecycleSystem/Carryover"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "Carryover",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.BoronRecycleSystem.Carryover)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.BoronRecycleSystem.Carryover = v
			return nil
		},
	}



	// CVCS automatic boron trim engage switch: rod-bank travel
	// position selects the BTRS mode so chemistry relieves what the
	// rods no longer can.
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_129.txt:112-115
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoBoronControl"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "AutoBoronControl",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoBoronControl = v
			return nil
		},
	}


	// CVCS letdown valve-open latch. The CVCS Step reads
	// LetdownValveOpen to decide LetdownFlow (in conjunction
	// with the operating mode — CVCSLetdown forces it closed).
	// Manual charging-flow throttle (chapter 4): scales the design
	// charging flow while the pump latch runs outside auto level
	// control.
	// "The charging pump flow and low-pressure letdown valve are
	// controlled manually during plant startup and cooldown."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:115-116
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlowDemand"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "ChargingFlowDemand",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingFlowDemand)
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
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingFlowDemand = v
			return nil
		},
	}


	// CVCS auxiliary spray valve: throttled charging flow routed
	// into the pressurizer for pressure control while the reactor
	// coolant pumps are secured.
	// "Charging flow is throttled to control pressurizer cooldown
	// rate and raise pressurizer level using auxiliary spray since
	// normal spray is not available when reactor coolant pumps have
	// been secured."
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/AuxSprayValve/Open"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "AuxSprayValve",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AuxSprayValveOpen)
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
			p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AuxSprayValveOpen = v
			return nil
		},
	}


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


		// CVCS automatic pressurizer level control engage switch.
		// "The pressurizer level control system adjusts charging
		// flow from the CVCS to maintain pressurizer level at a
		// programmed value which varies with RCS average coolant
		// temperature."
		// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:36-40
		m["AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoLevelControl"] = &bridge.Control{
			Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
			Label: "AutoLevelControl",
			Kind:  "bool",
			Get: func() string {
				return bridge.BoolToStr(
					p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl)
			},
			Set: func(s string) error {
				v, err := bridge.StrToBool(s)
				if err != nil {
					return err
				}
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AutoLevelControl = v
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

	// Train-B diesel controls mirror the train-A entries above:
	// each train's diesel is an independent unit with its own
	// start, load-fraction, and breaker controls on physically
	// separated portions of the main control board
	// "controls for the governor, voltage regulator, generator
	// breaker, and synchronization for each diesel generator are
	// located on physically separated portions of the main control
	// board"
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:2-4.
	m["AuxiliaryBuilding/DieselGeneratorB/StartSignal"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/DieselGeneratorB",
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
				p.AuxiliaryBuilding.DieselGeneratorB.StartSignal)
		},
		Set: func(s string) error {
			switch s {
			case "0":
				p.AuxiliaryBuilding.DieselGeneratorB.StartSignal = DieselStartNone
			case "1":
				p.AuxiliaryBuilding.DieselGeneratorB.StartSignal = DieselStartManual
			case "2":
				p.AuxiliaryBuilding.DieselGeneratorB.StartSignal = DieselStartSafetyInjection
			case "3":
				p.AuxiliaryBuilding.DieselGeneratorB.StartSignal = DieselStartLossOfOffsitePower
			case "4":
				p.AuxiliaryBuilding.DieselGeneratorB.StartSignal = DieselStartTest
			default:
				return fmt.Errorf("invalid StartSignal %q (want 0..4)", s)
			}
			return nil
		},
	}
	m["AuxiliaryBuilding/DieselGeneratorB/RunningFraction"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/DieselGeneratorB",
		Label: "RunningFraction",
		Kind:  "normalized",
		Get: func() string {
			return fmt.Sprintf("%v",
				p.AuxiliaryBuilding.DieselGeneratorB.RunningFraction)
		},
		Set: func(s string) error {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			if f < 0 || f > 1 {
				return fmt.Errorf("normalized %v out of range [0,1]", f)
			}
			p.AuxiliaryBuilding.DieselGeneratorB.RunningFraction = float32(f)
			return nil
		},
	}
	m["AuxiliaryBuilding/DieselGeneratorB/Generator/BreakerClosed"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/DieselGeneratorB/Generator",
		Label: "BreakerClosed",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.DieselGeneratorB.Generator.BreakerClosed)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.DieselGeneratorB.Generator.BreakerClosed = v
			return nil
		},
	}

	// SFPCS cooler-loop run commands, one per duty loop: "Two of
	// three pump-heat-exchanger-filter-demineralizer loops remove
	// decay heat with ... the third pump serving as a standby."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	sfpcLoopKeys := []struct{ key, field string }{
		{"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/LoopAPumpRunning", "A"},
		{"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/LoopBPumpRunning", "B"},
	}
	for _, l := range sfpcLoopKeys {
		l := l
		m[l.key] = &bridge.Control{
			Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
			Label: "Loop" + l.field + "PumpRunning",
			Kind:  "bool",
			Get: func() string {
				sfpc := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
				if l.field == "A" {
					return bridge.BoolToStr(sfpc.LoopAPumpRunning)
				}
				return bridge.BoolToStr(sfpc.LoopBPumpRunning)
			},
			Set: func(s string) error {
				v, err := bridge.StrToBool(s)
				if err != nil {
					return err
				}
				sfpc := &p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
				if l.field == "A" {
					sfpc.LoopAPumpRunning = v
				} else {
					sfpc.LoopBPumpRunning = v
				}
				return nil
			},
		}
	}

	// SFPCS supply-header leak latch. Instrumentation detects the
	// leakage and motor-operated valves isolate it:
	// "Detection of leakage which might affect system operation is
	// provided by means of installed pressure, temperature, and flow
	// instrumentation [...] system isolation is provided by means of
	// motor-operated valves."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:75-83
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Leakage"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "Leakage",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.Leakage)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.Leakage = v
			return nil
		},
	}

	// Refueling-canal crud burst latch: "to regain refueling canal
	// water clarity after a crud burst."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-90
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/CrudBurst"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "CrudBurst",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.CrudBurst)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			sfpc := &p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
			if v && !sfpc.CrudBurst {
				sfpc.Turbidity += SFPCCrudBurstTurbidityPercent
			}
			sfpc.CrudBurst = v
			return nil
		},
	}

	// Refueling water purification alignment beside the pool
	// demineralizer: the pump and filter "can be used separately or
	// in conjunction with the spent fuel pool demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-94
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/RWPUPurifying"] = &bridge.Control{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "RWPUPurifying",
		Kind:  "bool",
		Get: func() string {
			return bridge.BoolToStr(
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.RWPUPurifying)
		},
		Set: func(s string) error {
			v, err := bridge.StrToBool(s)
			if err != nil {
				return err
			}
			p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.RWPUPurifying = v
			return nil
		},
	}

}

// addAuxiliaryBuildingSensors adds Sensor entries keyed by the namespace managed by this file to m.
func addAuxiliaryBuildingSensors(p *Plant, m bridge.SensorMap) {

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

	// Train-B diesel sensors mirror the train-A entries.
	m["AuxiliaryBuilding/DieselGeneratorB/CoolantTemp"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGeneratorB",
		Label: "CoolantTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.AuxiliaryBuilding.DieselGeneratorB.CoolantTemp.Float())
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
			// Stored gal/s; presented gpm (x60 at the presentation boundary).
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingFlow.Float() * 60)
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
			// Stored gal/s; presented gpm (x60 at the presentation boundary).
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownFlow.Float() * 60)
		},
	}


	// Boron setpoint target: the operator-written concentration the
	// tracking drive ramps toward through the BTRS flow modes. While
	// tracking is released the field holds the last target.
	//   "A boron dilution is started to bring the boron concentration
	//   to the estimated critical concentration."
	//   specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:36-38
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/BoronTarget"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "BoronTarget",
		Kind:  "ppm",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("ppm",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronTargetConcentration)
		},
	}


	// Pressurizer level program output: the Tavg-referenced
	// programmed value the automatic level controller holds.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:36-40
	// Auxiliary spray drive actually delivered last step: the
	// valve position gated by the charging pump.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:100-104
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/AuxSprayDrive"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "AuxSprayDrive",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.AuxSprayDrive)
		},
	}

	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/ProgrammedLevel"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "ProgrammedLevel",
		Kind:  "percent",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("percent",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ProgrammedLevel)
		},
	}


	// BRS holdup tank level: diverted letdown accumulates here
	// while the VCT is in its high-level condition.
	// "If the volume control tank is full, a high-water level
	// signal diverts letdown flow to the Boron Recycle"
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:19-21
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/BRSHoldupLevel"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "BRSHoldupLevel",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BRSHoldupLevel.Float())
		},
	}


	// BTRS demineralizer bed saturation: boron changes halt when
	// the resin beds are depleted.
	// "This operation continues until the resin beds are fully
	// depleted as indicated by the boron concentration meter."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:11-13
	m["AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSBedSaturation"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ChemicalVolumeControlSystem",
		Label: "BTRSBedSaturation",
		Kind:  "percent",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("percent",
				p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSBedSaturation)
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

	m["AuxiliaryBuilding/DieselGeneratorB/ReadyToLoad"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/DieselGeneratorB",
		Label: "ReadyToLoad",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.DieselGeneratorB.ReadyToLoad)
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

	// Spray additive tank isolation valves "B" (chapter 6):
	// "opens the SAT isolation valves "B""
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:60-61
	m["AuxiliaryBuilding/ContainmentSpraySystem/SATIsolationValveBOpen"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ContainmentSpraySystem",
		Label: "SATIsolationValveBOpen",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.ContainmentSpraySystem.SATIsolationValveBOpen)
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

	// Solid-plant bleed flow delivered to the CVC letdown path
	// last step through the RHR/CVC pressure control valve.
	// specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:117-122
	m["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantReliefFlow"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/ResidualHeatRemovalSystem",
		Label: "SolidPlantReliefFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			// Stored gal/s; presented gpm (x60 at the presentation boundary).
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantReliefFlow.Float() * 60)
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
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.MotorPump.Running)
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/MotorPumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "MotorPumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.MotorPump.Running)
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
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.Running)
		},
	}

	m["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB/TurbinePumpRunning"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemB",
		Label: "TurbinePumpRunning",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.TurbinePump.Running)
		},
	}


	// SFPCS pump running. Two pumps normal, third standby.
	// "Normal operation requires two pumps with the third pump
	// serving as a standby."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:43-44
	for _, field := range []string{"A", "B"} {
		field := field
		m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Loop"+field+"PumpRunning"] = &bridge.Sensor{
			Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
			Label: "Loop" + field + "PumpRunning",
			Kind:  "bool",
			Get: func() (string, error) {
				sfpc := p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem
				if field == "A" {
					return bridge.FormatSensorValue("bool",
						sfpc.LoopAPumpRunning)
				}
				return bridge.FormatSensorValue("bool",
					sfpc.LoopBPumpRunning)
			},
		}
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

	// SFPCS upstream-power readiness. The pumps ride on a 480 V MCC
	// downstream of an ESF bus (see plans/x/uat_sat_gating.md).
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/DriveReady"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "DriveReady",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.DriveReady)
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
				// Backing constant is stored in true gpm; no conversion.
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PurificationFlow)
		},
	}

	// SFPCS supply-header pressure: "The system pressure is maintained
	// at 55 to 75 psig (4.8 to 6.2 bar) in the main supply header."
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:45-47
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/HeaderPressure"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "HeaderPressure",
		Kind:  "psig",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("psig",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.HeaderPressure)
		},
	}

	// Spent fuel pool water temperature; decay heat warms an uncooled
	// pool toward boiling.
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:24-29
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PoolTemp"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "PoolTemp",
		Kind:  "fahrenheit",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("fahrenheit",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PoolTemp.Float())
		},
	}

	// Pool-water turbidity normalized to the design crud burst;
	// clarity gates refueling speed because a crud burst robs
	// operators of visual access to the fuel.
	// specs/the_westinghouse_pressurized_water_react/chapter_12/page_171.txt:87-94
	m["AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Turbidity"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/SpentFuelPoolCoolingSystem",
		Label: "Turbidity",
		Kind:  "percent",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("percent",
				p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.Turbidity)
		},
	}

	// BRS processing rate drawn from the holdup tanks into the
	// BRS processing rate drawn from the holdup tanks into the
	// evaporator package.
	// "use of the BTRS reduces the average evaporator duty from
	// approximately 10 gpm (38 l/m) to less than 2 gpm (8 l/m) per unit."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:71-74
	m["AuxiliaryBuilding/BoronRecycleSystem/FeedFlow"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "FeedFlow",
		Kind:  "gpm",
		Get: func() (string, error) {
			// Stored gal/s; presented gpm (x60 at the presentation boundary).
			return bridge.FormatSensorValue("gpm",
				p.AuxiliaryBuilding.BoronRecycleSystem.FeedFlow * 60)
		},
	}

	// Distillate radiation monitor, normalized against its trip level.
	m["AuxiliaryBuilding/BoronRecycleSystem/DistillateRadiation"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "DistillateRadiation",
		Kind:  "normalized",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("normalized",
				p.AuxiliaryBuilding.BoronRecycleSystem.DistillateRadiation)
		},
	}

	// Auto-divert status: distillate routed back to the feed
	// demineralizer by the high-radiation signal.
	// "A high radiation level signal at the monitor directs distillate
	// back to the recycle evaporator feed demineralizer."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:28-30
	m["AuxiliaryBuilding/BoronRecycleSystem/DistillateDiverted"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "DistillateDiverted",
		Kind:  "bool",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("bool",
				p.AuxiliaryBuilding.BoronRecycleSystem.DistillateDiverted)
		},
	}

	// Distillate inventory recirculating inside the train while the
	// auto-divert is latched.
	m["AuxiliaryBuilding/BoronRecycleSystem/Recirculating"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "Recirculating",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.AuxiliaryBuilding.BoronRecycleSystem.Recirculating.Float())
		},
	}

	// Cumulative 4 wt% boric acid concentrate sent to the boric acid
	// tanks through the concentrates filter.
	// "Concentrates at 4-weight-percent boric acid are normally pumped
	// to the boric acid tanks through the recycle evaporator
	// concentrates filter."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:53-55
	m["AuxiliaryBuilding/BoronRecycleSystem/BoricAcidProduced"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "BoricAcidProduced",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.AuxiliaryBuilding.BoronRecycleSystem.BoricAcidProduced.Float())
		},
	}

	// Cumulative clean distillate delivered to the reactor makeup water
	// storage tank.
	// "Distillate from the evaporator is pumped to the evaporator
	// condensate demineralizer and then to the reactor makeup water
	// storage tank."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_088.txt:20-22
	m["AuxiliaryBuilding/BoronRecycleSystem/DistillateDelivered"] = &bridge.Sensor{
		Part:  "AuxiliaryBuilding/BoronRecycleSystem",
		Label: "DistillateDelivered",
		Kind:  "gallon",
		Get: func() (string, error) {
			return bridge.FormatSensorValue("gallon",
				p.AuxiliaryBuilding.BoronRecycleSystem.DistillateDelivered.Float())
		},
	}

}
