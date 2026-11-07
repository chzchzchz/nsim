# nsim

A nuclear power plant simulator for a Westinghouse pressurized-water reactor (PWR).

`nsim` models the full plant — reactor, steam generators, turbine, and all supporting systems — at a 1 ms timestep, with physics and procedures traced back to the Westinghouse PWR documentation in `specs/` (see `AGENTS.md`). It runs headlessly from the command line or interactively in a browser via WebAssembly.

> This is a training / simulation model, not a safety-grade tool.

## What it models

- Reactor vessel, pressurizer, steam generators, reactor coolant pumps, control rods, boron chemistry (CVCS), reactor protection system, safety injection, residual heat removal, containment
- Turbine, condenser, feedwater, moisture separator/reheater, generator, and plant electrical distribution (offsite power, unit auxiliary transformer, ESF buses, emergency diesels)
- Auxiliary systems: component cooling, service water, circulating water, cooling tower, spent fuel pool, containment spray, hydrogen control
- Plant operation modes (cold shutdown → hot standby → power operation), load follow, rod control, and power-rate limits
- ~38 operator training scenarios in `core/scenarios.go` — trips, LOCAs, malfunctions, and normal evolutions, each with a start state, trigger script, and objective

## Quick start

```bash
# Headless: simulate the default plant (full-load operating point) for 5 s, output JSON
go run ./cmd/nsimrun/ -seconds 5 > plant.json

# From a training scenario
go run ./cmd/nsimrun/ -scenario rcp-trip -seconds 60 > result.json

# With time-stamped control commands (TimeOffsetMs  Part.Label  Value)
printf '0  ReactorBuilding.ReactorCoolantPump.Running 1\n1000  ReactorBuilding.ReactorCoolantPump.Running 0\n' > commands.txt
go run ./cmd/nsimrun/ -seconds 2 -cmd commands.txt > result.json

# Inspect state / render the P&ID
go run ./cmd/nsim/                 # dump full plant state
go run ./cmd/nsim/ -svg plant.svg  # render P&ID diagram

# Interactive web UI — live P&ID, controls, alarms, scenario picker
cd web && make dev                 # http://localhost:5173
```

Details: [`cmd/nsimrun/README.md`](cmd/nsimrun/README.md) (flags, command files, JSON I/O), [`cmd/nsim/README.md`](cmd/nsim/README.md), [`web/README.md`](web/README.md).

## Repository layout

```
core/             Simulation model: Plant, buildings, systems, scenarios, SVG P&ID generation
bridge/           Control/sensor registration surface (used by the web UI and cmdfile)
cmdfile/          Time-stamped command files and journaling shared by the CLIs
cmd/nsim/         CLI inspector: dumps state, renders P&ID (DOT/SVG)
cmd/nsimrun/      CLI runner: JSON plant in, JSON plant out
web/              TypeScript + Vite dashboard over the Wasm build
assets/           Operator help text baked into the web UI
scripts/chart.sh  Sample a run over time and plot it with gnuplot
skills/           Agent-facing workflow guides (scenario development, SVG, web UI, verification)
specs/            Source documentation (Westinghouse PWR manual, ops manuals, Wikipedia cache)
plans/            Design notes and scenario catalogs
```

## Using the model as a library

```go
import "github.com/chzchzchz/nsim/core"

p := core.MakePlant()          // design defaults: full-load operating point
for i := 0; i < 1000; i++ {    // advance 1 second (1 ms per step)
    p.Step()
}
controls := p.GetControlMap()  // settable controls (e.g. ReactorBuilding.ReactorCoolantPump.Running)
sensors  := p.GetSensorMap()   // readable process variables
```

Start from a training scenario instead: `core.Scenarios()` returns the catalog; each `Scenario.Apply` layers its starting state onto a fresh plant and `Scenario.Script` schedules its trigger commands.

Units are typed (`core/units.go`) so temperatures, pressures, volumes, and powers are never bare floats; the timestep is `core.TimeStep` (1 ms).

## Development

```bash
go build ./...        # build everything
go test ./...         # Go test suite (scenario replays + unit tests)
cd web && make build  # Wasm + TypeScript typecheck + help-text bundle
cd web && make test   # Vitest front-end unit tests
```

Requirements: Go ≥ 1.27, Node ≥ 18.

Modeling decisions must cite the spec they come from with full paths (e.g. `specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:73-74`); see `AGENTS.md` for the full conventions, including the godoc-by-relative-path rule for this unpublished module.
