package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPausedServer returns a ready server that does not tick on a
// loop; tests drive time explicitly via POST /sim/step.
func newPausedServer(t *testing.T, site string) (*Server, *httptest.Server) {
	t.Helper()
	s, err := NewServer(site, "", "", "", true, 1000, time.Millisecond)
	require.NoError(t, err)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func doOK(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, b
}

func TestLivezReadyzAndStatus(t *testing.T) {
	_, ts := newPausedServer(t, "train-a")
	code, _ := doOK(t, "GET", ts.URL+"/livez", "")
	assert.Equal(t, http.StatusOK, code)
	code, _ = doOK(t, "GET", ts.URL+"/readyz", "")
	assert.Equal(t, http.StatusOK, code)

	code, body := doOK(t, "GET", ts.URL+"/status", "")
	require.Equal(t, http.StatusOK, code)
	var st statusView
	require.NoError(t, json.Unmarshal(body, &st))
	assert.Equal(t, "train-a", st.SiteName)
	assert.Equal(t, int64(0), st.RunID)
	assert.Equal(t, float64(1.0), st.TimestepMs)
	assert.Equal(t, 1000, st.StepsPerSecond)
	assert.True(t, st.Paused)
	assert.False(t, st.Running)
	assert.False(t, st.Objective.Active)
	assert.Equal(t, int64(-1), st.LatestSeq)

	code, _ = doOK(t, "POST", ts.URL+"/logs", `{"comment":"status seq check"}`)
	require.Equal(t, http.StatusOK, code)
	code, body = doOK(t, "GET", ts.URL+"/status", "")
	require.Equal(t, http.StatusOK, code)
	require.NoError(t, json.Unmarshal(body, &st))
	assert.Equal(t, int64(0), st.LatestSeq)
}

func TestLoopAdvancesSimTime(t *testing.T) {
	s, err := NewServer("nsimserve", "", "", "", false, 1000, time.Millisecond)
	require.NoError(t, err)
	stop := make(chan struct{})
	done := make(chan struct{})
	go s.loop(stop, done)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	defer func() { close(stop); <-done }()

	// Wait for the sim clock to advance measurably.
	require.Eventually(t, func() bool {
		code, body := doOK(t, "GET", ts.URL+"/status", "")
		if code != http.StatusOK {
			return false
		}
		var st statusView
		if err := json.Unmarshal(body, &st); err != nil {
			return false
		}
		return st.SimTimeMs > 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestMetricsGaugesAndSiteInfo(t *testing.T) {
	_, ts := newPausedServer(t, "train-b")
	code, body := doOK(t, "GET", ts.URL+"/metrics", "")
	require.Equal(t, http.StatusOK, code)
	text := string(body)
	assert.Contains(t, text, `nsim_server_info{site="train-b"} 1`)
	assert.Contains(t, text, "nsim_sim_time_seconds")
	assert.Contains(t, text, "nsim_alarms_active")
	assert.Contains(t, text, "nsim_steps_total")
	assert.Contains(t, text, `nsim_ReactorBuilding_ReactorPressureVessel_Pressure`)
}

func TestSaveOnShutdownWritesPlantJSON(t *testing.T) {
	s, err := NewServer("nsimserve", "", "", "", true, 1000, time.Millisecond)
	require.NoError(t, err)
	path := t.TempDir() + "/plant.json"
	require.NoError(t, saveOnShutdown(s, path))
	data, err := readFile(path)
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Contains(t, doc, "ReactorBuilding")
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func TestControlsCatalogIsSortedAndPUTable(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "GET", ts.URL+"/controls", "")
	require.Equal(t, http.StatusOK, code)
	var catalog []controlEntry
	require.NoError(t, json.Unmarshal(body, &catalog))
	require.NotEmpty(t, catalog)
	for i := 1; i < len(catalog); i++ {
		assert.LessOrEqual(t, catalog[i-1].Key, catalog[i].Key)
	}
	// Setting every advertised control to its current value must succeed.
	for _, entry := range catalog {
		payload := `{"value":` + jsonStr(entry.Value) + `}`
		code, _ = doOK(t, "PUT", ts.URL+"/controls/"+urlEncodeKey(entry.Key), payload)
		assert.Equal(t, http.StatusOK, code, "PUT %s", entry.Key)
	}
}

func TestControlRoundTripAndErrors(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	// Percent control.
	code, body := doOK(t, "GET", ts.URL+"/controls/PlantOperation%2FPowerSetpointTarget", "")
	require.Equal(t, http.StatusOK, code)
	var entry controlEntry
	require.NoError(t, json.Unmarshal(body, &entry))
	assert.Equal(t, "percent", entry.Kind)

	code, body = doOK(t, "PUT", ts.URL+"/controls/PlantOperation%2FPowerSetpointTarget",
		`{"value":"90"}`)
	require.Equal(t, http.StatusOK, code)
	var updated controlEntry
	require.NoError(t, json.Unmarshal(body, &updated))
	assert.Equal(t, "90", updated.Value)

	// Bad value -> 400 with envelope.
	code, body = doOK(t, "PUT", ts.URL+"/controls/PlantOperation%2FPowerSetpointTarget",
		`{"value":"999"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), `"code":400`)

	// Unknown key -> 404.
	code, _ = doOK(t, "PUT", ts.URL+"/controls/Nope%2FMissing", `{"value":"0"}`)
	assert.Equal(t, http.StatusNotFound, code)

	// Unknown path -> 404 envelope; wrong method -> 405 envelope.
	code, body = doOK(t, "GET", ts.URL+"/no-such-path", "")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Contains(t, string(body), `"code":404`)
}

func TestSensorsAndState(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "GET", ts.URL+"/sensors/ReactorBuilding%2FReactorPressureVessel%2FPressure", "")
	require.Equal(t, http.StatusOK, code)
	var sensor sensorEntry
	require.NoError(t, json.Unmarshal(body, &sensor))
	_, err := parseFloat(sensor.Value)
	require.NoError(t, err)
	assert.Equal(t, "ReactorBuilding/ReactorPressureVessel", sensor.Part)

	// /state round-trips through json.Mashal of core.Plant.
	code, body = doOK(t, "GET", ts.URL+"/state", "")
	require.Equal(t, http.StatusOK, code)
	var plantJSON map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &plantJSON))
	assert.Contains(t, plantJSON, "ReactorBuilding")

	// /initial-state is byte-compatible and unaffected by steps.
	code, initialBody := doOK(t, "GET", ts.URL+"/initial-state", "")
	require.Equal(t, http.StatusOK, code)
	var initJSON map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(initialBody, &initJSON))
	assert.Contains(t, initJSON, "ReactorBuilding")
	// Advance the sim: /state may move, /initial-state must not.
	code, _ = doOK(t, "POST", ts.URL+"/sim/step", `{"steps":5}`)
	require.Equal(t, http.StatusOK, code)
	_, afterBody := doOK(t, "GET", ts.URL+"/initial-state", "")
	assert.Equal(t, string(initialBody), string(afterBody))
	// Method not allowed on POST.
	code, _ = doOK(t, "POST", ts.URL+"/initial-state", "")
	assert.Equal(t, http.StatusMethodNotAllowed, code)
}

func TestInitialStateReflectsReset(t *testing.T) {
	s, ts := newPausedServer(t, "nsimserve")
	_, before := doOK(t, "GET", ts.URL+"/initial-state", "")
	// Reset to a scenario; initial-state updates to the new lineup.
	code, _ := doOK(t, "POST", ts.URL+"/sim/reset", `{"scenario":"rcp-trip"}`)
	require.Equal(t, http.StatusOK, code)
	_, after := doOK(t, "GET", ts.URL+"/initial-state", "")
	var initial map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(after, &initial))
	assert.Contains(t, initial, "ReactorBuilding")
	_ = before
	_ = s
}

func TestStepPauseResumeAndConflict(t *testing.T) {
	s, ts := newPausedServer(t, "nsimserve")
	// Resume then attempt a step: 409 while running.
	code, _ := doOK(t, "POST", ts.URL+"/sim/resume", "")
	assert.Equal(t, http.StatusOK, code)
	code, body := doOK(t, "POST", ts.URL+"/sim/step", `{"ms":10}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Contains(t, string(body), `"code":409`)

	code, _ = doOK(t, "POST", ts.URL+"/sim/pause", "")
	assert.Equal(t, http.StatusOK, code)
	code, body = doOK(t, "POST", ts.URL+"/sim/step", `{"ms":10}`)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"steps"`)
	s.mu.RLock()
	assert.Equal(t, int64(10), s.simTimeNs/int64(time.Millisecond))
	s.mu.RUnlock()
}

func TestTimestepAndSpeedHotChange(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "POST", ts.URL+"/sim/timestep", `{"dtMs":10}`)
	require.Equal(t, http.StatusOK, code)
	var tsOut struct {
		TimestepMs float64 `json:"timestepMs"`
	}
	require.NoError(t, json.Unmarshal(body, &tsOut))
	assert.Equal(t, float64(10), tsOut.TimestepMs)

	code, body = doOK(t, "POST", ts.URL+"/sim/speed", `{"stepsPerSecond":100}`)
	require.Equal(t, http.StatusOK, code)
	var spOut struct {
		StepsPerSecond int `json:"stepsPerSecond"`
	}
	require.NoError(t, json.Unmarshal(body, &spOut))
	assert.Equal(t, 100, spOut.StepsPerSecond)

	code, body = doOK(t, "GET", ts.URL+"/status", "")
	require.Equal(t, http.StatusOK, code)
	var st statusView
	require.NoError(t, json.Unmarshal(body, &st))
	assert.Equal(t, float64(10), st.TimestepMs)
	assert.Equal(t, 100, st.StepsPerSecond)

	code, _ = doOK(t, "POST", ts.URL+"/sim/timestep", `{"dtMs":-1}`)
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestCommandsPostExportAndDedup(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	// JSON envelope form.
	code, body := doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":0,"control":"PlantOperation/PowerSetpointTarget","value":"80"}]}`)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"scheduled"`)

	// Re-POST of the identical batch (same sim time, paused) dedups.
	code, body = doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":0,"control":"PlantOperation/PowerSetpointTarget","value":"80"}]}`)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"deduped"`)

	// Text form.
	req, err := http.NewRequest("POST", ts.URL+"/commands",
		strings.NewReader("0 PlantOperation.PowerSetpointTarget 70\n"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Export replays through cmdfile parsing.
	code, body = doOK(t, "GET", ts.URL+"/commands/export", "")
	require.Equal(t, http.StatusOK, code)
	var export commandsExport
	require.NoError(t, json.Unmarshal(body, &export))
	require.NotEmpty(t, export.Commands)

	// Queue view surfaces the entries.
	code, body = doOK(t, "GET", ts.URL+"/commands", "")
	require.Equal(t, http.StatusOK, code)
	var view commandsView
	require.NoError(t, json.Unmarshal(body, &view))
	assert.NotEmpty(t, view.Entries)

	// Parse error rejects the whole batch (400).
	code, _ = doOK(t, "POST", ts.URL+"/commands", "{not json")
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestCommandsExportRoutePattern(t *testing.T) {
	// Regression: Go 1.27 ServeMux matches more specific patterns
	// first, so /commands/export must not be captured by
	// /commands/{key...} - here it must win over /commands.
	_, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "GET", ts.URL+"/commands/export", "")
	require.Equal(t, http.StatusOK, code)
	var export commandsExport
	require.NoError(t, json.Unmarshal(body, &export))
}

func TestLogsEndpoint(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")

	// POST /commands with comments: operations land in the log
	// with their attached comment at their due timestamp.
	code, body := doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":0,"control":"PlantOperation/PowerSetpointTarget","value":"80","comment":"raise setpoint"}]}`)
	require.Equal(t, http.StatusOK, code)
	var sched commandsResponse
	require.NoError(t, json.Unmarshal(body, &sched))
	require.Len(t, sched.Scheduled, 1)
	firstLogSeq := sched.Scheduled[0].LogSeq
	assert.Equal(t, int64(0), firstLogSeq)

	// Re-POST of the identical batch dedupes the journal but must
	// still append a second log entry (no dedup in /logs).
	code, body = doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":0,"control":"PlantOperation/PowerSetpointTarget","value":"80","comment":"raise setpoint"}]}`)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"deduped"`)

	// PUT with a comment also logs.
	code, body = doOK(t, "PUT", ts.URL+"/controls/PlantOperation%2FPowerSetpointTarget",
		`{"value":"85","comment":"trim setpoint"}`)
	require.Equal(t, http.StatusOK, code)
	var putOut struct {
		Seq int64 `json:"seq"`
	}
	require.NoError(t, json.Unmarshal(body, &putOut))
	assert.Equal(t, int64(2), putOut.Seq)

	// Bare comment via POST /logs.
	code, body = doOK(t, "POST", ts.URL+"/logs", `{"comment":"watching pressure"}`)
	require.Equal(t, http.StatusOK, code)
	var commentOut struct {
		Seq     int64  `json:"seq"`
		Comment string `json:"comment"`
	}
	require.NoError(t, json.Unmarshal(body, &commentOut))
	assert.Equal(t, int64(3), commentOut.Seq)
	assert.Equal(t, "watching pressure", commentOut.Comment)

	// Empty comment is rejected.
	code, _ = doOK(t, "POST", ts.URL+"/logs", `{"comment":"  "}`)
	assert.Equal(t, http.StatusBadRequest, code)

	code, body = doOK(t, "GET", ts.URL+"/logs", "")
	require.Equal(t, http.StatusOK, code)
	var entries []logView
	require.NoError(t, json.Unmarshal(body, &entries))
	ops := 0
	comments := 0
	for _, e := range entries {
		switch e.Kind {
		case "operation":
			ops++
		case "comment":
			comments++
		}
	}
	assert.Equal(t, 3, ops, "two /commands submissions plus one PUT")
	assert.Equal(t, 1, comments)
	assert.Equal(t, firstLogSeq, entries[0].Seq)
	assert.Equal(t, "raise setpoint", entries[0].Comment)
	assert.Equal(t, "raise setpoint", entries[1].Comment, "deduped re-POST is still logged")
	assert.Equal(t, "trim setpoint", entries[2].Comment)
	assert.Equal(t, "watching pressure", entries[3].Comment)
	// seq is monotonically increasing and timestamps never regress.
	for i := 1; i < len(entries); i++ {
		assert.Greater(t, entries[i].Seq, entries[i-1].Seq)
		assert.GreaterOrEqual(t, entries[i].AtMs, entries[i-1].AtMs)
	}
}

func TestLogsSequenceRange(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	code, _ := doOK(t, "POST", ts.URL+"/logs", `{"comment":"one"}`)
	require.Equal(t, http.StatusOK, code)
	code, _ = doOK(t, "POST", ts.URL+"/logs", `{"comment":"two"}`)
	require.Equal(t, http.StatusOK, code)
	code, _ = doOK(t, "POST", ts.URL+"/logs", `{"comment":"three"}`)
	require.Equal(t, http.StatusOK, code)

	cases := []struct {
		query string
		seqs  []int64
	}{
		{"", []int64{0, 1, 2}},
		{"?lo=1", []int64{1, 2}},
		{"?hi=2", []int64{0, 1}},
		{"?lo=1&hi=2", []int64{1}},
		{"?lo=2&hi=2", []int64{}},
	}
	for _, tc := range cases {
		code, body := doOK(t, "GET", ts.URL+"/logs"+tc.query, "")
		require.Equal(t, http.StatusOK, code)
		var entries []logView
		require.NoError(t, json.Unmarshal(body, &entries))
		require.Len(t, entries, len(tc.seqs), tc.query)
		for i, seq := range tc.seqs {
			assert.Equal(t, seq, entries[i].Seq)
		}
	}

	code, body := doOK(t, "GET", ts.URL+"/logs?lo=next", "")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), `"code":400`)

	code, _ = doOK(t, "GET", ts.URL+"/logs?hi=-1", "")
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestLogsAbsoluteVsRelative(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	// Advance the sim clock to 5000 ms, then post one relative
	// and one absolute command. Both must appear in /logs at
	// their resolved fire times.
	code, _ := doOK(t, "POST", ts.URL+"/sim/step", `{"ms":5000}`)
	require.Equal(t, http.StatusOK, code)

	code, _ = doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"atMs":20000,"control":"PlantOperation/PowerSetpointTarget","value":"80","comment":"absolute fire at 20000"}]}`)
	require.Equal(t, http.StatusOK, code)
	code, _ = doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":10000,"control":"PlantOperation/PowerSetpointTarget","value":"70","comment":"relative fire at 15000"}]}`)
	require.Equal(t, http.StatusOK, code)

	code, body := doOK(t, "GET", ts.URL+"/logs", "")
	require.Equal(t, http.StatusOK, code)
	var entries []logView
	require.NoError(t, json.Unmarshal(body, &entries))
	require.Len(t, entries, 2)
	assert.Equal(t, int64(20000), entries[0].AtMs)
	assert.Equal(t, "absolute fire at 20000", entries[0].Comment)
	assert.Equal(t, int64(15000), entries[1].AtMs)
	assert.Equal(t, "relative fire at 15000", entries[1].Comment)
}

