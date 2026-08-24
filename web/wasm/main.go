//go:build js

// Command wasm is the browser entry point for the simulator.
// It exports a small JavaScript-friendly surface that drives a
// *core.Plant through syscall/js. The TypeScript front-end in
// web/src imports the glue module produced by `go build` and
// calls into these functions.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/cmdfile"
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
	queue      *cmdfile.Queue
	journal    *cmdfile.Journal
	controls   bridge.ControlMap
	sensors    bridge.SensorMap
}

func newPlantHandle() *plantHandle {
	p := core.MakePlant()
	return &plantHandle{
		p:        &p,
		createdAt: time.Now(),
		queue:    &cmdfile.Queue{},
		journal:  &cmdfile.Journal{},
		controls: p.GetControlMap(),
		sensors:  p.GetSensorMap(),
	}
}

// step advances the plant by ticks `Step()`/`Update()` cycles.
// Each cycle is one `core.TimeStep` (1 ms) of simulated time.
// Queued console commands whose due time is reached fire at the
// exact simulated millisecond, independent of ticks per call.
func (h *plantHandle) step(ticks int) {
	if ticks <= 0 {
		return
	}
	for range ticks {
		next := h.simTimeNs + int64(core.TimeStep)
		h.queue.FireDue(next, h.controls)
		h.p.Step()
		h.p.Update()
		h.simTimeNs = next
	}
}

// controlKeys and sensorKeys reflect the cached bridge maps,
// whose closures dereference live plant state on every call (see
// core.GetControlMap), so the lists survive Step/Update cycles.
func (h *plantHandle) controlKeys() []string {
	keys := make([]string, 0, len(h.controls))
	for k := range h.controls {
		keys = append(keys, k)
	}
	return keys
}

func (h *plantHandle) sensorKeys() []string {
	keys := make([]string, 0, len(h.sensors))
	for k := range h.sensors {
		keys = append(keys, k)
	}
	return keys
}

// lookupControl and lookupSensor look up an entry by id in the
// corresponding map. They return (nil, false) when the id is
// absent, which jsMeta uses to fall through to the other map and
// ultimately to an error.
func (h *plantHandle) lookupControl(id string) (*bridge.Control, bool) {
	c, ok := h.controls[id]
	return c, ok
}

func (h *plantHandle) lookupSensor(id string) (*bridge.Sensor, bool) {
	s, ok := h.sensors[id]
	return s, ok
}

// setControl applies a UI control change and journals it at the
// current simulated time so sessions can be exported and
// replayed with nsimrun.
func (h *plantHandle) setControl(id, value string) error {
	key := cmdfile.ResolveKey(h.controls, id)
	c, ok := h.controls[key]
	if !ok {
		return fmt.Errorf("unknown control %q", id)
	}
	if err := c.Set(value); err != nil {
		return err
	}
	h.journal.Add(cmdfile.Command{ControlKey: key, Value: value}, h.simTimeNs)
	return nil
}

