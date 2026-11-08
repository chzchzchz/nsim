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
  `TimeOffset Part.Label Value` (relative times; `.json` files load
  the JSON envelope, including `atMs` absolute times). A `scenario`
  field names the evolution the session ran under and is applied
  automatically when `-scenario` is absent (a conflicting explicit
  flag is an error). See [Command file format](#command-file-format):
  `#` comments attach to the command below and log with it; agents
  should comment every line.
- `-stepsPerSecond` `1000` / `-timestep` `1ms` — Wall pacing and
  per-step simulated duration; cadence is in `/status`. Hot-change
  knobs are in [ENDPOINTS.md](ENDPOINTS.md).
- `-paused` `false` — Start paused. By default the server starts
  stepping immediately.
- `-webdir <dir>` — Optional: serve the built `web/` UI statically
  at `/`. API routes take precedence; no SVG is rendered server-side.
  P&ID rendering is client-side from sensor data.
- `-saveOnShutdown <file>` — Optional: write the full `core.Plant`
  JSON to `<file>` on `SIGINT`/`SIGTERM`. The write is atomic
  (temporary file in the same directory, renamed over the target);
  a failed write is logged to stderr and exits 1.

## Operating rules for agents/clients

**Comment every operation you submit** (see
[Command file format](#command-file-format) and
[Operation log](#operation-log-logs)): every `POST /commands` command,
every `PUT /controls` write, and every `-cmd` file line should carry a
comment explaining why the action is being taken. Bare annotations go
through `POST /logs`. The log is the audit trail of the run: an
uncommented operation reads as an unexplained one, and a comment-only
post is a message rather than an action.

## Command file format

The `cmdfile` format shared by `-cmd`, `POST /commands`, and
`GET /commands/export` (see `cmdfile/cmdfile.go`):

```text
# Scenario: RCP trip drill. Stop pump, then raise setpoint 10 s later.
0 ReactorBuilding.ReactorCoolantPump.Running 0  # stop the pump
10000 PlantOperation.PowerSetpointTarget 90  # raise setpoint 10 s after submission
```

One command per line: `TimeOffset Part.Label Value`. Blank lines are
skipped. Commands are sorted by scheduled time before scheduling, so
file order only matters for equal times. Both dot (`Part.Label`) and
slash (`Part/Label`) key spellings resolve to the same control.

A run of `#` lines immediately above a command and an optional
trailing `# ...` on the command line itself are attached to that
command and recorded with it in the operation log (`GET /logs`),
which is where the run's rationale is read back from. A blank line
detaches a comment block from the next command, so a file-wide
header comment stays a header instead of leaking onto the first
operation.

### Times: relative and absolute

`TimeOffset` (`offsetMs`) is milliseconds after submission (or after
run start for `-cmd`), relative by default. At sim time 5000, offset
10000 fires at 15000. An `atMs` field sets an absolute simulated
time instead; when both are present, `atMs` wins, and times in the
past clamp to now. Text lines cannot express `atMs`, so absolute
scheduling needs the JSON envelope:

```json
{
  "scenario": "rcp-trip",
  "commands": [
    {"offsetMs": 0, "control": "ReactorBuilding/ReactorCoolantPump/Running", "value": "0", "comment": "stop the pump"},
    {"offsetMs": 10000, "control": "PlantOperation/PowerSetpointTarget", "value": "90", "comment": "raise setpoint at 10 s"}
  ]
}
```

**Which to use:** relative when the action is "N seconds from now"
(decisions you make while watching), absolute when the action is
pinned to a sim-time milestone (a scenario script's fixed timeline,
a coordinated multi-agent step). Relative is robust to the submission
moment moving; absolute pins the milestone. The JSON form is what
`GET /commands/export` emits and `POST /commands` accepts (sniffed by
a leading `{` when no `Content-Type` is set, so `curl -d @file.json`
works); `scenario` is optional on upload. `GET /commands` reports the
resolved absolute instant as `dueMs`, `/logs` logs it as `atMs`.

## Reading controls and sensors with curl

The catalogs are the source of truth for keys and values. Keys are
slash-form in responses; encode `/` as `%2F` in paths (dot-form is
also accepted by the server, e.g.
`/controls/PlantOperation.PowerSetpointTarget`). Control values are
strings validated by kind: `bool` accepts `1`/`true`/`0`/`false`,
`percent` is `[0,100]`, `normalized` is `[0,1]`, `enum` is one of the
`enum` array, and `rpm`/`psig`/`psia`/`fahrenheit`/`gallon`/`watt`/
`ppm`/`volt`/`hz`/`gpm`/`mw`/`pctpersec` are non-negative numbers. A
bad value is a 400 with the uniform envelope
`{"error":"<message>","code":<http status>}`; wrong method is 405,
unknown path or key is 404.

```bash
# What operations exist? Every control with its live value.
curl -s localhost:8080/controls | jq -r '.[].key'
# -> PlantOperation/PowerSetpointTarget
# -> ReactorBuilding/ReactorCoolantPump/Running
# -> ...

# Read one control.
curl -s localhost:8080/controls/PlantOperation%2FPowerSetpointTarget | jq
# {
#   "key": "PlantOperation/PowerSetpointTarget",
#   "part": "PlantOperation",
#   "label": "PowerSetpointTarget",
#   "kind": "percent",
#   "value": "0"
# }

# What can be read? Every sensor with its live value.
curl -s localhost:8080/sensors | jq -r '.[].key'
# -> ReactorBuilding/ReactorPressureVessel/Pressure
# -> ...

# Read one sensor (value is always a string; kind gives the unit).
curl -s localhost:8080/sensors/ReactorBuilding%2FReactorPressureVessel%2FPressure | jq
# {
#   "key": "ReactorBuilding/ReactorPressureVessel/Pressure",
#   "part": "ReactorBuilding/ReactorPressureVessel",
#   "label": "Pressure",
#   "kind": "psia",
#   "value": "2500",
#   "simTimeMs": 0
# }

# Poll one reading in a loop.
watch -n 1 'curl -s localhost:8080/sensors/ReactorBuilding%2FReactorPressureVessel%2FPressure | jq -r .value'
```

## Operation log (`/logs`)

`GET /logs` is the append-only, timestamped audit trail of a run:
every cmdfile operation submitted (via `-cmd`, `POST /commands`, or
scenario script) is recorded with the simulated time it takes effect,
its control and value, and the comment the author attached. Bare
comments posted with `POST /logs {"comment":"..."}` interleave with
operations in the same timestamp order, so the log reads as a logbook.
The log never deduplicates: re-submitting an identical command is
journaled once for replay but logged every time it is submitted.

**The log is a comms channel.** Staff and agents use plain comments
(no control) to coordinate: `POST /logs {"comment":"..."}` sends an
announcement or observation at the current sim time, and `GET /logs`
is where recipients read it. Agents should check `/logs`
periodically — after stepping, before issuing a control action, and
whenever a sensor value changes unexpectedly — and post short status
notes of their own so the log shows a conversation, not just a
record.

```bash
# Schedule two operations with rationale; the comments ride along.
# The first command's comment is the '#' line above it plus its
# trailing '# stop the pump'; both are joined into the log entry.
curl -XPOST localhost:8080/commands -H 'Content-Type: text/plain' -d \
'# RCP trip drill: stop the pump, raise setpoint 10 s later.
0 ReactorBuilding.ReactorCoolantPump.Running 0  # stop the pump
10000 PlantOperation.PowerSetpointTarget 90  # raise setpoint 10 s after submission'

# Annotate the current sim time with a bare comment. This is
# NOT a cmdfile line: there is no TimeOffset, control, or value.
# It is just a comment posted to the log endpoint.
curl -XPOST localhost:8080/logs \
  -H 'Content-Type: application/json' \
  -d '{"comment":"pump stopped, watching pressure"}'

# Read the log back in timestamp order. Offsets are from
# submission, so atMs = submission sim time + offsetMs; here
# the submission happened at sim time 0, so the second
# operation lands at atMs 10000.
curl -s localhost:8080/logs | jq
# [
#   {"seq":0,"kind":"operation","atMs":0,
#    "control":"ReactorBuilding/ReactorCoolantPump/Running",
#    "value":"0",
#    "comment":"RCP trip drill: stop the pump, raise setpoint 10 s later. stop the pump"},
#   {"seq":1,"kind":"operation","atMs":10000,
#    "control":"PlantOperation/PowerSetpointTarget",
#    "value":"90","comment":"raise setpoint 10 s after submission"},
#   {"seq":2,"kind":"comment","atMs":0,
#    "comment":"pump stopped, watching pressure"}
# ]
```

A comment-only entry (no `control`/`value`, `kind:"comment"`) is
what `POST /logs` produces — that is the shape you get from `/logs`
for a plain announcement:

```json
{"seq": 2, "kind": "comment", "atMs": 0, "comment": "pump stopped, watching pressure"}
```

Scenario-scripted operations carry `"source":"script"` so an operator
can tell a scenario's opening commands from the ones they issued
themselves; startup `-cmd` entries and `POST /commands` entries have
no `source`.

Timing in `/logs`: `atMs` is always the absolute sim time the
operation takes effect. A command posted at sim time `T` with
`offsetMs:5000` is logged at `T+5s`; one with `atMs:15000` is logged
at 15000. Scheduled status (`pending`/`done`/`failed`) is in
`GET /commands`, not in `/logs`. (The log is cleared on a server
restart; see [ENDPOINTS.md](ENDPOINTS.md).)

## Operating loop

1. **Observe** — read `/status` (clock, paused/running, `runId`),
   `/sensors` (what the plant is doing), and `/logs` (what others
   have done or announced), before issuing a control action.
2. **Decide** the operation from the catalogs (`GET /controls`,
   `GET /sensors`) and the specs; every write carries a comment.
3. **Act** — immediate `PUT /controls/{key}`, or scheduled
   `POST /commands` (relative `offsetMs` or absolute `atMs`).
4. **Verify** — re-read the sensor/control and `/commands` `status`;
   a failed write surfaces as `status:"failed"` with `err` in
   `GET /commands`, not as a plant change.
5. **Announce** — post a short `POST /logs` note when you act,
   and check `/logs` before your next action so the log stays a
   conversation.

## Endpoints

Every non-2xx response returns the uniform envelope
`{"error":"<message>","code":<http status>}`. `OPTIONS` on any route
answers preflight `204` with CORS headers.

| Method & path | Response |
|---|---|
| `GET /livez` | `{"ok":true}` — process is up. |
| `GET /readyz` | `{"ready":true}` while stepping. |
| `GET /status` | Identity + clock: `{siteName, runId, simTimeMs, timestepMs, stepsPerSecond, running, paused, rev, objective:{active,id,title,done,metMs,detail}}`. |
| `GET /metrics` | Prometheus text: `nsim_server_info{site="..."}`, one gauge per numeric sensor, `nsim_sim_time_seconds`, `nsim_alarms_active`, `nsim_steps_total`. Enum sensors are omitted (Prometheus has no string value type); read them via `/sensors/{key}`. |
| `GET /sensors` | Bulk sensor catalog with live values (one poll per frame for a remote UI). |
| `GET /sensors/{key}` | One sensor. Keys are `%`-encoded (`%2F`) when they contain `/`. |
| `GET /controls` | Control-operations catalog: sorted array of `{key, part, label, kind, enum, value}` — every control the server accepts, with live value. This is the "what operations exist?" endpoint. |
| `GET /controls/{key}` | One control; 404 if unknown. |
| `PUT /controls/{key}` `{"value":"...", "comment":"..."}` | Apply one control immediately and journal it at the current sim time. `comment` is optional but **encouraged for every write**; it is recorded in `/logs`. 404 unknown key, 400 bad value. |
| `POST /commands` | Accept a control-operations list: JSON envelope `{"scenario":"...","commands":[{"offsetMs":...,"atMs":...,"control":"...","value":"...","comment":"..."}]}` **or** `text/plain` lines `TimeOffset Part.Label Value` with `#` comments. `offsetMs` is relative to submission; `atMs` is absolute and wins. **Comment every command**; comments are logged with the operation. Scheduling is all-or-nothing; re-POSTing an already-journaled `(control, value, dueMs)` identity returns `{"deduped":true}`. |
| `GET /commands` | Journal + queue view: `{rev, entries:[{seq, text, control, value, offsetMs, dueMs, status, err}]}`. `dueMs` is the absolute sim time the command fires. |
| `GET /commands/export` | Journaled operations as the `nsimrun -cmd *.json` envelope. Round-trip: export and replay it through `nsimrun`. |
| `GET /logs`, `POST /logs` | Append-only timestamped log of cmdfile operations (with their attached comments) plus bare comments. `GET` returns `[{seq, kind, atMs, control, value, comment, source}]` in timestamp order; `POST {"comment":"..."}` appends a comment at the current sim time. Never deduplicated: re-submitting an identical command appends a second entry. |
| `GET /alarms` | `Plant.Alarms()` (same shape as the extra `Alarms` key of `nsimrun` stdout). |
| `GET /state` | Full plant JSON — byte-compatible with `nsimrun` stdout and `nsimrun -plant` input. The single state endpoint; there is no `/snapshot` and no `POST /state`. |
| `GET /scenarios` | `core.Scenarios` catalog list. |
| `GET /` (with `-webdir`) | Static files from the web build; API routes take precedence. |

Clock and pacing knobs are in
[ENDPOINTS.md](ENDPOINTS.md).

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
     -H 'Content-Type: application/json' \
     -d '{"value":"90","comment":"raise setpoint"}'
```

Schedule operations and inspect the queue:

```bash
curl -XPOST localhost:8080/commands -H 'Content-Type: text/plain' \
     -d '# RCP trip drill: stop the pump
0 ReactorBuilding.ReactorCoolantPump.Running 0  # stop the pump'
curl localhost:8080/commands
```

Export the session and replay it with the batch runner, or read
operator state and chain it into `nsimrun`:

```bash
curl localhost:8080/commands/export > session.json
go run ./cmd/nsimrun/ -cmd session.json -seconds 60
curl localhost:8080/state > plant.json
go run ./cmd/nsimrun/ -plant plant.json -seconds 60
```

Scrape metrics and check identity:

```bash
curl localhost:8080/metrics | grep ReactorPressureVessel
curl localhost:8080/status | jq '{siteName, simTimeMs, timestepMs, stepsPerSecond}'
```

Save plant state on Ctrl-C:

```bash
go run ./cmd/nsimserve/ -saveOnShutdown plant.json
```
