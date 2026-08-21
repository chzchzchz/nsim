package bridge

type GetControlFunc func() string
type SetControlFunc func(s string) error

type ControlMap map[string]*Control

type Control struct {
	Part string
	Label string
	Kind string
	Get GetControlFunc
	Set SetControlFunc
}