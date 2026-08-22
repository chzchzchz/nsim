//go:build js

// Command wasm is the browser entry point for the simulator.
// It exports a small JavaScript-friendly surface that drives a
// *core.Plant through syscall/js. The TypeScript front-end in
// web/src imports the glue module produced by `go build` and
// calls into these functions.
package main

import (
	"bytes"
	"fmt"
	"syscall/js"
	"time"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/core/svg"
)

// plantHandle is the JavaScript-visible wrapper around a
// *core.Plant. JavaScript holds a reference to the wrapper; the
// wrapper keeps the plant pointer. Step / GetSvg / SetControl
// / GetSensor all delegate through this struct.
type plantHandle struct {
	p          *core.Plant
	simTimeNs  int64
	createdAt  time.Time
}

func newPlantHandle() *plantHandle {
	p := core.MakePlant()
	return &plantHandle{p: &p, createdAt: time.Now()}
}

// step advances the plant by ticks `Step()`/`Update()` cycles.
// Each cycle is one `core.TimeStep` (1 ms) of simulated time.
func (h *plantHandle) step(ticks int) {
	if ticks <= 0 {
		return
	}
	for i := 0; i < ticks; i++ {
		h.p.Step()
		h.p.Update()
	}
	h.simTimeNs += int64(ticks) * int64(core.TimeStep)
}

func (h *plantHandle) controlKeys() []string {
	cm := h.p.GetControlMap()
	keys := make([]string, 0, len(cm))
	for k := range cm {
		keys = append(keys, k)
	}
	return keys
}

func (h *plantHandle) sensorKeys() []string {
	sm := h.p.GetSensorMap()
	keys := make([]string, 0, len(sm))
	for k := range sm {
		keys = append(keys, k)
	}
	return keys
}

// lookupControl and lookupSensor look up an entry by id in the
// corresponding map. They return (nil, false) when the id is
// absent, which jsMeta uses to fall through to the other map and
// ultimately to an error.
func (h *plantHandle) lookupControl(id string) (*bridge.Control, bool) {
	c, ok := h.p.GetControlMap()[id]
	return c, ok
}

func (h *plantHandle) lookupSensor(id string) (*bridge.Sensor, bool) {
	s, ok := h.p.GetSensorMap()[id]
	return s, ok
}

func (h *plantHandle) setControl(id, value string) error {
	cm := h.p.GetControlMap()
	c, ok := cm[id]
	if !ok {
		return fmt.Errorf("unknown control %q", id)
	}
	return c.Set(value)
}

// sensorJSON returns the sensor value as a JSON-quoted string.
// The TypeScript side parses it with JSON.parse.
func (h *plantHandle) sensorJSON(id string) (string, error) {
	sm := h.p.GetSensorMap()
	s, ok := sm[id]
	if !ok {
		return "", fmt.Errorf("unknown sensor %q", id)
	}
	v, err := s.Get()
	if err != nil {
		return "", err
	}
	return v, nil
}

func (h *plantHandle) getSvg() (string, error) {
	var buf bytes.Buffer
	if err := svg.WritePlantSVG(&buf, h.p); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (h *plantHandle) simTimeMs() int64 {
	return h.simTimeNs / int64(time.Millisecond)
}

// jsStringSlice converts a Go []string into a JS Array<string>.
func jsStringSlice(s []string) js.Value {
	arr := js.Global().Get("Array").New(len(s))
	for i, v := range s {
		arr.SetIndex(i, js.ValueOf(v))
	}
	return arr
}

// jsMeta returns {part, label, kind} for the given bridge entry,
// or an Error if the id is unknown. The TypeScript UI uses this
// to label, group, and unit-tag controls and sensors instead of
// reverse-engineering the id string. The two closures extract
// Part/Label/Kind from the bridge.Control and bridge.Sensor
// structs, which differ in map type but share these fields.
func jsMeta(
	id string,
	getControl func(string) (*bridge.Control, bool),
	getSensor func(string) (*bridge.Sensor, bool),
) js.Value {
	meta := func(part, label, kind string) js.Value {
		obj := js.Global().Get("Object").New()
		obj.Set("part", part)
		obj.Set("label", label)
		obj.Set("kind", kind)
		return obj
	}
	if c, ok := getControl(id); ok {
		return meta(c.Part, c.Label, c.Kind)
	}
	if s, ok := getSensor(id); ok {
		return meta(s.Part, s.Label, s.Kind)
	}
	return js.Global().Get("Error").New(fmt.Sprintf("unknown id %q", id))
}

// jsHandle boxes the *plantHandle as a JS object whose methods
// the TypeScript wrapper invokes directly.
func jsHandle(h *plantHandle) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("step", js.FuncOf(func(_ js.Value, args []js.Value) any {
		ticks := args[0].Int()
		h.step(ticks)
		return nil
	}))
	obj.Set("controlKeys", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return jsStringSlice(h.controlKeys())
	}))
	obj.Set("sensorKeys", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return jsStringSlice(h.sensorKeys())
	}))
	obj.Set("getMeta", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return jsMeta(args[0].String(), h.lookupControl, h.lookupSensor)
	}))
	obj.Set("setControl", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if err := h.setControl(args[0].String(), args[1].String()); err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return nil
	}))
	obj.Set("sensorJSON", js.FuncOf(func(_ js.Value, args []js.Value) any {
		v, err := h.sensorJSON(args[0].String())
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return v
	}))
	obj.Set("getSvg", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := h.getSvg()
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return s
	}))
	obj.Set("simTimeMs", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return h.simTimeMs()
	}))
	return obj
}

func main() {
	// Expose a single factory to JS. The TypeScript wrapper calls
	// nsimNewPlant() to obtain a handle, then drives it through
	// the methods exposed above.
	js.Global().Set("nsimNewPlant", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return jsHandle(newPlantHandle())
	}))

	// Block forever. The wasm module's lifecycle is tied to the
	// JavaScript runtime that loaded it.
	select {}
}
