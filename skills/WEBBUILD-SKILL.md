# Skill: Debugging the `web/` Build (Vite + Go WASM)

A deep-dive troubleshooting guide for the `web/` directory, which is a
hybrid static site: TypeScript + Vite on the front end, a Go
`syscall/js` module compiled to WebAssembly on the back end. Born from
a real failure where a help-tip "?" button worked in `make dev` but
returned 404 in `make web.tgz` — the root cause was a stale `.js` file
shad *For the coding agent encountering similar issues*, this file
explains the mental model, the directory contracts, the tooling
surprises, and the step-by-step methodology used to find and fix the
bug.

---

## 1. The architecture in one picture

```
web/
├── wasm/
│   └── main.go              # Go entrypoint, //go:build js tag
├── scripts/
│   └── gen-descriptions.mjs # Builds public/descriptions.json
├── public/
│   ├── nsim.wasm            # GOOS=js GOARCH=wasm go build output
│   └── descriptions.json    # Generated from assets/descriptions/*.md
├── src/
│   ├── main.ts              # Bootstrap, tick loop, panel wiring
│   ├── wasm.ts              # loadWasm() — fetches nsim.wasm
│   ├── descriptions.ts      # Loads descriptions.json into a Map
│   ├── *_test.ts            # vitest unit tests (happy-dom env)
│   ├── styles.css           # Bundled stylesheet
│   └── wasm_exec.js         # Go runtime polyfill (copied from GOROOT, lives in js/)
├── index.html               # Dashboard skeleton (root-absolute links)
├── package.json             # vite, typescript, vitest, marked
├── tsconfig.json            # ES2020, strict, outDir: "js"
├── vite.config.ts           # base: "./", publicDir: "public"
└── Makefile                 # Orchestrates wasm + node + vite
```

Runtime data flow:

```
Browser
  └─ fetches index.html (root-absolute: /src/main.ts, /src/styles.css)
       └─ Vite rewrites these to relative paths in the bundle
            ├─ loads /assets/index-*.js  (compiled TS)
            │    ├─ import "../js/wasm_exec.js"  → Go runtime polyfill
            │    ├─ fetch("./nsim.wasm")    → WebAssembly module
            │    └─ fetch("./descriptions.json") → help text map
            └─ loads /assets/index-*.css (compiled from styles.css)
```

---

## 2. The Go/WASM half — what an agent needs to know

### 2.1 One module, two build targets

The repository is a single Go module (root `go.mod` declares `module
github.com/chzchzchz/nsim`). There is **no separate `go.mod`** in
`web/`. The file `web/wasm/main.go` is excluded from normal Go builds
by a build tag:

```go
//go:build js
```

Only `GOOS=js GOARCH=wasm go build` sees this file. The `host`
targets (`go build ./...` on linux/darwin/windows) skip it. This means:

- **Running tests in the repo root does NOT exercise the wasm entrypoint.**
- `make dev` or `make web.tgz` is the only way the wasm code is compiled.
- The wasm build pulls in `core`, `bridge`, and `core/svg` from the
  same module graph automatically.

### 2.2 The Go polyfill and syscall/js

Browsers cannot run raw Go. The Go toolchain ships `wasm_exec.js`,
which implements `syscall/js` (window.Go, js.Global(), js.Value,
etc.). `web/wasm.ts` does:

```typescript
import "../js/wasm_exec.js";   // makes window.Go available globally
```

The Makefile copies `wasm_exec.js` from `$(go env GOROOT)/lib/wasm/`
into `js/wasm_exec.js`. Vite then inlines this into the JS bundle
alongside all the TypeScript. **Nothing else loads `wasm_exec.js`** —
it is purely an import side-effect.

### 2.3 The wasm module is a static asset

`GOOS=js GOARCH=wasm go build` produces `public/nsim.wasm`. Vite's
`publicDir: "public"` copies it verbatim into `dist/nsim.wasm`. The
TS loader constructs a URL:

```typescript
const wasmUrl = import.meta.env.BASE_URL + "nsim.wasm";
```

