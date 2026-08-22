package bridge

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