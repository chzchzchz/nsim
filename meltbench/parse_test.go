// Package meltbench holds the meltbench/ cmdfile regression tests.
//
// Every meltbench/**/*.txt file is parsed and applied against a fresh
// plant so stale control keys fail loudly instead of silently
// degrading an exercise; the fault-injector header convention is
// enforced here too.
package meltbench

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
)

var faultPathRe = regexp.MustCompile(
	`^(ReactorBuilding|AuxiliaryBuilding|TurbineBuilding|PlantOperation|Outside|Electrical)/Faults/[A-Za-z0-9]+$`)

var faultKeyRe = regexp.MustCompile(
	`(?:ReactorBuilding|AuxiliaryBuilding|TurbineBuilding|PlantOperation|Outside|Electrical)/Faults/[A-Za-z0-9]+`)

var faultsHeaderRe = regexp.MustCompile(`^#\s*Faults injected:\s*(.*)$`)

func meltbenchFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "plantjson" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".txt") {
			out = append(out, path)
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, out, "expected at least one meltbench cmdfile")
	return out
}

func TestCmdfilesParseAndApply(t *testing.T) {
	for _, f := range meltbenchFiles(t) {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f)
			require.NoError(t, err)
			cmds, err := cmdfile.Parse(strings.NewReader(string(data)))
			require.NoErrorf(t, err, "parse %s", f)
			p := core.MakePlant()
			m := p.GetControlMap()
			for _, c := range cmds {
				require.NoErrorf(t, cmdfile.Apply(m, c), "apply %q in %s", c.ControlKey, f)
			}
		})
	}
}

// checkFaultsHeader verifies the '# Faults injected:' convention for
// one cmdfile body:
//   - a file with no /Faults/ command must not carry the marker;
//   - otherwise the marker must be the last comment before the
//     first command and name exactly the set of /Faults/ keys used
//     on command lines (any order).
func checkFaultsHeader(f, content string) error {
	var cmdKeys []string
	var header string
	headerLine, firstCmdLine := -1, -1
	for i, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			if m := faultsHeaderRe.FindStringSubmatch(trimmed); m != nil {
				header = m[1]
				headerLine = i
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		if firstCmdLine == -1 {
			firstCmdLine = i
		}
		cmdKeys = append(cmdKeys, faultKeyRe.FindAllString(line, -1)...)
	}
	cmdKeys = dedupe(cmdKeys)
	if len(cmdKeys) == 0 {
		if header != "" {
			return fmt.Errorf("%s names no /Faults/ command but carries a Faults injected header", f)
		}
		return nil
	}
	if header == "" {
		return fmt.Errorf("%s uses fault keys %v but lacks a '# Faults injected:' line", f, cmdKeys)
	}
	// The marker must be the last comment before the first command.
	var headerKeys []string
	for _, k := range strings.Split(header, ",") {
		if k = strings.TrimSpace(k); k != "" {
			headerKeys = append(headerKeys, k)
		}
	}
	if len(headerKeys) != len(cmdKeys) {
		return fmt.Errorf("%s: '# Faults injected:' names %d keys, want %v", f, len(headerKeys), cmdKeys)
	}
	for _, k := range cmdKeys {
		found := false
		for _, h := range headerKeys {
			if h == k {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: '# Faults injected:' omits fault key %q", f, k)
		}
	}
	// Position: the marker must be the last comment before the
	// first command line.
	lines := strings.Split(content, "\n")
	lastComment := -1
	for i := 0; i < firstCmdLine && i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
			lastComment = i
		}
	}
	if lastComment != headerLine {
		return fmt.Errorf("%s: '# Faults injected:' must be the last comment before the first command", f)
	}
	return nil
}

func TestFaultHeaderConvention(t *testing.T) {
	for _, f := range meltbenchFiles(t) {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f)
			require.NoError(t, err)
			require.NoErrorf(t, checkFaultsHeader(f, string(data)), "fault header convention violated")
		})
	}
}

func TestCheckFaultsHeader(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "no faults, no marker",
			content: "# header\n0 PlantOperation/RodControlAuto 1\n",
		},
		{
			name:    "no faults, stale marker",
			content: "# Faults injected: ReactorBuilding/Faults/PORVStuckOpen\n0 PlantOperation/RodControlAuto 1\n",
			wantErr: "names no /Faults/ command",
		},
		{
			name:    "marker names exactly the command keys",
			content: "# Purpose\n# Faults injected: ReactorBuilding/Faults/PORVStuckOpen\n0 ReactorBuilding/Faults/PORVStuckOpen 1\n0 PlantOperation/RodControlAuto 1\n",
		},
		{
			name:    "marker order need not match file order",
			content: "# Faults injected: ReactorBuilding/Faults/PORVStuckOpen, ReactorBuilding/Faults/HotLegBreak\n0 ReactorBuilding/Faults/HotLegBreak 1\n0 ReactorBuilding/Faults/PORVStuckOpen 1\n",
		},
		{
			name:    "fault command without marker fails",
			content: "# Purpose\n0 ReactorBuilding/Faults/PORVStuckOpen 1\n",
			wantErr: "lacks a '# Faults injected:' line",
		},
		{
			name:    "marker missing a command key fails",
			content: "# Faults injected: ReactorBuilding/Faults/PORVStuckOpen\n0 ReactorBuilding/Faults/PORVStuckOpen 1\n0 ReactorBuilding/Faults/HotLegBreak 1\n",
			wantErr: "names 1 keys, want",
		},
		{
			name:    "marker naming an unused key fails",
			content: "# Faults injected: ReactorBuilding/Faults/PORVStuckOpen, ReactorBuilding/Faults/HotLegBreak\n0 ReactorBuilding/Faults/PORVStuckOpen 1\n",
			wantErr: "names 2 keys, want [ReactorBuilding/Faults/PORVStuckOpen]",
		},
		{
			name:    "marker not the last comment before commands fails",
			content: "# Purpose\n# Faults injected: ReactorBuilding/Faults/PORVStuckOpen\n# trailing comment\n0 ReactorBuilding/Faults/PORVStuckOpen 1\n",
			wantErr: "must be the last comment before the first command",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkFaultsHeader("test.txt", tc.content)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestFaultRegistrationInvariant(t *testing.T) {
	p := core.MakePlant()
	m := p.GetControlMap()
	count := 0
	for key, ctrl := range m {
		if !ctrl.IsFault() {
			continue
		}
		count++
		require.Regexpf(t, faultPathRe, key, "fault key %q must match <System>/Faults/<FaultName>", key)
		require.Equalf(t, key, ctrl.Part+"/"+ctrl.Label,
			"fault control %q: key must equal Part + \"/\" + Label", key)

	}
	require.Positive(t, count, "expected at least one fault control")
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

