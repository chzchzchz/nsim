# nsimserve

Long-lived HTTP server for the `nsim` nuclear power plant simulator.
It runs the simulator continuously and exposes Prometheus-style metrics
plus control endpoints. `cmd/nsimserve` is a new binary; it is not an
extension of the batch CLI `cmd/nsimrun` or the inspector `cmd/nsim`.

## Usage

```bash
go run ./cmd/nsimserve/ [flags]
```

Flags:

- `-addr` `127.0.0.1:8080` — Listen address. Loopback default; the
  control endpoints mutate plant state and there is no auth in this
  slice. Binding a non-loopback address exposes unauthenticated
  control. CORS is intentionally permissive
  (`Access-Control-Allow-Origin: *`) because the future remote `web/`
  UI is a browser client that must reach the server from any origin.
- `-name <site>` — Site Name shown in `/status.siteName` and as the
  `nsim_server_info{site="..."}` Prometheus series. It is server
  identity only and never enters the plant state. Default
  `nsimserve`.
- `-plant <file.json>` — Plant state JSON file. Same semantics as
  `nsimrun -plant` (merged over design defaults). Mutually exclusive
  with `-scenario`.
- `-scenario <id>` — Starting scenario id from the catalog
  (`core.Scenarios`; see `plans/x/scenarios.txt`, e.g. `rcp-trip`,
  `dilution-to-criticality`). Applies the scenario's minimal
  starting-state override and its opening script; its objective is
  reported in `/status.objective`.
- `-cmd <file>` — Startup command file. Each line is
  `TimeOffset Part.Label Value`; the control is set once elapsed
  simulated time reaches the offset (in milliseconds). Files ending
  in `.json` are loaded as web-console exports. A `scenario` field
  names the evolution the session ran under and is applied
  automatically when `-scenario` is absent (a conflicting explicit
  flag is an error).
- `-stepsPerSecond` `1000` — Steps executed per wall-clock second
  (wall pacing). This is the same unit the WASM UI exposes as
  "Steps/s"; at `-timestep 1ms` this is real-time. Hot-change with
  `POST /speed` (clamped to 1e6).
- `-timestep` `1ms` — Per-step simulated duration (`time.Duration`).
  Hot-change with `POST /timestep {"dtMs":10}`; the live cadence is
  reported as `timestepMs` in `/status`.
- `-paused` `false` — Start paused. By default the server starts
  stepping immediately. `POST /step` only advances while paused
  (409 otherwise).
- `-webdir <dir>` — Optional: serve the built `web/` UI statically
  at `/`. API routes take precedence; no SVG is rendered server-side.
  P&ID rendering is client-side from sensor data.
- `-saveOnShutdown <file>` — Optional: write the full `core.Plant`
  JSON to `<file>` on `SIGINT`/`SIGTERM`. The write is atomic
  (temporary file in the same directory, renamed over the target);
  a failed write is logged to stderr and exits 1.

## Endpoints

| Method & path | Response |
|---|---|
| `GET /livez` | `{"ok":true}` — process is up. |
| `GET /readyz` | `{"ready":true}` while stepping; 503 with `{}` while `/reset` rebuilds. |
| `GET /status` | Identity + clock: `{siteName, runId, simTimeMs, timestepMs, stepsPerSecond, running, paused, rev, objective:{active,id,title,done,metMs,detail}}`. `runId` bumps on `/reset`. |
| `GET /metrics` | Prometheus text: `nsim_server_info{site="..."}`, one gauge per numeric sensor, `nsim_sim_time_seconds`, `nsim_alarms_active`, `nsim_steps_total`. Enum sensors are omitted (Prometheus has no string value type); read them via `/sensors/{key}`. |
| `GET /sensors` | Bulk sensor catalog with live values (one poll per frame for a remote UI). |
| `GET /sensors/{key}` | One sensor. Keys are `%`-encoded (`%2F`) when they contain `/`. |
| `GET /controls` | Control-operations catalog: sorted array of `{key, part, label, kind, enum, value}` — every control the server accepts, with live value. This is the "what operations exist?" endpoint. |
| `GET /controls/{key}` | One control; 404 if unknown. |
| `PUT /controls/{key}` `{"value":"..."}` | Apply one control immediately and journal it at the current sim time. 404 unknown key, 400 bad value. |
| `POST /commands` | Accept a control-operations list: JSON envelope `{"scenario":"...","commands":[{"offsetMs":...,"control":"...","value":"..."}]}` **or** `text/plain` lines `TimeOffset Part.Label Value`. Scheduling is all-or-nothing; re-POSTing an already-journaled `(control, value, dueMs)` identity returns `{"deduped":true}`. |
| `GET /commands` | Journal + queue view: `{rev, entries:[{seq, text, control, value, offsetMs, dueMs, status, err}]}`. `rev` lets paused clients skip re-render when nothing changed. |
| `GET /commands/export` | Journaled operations as the `nsimrun -cmd *.json` envelope. Round-trip: export and replay it through `nsimrun`. |
| `POST /pause`, `POST /resume` | Loop control. |
| `POST /step` `{"ms":100}` | Advance N ms (quantized to `timestep`); 409 while running. |
| `POST /reset` `{"scenario":"..."}` | Rebuild the plant via `internal/plantinit`, clear the queue, journal, and clock; bump `runId`. |
| `POST /timestep` `{"dtMs":10}` | Hot-change step duration; returns the live `timestepMs`. |
| `POST /speed` `{"stepsPerSecond":100}` | Hot-change wall pacing; returns the live `stepsPerSecond`. |
| `GET /alarms` | `Plant.Alarms()` (same shape as the extra `Alarms` key of `nsimrun` stdout). |
| `GET /state` | Full plant JSON — byte-compatible with `nsimrun` stdout and `nsimrun -plant` input. The single state endpoint; there is no `/snapshot` and no `POST /state`. |
| `GET /scenarios` | `core.Scenarios` catalog list. |
| `GET /` (with `-webdir`) | Static files from the web build; API routes take precedence. |

