package cmdfile

import (
	"sort"
	"time"
)

// JournalEntry records a submitted command and the simulated
// time at which it takes effect.
type JournalEntry struct {
	AtNs int64
	Cmd  Command
}

// Journal accumulates control commands issued during one
// simulation run. It lives outside the js/wasm build tag so
// ordering and conversion stay unit-testable.
type Journal struct {
	entries []JournalEntry
}

// Add appends a command with its effect time.
func (j *Journal) Add(c Command, atNs int64) {
	j.entries = append(j.entries, JournalEntry{AtNs: atNs, Cmd: c})
}

// Entries returns a copy of the entries in insertion order.
func (j *Journal) Entries() []JournalEntry {
	out := make([]JournalEntry, len(j.entries))
	copy(out, j.entries)
	return out
}

// CommandsFrom renders entries as Commands whose offsets are
// measured from baseNs, ordered by offset with ties broken by
// insertion order (the order nsimrun's apply loop requires).
func (j *Journal) CommandsFrom(baseNs int64) []Command {
	cmds := make([]Command, len(j.entries))
	for i, e := range j.entries {
		cmds[i] = e.Cmd
		cmds[i].TimeOffsetMs = (e.AtNs - baseNs) / int64(time.Millisecond)
	}
	sort.SliceStable(cmds, func(a, b int) bool {
		return cmds[a].TimeOffsetMs < cmds[b].TimeOffsetMs
	})
	return cmds
}
