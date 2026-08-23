package cmdfile

import (
	"testing"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueSubmit(t *testing.T) {
	t.Run("schedules at now plus offset", func(t *testing.T) {
		var q Queue
		require.NoError(t, q.Submit("1000 Part.Label 1\n2000 Part.Label 0\n", 10_000_000))
		entries := q.Entries()
		require.Len(t, entries, 2)
		assert.Equal(t, int64(1_010_000_000), entries[0].DueNs)
		assert.Equal(t, int64(2_010_000_000), entries[1].DueNs)
		assert.Equal(t, StatusPending, entries[0].Status)
		assert.Equal(t, "1000 Part.Label 1", entries[0].Text)
		assert.Equal(t, int64(0), entries[0].Seq)
		assert.Equal(t, int64(1), entries[1].Seq)
	})

	t.Run("negative offset clamps to now", func(t *testing.T) {
		var q Queue
		require.NoError(t, q.Submit("-5 K V", 7_000_000))
		require.Len(t, q.Entries(), 1)
		assert.Equal(t, int64(7_000_000), q.Entries()[0].DueNs)
	})

	t.Run("malformed batch rejects atomically", func(t *testing.T) {
		var q Queue
		err := q.Submit("100 K V\nbadline\n", 0)
		require.Error(t, err)
		assert.Empty(t, q.Entries())
	})

	t.Run("empty text is a no-op", func(t *testing.T) {
		var q Queue
		rev := q.Rev()
		require.NoError(t, q.Submit("\n# comment only\n", 0))
		assert.Empty(t, q.Entries())
		assert.Equal(t, rev, q.Rev())
	})
}

// countingControl returns a ControlMap whose Set records calls.
func countingControl(calls *[]string) bridge.ControlMap {
	return bridge.ControlMap{
		"Part/Label": {Set: func(s string) error {
			*calls = append(*calls, s)
			return nil
		}},
	}
}

func TestQueueFireDue(t *testing.T) {
	t.Run("fires in due then seq order", func(t *testing.T) {
		var q Queue
		require.NoError(t, q.Submit(
			"20 A.B 1\n10 C.D 2\n15 A.B 3\n",
			0,
		))
		m := bridge.ControlMap{
			"A/B": {Set: func(s string) error { return nil }},
			"C/D": {Set: func(s string) error { return nil }},
		}
		q.FireDue(16_000_000, m)
		// Parse sorts by offset, so pair assertions with the
		// command timestamps rather than file positions.
		statusByOffset := map[int64]Status{}
		for _, e := range q.Entries() {
			statusByOffset[e.Cmd.TimeOffsetMs] = e.Status
		}
		assert.Equal(t, StatusDone, statusByOffset[10])
		assert.Equal(t, StatusDone, statusByOffset[15])
		assert.Equal(t, StatusPending, statusByOffset[20])

		q.FireDue(21_000_000, m)
		for _, e := range q.Entries() {
			assert.Equal(t, StatusDone, e.Status)
		}
	})

	t.Run("boundary fires at exactly due", func(t *testing.T) {
		var q Queue
		require.NoError(t, q.Submit("5 K.V 1", 1_000_000))
		calls := &[]string{}
		m := bridge.ControlMap{
			"K/V": {Set: func(s string) error {
				*calls = append(*calls, s)
				return nil
			}},
		}
		q.FireDue(5_999_999, m)
		assert.Empty(t, *calls)
		q.FireDue(6_000_000, m)
		require.Len(t, *calls, 1)
	})

	t.Run("failure does not block later entries", func(t *testing.T) {
		var q Queue
		require.NoError(t, q.Submit("0 Bad.K 1\n0 Good.K 2", 0))
		m := bridge.ControlMap{
			"Good/K": {Set: func(s string) error { return nil }},
		}
		q.FireDue(1_000_000, m)
		entries := q.Entries()
		require.Len(t, entries, 2)
		assert.Equal(t, StatusFailed, entries[0].Status)
		assert.Contains(t, entries[0].Err, "unknown control")
		assert.Equal(t, StatusDone, entries[1].Status)
	})

	t.Run("rev bumps only on change", func(t *testing.T) {
		var q Queue
		m := countingControl(&[]string{})
		require.NoError(t, q.Submit("10 K.V 1", 0))
		rev := q.Rev()
		q.FireDue(5_000_000, m)
		assert.Equal(t, rev, q.Rev())
		q.FireDue(10_000_000, m)
		assert.Greater(t, q.Rev(), rev)
		again := q.Rev()
		q.FireDue(10_000_000, m)
		assert.Equal(t, again, q.Rev())
	})
}

func TestQueueApplyMutatesPlant(t *testing.T) {
	var q Queue
	p := core.MakePlant()
	m := p.GetControlMap()
	require.NoError(t, q.Submit("0 ReactorBuilding.ReactorCoolantPump.Running 1", 0))
	q.FireDue(0, m)
	assert.True(t, p.ReactorBuilding.ReactorCoolantPump.Running)
}
