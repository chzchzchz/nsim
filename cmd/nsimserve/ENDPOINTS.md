# nsimserve endpoints

Server-side reference for `cmd/nsimserve` lifecycle controls that
live outside the plant-facing surface (`README.md` covers reading
controls/sensors, scheduling `POST /commands` with comments, and the
`/logs` audit trail).

Every non-2xx response returns the uniform envelope
`{"error":"<message>","code":<http status>}`. `OPTIONS` on any route
answers preflight `204` with CORS headers.

Base: `http://127.0.0.1:8080` (or `-addr`).

## Lifecycle

| Method & path | Body / response |
|---|---|
| `GET /sim/scenarios` | `core.Scenarios` catalog list. |
| `POST /sim/pause`, `POST /sim/resume` | Stop/start the loop; `{"paused":bool}`. |
| `POST /sim/shutdown` | Request clean process shutdown; `{"shuttingDown":true}`. Shares the signal-shutdown path and writes `-resume`/`-saveOnShutdown` files when configured. |
| `POST /sim/step` | Paused-mode only (409 while running). Bodies are mutually exclusive: `{"ms":N}` advances N ms quantized to `-timestep`; `{"steps":N}` advances N steps (≤1e6); `{}` advances one step. Returns `{"steps":N}`. |
| `POST /sim/reset` `{"scenario":"...","plant":"..."}` | Rebuild the plant via `internal/plantinit`; empty fields reuse the startup lineup (`-scenario`/`-plant` IDs, not file contents). Clears clock, queue, journal (dedup), and log; reschedules startup `-cmd` commands; bumps `runId`. `GET /readyz` is 503 during the rebuild. |
| `POST /sim/timestep {"dtMs":10}` | Hot-change step duration; returns `{"timestepMs":10}`. |
| `POST /sim/speed {"stepsPerSecond":100}` | Hot-change wall pacing (≤1e6); returns `{"stepsPerSecond":100}`. |
| `-paused` (flag) | Start paused; `POST /sim/step` only advances while paused (409 otherwise). |
| `-resume <file>` (flag) | Load complete server state at startup and atomically rewrite it on clean shutdown; startup `-cmd` work is not rescheduled because the saved queue/journal/log carry it. |

## Fault injectors

Fault controls (`<System>/Faults/<FaultName>`, e.g.
`ReactorBuilding/Faults/HotLegBreak`) are deliberate malfunction
latches, not operator actions. They are excluded from `GET /controls`
and from `GET /controls/{key...}` (404), so the operator surface never
carries them.

| Method & path | Body / response |
|---|---|
| `GET /sim/faults` | Same wire shape as `/controls` entries, restricted to fault controls and with `"fault":true` on each. Read-only; always registered. Dot-spelling is not accepted on this route; use the canonical slash form. |
| `PUT /controls/{key}` with a fault key | `403 {"error":"fault control <resolved-key> requires -allowFaultCommands","code":403}` in default mode; nothing applied, journaled, or logged. Resolution runs before the gate, so `ReactorBuilding.Faults.HotLegBreak` is rejected the same as the slash form. |
| `POST /commands` with any fault key in the batch | `403` in default mode; the whole batch is rejected atomically - nothing is scheduled, journaled, or logged. |

- `-allowFaultCommands` (flag, default off): when set, fault commands
  flow through apply/queue/journal/log exactly like any other command.
  With the flag on, fault commands are journaled/logged normally.
- `-cmd` startup files and scenario scripting apply in-process via
  `ApplyScript` and are not gated; the gate guards only the network
  surface.
- `-resume` snapshots written before a fault key was renamed silently
  drop the unknown key (`ApplyScript` degrades without stopping);
  re-record saved lineups after such a rename.

## Clock and pacing

- `-timestep 1ms` at `-stepsPerSecond 1000` is real-time; smaller
  timesteps with high speed move the plant faster than wall clock.
- `/status` reports the live `simTimeMs`, `timestepMs`,
  `stepsPerSecond`, `running`/`paused`, `rev`, `latestSeq`, and `runId`
  (bumps on `/sim/reset`). `GET /readyz` is `{"ready":true}` while
  stepping and 503 with `{}` while `/sim/reset` rebuilds.
- `POST /commands` offsets are relative to the submission sim time,
  so log timestamps (`atMs`) are `submission + offset`; scheduled
  status (`pending`/`done`/`failed`) is in `GET /commands`.
- `GET /logs` accepts an optional half-open sequence range `lo`/`hi`:
  entries satisfy `seq >= lo` and `seq < hi`; absent bounds are
  unbounded.