Every non-2xx response returns the uniform envelope
`{"error":"<message>","code":<http status>}`. `OPTIONS` on any route
answers preflight `204` with CORS headers.

Keys contain `/`, so control and sensor routes capture a wildcard
(`{key...}` in `ServeMux` syntax). Clients percent-encode slashes as
`%2F`, e.g. `GET /controls/ReactorBuilding%2FReactorPressureVessel%2FPressure`;
the server decodes `%2F` and hands the key to `cmdfile.ResolveKey`,
which accepts both dot and slash forms.

## Examples

Start a server for a scenario:

```bash
go run ./cmd/nsimserve/ -scenario rcp-trip
```

List every operation, read one control, and set it:

```bash
curl localhost:8080/controls | jq -r '.[].key'
curl localhost:8080/controls/PlantOperation%2FPowerSetpointTarget
curl -XPUT localhost:8080/controls/PlantOperation%2FPowerSetpointTarget \
     -H 'Content-Type: application/json' -d '{"value":"90"}'
```

Schedule operations and inspect the queue:

```bash
curl -XPOST localhost:8080/commands -H 'Content-Type: text/plain' \
     -d '0 ReactorBuilding.ReactorCoolantPump.Running 0'
curl localhost:8080/commands
```

Export the session and replay it with the batch runner:

```bash
curl localhost:8080/commands/export > session.json
go run ./cmd/nsimrun/ -cmd session.json -seconds 60
```

Read operator state and chain it into `nsimrun`:

```bash
curl localhost:8080/state > plant.json
go run ./cmd/nsimrun/ -plant plant.json -seconds 60
```

Scrape metrics and check identity:

```bash
curl localhost:8080/metrics | grep ReactorPressureVessel
curl localhost:8080/status | jq '{siteName, simTimeMs, timestepMs, stepsPerSecond}'
```

Fast-forward debugging without restarting:

```bash
curl -XPOST localhost:8080/timestep -H 'Content-Type: application/json' \
     -d '{"dtMs":10}'
curl -XPOST localhost:8080/speed -H 'Content-Type: application/json' \
     -d '{"stepsPerSecond":100}'
```

Save plant state on Ctrl-C:

```bash
go run ./cmd/nsimserve/ -saveOnShutdown plant.json
```

## Implementation notes

- The loop goroutine is the sole plant writer. Handlers take an
  `sync.RWMutex` read lock and the loop takes the write lock per step
  batch; this mirrors the WASM handle's step-dispatch pattern.
- Startup and `/reset` plant construction, plus `-cmd` loading, share
  `internal/plantinit` with `cmd/nsimrun`.
- Control-operations format is canonical (`cmdfile.Command`):
  dot-or-slash resolution via `cmdfile.ResolveKey`, and value forms
  per `bridge.ConvertControlValue`. Momentary one-shot enum controls
  (e.g. `Alarms/AcknowledgeAll`) fire once per `PUT` and must not
  latch.
- P&ID rendering is client-side: the web assets carry a static SVG
  and derive pump dots, valve wedges, flow dashes, level gauges,
  alarm halos, washes, and readouts from `/sensors` values. No
  server `/svg` endpoint exists.
