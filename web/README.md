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

## Scenarios

The 🧭 Scenario picker lists the operator evolutions from `core/scenarios.go` (~38), grouped by section. Choosing one rebuilds the plant with that start state through the same reset path as Reset; Reset re-applies it, Free play uses the default plant. Malfunctions script their trigger ~30 s in (`Scenario.Script`), and console exports embed the scenario id so `nsimrun -cmd session.json` reproduces the run.

## Extending

The wasm surface (`plant.controlKeys`, `plant.sensorKeys`) mirrors the bridge factory. Add entries in `core/bridge.go` to surface new parts; the web UI picks them up on the next reset. Help text lives in `assets/descriptions/**` and is bundled as `public/descriptions.json` by `make descriptions`.
