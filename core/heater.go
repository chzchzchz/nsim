package core

import (
	u "github.com/bcicen/go-units"
	"time"
)

// Step advances the low-pressure feedwater heater by ProductionStep. The
// caller supplies the feedwater inlet temperature and the per-step
// extraction steam mass flow.
//
// Model:
//
//	Per-stage temperature rise is proportional to the ratio of the
//	supplied extraction steam flow to the design extraction steam
//	flow, capped at 2× the design per-stage rise. This is a "gain"
//	model that captures the qualitative behaviour (more steam → more
//	heat → larger rise) without modelling the explicit extraction-
//	steam chest per turbine stage (which the spec describes but
//	the simulator does not yet have a turbine-extraction model for).
//
// Spec justification for the upstream topology:
//   - "The condensate pumps take suction from the condenser hotwell
//     and pump condensate [...] through four stages of low-pressure
//     feedwater heaters to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:41-45
//   - "Drains from the four low-pressure feedwater heaters are
//     cascaded back to the condenser."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-15
//
// The single-struct LP heater is iterated four times in
// TurbineBuilding.StepWithSteam; the per-stage design rise is
// LPTemperatureRisePerStage. DrainValve gates the drain path; the
// heater produces no useful rise when the valve is closed or no
// extraction steam flows.
func (h *LowPressureHeater) Step(dt time.Duration, fwInletF float64, steamFlowGalPerStep float64) LowPressureHeater {
	ret := *h

	ret.Inlet = Temperature{Val: fwInletF, Unit: u.Fahrenheit}

	if steamFlowGalPerStep <= 0 || ret.DrainValve.Open == 0 {
		ret.TemperatureRise = Temperature{Val: 0, Unit: u.Fahrenheit}
		return ret
	}

	// Per-stage rise scales linearly with supplied steam flow
	// relative to the design extraction flow (10% of the design
	// feedwater flow is the spec's implied extraction fraction
	// across the LP train).
	designExtractionPerStage := FeedwaterDesignFlow.Float() * 0.1
	ratio := steamFlowGalPerStep / designExtractionPerStage
	ratio = min(ratio, 1.0)
	deltaTF := ratio * LPTemperatureRisePerStage
	deltaTF = min(deltaTF, LPTemperatureRisePerStage*2)
	ret.TemperatureRise = Temperature{Val: deltaTF, Unit: u.Fahrenheit}

	// Drain accumulation: condensed steam goes to drain level.
	// LP heater drains cascade back to the condenser; this
	// accumulation is consumed by TurbineBuilding.StepWithSteam.
	ret.DrainLevel = Volume{Val: ret.DrainLevel.Float() + steamFlowGalPerStep*dt.Seconds(), Unit: u.Gallon}

	return ret
}

// Step advances the high-pressure feedwater heater by ProductionStep. The
// model is identical to the LP heater but the per-stage rise is
// smaller (HPTemperatureRisePerStage) because the feedwater is
// already pre-heated by the LP train.
//
// Spec justification:
//   - "The water discharge from the feedwater pumps flows through
//     two stages of high-pressure heaters then into the steam
//     generators."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_220.txt:50-51
//   - "Drains from the high-pressure feedwater heaters are routed
//     to the deaerator."
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_221.txt:13-14
func (h *HighPressureHeater) Step(dt time.Duration, fwInletF float64, steamFlowGalPerStep float64) HighPressureHeater {
	ret := *h

	ret.Inlet = Temperature{Val: fwInletF, Unit: u.Fahrenheit}

	if steamFlowGalPerStep <= 0 || ret.DrainValve.Open == 0 {
		ret.TemperatureRise = Temperature{Val: 0, Unit: u.Fahrenheit}
		return ret
	}

	designExtractionPerStage := FeedwaterDesignFlow.Float() * 0.05
	ratio := steamFlowGalPerStep / designExtractionPerStage
	ratio = min(ratio, 1.0)
	deltaTF := ratio * HPTemperatureRisePerStage
	deltaTF = min(deltaTF, HPTemperatureRisePerStage*2)
	ret.TemperatureRise = Temperature{Val: deltaTF, Unit: u.Fahrenheit}

	ret.DrainLevel = Volume{Val: ret.DrainLevel.Float() + steamFlowGalPerStep*dt.Seconds(), Unit: u.Gallon}

	return ret
}
