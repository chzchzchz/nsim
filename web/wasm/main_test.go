//go:build js

// Command wasm tests exercise the generation-cache layer and
// the invalidation points of plantHandle without touching
// syscall/js globals. GOROOT's lib/wasm/wasm_exec.js ships the
// browser-only variant (no Node runner tail), so plain
// `GOOS=js GOARCH=wasm go test` cannot exec them; compile and
// drive it instead:
//
//	GOOS=js GOARCH=wasm go test -c -o /tmp/nsim-nsim-alt5/wasm.test ./web/wasm
//	node <driver> /tmp/nsim-nsim-alt5/wasm.test -test.v=true
//
// where <driver> instantiates globalThis.Go from GOROOT's
// wasm_exec.js, sets go.argv/go.exit, runs the instance, and maps
// its exit code onto process.exitCode.

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func newTestHandle(t *testing.T) *plantHandle {
	t.Helper()
	h, err := newPlantHandle("")
	if err != nil {
		t.Fatalf("newPlantHandle: %v", err)
	}
	return h
}

func TestCachedReusesValueWithinGeneration(t *testing.T) {
	h := newTestHandle(t)
	calls := 0
	gc := genCache{}
	get := func() (string, error) {
		calls++
		return "value", nil
	}
	if v, err := h.cached(&gc, get); err != nil || v != "value" {
		t.Fatalf("first get: %q %v", v, err)
	}
	if v, _ := h.cached(&gc, get); v != "value" {
		t.Fatalf("second get: %q", v)
	}
	if calls != 1 {
		t.Fatalf("compute ran %d times within one generation", calls)
	}
}

func TestCachedRecomputesAfterBump(t *testing.T) {
	h := newTestHandle(t)
	gc := genCache{}
	if _, err := h.cached(&gc, func() (string, error) { return "old", nil }); err != nil {
		t.Fatal(err)
	}
	before := h.stateGen
	h.bumpState()
	if h.stateGen != before+1 {
		t.Fatalf("stateGen = %d, want %d", h.stateGen, before+1)
	}
	v, err := h.cached(&gc, func() (string, error) { return "new", nil })
	if err != nil || v != "new" {
		t.Fatalf("after bump got %q %v, want fresh compute", v, err)
	}
}

func TestSensorValuesCacheHitAndInvalidation(t *testing.T) {
	h := newTestHandle(t)
	fresh, err := h.sensorValuesJSON()
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal([]byte(fresh), &snap); err != nil {
		t.Fatalf("snapshot not JSON: %v", err)
	}
	if len(snap.Values) == 0 {
		t.Fatal("expected sensors in snapshot")
	}

	h.sensorCache.val = "SENTINEL"
	if got, _ := h.sensorValuesJSON(); got != "SENTINEL" {
		t.Fatalf("cache miss within one generation: %q", got)
	}

	h.bumpState()
	got, err := h.sensorValuesJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got == "SENTINEL" || !strings.Contains(got, `"values"`) {
		t.Fatalf("stale cache survived bump: %q", got)
	}
}

func TestStepBumpsOnlyWhenTicksRun(t *testing.T) {
	h := newTestHandle(t)
	gen0 := h.stateGen
	h.step(0)
	if h.stateGen != gen0 {
		t.Fatal("step(0) must not invalidate caches")
	}
	ms0 := h.simTimeMs()
	h.step(5)
	if h.stateGen != gen0+1 {
		t.Fatalf("stateGen = %d after step(5), want %d", h.stateGen, gen0+1)
	}
	if h.simTimeMs() != ms0+5 {
		t.Fatalf("simTimeMs = %d, want %d", h.simTimeMs(), ms0+5)
	}
}

func TestSetControlBumpsAndValidates(t *testing.T) {
	h := newTestHandle(t)

	var id, current string
	for k, c := range h.controls {
		id, current = k, c.Get()
		break
	}
	if id == "" {
		t.Fatal("plant exposes no controls")
	}

	gen0 := h.stateGen
	if _, ok := h.controls[id]; !ok {
		t.Fatalf("control %q vanished", id)
	}
	if err := h.setControl(id, current); err != nil {
		t.Fatalf("setControl(%q, %q): %v", id, current, err)
	}
	if h.stateGen != gen0+1 {
		t.Fatalf("successful set did not bump: %d -> %d", gen0, h.stateGen)
	}

	if err := h.setControl("No/Such/Control", "1"); err == nil {
		t.Fatal("unknown control accepted")
	}
	if h.stateGen != gen0+1 {
		t.Fatal("failed set must not bump")
	}
}

func TestPlantFromJSONRoundTrip(t *testing.T) {
	src := newTestHandle(t)
	raw, err := src.plantJSON()
	if err != nil {
		t.Fatal(err)
	}
	h, err := newPlantFromJSON(raw)
	if err != nil {
		t.Fatalf("newPlantFromJSON: %v", err)
	}
	if len(h.controls) != len(src.controls) {
		t.Fatalf("controls: got %d want %d", len(h.controls), len(src.controls))
	}
	if len(h.sensors) != len(src.sensors) {
		t.Fatalf("sensors: got %d want %d", len(h.sensors), len(src.sensors))
	}
	svg, err := h.getSvg()
	if err != nil {
		t.Fatalf("getSvg after round-trip: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Fatal("expected svg document after round-trip")
	}
	if h.scenarioID != "" {
		t.Fatalf("scenarioID should be empty, got %q", h.scenarioID)
	}
	if h.progress != nil {
		t.Fatal("progress should be nil for JSON-restored plants")
	}
}

func TestPlantFromJSONRejectsBadInput(t *testing.T) {
	if _, err := newPlantFromJSON("not-json"); err == nil {
		t.Fatal("expected error for non-JSON input")
	}
}

func TestObjectiveCachePerGeneration(t *testing.T) {
	h := newTestHandle(t)
	first, err := h.objectiveJSON()
	if err != nil {
		t.Fatal(err)
	}
	h.objectiveCch.val = "SENTINEL"
	if got, _ := h.objectiveJSON(); got != "SENTINEL" {
		t.Fatalf("objective cache missed within generation: %q", got)
	}
	h.bumpState()
	got, err := h.objectiveJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got == "SENTINEL" || !strings.Contains(got, `"active"`) {
		t.Fatalf("stale objective cache survived bump: %q", got)
	}
	_ = first
}
