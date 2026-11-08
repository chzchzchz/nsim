package cmdfile

import (
	"sort"
	"strings"

	"github.com/chzchzchz/nsim/bridge"
)

// Status classifies a queued command.
type Status uint8

const (
	StatusPending Status = iota
	StatusDone
	StatusFailed
)

// String renders the status for JSON marshaling in
// QueueJSON-style payloads.
func (s Status) String() string {
	switch s {
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "pending"
	}
}

// Entry is a queued command plus its scheduling state.
type Entry struct {
	Seq    int64 // submission order, stable sort key
	Text   string
	Cmd    Command
	DueNs  int64 // absolute sim time (submit time + offset)
	Status Status
	Err    string
}

// Queue holds commands scheduled against a simulation clock.
type Queue struct {
	entries []Entry
	nextSeq int64
	rev     int64
}

// Submit parses text (same grammar as Parse), schedules each
// command by its timing field (absolute AtMs when present,
// otherwise nowNs + offset; times before now clamp to nowNs),
// and returns a parse error rejecting the whole batch if any
// line is malformed.
func (q *Queue) Submit(text string, nowNs int64) error {
	cmds, lines, err := parseCmds(strings.NewReader(text))
	if err != nil {
		return err
	}
	entries := q.schedule(cmds, lines, nowNs)
	q.entries = append(q.entries, entries...)
	if len(cmds) > 0 {
		q.rev++
	}
	return nil
}

// SubmitAll schedules already-parsed commands by their timing
// (absolute AtMs when present, otherwise nowNs + offset; both
// clamp to nowNs) and returns the new entries. The caller
// supplies the display text for each command.
func (q *Queue) SubmitAll(cmds []Command, texts []string, nowNs int64) []Entry {
	entries := q.scheduleAll(cmds, texts, nowNs)
	q.entries = append(q.entries, entries...)
	if len(cmds) > 0 {
		q.rev++
	}
	return entries
}

func (q *Queue) schedule(cmds []Command, lines []string, nowNs int64) []Entry {
	return q.scheduleAll(cmds, lines, nowNs)
}

func (q *Queue) scheduleAll(cmds []Command, texts []string, nowNs int64) []Entry {
	out := make([]Entry, 0, len(cmds))
	for i, c := range cmds {
		due := c.DueNs(nowNs)
		text := ""
		if i < len(texts) {
			text = texts[i]
		}
		out = append(out, Entry{
			Seq:   q.nextSeq,
			Text:  text,
			Cmd:   c,
			DueNs: due,
		})
		q.nextSeq++
	}
	return out
}

// Clear drops all queued entries and resets the sequence counter.
func (q *Queue) Clear() {
	q.entries = nil
	q.nextSeq = 0
	q.rev++
}

// FireDue applies every pending entry with DueNs <= nowNs, in
// (DueNs, Seq) order, marking each Done or Failed with the Set
// error. Applied entries are kept for UI display.
func (q *Queue) FireDue(nowNs int64, m bridge.ControlMap) {
	var due []int
	for i := range q.entries {
		if q.entries[i].Status == StatusPending && q.entries[i].DueNs <= nowNs {
			due = append(due, i)
		}
	}
	if len(due) == 0 {
		return
	}
	sort.Slice(due, func(a, b int) bool {
		ea, eb := &q.entries[due[a]], &q.entries[due[b]]
		if ea.DueNs != eb.DueNs {
			return ea.DueNs < eb.DueNs
		}
		return ea.Seq < eb.Seq
	})
	for _, i := range due {
		e := &q.entries[i]
		if err := Apply(m, e.Cmd); err != nil {
			e.Status = StatusFailed
			e.Err = err.Error()
		} else {
			e.Status = StatusDone
		}
	}
	q.rev++
}

// Entries returns a copy of the entries.
func (q *Queue) Entries() []Entry {
	out := make([]Entry, len(q.entries))
	copy(out, q.entries)
	return out
}

// Rev increments whenever any entry changes state; UIs poll it
// to skip redundant re-renders.
func (q *Queue) Rev() int64 {
	return q.rev
}
