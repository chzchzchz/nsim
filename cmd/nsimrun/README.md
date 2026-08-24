# nsimrun

Command-line runner for the `nsim` nuclear power plant simulator.

## Usage

```bash
go run ./cmd/nsimrun/ [flags]
```

Flags:

- `-plant <file.json>` — Plant state JSON file. Optional; if omitted the plant is initialized with design defaults via `MakePlant`.
- `-seconds <number>` — Number of seconds to simulate. Plain number (e.g. `5`, `1.5`), not a duration string like `1s`.
- `-cmd <file>` — Command file. Each line is `TimeOffset Part.Label Value`. The control is set once elapsed time reaches the offset (in milliseconds). Supports dot or slash paths. Blank lines and lines starting with `#` are ignored. Logs errors and continues on parse/set failures. Files ending in `.json` (case insensitive) are loaded as web-console exports: `{"commands":[{"offsetMs":...,"control":...,"value":...}]}`, with offsets measured from simulation start.

Command-file notes:

- Only registered controls can be set. Each command resolves against the plant's control map (`Plant.GetControlMap()`); state fields with no control entry (sensor-only values) cannot be commanded. Unknown controls log an error and simulation continues.
- Values must match the control's kind: bool controls accept `1`/`0`/`true`/`false`, normalized controls accept fractions in `0..1`. Enum-style momentary controls (e.g. `Reset`) take their enum word as the value.
- Commands whose offset exceeds the simulated duration are still applied after the final step, so the exported state reflects them.

## Behavior

1. Initializes a plant from `-plant` if provided, otherwise from built-in defaults.
2. Merges the JSON over the defaults per top-level key: a top-level object present in the file (e.g. `TurbineBuilding`) replaces that entire default subtree, so fields absent from the file become zero values rather than design defaults. Top-level buildings not named in the file keep their defaults.
3. If `-cmd` is provided, reads time-stamped control commands and applies them as the simulation advances.
4. Steps the simulation forward by the requested duration (1 ms per internal step).
5. Writes the final plant state as JSON to stdout.

## Examples

Run the default plant for 1.5 seconds:

```bash
go run ./cmd/nsimrun/ -seconds 1.5
```

Override one building's fields (the whole `TurbineBuilding` subtree comes
from this file; other buildings keep their defaults):

```bash
cat > my_plant.json <<'EOF'
{
  "TurbineBuilding": {
    "CondensatePump": {
      "Running": true
    }
  }
}
EOF

go run ./cmd/nsimrun/ -plant my_plant.json -seconds 5 > result.json
```

Apply time-stamped control commands during the run:

```bash
cat > commands.txt <<'EOF'
0 ReactorBuilding.ReactorCoolantPump.Running 1
1000 ReactorBuilding.ReactorCoolantPump.Running 0
EOF

go run ./cmd/nsimrun/ -seconds 2 -cmd commands.txt > result.json
```

## Output

The stdout JSON uses the same schema as the internal `core.Plant` struct. Use `core.MakePlant()` or the `plant_json_test.go` reference to inspect the full set of fields.

One extra top-level key, `Alarms`, carries the active alarm list at end of run (same shape as `Plant.Alarms()`); `-plant` re-ingest ignores it.