// Package cmdfile parses and applies time-stamped control
// commands shared by cmd/nsimrun -cmd files and the web UI
// command console.
package cmdfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/chzchzchz/nsim/bridge"
)

// Command is one time-stamped control action, identical to a
// line in an nsimrun -cmd file (see cmd/nsimrun/README.md).
// The JSON tags define the web console export format consumed
// by `nsimrun -cmd *.json`. Comment explains why the action is
// being taken; authors are encouraged to attach one to every
// command so the operation log tells the story of a run.
type Command struct {
	// TimeOffsetMs is relative to submission: this command fires
	// at simTime + offset. Negative offsets clamp to now. For
	// files that intend absolute times, use AtMs instead.
	TimeOffsetMs int64  `json:"offsetMs"`
	// AtMs, when present (>0), is the absolute simulated time at
	// which the command fires. AtMs wins over TimeOffsetMs when
	// both are set; times before now clamp to now.
	AtMs       *int64 `json:"atMs,omitempty"`
	ControlKey string `json:"control"`
	Value      string `json:"value"`
	Comment    string `json:"comment,omitempty"`
}

// commandFile is the JSON envelope shared by the web UI
// command-log export and the nsimrun -cmd *.json loader.
type commandFile struct {
	Commands []Command `json:"commands"`
}

// DueNs resolves the absolute sim time at which c fires,
// given the submission time nowNs. An explicit AtMs wins over
// TimeOffsetMs; both fall back to clamping past times to nowNs
// so a command never fires in the past.
func (c Command) DueNs(nowNs int64) int64 {
	if c.AtMs != nil {
		return max(nowNs, *c.AtMs*int64(time.Millisecond))
	}
	return max(nowNs, nowNs+c.TimeOffsetMs*int64(time.Millisecond))
}

// ParseJSON reads the envelope written by WriteJSON.
func ParseJSON(r io.Reader) ([]Command, error) {
	var cf commandFile
	if err := json.NewDecoder(r).Decode(&cf); err != nil {
		return nil, fmt.Errorf("parse command JSON: %w", err)
	}
	return cf.Commands, nil
}

// WriteJSON emits the envelope consumed by nsimrun -cmd *.json.
func WriteJSON(w io.Writer, cmds []Command) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(commandFile{Commands: cmds})
}

// Parse reads zero or more command lines. Blank lines and
// lines starting with '#' are skipped. Returns an error naming
// the first offending line; nothing is half-parsed.
func Parse(r io.Reader) ([]Command, error) {
	cmds, _, err := parseCmds(r)
	return cmds, err
}

// parseCmds returns the commands paired with their trimmed
// source lines so callers can echo the text as typed. A run of
// consecutive '#' lines immediately preceding a command is
// attached to it as its Comment (joined with spaces); comment
// authors are encouraged to explain why each operation is
// being taken, and the text is recorded in the operation log.
// A blank line detaches a pending comment block from the next
// command, so a header comment at the top of a file does not
// leak onto the first command.
func parseCmds(r io.Reader) ([]Command, []string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read command file: %w", err)
	}
	type pendingCmd struct {
		cmd Command
		// comments holds the '#' lines immediately above the
		// command and the command's own trailing comment, in
		// source order, so the log reproduces the file.
		comments []string
		line     string
	}
	var pending []pendingCmd
	var block []string
	for lineNum, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "#") {
			block = append(block, strings.TrimSpace(strings.TrimPrefix(line, "#")))
			continue
		}
		if line == "" {
			block = nil
			continue
		}
		fields := strings.Fields(line)
		code := line
		var trailing string
		if i := strings.Index(line, "#"); i >= 0 {
			code = strings.TrimSpace(line[:i])
			trailing = strings.TrimSpace(line[i+1:])
			fields = strings.Fields(code)
		}
		if len(fields) != 3 {
			return nil, nil, fmt.Errorf("line %d: want 3 fields (TimeOffset Part.Label Value), got %d", lineNum+1, len(fields))
		}
		var offset int64
		if _, err := fmt.Sscanf(fields[0], "%d", &offset); err != nil {
			return nil, nil, fmt.Errorf("line %d: invalid TimeOffset %q: %w", lineNum+1, fields[0], err)
		}
		comments := append([]string(nil), block...)
		if trailing != "" {
			comments = append(comments, trailing)
		}
		block = nil
		pending = append(pending, pendingCmd{
			cmd: Command{
				TimeOffsetMs: offset,
				ControlKey:   fields[1],
				Value:        fields[2],
				Comment:      strings.Join(comments, " "),
			},
			comments: comments,
			line:     line,
		})
	}
	// Single-pass consumers (cmd/nsimrun) advance one cursor and
	// stall behind the first not-yet-due command, so a file written
	// out of order would silently delay every later-line command
	// until its own offset arrives. Sorting by offset keeps
	// application time faithful to the timestamps; equal offsets
	// keep file order.
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].cmd.TimeOffsetMs < pending[j].cmd.TimeOffsetMs
	})
	cmds := make([]Command, 0, len(pending))
	lines := make([]string, 0, len(pending))
	for _, p := range pending {
		cmds = append(cmds, p.cmd)
		lines = append(lines, p.line)
	}
	return cmds, lines, nil
}

// CommentsOf returns the individual '#' lines that were attached
// to c by Parse, as they appeared in the source file. Commands
// built programmatically (JSON envelope, scenario script) carry
// their Comment as a single line.
func CommentsOf(c Command) []string {
	if c.Comment == "" {
		return nil
	}
	return []string{c.Comment}
}

// ResolveKey accepts slash or dot part paths and returns the
// key present in m, or raw unchanged if neither matches.
func ResolveKey(m bridge.ControlMap, raw string) string {
	if _, ok := m[raw]; ok {
		return raw
	}
	slashKey := strings.ReplaceAll(raw, ".", "/")
	if _, ok := m[slashKey]; ok {
		return slashKey
	}
	return raw
}

// ErrUnknownControl is returned when a command names a control
// that does not exist in the map.
var ErrUnknownControl = errors.New("unknown control")

// Apply resolves the key and invokes the control's Set.
func Apply(m bridge.ControlMap, c Command) error {
	key := ResolveKey(m, c.ControlKey)
	ctrl, ok := m[key]
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownControl, c.ControlKey)
	}
	if ctrl.Set == nil {
		return fmt.Errorf("control %q has no Set function", c.ControlKey)
	}
	if err := ctrl.Set(c.Value); err != nil {
		return fmt.Errorf("set control %q to %q: %w", c.ControlKey, c.Value, err)
	}
	return nil
}
