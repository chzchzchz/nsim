# nsim-web

Browser-based P&ID dashboard for the nsim PWR simulator. Compiles `core` + `bridge` to WebAssembly, runs the simulation in a `requestAnimationFrame` loop, and renders the live plant SVG with controls, sensors, and alarms.

## Prerequisites

* Go ≥ 1.27 (wasm target: `GOOS=js GOARCH=wasm`)
* Node.js ≥ 18, npm ≥ 9
* A modern browser with `WebAssembly.instantiateStreaming`

## Run

```sh
cd web
make dev   # http://localhost:5173
```

Vite serves `index.html`, the wasm build from `public/`, and TypeScript from `src/`. For a plain static server instead: `make build`, then serve the directory (e.g. `npx serve .`). Only requirement: `nsim.wasm` reachable at the deploy base URL (Vite `base: "./"`); the Go runtime polyfill ships inside the JS bundle.

## Build / test

```sh
make build         # wasm + tsc + descriptions.json
make test          # vitest unit tests
make typecheck     # tsc --noEmit
make clean
```

## Deploy

```sh
make bundle        # web.tgz — minified JS, stripped wasm (release)
make bundle-debug  # web-debug.tgz — readable JS + sourcemaps, unstripped wasm
```

Both extract to `nsim-web/` with relative asset URLs, ready for any static host:

```sh
mkdir /srv/www && tar xzf web.tgz -C /srv/www
# serve /srv/www/nsim-web/
```

## How it works

`web/wasm/main.go` is the `syscall/js` entrypoint, built only under `GOOS=js GOARCH=wasm` (`//go:build js`). It lives in the root Go module — there is no second `go.mod` or `replace` directive — so it reuses `core`, `bridge`, and `core/svg` directly. `make build` runs `GOOS=js GOARCH=wasm go build -o public/nsim.wasm ./wasm`, copies `wasm_exec.js` from GOROOT for bundling, and type-checks the TypeScript (`src/wasm.ts` loads the wasm with the Vite base URL).

## UI

* **Control bar** — Start/Pause, Step, Reset, Frames/s, Steps/s (actual vs. target), copy state JSON, status.
* **P&ID SVG** — live plant diagram; parts bind by name (`data-part` matches `bridge.Control.Part` / `bridge.Sensor.Part`).
* **Alarm panel** — acknowledge single / all.
* **Control & sensor tiles** — every entry from `bridge.GetControlMap` / `GetSensorMap`, grouped by building; drag headings to reorder, `−`/`+` to collapse; order persists in `localStorage`.
* **Command console** — `TimeOffsetMs Part.Label Value` lines; export the journal as JSON for headless replay with `nsimrun -cmd`.
* **Scenario banner** — active evolution, objective, unmet bounds; latches green when complete.
* **Faults toggle** — hidden by default; reveals the fault injectors (`<System>/Faults/*`, simulator malfunction latches, not operator actions). One header checkbox drives both the local WASM console and the remote console; the choice persists in `localStorage`. Not a security boundary — remote fault writes still require `-allowFaultCommands` on the server.

## Scenarios

The 🧭 Scenario picker lists the operator evolutions from `core/scenarios.go` (~38), grouped by section. Choosing one rebuilds the plant with that start state through the same reset path as Reset; Reset re-applies it, Free play uses the default plant. Malfunctions script their trigger ~30 s in (`Scenario.Script`), and console exports embed the scenario id so `nsimrun -cmd session.json` reproduces the run.

## Remote mode (nsimserve)

The UI can attach to a running `nsimserve` (`cmd/nsimserve`) instead of the local WASM plant. Click **🌐 Server**, enter a URL (default `http://127.0.0.1:8080`), and the header probes `GET {base}/livez` — healthy is HTTP 200 with `{"ok":true}`. `GET {base}/readyz` 503 means the server is up but rebuilding its plant. Errors show inline in the dialog; the app never half-switches.

While connected, a 1 s poll loop refreshes the header clock, sensor/control tiles, alarms, console list, and the **Logs** panel. `GET /status` carries `siteName`, `runId`, `simTimeMs`, `latestSeq`, and the objective; a `runId` change rebuilds the tiles and resets the log cursor. New log rows appear live; a small banner appears above the panel when `latestSeq` advances past what you last saw (your own writes don't bump it). The P&ID stays a static local template in remote mode; tiles and logs are the source of truth.

Remote mode starts **read-only**: control widgets, Start/Step/Reset, console submit, alarm acknowledge, and the log composer are disabled. The **🔒 Read-only** toggle switches to **✏️ Writable** — a UI gate only; the server accepts writes from any client. In writable mode, every individual control change asks for a comment in a confirm dialog before a `PUT /controls/{key}` is sent; cancelling submits nothing and reverts the widget. Lifecycle buttons (pause/resume/step/reset, Steps/s) and alarm acknowledge act directly. The Logs panel has a comment-only box (**Add comment to log…**) that posts `POST /logs {"comment":...}` at the current sim time — useful to annotate the run without touching the plant.

Remote-only UI (Server button state, writable toggle, Logs panel, connected banner) renders only while connected; disconnecting restores local mode exactly.

The fault injectors do not appear in `GET /controls`; the remote console fetches `GET /sim/faults` only while the **Faults** header toggle is on and renders them as ordinary control widgets. The server still rejects fault writes with 403 unless it was started with `-allowFaultCommands`.

See `plans/web_remote_plan.md` for the full design and `cmd/nsimserve/README.md` for the server API.

## Extending

The wasm surface (`plant.controlKeys`, `plant.sensorKeys`) mirrors the bridge factory. Add entries in `core/bridge.go` to surface new parts; the web UI picks them up on the next reset. Help text lives in `assets/descriptions/**` and is bundled as `public/descriptions.json` by `make descriptions`.
