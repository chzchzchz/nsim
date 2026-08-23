# nsimrun

Command-line runner for the `nsim` nuclear power plant simulator.

## Usage

```bash
go run ./cmd/nsimrun/ [flags]
```

Flags:

- `-plant <file.json>` — Plant state JSON file. Optional; if omitted the plant is initialized with design defaults via `MakePlant`.
- `-seconds <number>` — Number of seconds to simulate. Plain number (e.g. `5`, `1.5`), not a duration string like `1s`.
- `-cmd <file>` — Command file. Each line is `TimeOffset Part.Label Value`. The control is set once elapsed time reaches the offset (in milliseconds). Supports dot or slash paths. Logs errors and continues on parse/set failures.

## Behavior

1. Initializes a plant from `-plant` if provided, otherwise from built-in defaults.
2. Partial JSON files are merged over the defaults: only fields present in the JSON are overridden.
3. If `-cmd` is provided, reads time-stamped control commands and applies them as the simulation advances.
4. Steps the simulation forward by the requested duration (1 ms per internal step).
5. Writes the final plant state as JSON to stdout.

## Examples

Run the default plant for 1.5 seconds:

```bash
go run ./cmd/nsimrun/ -seconds 1.5
```

Override a few fields and run:

```bash
cat > my_plant.json <<'EOF'
{
  "Trip": false,
  "ReactorCoolantPump": {
    "Running": true
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