// controlValuesJSON returns {"values":{id:value}} for every
// control so the UI seeds all widget initial states with a single
// JS<->Go crossing per panel rebuild.
func (h *plantHandle) controlValuesJSON() (string, error) {
	values := make(map[string]string, len(h.controls))
	for id, c := range h.controls {
		values[id] = c.Get()
	}
	b, err := json.Marshal(struct {
		Values map[string]string `json:"values"`
	}{values})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// sensorJSON returns the sensor value as a plain string.
func (h *plantHandle) sensorJSON(id string) (string, error) {
	s, ok := h.sensors[id]
	if !ok {
		return "", fmt.Errorf("unknown sensor %q", id)
	}
	return s.Get()
}

// sensorValuesJSON returns {"values":{id:value},"errors":{id:msg}}
// for every sensor so the UI polls all readings with a single
// JS<->Go crossing per frame.
func (h *plantHandle) sensorValuesJSON() (string, error) {
	values := make(map[string]string, len(h.sensors))
	errs := make(map[string]string)
	for id, s := range h.sensors {
		v, err := s.Get()
		if err != nil {
			errs[id] = err.Error()
			continue
		}
		values[id] = v
	}
	out := struct {
		Values map[string]string `json:"values"`
		Errors map[string]string `json:"errors"`
	}{values, errs}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// submitCommands parses console text and schedules each
// command relative to the current simulation time. Scheduled
// entries are journaled at their due time.
func (h *plantHandle) submitCommands(text string) error {
	before := len(h.queue.Entries())
	if err := h.queue.Submit(text, h.simTimeNs); err != nil {
		return err
	}
	for _, e := range h.queue.Entries()[before:] {
		h.journal.Add(e.Cmd, e.DueNs)
	}
	return nil
}

type queueEntryJSON struct {
	Seq    int64  `json:"seq"`
	Text   string `json:"text"`
	DueMs  int64  `json:"dueMs"`
	Status string `json:"status"`
	Err    string `json:"err,omitempty"`
}

type queueJSON struct {
	Rev     int64            `json:"rev"`
	Entries []queueEntryJSON `json:"entries"`
}

// commandQueueRev returns the queue revision so the UI can skip
// marshaling the full snapshot on idle frames.
func (h *plantHandle) commandQueueRev() int64 {
	return h.queue.Rev()
}

// commandQueueJSON snapshots the console queue for the UI list.
func (h *plantHandle) commandQueueJSON() (string, error) {
	entries := h.queue.Entries()
	out := queueJSON{Rev: h.queue.Rev(), Entries: make([]queueEntryJSON, len(entries))}
	for i, e := range entries {
		out.Entries[i] = queueEntryJSON{
			Seq:    e.Seq,
			Text:   e.Text,
			DueMs:  e.DueNs / int64(time.Millisecond),
			Status: e.Status.String(),
			Err:    e.Err,
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// commandLogJSON renders every journaled control command as a
// JSON command file loadable via `nsimrun -cmd *.json`.
func (h *plantHandle) commandLogJSON() (string, error) {
	var buf bytes.Buffer
	if err := cmdfile.WriteJSON(&buf, h.journal.CommandsFrom(0)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (h *plantHandle) getSvg() (string, error) {
	var buf bytes.Buffer
	opts := svg.RenderOptions{PhaseMs: float64(h.simTimeNs) / 1e6}
	if err := svg.WritePlantSVGWithOptions(&buf, h.p, svg.CurrentLayout(), opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (h *plantHandle) simTimeMs() int64 {
	return h.simTimeNs / int64(time.Millisecond)
}

func (h *plantHandle) plantJSON() (string, error) {
	b, err := json.Marshal(h.p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// jsStringSlice converts a Go []string into a JS Array<string>.
func jsStringSlice(s []string) js.Value {
	arr := js.Global().Get("Array").New(len(s))
	for i, v := range s {
		arr.SetIndex(i, js.ValueOf(v))
	}
	return arr
}

// jsMeta returns {part, label, kind, enum?} for the given
// bridge entry, or an Error if the id is unknown. The
// TypeScript UI uses this to label, group, and unit-tag
// controls and sensors instead of reverse-engineering the id
// string. The two closures extract Part/Label/Kind from the
// bridge.Control and bridge.Sensor structs, which differ in
// map type but share these fields. Enum is only set for
// controls that populate the optional bridge.Control.Enum
// slice; nil slices are skipped so the TS side can use a
// presence test.
func jsMeta(
	id string,
	getControl func(string) (*bridge.Control, bool),
	getSensor func(string) (*bridge.Sensor, bool),
) js.Value {
	meta := func(part, label, kind string, enumVals []string) js.Value {
		obj := js.Global().Get("Object").New()
		obj.Set("part", part)
		obj.Set("label", label)
		obj.Set("kind", kind)
		if len(enumVals) > 0 {
			obj.Set("enum", jsStringSlice(enumVals))
		}
		return obj
	}
	if c, ok := getControl(id); ok {
		return meta(c.Part, c.Label, c.Kind, c.Enum)
	}
	if s, ok := getSensor(id); ok {
		return meta(s.Part, s.Label, s.Kind, nil)
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
	obj.Set("controlValuesJSON", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := h.controlValuesJSON()
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return s
	}))
	obj.Set("sensorJSON", js.FuncOf(func(_ js.Value, args []js.Value) any {
		v, err := h.sensorJSON(args[0].String())
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return v
	}))
	obj.Set("sensorValuesJSON", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := h.sensorValuesJSON()
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return s
	}))
	obj.Set("submitCommands", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if err := h.submitCommands(args[0].String()); err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return nil
	}))
	obj.Set("commandQueueRev", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return h.commandQueueRev()
	}))
	obj.Set("commandQueueJSON", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := h.commandQueueJSON()
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return s
	}))
	obj.Set("commandLogJSON", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := h.commandLogJSON()
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return s
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
	obj.Set("toJSON", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s, err := h.plantJSON()
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		return s
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
