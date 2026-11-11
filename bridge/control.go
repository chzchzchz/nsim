package bridge

import "strings"

type GetControlFunc func() string
type SetControlFunc func(s string) error

type ControlMap map[string]*Control

type Control struct {
	Part string
	Label string
	Kind string
	// Enum holds optional human-readable labels for an
	// enum-typed control. The index of each label is the
	// value the Set callback receives. When nil or empty
	// the bridge renders the legacy free-text input.
	Enum []string
	Get GetControlFunc
	Set SetControlFunc
}

// IsFault marks deliberate malfunction injectors. It is a pure
// function of the registration's own group path: fault controls are
// filed under <System>/Faults, so the namespace IS the marker. No
// stored field — one source of truth.
func (c *Control) IsFault() bool {
	return strings.HasSuffix(c.Part, "/Faults")
}