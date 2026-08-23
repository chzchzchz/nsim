# nsim-web

Browser-based P&ID dashboard for the nsim PWR simulator.
Compiles `core` + `bridge` to WebAssembly, drives the simulation
in a `requestAnimationFrame` loop, and renders the live plant
SVG alongside the bridge sensors and controls.

## Layout

```
web/
├── wasm/main.go              syscall/js entrypoint (//go:build js)
├── public/                   static assets served by Vite / any host
│   ├── nsim.wasm             compiled simulator
│   └── wasm_exec.js          Go runtime polyfill
├── src/                      TypeScript sources
│   ├── main.ts               bootstrap + tick loop
│   ├── plant.ts              wasm handle wrapper
│   ├── sim.ts                start / pause / step / reset state machine
│   ├── svg.ts                SVG mount + part-name lookup
│   ├── bindings.ts           map control/sensor keys → part names
│   ├── controls.ts           build the control panel
│   ├── sensors.ts            build + refresh the read-out table
│   ├── alarms.ts             alarm panel (structured "alarm" sensor)
│   ├── console.ts            command console + journal export
│   ├── units.ts              display metadata for bridge Kind values
│   ├── wasm.ts               loadWasm()
│   ├── styles.css            shared stylesheet (linked from index.html)
│   ├── vite-env.d.ts         Vite client types (import.meta.env)
│   └── wasm_exec.js          (copied from $(go env GOROOT)/lib/wasm)
├── index.html                dashboard skeleton
├── package.json              vite + typescript dev deps
├── tsconfig.json             ES2020, strict
├── vite.config.ts            dev server on :5173
├── Makefile                  build / wasm / dev / clean
└── README.md                 this file
```

The wasm entrypoint lives in the same Go module as `core`,
`bridge`, and `cmd/nsim` (the repo root). There is no second
`go.mod`; the `//go:build js` build tag is what makes
`web/wasm/main.go` only visible to `GOOS=js GOARCH=wasm`
builds. The host (`go build ./...`) skips it.

## Prerequisites

* Go ≥ 1.23 (the wasm target needs `GOOS=js GOARCH=wasm`)
* Node.js ≥ 18 and npm ≥ 9 (for `vite` and `tsc`)
* A modern browser (anything shipping `WebAssembly.instantiateStreaming`)

## Build

```sh
cd web
make build         # wasm + tsc; copies wasm_exec.js into public/ and src/
```

`make build` is just three commands:

```sh
GOOS=js GOARCH=wasm go build -o public/nsim.wasm ./wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" public/wasm_exec.js
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" src/wasm_exec.js
npx tsc
```

The wasm build pulls `core`, `bridge`, and `core/svg` from
the repo's own module path — there is no `replace` directive
because `web/wasm` is part of the root module.

## Run

```sh
cd web
make dev           # serves http://localhost:5173
```

`vite` serves `index.html` from the project root, the wasm
artefact from `public/`, and TypeScript sources from `src/`.

To host with a plain static server instead:

```sh
make build
npx serve .        # serves index.html, public/, src/
```

The only requirement is that `nsim.wasm` is reachable at the
same origin as `wasm_exec.js` so the `fetch()` inside
`src/wasm.ts` succeeds.

## UI

The dashboard has these regions:

1. **Simulation control bar** (top, `position: sticky` — stays pinned
   while the page scrolls):
   * **Start / Pause** — toggle the tick loop
   * **Step** — advance one tick rate while paused
   * **Reset** — fresh plant, re-rendered SVG and panels
   * **Tick rate** — 1× / 16× / 100× / 1000× `core.TimeStep` per frame
   * **📋** — copy plant state JSON to the clipboard
   * **Status** — `running @ N s simulated` / `paused @ N s simulated`

2. **P&ID SVG** — the live plant diagram, refreshed at most once per
   second and only when the markup actually changed. `<g
   data-part="Part/Path">` wrappers match `bridge.Control.Part` /
   `bridge.Sensor.Part` byte-for-byte so the front end binds by part
   name without coordinate hacks.

3. **Alarm panel** — structured rows from the "Plant/Alarms" sensor.
   Click toggles the acknowledged dim, double-click acknowledges one
   alarm, **Acknowledge All** clears the annunciator.

4. **Control panel + Sensor read-out tiles** — every entry from
   `bridge.GetControlMap` / `bridge.GetSensorMap` becomes a widget /
   table row, grouped one tile per building prefix. Rejected control
   values flash the widget red and log instead of failing silently.
   Tiles rearrange by mouse: press a tile's prefix heading, drag
   ~100 px toward a neighbour to mark it as the pending swap
   (outlined tiles show the pair), drag back inside that band to
   cancel, and release the button to apply the swap.

5. **Command console** (bottom) — submit `TimeOffsetMs Part.Label Value`
   lines that fire relative to the simulation clock; completed rows are
   struck through, failures show their error. **⬇ Export log (.json)**
   downloads every control change of the run for headless replay via
   `nsimrun -cmd nsim-commands.json` (see cmd/nsimrun/README.md).
   Reset clears the queue and journal together with the old plant.

## Wiring new controls / sensors

The wasm surface (`plant.controlKeys`, `plant.sensorKeys`)
reflects whatever the bridge factory exposes. Add entries in
`core/bridge.go` to surface more parts; the web UI picks them
up on the next reset.

The `Kind` field on each `bridge.Control` / `bridge.Sensor`
selects the widget. Currently `controls.ts` infers the widget
type from the label (`Running` / `HeatersOn` are bool, others
are normalized numeric); extend that switch when adding kinds
the UI doesn't yet understand.

## Testing

The wasm build is verified by `make build` succeeding. The
front-end TypeScript layer is small enough that the manual
loop in `make dev` is the canonical test:

1. Open `http://localhost:5173`
2. Click **Start** — sensor read-out updates at 60 Hz
3. Click **Pause**, click **Step** — read-out jumps once
4. Toggle `ReactorBuilding/ReactorCoolantPump/Running` — the
   RCP RPM overlay changes within one frame
5. Enter an out-of-range value in a numeric control — the
   widget flashes red and the console logs the rejection
6. Submit `1000 ReactorBuilding.ReactorCoolantPump.Running 0`
   in the command console — the row is struck through ≈1 sim-s
   later; **⬇ Export log (.json)** downloads the journal
7. Scroll the page — the simulation control bar stays pinned
   at the top
8. Click **Reset** — read-out, SVG, alarms, console list, and
   journal all return to their initial state