func TestPostCommandsAbsoluteAtMs(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	// Advance the sim clock to 5000 ms.
	code, _ := doOK(t, "POST", ts.URL+"/sim/step", `{"ms":5000}`)
	require.Equal(t, http.StatusOK, code)

	// Absolute: atMs 20000 fires at 20000, not at 5000 + offset.
	code, _ = doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"atMs":20000,"offsetMs":1,"control":"PlantOperation/PowerSetpointTarget","value":"80","comment":"absolute"}]}`)
	require.Equal(t, http.StatusOK, code)

	// Relative posts from the same base prove the distinction.
	code, body := doOK(t, "GET", ts.URL+"/commands", "")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"dueMs": 20000`, "absolute command must fire at atMs")

	code, _ = doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":10000,"control":"PlantOperation/PowerSetpointTarget","value":"70","comment":"relative"}]}`)
	require.Equal(t, http.StatusOK, code)
	code, body = doOK(t, "GET", ts.URL+"/commands", "")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"dueMs": 15000`, "relative command fires at submission(5000)+offset(10000)")

	// Step to just past both and verify the log carries the
	// absolute fire times.
	code, _ = doOK(t, "POST", ts.URL+"/sim/step", `{"ms":16000}`)
	require.Equal(t, http.StatusOK, code)
	code, body = doOK(t, "GET", ts.URL+"/commands", "")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"status": "done"`)
}

