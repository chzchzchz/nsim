package cmdfile

import (
	"encoding/json"
	"sort"
	"time"
)

// JournalEntry records a submitted command and the simulated
// time at which it takes effect.
type JournalEntry struct {
	AtNs int64   `json:"atNs"` // Absolute simulated effect time in nanoseconds
	Cmd  Command `json:"cmd"`  // Journaled control action
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

// Has reports whether a command with the same control, value, and
// effect time already exists in the journal.
func (j *Journal) Has(control, value string, atNs int64) bool {
	return j.SeqOf(control, value, atNs) > 0
}

// SeqOf returns the 1-based insertion index of the first journal
// entry with the given identity, or -1 when there is none. Clients
// use it to reference the original journaled command after dedup.
func (j *Journal) SeqOf(control, value string, atNs int64) int64 {
	for i, e := range j.entries {
		if e.Cmd.ControlKey == control && e.Cmd.Value == value && e.AtNs == atNs {
			return int64(i) + 1
		}
	}
	return -1
}

// Clear drops all journaled entries.
func (j *Journal) Clear() {
	j.entries = nil
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

// MarshalJSON serializes every journaled entry in insertion
// order so command dedup and history survive a server restart.
func (j Journal) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.entries)
}

// UnmarshalJSON restores a journal serialized by MarshalJSON.
func (j *Journal) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &j.entries)
}
