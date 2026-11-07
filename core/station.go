package core

import "time"

// Step advances the station transformer and house-load model.
//
// The UAT is energised when the generator breaker is closed and
// the generator terminal voltage is non-zero. The SAT is
// energised whenever the off-site grid is available (gridVoltage
// > 0). House load is supplied from the UAT when it is
// energised; on a turbine trip or grid loss the SAT picks up
// house load automatically.
//
// Spec justification:
//   - "the unit auxiliary power is transformed from the output of
//     the plant main generator through the unit auxiliary
//     transformer to the plant electrical distribution voltage."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:45-48
//   - "Because plant buses are capable of receiving power from
//     either of two supplies and because automatic transfer of
//     supplies is provided on sensing loss of power, reliability
//     of power is assured."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:56-59
//   - "house loads automatically transferred to the offsite
//     sources of power."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:64-66
func (s *StationTransformer) Step(dt time.Duration,
	generatorVoltage float64,
	gridVoltage float64,
	breakerClosed bool,
) StationTransformer {
	ret := *s
	ret.UATEnergized = breakerClosed && generatorVoltage > 0
	ret.SATEnergized = gridVoltage > 0
	if ret.UATEnergized {
		ret.UATOutputVoltage = StationRatedBusVoltage
	} else {
		ret.UATOutputVoltage = 0
	}
	if ret.SATEnergized {
		ret.SATOutputVoltage = StationRatedBusVoltage
	} else {
		ret.SATOutputVoltage = 0
	}
	switch {
	case ret.UATEnergized:
		ret.HouseLoadSuppliedFrom = "UAT"
	case ret.SATEnergized:
		ret.HouseLoadSuppliedFrom = "SAT"
	default:
		ret.HouseLoadSuppliedFrom = ""
	}
	return ret
}

// Step computes the station's own electrical demand. The
// variable component scales linearly with the turbine's load
// fraction when the generator is online; the fixed component is
// always present.
//
// Spec justification:
//   - "supply power for its own auxiliaries."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:57-58
func (h *HouseLoad) Step(dt time.Duration,
	generatorOnline bool,
	generatorOutputFraction float64,
) HouseLoad {
	ret := *h
	ret.TotalMW = ret.FixedMW
	if generatorOnline {
		f := generatorOutputFraction
		f = min(max(f, 0), 1)
		ret.TotalMW += ret.VariableMW * f
	}
	return ret
}