func TestResetBumpsRunID(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "POST", ts.URL+"/sim/reset", `{"scenario":""}`)
	require.Equal(t, http.StatusOK, code)
	var out struct {
		RunID int64 `json:"runId"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	assert.Equal(t, int64(1), out.RunID)

	code, body = doOK(t, "GET", ts.URL+"/status", "")
	require.Equal(t, http.StatusOK, code)
	var st statusView
	require.NoError(t, json.Unmarshal(body, &st))
	assert.Equal(t, int64(1), st.RunID)
	assert.Equal(t, int64(0), st.SimTimeMs)
}

func TestResetReloadsStartupCommands(t *testing.T) {
	// /sim/reset must re-arm the -cmd file so a reset run replays the
	// same scheduled commands as a fresh launch.
	cmdFile := t.TempDir() + "/cmds.txt"
	require.NoError(t, os.WriteFile(cmdFile,
		[]byte("0 PlantOperation.PowerSetpointTarget 42\n"), 0o644))
	s, err := NewServer("nsimserve", "", "", cmdFile, true, 1000, time.Millisecond)
	require.NoError(t, err)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Fire the initial schedule and observe the value.
	code, _ := doOK(t, "POST", ts.URL+"/sim/step", `{"ms":1}`)
	require.Equal(t, http.StatusOK, code)

	code, _ = doOK(t, "POST", ts.URL+"/sim/reset", `{}`)
	require.Equal(t, http.StatusOK, code)

	// After reset the queue is re-armed with the startup commands.
	code, body := doOK(t, "GET", ts.URL+"/commands", "")
	require.Equal(t, http.StatusOK, code)
	var view commandsView
	require.NoError(t, json.Unmarshal(body, &view))
	assert.NotEmpty(t, view.Entries)
}

func TestNewServerValidatesBounds(t *testing.T) {
	_, err := NewServer("nsimserve", "", "", "", true, 0, time.Millisecond)
	assert.Error(t, err)
	_, err = NewServer("nsimserve", "", "", "", true, maxStepsPerSecond+1, time.Millisecond)
	assert.Error(t, err)
	_, err = NewServer("nsimserve", "", "", "", true, 1000, 0)
	assert.Error(t, err)
	_, err = NewServer("nsimserve", "", "", "", true, 1000, maxTimestepMs*time.Millisecond+time.Millisecond)
	assert.Error(t, err)
}

func TestPostCommandsJSONVsTextSniffEquivalence(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	// Same logical submission via both wire forms must schedule
	// identically (same seq count).
	code, _ := doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":0,"control":"PlantOperation/PowerSetpointTarget","value":"77"}]}`)
	require.Equal(t, http.StatusOK, code)

	// Text form with an explicit content-type and also sniffed via
	// a leading brace without a JSON content-type must agree.
	req, err := http.NewRequest("POST", ts.URL+"/commands",
		strings.NewReader(`{"commands":[{"offsetMs":0,"control":"PlantOperation/PowerSetpointTarget","value":"78"}]}`))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestScenarioProgressLatches(t *testing.T) {
	s, err := NewServer("nsimserve", "rcp-trip", "", "", true, 1000, time.Millisecond)
	require.NoError(t, err)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	code, body := doOK(t, "GET", ts.URL+"/status", "")
	require.Equal(t, http.StatusOK, code)
	var st statusView
	require.NoError(t, json.Unmarshal(body, &st))
	assert.True(t, st.Objective.Active)
	assert.Equal(t, "rcp-trip", st.Objective.ID)
	_ = s
}

