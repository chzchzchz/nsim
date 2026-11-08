package cmdfile

import "time"

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
	Seq     int64        `json:"seq"`
	Kind    LogKind  `json:"kind"`
	AtMs    int64        `json:"atMs"`
	Control string       `json:"control,omitempty"`
	Value   string       `json:"value,omitempty"`
	Comment string       `json:"comment,omitempty"`
	Source  string       `json:"source,omitempty"`
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
// so rationale and operation stay together in one entry.
func (l *Log) AddOperation(c Command, atNs int64, source string) {
	l.entries = append(l.entries, LogEntry{
		Seq:     l.nextSeq,
		Kind:    LogKindOperation,
		AtMs:    atNs / int64(time.Millisecond),
		Control: c.ControlKey,
		Value:   c.Value,
		Comment: c.Comment,
		Source:  source,
	})
	l.nextSeq++
}

// AddComment appends a bare comment at atNs.
func (l *Log) AddComment(text string, atNs int64) {
	l.entries = append(l.entries, LogEntry{
		Seq:     l.nextSeq,
		Kind:    LogKindComment,
		AtMs:    atNs / int64(time.Millisecond),
		Comment: text,
	})
	l.nextSeq++
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
