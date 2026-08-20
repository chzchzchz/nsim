package core

// Step advances the grid by one timestep. For an infinite-bus
// model the only dynamics are the operator-applied perturbations;
// the grid itself is a passive source/sink of frequency and
// voltage that the operator (or scenario script) controls.
//
// Spec justification for the infinite-bus assumption:
//   - "transmit power to the off-site distribution grid."
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:18-19
//   - "critical importance in the event of large grid system
//     disturbances."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_233.txt:59-60
//
// In a fuller model this would include grid inertia, other
// generators, and load-frequency control. For "important aspects"
// the infinite-bus assumption is sufficient: Frequency and
// Voltage are pure inputs held at their current values unless
// the operator changes them.
func (g *Grid) Step() Grid {
	ret := *g
	return ret
}