`import.meta.env.BASE_URL` is a Vite constant rewritten at build time
from `vite.config.ts`'s `base` field. Since `base` is `"./"`, the URL
is `"./nsim.wasm"` in production and `"/nsim.wasm"` (dev-server
mounted route) in development. Either way, the fetch is relative.

---

## 3. The front-end half — Vite, TypeScript, and the dev/build split

### 3.1 What Vite actually does

Vite does two very different things depending on the command:

| Invocation | What it does with `src/` | What it does with `public/` | What it does with `index.html` |
|---|---|---|---|
| `vite` (dev) | Transpiles `.ts` on-the-fly in memory. The browser requests `/src/main.ts`; Vite serves the transpiled JS over HTTP/2. **No files are written to disk.** | Serves the directory directly. `/` maps to `public/` on the dev server. | Serves as-is. Root-absolute links (`/src/styles.css`, `/src/main.ts`) resolve because Vite controls the origin. |
| `vite build` | Runs Rollup. Walks imports starting from `index.html`'s `<script type="module" src="/src/main.ts">`. Emits minified JS into `dist/assets/`. **Only files actually imported are included.** | Copies `public/*` into `dist/` verbatim. | Rewrites asset URLs using the `base` config (`"./"` → relative paths in emitted HTML). |

The critical insight: **dev and production execute different code paths
through Vite.** Dev hides problems that build exposes, and vice versa.

### 3.2 TypeScript compilation vs Vite bundling

`package.json` defines:

```json
"scripts": {
  "build": "tsc",
  "dev": "vite"
}
```

**`npm run build` runs `tsc` with `tsconfig.json`.** That config has:

- `target: "ES2020"`
- `module: "ES2020"`
- `outDir: "js"`
- `noEmit: true`

Today `tsc` and `npm run build` type-check only; no `.js` files are
written anywhere. `web/js/*.js` and `web/src/*.js` are both **gitignored**.

**Historical note:** Before `noEmit: true` and `outDir: "js"` were
added, `npm run build` wrote `.js` twins next to `.ts` in `src/`.
Because those paths are gitignored, they did not appear in `git
status`, but they remained on disk and silently shadowed the `.ts`
sources during Rollup resolution. The fix was to delete them, not to
keep them in sync.

### 3.3 Module resolution and the ".js before .ts" surprise

