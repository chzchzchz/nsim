package cmdfile

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJSON(t *testing.T) {
	t.Run("valid envelope", func(t *testing.T) {
		cmds, err := ParseJSON(bytes.NewReader([]byte(
			`{"commands":[` +
				`{"offsetMs":500,"control":"ReactorBuilding/ReactorCoolantPump/Running","value":"1"},` +
				`{"offsetMs":1500,"control":"ReactorBuilding.ReactorCoolantPump.Running","value":"0"}` +
				`]}`,
		)))
		require.NoError(t, err)
		require.Len(t, cmds, 2)
		assert.Equal(t, int64(500), cmds[0].TimeOffsetMs)
		assert.Equal(t, "ReactorBuilding/ReactorCoolantPump/Running", cmds[0].ControlKey)
		assert.Equal(t, "1", cmds[0].Value)
		assert.Equal(t, int64(1500), cmds[1].TimeOffsetMs)
	})

	t.Run("empty envelope", func(t *testing.T) {
		cmds, err := ParseJSON(bytes.NewReader([]byte(`{"commands":[]}`)))
		require.NoError(t, err)
		assert.Empty(t, cmds)
	})

	t.Run("bare array rejected", func(t *testing.T) {
		_, err := ParseJSON(bytes.NewReader([]byte(`[{"offsetMs":0}]`)))
		require.Error(t, err)
	})

	t.Run("bad json rejected", func(t *testing.T) {
		_, err := ParseJSON(bytes.NewReader([]byte(`{"commands":`)))
		require.Error(t, err)
	})
}

func TestWriteJSONRoundTrip(t *testing.T) {
	want := []Command{
		{TimeOffsetMs: 0, ControlKey: "Part/Label", Value: "1"},
		{TimeOffsetMs: 2500, ControlKey: "Part.Label", Value: "0.5", Comment: "why this op"},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteJSON(&buf, want))
	got, err := ParseJSON(&buf)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestServerHistorySerializationRoundTrips(t *testing.T) {
	var queue Queue
	queue.SubmitAll([]Command{{ControlKey: "Part/Label", Value: "1"}}, []string{"0 Part.Label 1"}, 0)
	data, err := json.Marshal(queue)
	require.NoError(t, err)
	var queueCopy Queue
	require.NoError(t, json.Unmarshal(data, &queueCopy))
	assert.Equal(t, queue.Entries(), queueCopy.Entries())
	assert.Equal(t, queue.Rev(), queueCopy.Rev())

	var journal Journal
	journal.Add(Command{ControlKey: "Part/Label", Value: "1", Comment: "why"}, 1_000_000)
	data, err = json.Marshal(journal)
	require.NoError(t, err)
	var journalCopy Journal
	require.NoError(t, json.Unmarshal(data, &journalCopy))
	assert.Equal(t, journal.Entries(), journalCopy.Entries())

	var log Log
	log.AddOperation(Command{ControlKey: "Part/Label", Value: "1"}, 1_000_000, "script")
	log.AddComment("watching", 2_000_000)
	data, err = json.Marshal(log)
	require.NoError(t, err)
	var logCopy Log
	require.NoError(t, json.Unmarshal(data, &logCopy))
	assert.Equal(t, log.Entries(), logCopy.Entries())
	assert.Equal(t, int64(2), logCopy.AddComment("after restore", 3_000_000))
}

func TestJournal(t *testing.T) {
	t.Run("entries accumulate in insertion order", func(t *testing.T) {
		var j Journal
		j.Add(Command{ControlKey: "A/B", Value: "1"}, 1_000_000)
		j.Add(Command{ControlKey: "C/D", Value: "2"}, 2_000_000)
		entries := j.Entries()
		require.Len(t, entries, 2)
		assert.Equal(t, "A/B", entries[0].Cmd.ControlKey)
		assert.Equal(t, int64(1_000_000), entries[0].AtNs)
		assert.Equal(t, "C/D", entries[1].Cmd.ControlKey)
	})

	t.Run("CommandsFrom sorts by offset, stable on ties", func(t *testing.T) {
		var j Journal
		j.Add(Command{ControlKey: "A/B", Value: "late"}, 5_000_000_000)
		j.Add(Command{ControlKey: "A/B", Value: "tie1"}, 1_000_000_000)
		j.Add(Command{ControlKey: "A/B", Value: "early"}, 1_000_000_000)
		cmds := j.CommandsFrom(0)
		require.Len(t, cmds, 3)
		assert.Equal(t, int64(1000), cmds[0].TimeOffsetMs)
		assert.Equal(t, "tie1", cmds[0].Value)
		assert.Equal(t, "early", cmds[1].Value)
		assert.Equal(t, int64(5000), cmds[2].TimeOffsetMs)
		assert.Equal(t, "late", cmds[2].Value)
	})

	t.Run("CommandsFrom shifts by base", func(t *testing.T) {
		var j Journal
		j.Add(Command{ControlKey: "A/B", Value: "1"}, 6_000_000)
		cmds := j.CommandsFrom(1_000_000)
		require.Len(t, cmds, 1)
		assert.Equal(t, int64(5), cmds[0].TimeOffsetMs)
	})
}
