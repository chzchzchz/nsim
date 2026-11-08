package cmdfile

import (
	"encoding/json"
	"time"
)

// LogKind distinguishes operation records from bare operator
// comments in the log.
type LogKind string

const (
	// LogKindOperation is a submitted or applied control action.
	LogKindOperation LogKind = "operation"
	// LogKindComment is an operator annotation with no control.
	LogKindComment LogKind = "comment"
)

// LogEntry is one append-only record in the operation log:
// what happened, when it takes effect in simulated time, and
// the rationale the author attached to it. Entries are stored
// by timestamp and never deduplicated, so re-submitting an
// identical command records a second entry, exactly like a
// second line written in a logbook.
//
// Source records where the operation came from so an operator
// can tell a scenario-scripted action from one they issued
// themselves; it is empty for comments and ad-hoc writes.
type LogEntry struct {
	Seq     int64   `json:"seq"`
	Kind    LogKind `json:"kind"`
	AtMs    int64   `json:"atMs"`
	Control string  `json:"control,omitempty"`
	Value   string  `json:"value,omitempty"`
	Comment string  `json:"comment,omitempty"`
	Source  string  `json:"source,omitempty"`
}

// Log is an append-only, timestamped record of command-file
// operations (including any comments attached to them) and of
// comments submitted on their own through the log endpoint.
//
// The log deliberately does not deduplicate: the journal owns
// dedup for replay, while the log is the human-readable audit
// trail, so every submission appends, preserving submission
// order at equal timestamps.
type Log struct {
	entries []LogEntry
	nextSeq int64
}

// AddOperation appends an operation record taking effect at
// atNs, tagged with the given source (for example "script"
// for scenario opening commands, or "" for client requests).
// The command's Comment, when present, is recorded with it
// so rationale and operation stay together in one entry. The
// returned seq is the log entry's stable identifier, matching
// the Seq field in GET /logs.
func (l *Log) AddOperation(c Command, atNs int64, source string) int64 {
	seq := l.nextSeq
	l.entries = append(l.entries, LogEntry{
		Seq:     seq,
		Kind:    LogKindOperation,
		AtMs:    atNs / int64(time.Millisecond),
		Control: c.ControlKey,
		Value:   c.Value,
		Comment: c.Comment,
		Source:  source,
	})
	l.nextSeq++
	return seq
}

// AddComment appends a bare comment at atNs and returns the
// entry's seq, matching the Seq field in GET /logs.
func (l *Log) AddComment(text string, atNs int64) int64 {
	seq := l.nextSeq
	l.entries = append(l.entries, LogEntry{
		Seq:     seq,
		Kind:    LogKindComment,
		AtMs:    atNs / int64(time.Millisecond),
		Comment: text,
	})
	l.nextSeq++
	return seq
}

// Clear drops all log entries.
func (l *Log) Clear() {
	l.entries = nil
	l.nextSeq = 0
}

// Entries returns a copy of the entries in insertion order,
// which is timestamp order because the simulated clock never
// runs backwards within a run.
func (l *Log) Entries() []LogEntry {
	out := make([]LogEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// LatestSeq returns the seq of the most recent log entry,
// or -1 when the log is empty. Agents use it to detect other
// users' submissions while analyzing.
func (l *Log) LatestSeq() int64 {
	if len(l.entries) == 0 {
		return -1
	}
	return l.entries[len(l.entries)-1].Seq
}

// EntriesWithSeqRange filters entries to the half-open seq range
// [lo, hi). Both bounds are optional in the sense that a negative
// value means no bound on that side: -1 for lo means all entries
// pass the lower bound, and -1 for hi means no upper bound applies.
func (l *Log) EntriesWithSeqRange(lo, hi int64) []LogEntry {
	out := make([]LogEntry, 0, len(l.entries))
	for _, e := range l.entries {
		if lo >= 0 && e.Seq < lo {
			continue
		}
		if hi >= 0 && e.Seq >= hi {
			continue
		}
		out = append(out, e)
	}
	return out
}

// logJSON is the explicit shape for Log serialization.
type logJSON struct {
	Entries []LogEntry `json:"entries"`
	NextSeq int64      `json:"nextSeq"`
}

// MarshalJSON serializes the full audit log and the next
// sequence value so /logs history and future IDs survive a
// server restart without collision.
func (l Log) MarshalJSON() ([]byte, error) {
	return json.Marshal(logJSON{Entries: l.entries, NextSeq: l.nextSeq})
}

// UnmarshalJSON restores a log serialized by MarshalJSON.
func (l *Log) UnmarshalJSON(data []byte) error {
	var raw logJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	l.entries = raw.Entries
	l.nextSeq = raw.NextSeq
	return nil
}
