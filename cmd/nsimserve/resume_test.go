package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResumeStateRoundTripRestoresRun(t *testing.T) {
	s, ts := newPausedServer(t, "nsimserve")
	code, _ := doOK(t, "POST", ts.URL+"/commands",
		`{"commands":[{"offsetMs":10000,"control":"PlantOperation/PowerSetpointTarget","value":"77","comment":"scheduled before restart"}]}`)
	require.Equal(t, 200, code)
	code, _ = doOK(t, "POST", ts.URL+"/logs", `{"comment":"before restart"}`)
	require.Equal(t, 200, code)
	code, _ = doOK(t, "POST", ts.URL+"/sim/step", `{"ms":5}`)
	require.Equal(t, 200, code)

	resumePath := t.TempDir() + "/resume.json"
	require.NoError(t, saveResumeState(s, resumePath))
	snap, err := readResumeState(resumePath)
	require.NoError(t, err)
	assert.Equal(t, int64(5*time.Millisecond), snap.SimTimeNs)
	assert.Equal(t, int64(5), snap.StepsTotal)
	require.Len(t, snap.Log.Entries(), 2)
	require.Len(t, snap.Queue.Entries(), 1)

	restored, err := NewServer("restored", "", "", "", false, 1, time.Millisecond, false)
	require.NoError(t, err)
	require.NoError(t, restored.restoreResumeState(snap))
	assert.Equal(t, int64(5*time.Millisecond), restored.simTimeNs)
	assert.Equal(t, int64(5), restored.stepsTotal)
	assert.True(t, restored.paused)
	assert.Equal(t, 1000, restored.stepsPerSecond)
	assert.Equal(t, time.Millisecond, restored.dt)
	require.Len(t, restored.log.Entries(), 2)
	assert.Equal(t, "before restart", restored.log.Entries()[1].Comment)
	require.Len(t, restored.queue.Entries(), 1)
	assert.Equal(t, "77", restored.queue.Entries()[0].Cmd.Value)
	assert.Equal(t, 10000*time.Millisecond, time.Duration(restored.queue.Entries()[0].DueNs))
}

func TestResumeStateRejectsBadVersion(t *testing.T) {
	path := t.TempDir() + "/bad.json"
	snap, err := readResumeState(path)
	assert.Error(t, err)
	assert.Equal(t, 0, snap.Version)
}

func TestResumeStateReditsEmptyMissingAndCorrupt(t *testing.T) {
	empty := t.TempDir() + "/empty.json"
	require.NoError(t, os.WriteFile(empty, []byte("  \n"), 0o600))
	_, err := readResumeState(empty)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)

	missing := t.TempDir() + "/missing.json"
	_, err = readResumeState(missing)
	assert.ErrorIs(t, err, os.ErrNotExist)

	corrupt := t.TempDir() + "/corrupt.json"
	require.NoError(t, os.WriteFile(corrupt, []byte("{"), 0o600))
	_, err = readResumeState(corrupt)
	assert.Error(t, err)
}

func TestReadResumeStateReportsInvalidFields(t *testing.T) {
	for name, fields := range map[string]string{
		"version":        `{"version":99}`,
		"stepsPerSecond": `{"version":1,"stepsPerSecond":0}`,
		"timestepNs":     `{"version":1,"stepsPerSecond":1,"timestepNs":0}`,
		"simTimeNs":      `{"version":1,"stepsPerSecond":1,"timestepNs":1,"simTimeNs":-1}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + "/state.json"
			require.NoError(t, os.WriteFile(path, []byte(fields), 0o600))
			_, err := readResumeState(path)
			assert.Error(t, err)
			assert.False(t, errors.Is(err, os.ErrNotExist))
		})
	}
}

func TestShutdownEndpointSignalsServer(t *testing.T) {
	s, ts := newPausedServer(t, "nsimserve")
	code, body := doOK(t, "POST", ts.URL+"/sim/shutdown", "")
	require.Equal(t, 200, code)
	assert.Contains(t, string(body), `"shuttingDown"`)
	select {
	case <-s.shutdown:
	default:
		t.Fatal("shutdown channel was not closed")
	}
}

func TestShutdownEndpointIsIdempotentConcurrently(t *testing.T) {
	s, _ := newPausedServer(t, "nsimserve")
	done := make(chan struct{}, 8)
	for range 8 {
		go func() {
			w := httptest.NewRecorder()
			s.handleShutdown(w, httptest.NewRequest("POST", "/sim/shutdown", nil))
			assert.Equal(t, 200, w.Code)
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}
	select {
	case <-s.shutdown:
	default:
		t.Fatal("shutdown channel was not closed")
	}
}

func TestCaptureSnapshotIncludesScenarioID(t *testing.T) {
	s, err := NewServer("nsimserve", "rcp-trip", "", "", true, 1000, time.Millisecond, false)
	require.NoError(t, err)
	s.mu.Lock()
	snap := s.captureLocked()
	s.mu.Unlock()
	assert.Equal(t, "rcp-trip", snap.ScenarioID)
	require.NotNil(t, s.scenario)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"scenarioId":"rcp-trip"`)
}