Node.js and Rollup (Vite's bundler) follow this import resolution
order for bare specifiers like `import "./descriptions"`:

1. `descriptions.js`
2. `descriptions.ts`
3. `descriptions.json`
4. `descriptions/index.js`
5. `descriptions/index.ts`

`.js` is tried **before** `.ts`. In a fresh clone where `src/`
contains only `.ts` files, resolution reaches `.ts` and Vite handles
the transpilation. But once a stale `.js` exists (emitted by `tsc`),
it is found first and loaded verbatim.

**This is why the fix in `descriptions.ts` did not propagate to the
production bundle:** Vite bundled `descriptions.js`, not
`descriptions.ts`, because the `.js` file shadowed it.

**Policy:** because `web/src/*.js` is gitignored, **delete the shadow
instead of synchronizing it.** Rollup will fall back to `.ts`
automatically. If `tsc` or another tool regenerates the `.js`, delete
again. The Makefile never emits `.js` into `src/` (it uses `tsc
--noEmit`); if you find `.js` twins there, someone ran bare `tsc` or
`npm run build` outside the Makefile.

**Policy:** because `web/src/*.js` is gitignored, **delete the shadow
instead of synchronizing it.** Rollup will fall back to `.ts`
automatically. If `tsc` or another tool regenerates the `.js`, delete
again.

### 3.4 Root-absolute vs relative URLs — the most common gotcha

The web app fetches runtime assets:
- `nsim.wasm` (the simulation binary)
- `descriptions.json` (the help-text map)

In the dev server, **every root-absolute path works** because Vite
mounts `public/` at `/`. A fetch for `/descriptions.json` succeeds
because the file lives at `web/public/descriptions.json` and the dev
server maps it to `http://localhost:5173/descriptions.json`.

In a static bundle (`make web.tgz`), the content is extracted to
`nsim-web/` and served by some external server. That server may not
mount `nsim-web/` at the domain root — it might be at `/nsim-web/`,
or `/app/`, or anything. A root-absolute fetch bypasses the subpath
and hits the domain root, where `/descriptions.json` almost certainly
does not exist.

**Correct pattern (used by `wasm.ts`):**

```typescript
const url = import.meta.env.BASE_URL + "nsim.wasm";
```

Vite rewrites `import.meta.env.BASE_URL` at build time to match the
`base` config. For `base: "./"` it becomes `"./"`, so the fetched URL
is `"./nsim.wasm"` — relative to the JS chunk's location inside
`dist/assets/`. One directory up reaches `dist/`, where both
`nsim.wasm` and `descriptions.json` were copied by Vite.

**Wrong pattern (the bug):**

```typescript
const res = await fetch("/descriptions.json");
```

Hardcoded `/descriptions.json` is resolved by the browser against the
document origin (or `<base>` href). It ignores the deploy subpath
entirely.

---

## 4. The Makefile as documentation

Understanding `make dev` vs `make web.tgz` prevents the most common
misdiagnosis. Read the Makefile as a script of "what Vite sees":

### 4.1 `make dev`

```makefile
dev: $(NODE_STAMP) descriptions
	npx vite
```

- `descriptions` target ensures `public/descriptions.json` is fresh.
- `npx vite` starts the dev server. It reads `vite.config.ts` and
  serves from disk. All source files (`src/*.ts`, `public/*`,
  `index.html`) are visible to the browser in their original form.

### 4.2 `make web.tgz` (and `web-debug.tgz`)

```makefile
web.tgz: VARIANT := release
web.tgz: DIST_DIR := dist
web.tgz: VITE_FLAGS :=

web-debug.tgz: VARIANT := debug
web-debug.tgz: DIST_DIR := dist-debug
web-debug.tgz: VITE_FLAGS := --minify false --sourcemap
```

Both targets share this recipe:

```makefile
web.tgz web-debug.tgz: $(NODE_STAMP) $(WASM_OUT) $(DESC_JSON) $(BUNDLE_DEPS)
	npx vite build $(VITE_FLAGS) --outDir $(DIST_DIR)
	rm -rf .bundle && mkdir -p .bundle/$(BUNDLE_DIR)
	cp -r $(DIST_DIR)/. .bundle/$(BUNDLE_DIR)/
	... (rebuild WASM, tar it)
```

- `vite build --outDir dist` runs Rollup. **This is where the `.js`
  shadow problem bites.** If `src/descriptions.js` exists, Rollup
  resolves it instead of `src/descriptions.ts`.
- The output `dist/` is copied into `.bundle/nsim-web/`, the WASM is
  rebuilt with stripped symbols, and the result is tarred.

**Key point:** `make web-debug.tgz` adds `--sourcemap`. If the two
tarballs behave differently, sourcemaps allow you to trace the bundled
JS back to the upstream `.ts` or `.js` file. A sourcemap pointing at
`src/descriptions.js` while you edited `descriptions.ts` is an instant
red flag.

### 4.3 What `make build` does

`make build` = `make wasm` + `make wasm-exec` + `make typecheck`:

```makefile
build: wasm typecheck descriptions
```

- **`make wasm`**: `GOOS=js GOARCH=wasm go build -o public/nsim.wasm ./wasm`
- **`make wasm-exec`**: `mkdir -p js/ && cp $(GOROOT)/lib/wasm/wasm_exec.js js/wasm_exec.js`
- **`make typecheck`**: `npx tsc --noEmit` (type-check only, no emit)

**`make typecheck` does NOT write `.js` files.** The problem comes from
running `tsc` *without* `--noEmit` (e.g., `npm run build`), which
writes `.js` next to `.ts`.

### 4.4 What `npm run build` does (and why it is dangerous here)

`package.json` has:

```json
"build": "tsc --noEmit"
```

This now runs `tsc` in type-check-only mode because `tsconfig.json`
sets `noEmit: true`. No `.js` files are written anywhere.

**Historical note:** Before `noEmit: true` and `outDir: "js"` were
added, `npm run build` wrote `.js` twins next to `.ts` in `src/`.
Because those paths were gitignored, they did not appear in `git
status`, but they remained on disk and silently shadowed the `.ts`
sources during Rollup resolution. The fix was to delete them, not to
keep them in sync.

---

## 5. Directory contracts — what belongs where

### 5.1 `public/` — static assets served as-is

Files in `public/` are copied into `dist/` by Vite unchanged. They are
accessible from the browser at URLs relative to the deploy base:
- `public/nsim.wasm` → `dist/nsim.wasm`
- `public/descriptions.json` → `dist/descriptions.json`

**Rule:** Any file fetched at runtime (`fetch(...)`, `<img src>`,
`<link href>`) that should ship statically belongs in `public/` (or
be imported by a TS/CSS module so Vite hashes it into `dist/assets/`).

### 5.2 `src/` — TypeScript sources only

**Rule:** `src/` should contain only `.ts` files. Any `.js` file in
`src/` is a stale build artifact and should be deleted. `wasm_exec.js`
lives in `js/` now (copied there by the Makefile).

Because `web/src/*.js` is **gitignored**, these artifacts do not
dirty `git status`, but they remain on disk and Rollup resolves them
before `.ts`. The Makefile never emits `.js` into `src/`; if you find
`.js` twins, someone ran bare `tsc` or `npm run build` outside the
Makefile. Delete them.

### 5.3 The `.js` inventory

At the time of the bug, `src/` had 27 `.js` files:
- `alarms.js`, `bindings.js`, `clipboard.js`, `console.js`,
  `controls.js`, `grid.js`, `help-button.js`, `layout.js`, `main.js`,
  `plant.js`, `scenarios.js`, `sensors.js`, `sim.js`, `svg.js`,
  `tile.js`, `units.js`, `wasm.js`
- Plus `*.dom.test.js` and `*.test.js` pairs for many of the above.

Each `.js` had no type annotations and (in the case of `descriptions.js`)
had not been updated when its `.ts` counterpart was.

### 5.4 `wasm_exec.js` — no longer in `src/`

`js/wasm_exec.js` is copied from `$(go env GOROOT)/lib/wasm/` by the
Makefile (`make wasm-exec`). It is loaded by `src/wasm.ts` via:

```typescript
import "../js/wasm_exec.js";
```

Vite bundles this import. The raw polyfill file also lives in
`public/` for the standalone `npx serve .` case described in the README.
**`wasm_exec.js` is no longer in `src/`; it lives in `js/` now.**

---

## 6. The descriptions system

### 6.1 Building the JSON

`make descriptions` runs `scripts/gen-descriptions.mjs`, which walks:
- `assets/descriptions/**/*.md` (help text for controls/sensors)
- `assets/alarms/**/*.md` (help text for alarm conditions)

Each `.md` file's trimmed content is keyed by its path relative to the
root folder, with the `.md` extension stripped. Example:

```
assets/descriptions/ReactorBuilding/RPV/Pressure.md
  → key: "ReactorBuilding/RPV/Pressure"
  → value: "<markdown text>"
```

Output: `public/descriptions.json`

### 6.2 Loading at runtime

`src/descriptions.ts` (and its .js shadow) exports:

```typescript
export async function loadDescriptions(): Promise<void>;
export function helpFor(id: string): string | undefined;
```

`main.ts` calls `await loadDescriptions()` during bootstrap. The
function fetches `descriptions.json`, parses it into a `Map`, and
`helpFor(id)` is consulted by `controls.ts`, `sensors.ts`, and
`alarms.ts` to decide whether to render a "?" button.

**If the fetch fails** (404, wrong path, offline), `loadDescriptions`
logs `"(?) help disabled:"` and continues. The simulation runs; only
the help buttons disappear. This graceful degradation is why the bug
was silent — the UI still worked, just with fewer tooltips.

### 6.3 The help button UI

- `bindings.ts:97` — `makeHelpButton()` creates `<button class="ctl-help">?</button>`
- `bindings.ts:137` — `openTip()` renders a `<span class="help-text md">`
  sibling with `marked.parse(text)`
- Clicking a "?" shows the popup; clicking a cross-reference link
  inside scrolls/flashes the target row via `revealElement()`

---

## 7. The specific bug — step-by-step reconstruction

### 7.1 Symptoms

- `make dev`: help buttons work, clicking "?" shows markdown popups.
- `make web.tgz`: extracted bundle served as static files, clicking "?"
  produces a console warning: `"(?) help disabled: HTTP 404"` or
  `"(?) help disabled: TypeError: Failed to fetch"`.

### 7.2 The initial (incorrect) hypothesis

The developer's first guess: "The fetch path for `descriptions.json` is
wrong in the bundled code." They inspected `src/descriptions.ts` and
saw it already used `import.meta.env.BASE_URL` or some other "correct"
path. They checked `vite.config.ts` and confirmed `base: "./"`. They
assumed the build was fine and looked elsewhere.

### 7.3 The correct diagnosis

Inspecting the **actual bundled file** (`dist/assets/index-*.js`):

```sh
rg -o 'fetch("[^"]*")' dist/assets/index-*.js | rg descriptions
# Output: fetch("/descriptions.json")
```

The bundle contained root-absolute `/descriptions.json`, despite
`descriptions.ts` having been "fixed" to use a relative path.

Tracing back: the Vite Rollup bundle resolves imports by trying
extensions in order. `src/descriptions.js` existed (emitted earlier by
`tsc`). Its content:

```javascript
// descriptions.js — stale artifact
const res = await fetch("/descriptions.json");   // <-- root-absolute
```

The canonical `descriptions.ts` had already been updated to:

```typescript
const res = await fetch("./descriptions.json");  // <-- relative
```

But Rollup bundled the `.js`, not the `.ts`.

### 7.4 Why `make dev` masked the bug

In dev mode, `vite` serves `src/main.ts` to the browser. That TS file
imports `./descriptions`. The dev server's module resolution also tries
`.js` before `.ts` — **but** Vite's dev transform pipeline treats
`.js` files as potential ES modules and applies HMR transforms. More
importantly, if `descriptions.js` was itself generated from a
*preexisting* version of `descriptions.ts`, it may have already
contained the relative path at some point, or the dev server's resolve
fallback reached `.ts` via a different code path (e.g., the import in
`main.ts` resolved through `descriptions.ts`'s own import of `/` rather
than its `.js` twin’s fetch).

Actually, a more likely explanation: `descriptions.js` **also** had
`fetch("/descriptions.json")` at the time `make dev` was tested, but
the Vite dev server's fallback behavior or a caching mechanism served
the `.ts` transpilation first, or the `.js` was newer/later-added. The
exact timing is less important than the invariant: **the `.js` shadow
was always there and always wrong; dev's tolerance for live reload
and source-over-source resolution made it less obvious.**

### 7.5 The fix

Fix A: change `descriptions.ts` to `fetch("./descriptions.json")`.  
Fix B (also applied for defense-in-depth): patch `descriptions.js` with
the same relative path so that even if the shadow reoccurs, it is
correct.  
Fix C (preventive, recommended but not enforced): delete all stale
`.js` files so `.ts` remains the single source of truth.

After rebuilding:

```sh
rg -o 'fetch("[^"]*")' dist/assets/index-*.js | rg descriptions
# Output: fetch("./descriptions.json")
```

---

## 8. Methodology for "works in dev, broken in prod"

When a frontend issue reproduces only in the bundled output:

1. **Trust the bundle, not the source.** `dist/assets/index-*.js` is
   the code the browser executes. Grep it for the symptom:
```sh
	rg -o 'fetch("[^"]*")' dist/assets/index-*.js
	```

2. **Trace back to the source file.** Vite prepends a comment like
   `// src/descriptions.ts` to each module in the bundle. Sourcemaps
   (`web-debug.tgz`) give exact line mappings.

3. **Check for shadow files.** If the upstream file is `*.js` but the
   project convention is `*.ts`, a duplicate exists:
   ```sh
   ls src/*.js | while read f; do
       base="${f%.js}"
       [ -f "${base}.ts" ] && echo "SHADOW: $f shadows ${base}.ts"
   done
   ```

4. **Rebuild with `vite build --outDir dist`.** Do not iterate through
   `make web.tgz` — wasm recompilation adds minutes and noise. The
   lightest path to a reproducible bundle is just the JS/CSS build.

5. **Verify the artifact.** Regrep the output. If the offending string
   is gone, move on. If it persists, check `src/` for yet another
   shadow, or for the string hardcoded in an imported third-party lib.

---

## 9. Generalizable principles

### 9.1 Static asset URLs in subpath-hosted SPAs

When a static bundle may be hosted under any subpath (not only `/`):

- **Never hardcode `/` in JavaScript `fetch()`, `XMLHttpRequest`, or
  `<img src>` / `<link href>` that points at a local asset.**
- Use **relative URLs** (`"./file.json"`) or **Vite-rewritten
  constants** (`import.meta.env.BASE_URL + "file.json"`).
- CSS `url()` references are rewritten by Vite's CSS handling; they
  inherit the same relative-path discipline.
- `index.html` may use root-absolute `/src/...` paths because Vite
  (dev) and Vite's emitted HTML rewrite (prod) both resolve them.

### 9.2 One source of truth per module

If a module exists as both `foo.ts` and `foo.js` in the same
directory, Node/Rollup/Vite will load `foo.js` first. This is not a
bug in the bundler — it is explicit Node.js ESM resolution order. The
fix is to keep **only the non-extant form** on disk:

- Author `.ts`.
- Let Vite (dev) transpile on the fly.
- Let Vite (build) bundle into `dist/`.
- Delete any in-tree `.js` duplicates.

If a toolchain absolutely requires emitted `.js` files, redirect them
to an out-of-tree directory (`outDir`, `.bundle/`, `dist/`) via
`tsconfig.json` or tool flags.

### 9.3 Treat `npm run build` as a command, not a workflow step

`npm run build` (i.e., `tsc`) is fine for type-checking if run with
`--noEmit`. If run without, it writes artifacts. In this project, the
command is unguarded:

```json
"build": "tsc"
```

and `tsconfig.json` lacks `"noEmit": true` but does set `"outDir":
"js"`, so emitted JS lands in `web/js/` (gitignored) rather than
`src/`. The Makefile avoids emit by using `tsc --noEmit` in `make
typecheck`. An agent seeing `npm run build` in a `package.json` of a
Vite-only project should immediately inspect whether it pollutes the
source tree or an emit directory.

### 9.4 The `base` field is a contract, not a suggestion

`vite.config.ts`'s `base: "./"` is the promise that the bundle works
from any subpath. It is honored **only for assets Vite knows about** —
imports, CSS `url()`, and the HTML rewrite. It is NOT honored for
strings passed at runtime to `fetch()` unless those strings are built
from `import.meta.env.BASE_URL` or are relative `./` paths.

### 9.5 Source maps debug burnt-bytecode issues

`make web-debug.tgz` produces readable JS + sourcemaps. When a
production bundle misbehaves:

1. Build the debug bundle: `make web-debug.tgz`.
2. Extract and inspect `dist/assets/index-*.js` directly (it is not
   minified).
3. Use the source map to trace a suspicious line back to `src/` and
   determine whether it came from `.ts` or `.js`.

If the traceback lands in `descriptions.js` while you edited
`descriptions.ts`, you have found a shadow file.

---

## 10. Complete checklist before declaring a web build fixed

- [ ] `npx vite build --outDir dist` succeeds.
- [ ] No root-absolute paths in `dist/assets/index-*.js` for assets
      that should be local:
      ```sh
      rg -o 'fetch("[^"]*")' dist/assets/index-*.js | rg '^fetch("\/'
      ```
      should return nothing relevant.
- [ ] No root-absolute `"/descriptions.json"` fetches in the bundle.
- [ ] `tsc --noEmit` passes (`make typecheck`).
- [ ] `make test` (vitest) passes — help-button tests exercise
      `seedDescriptions` and `helpFor` paths.
- [ ] `make web.tgz` builds and its `dist/` contents contain
      `descriptions.json` at the top level:
      ```sh
      tar tzf web.tgz | rg 'nsim-web/descriptions.json'
      ```
- [ ] `src/` contains no stale `.js` files (`wasm_exec.js` moved to `js/`):
      ```sh
      ls src/*.js
      ```
      should produce no output. (If it does, the files are stale
      artifacts from a time when `tsconfig.json` had no `outDir`.)
- [ ] `git status` confirms only the intended `.ts` changes are staged.

---

## 11. Red flags

- "Works in `make dev` but not in `make web.tgz`" — 90 % probability
  the fault is in the **bundle**, not the source. Inspect `dist/`.
- `git status` shows ~20 untracked `.js` files in `web/src/`.
- `npm run build` was historically run without `--noEmit` (it emitted `.js` twins before `noEmit: true` was added).
- A console `"(?) help disabled:"` warning in the bundled app with no
  corresponding network error in the Network tab (because the browser
  is resolving `/descriptions.json` against the wrong origin/subpath).
- The bundle contains a hardcoded leading `/` in a `fetch()` string.
- Sourcemaps (debug bundle) trace a fix to a `.js` file you never
  opened.
- `make clean` does not remove the `.js` twins (only `dist/`,
  `dist-debug`, and `.bundle` are cleaned).

---

## 12. Part-path wrapping in tile headings

Long **part paths** (e.g. `DieselGeneratorB/Generator`) displayed as
tile headings can exceed their container width. The correct fix is
**`<wbr>` inserted by `displayPart`** (`web/src/bindings.ts`), not CSS
and not label mutation:

```typescript
export function displayPart(prefix: string): PartDisplay {
    const head = prefix + "/";
    return (part: string): string =>
        part.startsWith(head)
            ? part.slice(head.length).replace(/\//g, "/<wbr>")
            : part.replace(/\//g, "/<wbr>");
}
```

The `PartDisplay` return type is reused by `controls.ts` and `sensors.ts`
for legends and sub-headings, so a single change here wraps every
rendered part-path consistently.

Because the return value contains `<wbr>` tags, **use `innerHTML`** when
rendering it:
```typescript
lg.innerHTML = display(part.part);   // controls.ts legend
h.innerHTML  = display(part.part);   // sensors.ts sub-heading
```
Using `textContent` will HTML-escape the `<wbr>` and display it as
literal `<wbr>`.

**Do not** insert spaces or `<wbr>` into control/sensor **labels** (the
`label` field, e.g. `BreakerClosed`). Those strings must stay
byte-identical to the bridge id for sorting, cross-reference lookups,
and screen readers.

## 13. Help-popup behavior

Inside `bindings.ts`, `openTip` attaches a click listener to the
popup. For cross-reference links the current contract is:

- `revealElement(href)` jumps to the rendered row and flashes it.
- If `revealElement` returns `false`, fall back to `helpFor(href)`
  and swap the popup text.
- **The popup is never closed by following a link.** Closing requires
  clicking the `?` button again.

If you change this behavior, update the DOM test in
`help-button.dom.test.ts` to match the new contract.

Test pattern: seed descriptions, call `makeHelpButton(text)`, click the
button to open, find the `<a>` inside the popup, click it, then assert:
- the popup `innerHTML` is unchanged (or swapped to the new description),
- the target row has the `ref-flash` class.

Run `npx vitest run src/help-button.dom.test.ts` in `web/`.

## 14. The web skill’s own context

The `web/` tree is authored in TypeScript but ships as a JavaScript
SPA. The Wasm module in `wasm/main.go` is compiled, not transpiled.
`public/descriptions.json` is a build artifact generated from
`assets/descriptions/*.md`. The only other generated file in `web/` is
`public/nsim.wasm`.

Static analysis that only reads `*.ts` files may miss the truth if a
stale `*.js` shadow exists. Always resolve by looking at the Vite
bundle output (`dist/assets/index-*.js`) when the reported symptom is
about runtime behavior in a production build. Gitignored `.js` files
in `src/` are a known failure mode: delete them, do not edit them.