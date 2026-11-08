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

	"github.com/chzchzchz/nsim/bridge"
)

// Command is one time-stamped control action, identical to a
// line in an nsimrun -cmd file (see cmd/nsimrun/README.md).
// The JSON tags define the web console export format consumed
// by `nsimrun -cmd *.json`.
type Command struct {
	TimeOffsetMs int64  `json:"offsetMs"`
	ControlKey   string `json:"control"`
	Value        string `json:"value"`
}

// commandFile is the JSON envelope shared by the web UI
// command-log export and the nsimrun -cmd *.json loader.
type commandFile struct {
	Commands []Command `json:"commands"`
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
// source lines so callers can echo the text as typed.
func parseCmds(r io.Reader) ([]Command, []string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read command file: %w", err)
	}
	var cmds []Command
	var lines []string
	for lineNum, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, nil, fmt.Errorf("line %d: want 3 fields (TimeOffset Part.Label Value), got %d", lineNum+1, len(fields))
		}
		var offset int64
		if _, err := fmt.Sscanf(fields[0], "%d", &offset); err != nil {
			return nil, nil, fmt.Errorf("line %d: invalid TimeOffset %q: %w", lineNum+1, fields[0], err)
		}
		cmds = append(cmds, Command{
			TimeOffsetMs: offset,
			ControlKey:   fields[1],
			Value:        fields[2],
		})
		lines = append(lines, line)
	}
	// Single-pass consumers (cmd/nsimrun) advance one cursor and
	// stall behind the first not-yet-due command, so a file written
	// out of order would silently delay every later-line command
	// until its own offset arrives. Sorting by offset keeps
	// application time faithful to the timestamps; equal offsets
	// keep file order.
	sort.SliceStable(cmds, func(i, j int) bool {
		return cmds[i].TimeOffsetMs < cmds[j].TimeOffsetMs
	})
	return cmds, lines, nil
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
