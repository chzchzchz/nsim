package cmdfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLog(t *testing.T) {
	t.Run("records operations with comments by timestamp", func(t *testing.T) {
		var l Log
		l.AddOperation(Command{ControlKey: "Part/Label", Value: "1", Comment: "stop the pump"}, 5_000_000_000, "")
		l.AddComment("watching pressure", 5_000_000_000)
		entries := l.Entries()
		require.Len(t, entries, 2)
		assert.Equal(t, LogKindOperation, entries[0].Kind)
		assert.Equal(t, int64(5000), entries[0].AtMs)
		assert.Equal(t, "Part/Label", entries[0].Control)
		assert.Equal(t, "1", entries[0].Value)
		assert.Equal(t, "stop the pump", entries[0].Comment)
		assert.Equal(t, LogKindComment, entries[1].Kind)
		assert.Equal(t, "watching pressure", entries[1].Comment)
		assert.Equal(t, int64(0), entries[0].Seq)
		assert.Equal(t, int64(1), entries[1].Seq)
	})

	t.Run("never deduplicates", func(t *testing.T) {
		var l Log
		cmd := Command{ControlKey: "Part/Label", Value: "1"}
		l.AddOperation(cmd, 1_000_000, "")
		l.AddOperation(cmd, 1_000_000, "")
		require.Len(t, l.Entries(), 2, "identical submissions must both be logged")
	})

	t.Run("carries source for scripted operations", func(t *testing.T) {
		var l Log
		l.AddOperation(Command{ControlKey: "Part/Label", Value: "1"}, 0, "script")
		require.Len(t, l.Entries(), 1)
		assert.Equal(t, "script", l.Entries()[0].Source)
	})

	t.Run("clear drops entries", func(t *testing.T) {
		var l Log
		l.AddComment("note", 0)
		l.Clear()
		assert.Empty(t, l.Entries())
	})

	t.Run("sequence range is half-open and optional", func(t *testing.T) {
		var l Log
		l.AddOperation(Command{ControlKey: "Part/Label", Value: "1"}, 0, "")
		l.AddOperation(Command{ControlKey: "Part/Label", Value: "2"}, 0, "")
		l.AddComment("note", 0)

		all := l.EntriesWithSeqRange(-1, -1)
		require.Len(t, all, 3)

		fromOne := l.EntriesWithSeqRange(1, -1)
		require.Len(t, fromOne, 2)
		assert.Equal(t, int64(1), fromOne[0].Seq)
		assert.Equal(t, int64(2), fromOne[1].Seq)

		underTwo := l.EntriesWithSeqRange(-1, 2)
		require.Len(t, underTwo, 2)

		between := l.EntriesWithSeqRange(1, 2)
		require.Len(t, between, 1)

		empty := l.EntriesWithSeqRange(3, 3)
		assert.Empty(t, empty)
	})

	t.Run("latest seq identifies newest entry", func(t *testing.T) {
		var l Log
		assert.Equal(t, int64(-1), l.LatestSeq())
		l.AddComment("first", 0)
		assert.Equal(t, int64(0), l.LatestSeq())
		l.AddComment("second", 0)
		assert.Equal(t, int64(1), l.LatestSeq())
	})
}
