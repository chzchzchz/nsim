package core

// Stepper represents the functions to step through the simulation.
type Stepper interface {
	// Compute one step based on current state into next buffer
	Step()
	// Write next buffer into current state
	Update()
}