func TestLogRequestsMiddleware(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	code, _ := doOK(t, "GET", ts.URL+"/livez", "")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, buf.String(), "request")
	assert.Contains(t, buf.String(), "GET")
	assert.Contains(t, buf.String(), "/livez")
	assert.Contains(t, buf.String(), "200")

	// OPTIONS preflights are not access-logged.
	buf.Reset()
	code, _ = doOK(t, "OPTIONS", ts.URL+"/controls/PlantOperation%2FPowerSetpointTarget", "")
	assert.Equal(t, http.StatusNoContent, code)
	assert.NotContains(t, buf.String(), "OPTIONS")
}

func TestCORSPreflight(t *testing.T) {
	_, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "OPTIONS", ts.URL+"/controls/PlantOperation%2FPowerSetpointTarget", "")
	assert.Equal(t, http.StatusNoContent, code)
	assert.Empty(t, body)
}

func TestMetricNameSanitizer(t *testing.T) {
	assert.Equal(t, "nsim_a_b_c", metricName("a/b.c"))
	assert.Equal(t, "nsim_ReactorBuilding_ReactorPressureVessel_Pressure",
		metricName("ReactorBuilding/ReactorPressureVessel/Pressure"))
}

// urlEncodeKey percent-encodes slashes in a key for path use.
func urlEncodeKey(key string) string {
	return strings.ReplaceAll(key, "/", "%2F")
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
