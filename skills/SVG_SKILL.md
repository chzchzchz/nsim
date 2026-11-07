# Skill: Developing the core/svg Package

Conventions and workflows for evolving the P&ID SVG generation in
`core/svg` — adding parts, overlays, layout behaviour, or fixing visual
bugs. Distilled from the session that converted the package from
hand-composed coordinates to a graphviz-driven renderer.

## Prime directive

**Graphviz owns every coordinate.** The Go code never places a symbol by
hand. If something looks wrong in the diagram, fix one of exactly three
things — never the renderer's pixel offsets:

1. the DOT description (`BuildDot`: clusters, node sizes, hops, extra nodes),
2. the DOT tuning attributes (sep / nodesep / ranksep), or
3. the single uniform transform (`computeFit`: scale + center + Y-flip).

A "quick nudge" of `-50px` in svg.go is always a regression. This session
removed several; do not add new ones.

## Architecture in one paragraph

`BuildDot` emits every rendered part as a DOT node — fluid-graph nodes,
edge-hop nodes for inline pumps/valves (`assignEdgeHops`, shared with
`ComputeLayout` so route stitching stays in sync), and `extraNodes` for
renderer-only parts such as Turbine/Generator/PRT. `ComputeLayout` runs
`dot -Tplain-ext`, parses node positions, edge polylines, and the drawing
size, then applies one uniform scale-to-fit with Y-flip into the fixed
1200x900 viewport, producing a `Layout{Pos, Routes, Scale, DPI}`.
`svg.go` composes: cluster-derived building outlines, routed pipe
polylines, symbols from the shared symbol table scaled by `Layout.Scale`,
and table-driven live overlays. Buildings are bounding boxes of cluster
membership; decorations yield to real parts via `overlapsPart`.

## Non-negotiable rules

- **Every rendered part is a DOT node.** Fluid node, hop node, or
  `extraNodes` entry. If `WritePlantSVG` draws it from an anchor, dot must
  position it. Renderer-only parts go in `extraNodes` with a cluster and a
  symbol-table kind.
- **Inline pumps/valves are hop nodes on subdivided edges.** Add new cases
  to `pumpHopFor`/`valveHopFor` only — both BuildDot emission and route
  stitching derive from those two functions. An id matching an existing
  plant-graph node (RCP) is reused verbatim; colliding names get `_N`
  suffixes (CSTValve_2/_3).
- **One symbol table.** `symbols.go` maps kind -> {wIn, hIn, image}.
  Symbol id = asset filename minus `.svg`. Never introduce a second
  kind->size or kind->symbol mapping; `dispatch.go` was deleted for this
  reason.
- **Y-up vs Y-down.** plain-ext/json coordinates are Y-up; the document is
  Y-down. The flip lives inside `fitTransform.point`. Do not flip anywhere
  else, and do not remove it there.
- **Scale, don't clamp.** Viewport fitting is one uniform transform.
  Per-node clamping piles everything on the edges — the original bug that
  motivated this architecture. The only clamp allowed is a margin guard on
  node centers after fitting.

## Workflows

### Adding a part to the diagram

1. Does it carry fluid? It already has a graph node in
   `core.Plant.BuildGraphFor` — give its Kind an entry in `symbolTable`.
2. Renderer-only part? Add it to `extraNodes` (id, cluster, kind).
3. Add a `diagramParts` row: id -> bridge part path, symbolTable kind,
   caption. Symbol id resolves automatically.
4. If it should sit inside a building outline, ensure its cluster entry in
   `clusterAssignments` (via `clusterFor` or `extraNodes`).
5. Isolated parts land in dot's first rank, which floats them above the
   plant after the Y flip. Auxiliary/electrical parts are sunk with
   invisible `Condenser -> id [style=invis]` edges in BuildDot; extend that
   loop if a new isolated building appears elsewhere.
6. Run `go generate ./core/svg`; the manifest test will fail if the symbol
   id has no `<symbol>` definition — fix the table, not the test.
7. Operator-facing elements the part exposes need help text: one `.md`
   per bridge control/sensor id under `assets/descriptions/<part path>`,
   written per plans/DESC_SKILL.md (body structure, numbers with units,
   Related lines, verified Source lines). Then run
   `cd web && make descriptions`; a missing entry only means no `(?)`
   button, so nothing else fails loudly — the gap is silent by design.

### Adding live state (readout, status dot, ring, tint)

- Text readouts: append to `readouts` (id, dy, optional show, text).
- Alarm rings / status halos: append to `rings`; running-state highlights
  set `running: true` instead of an alarm id.
- Pump-backed parts: add an accessor in `pumpAccessors` — the run-status
  dot and rpm readout come for free.
- Alarm tinting beyond rings: map alarm id -> affected part ids + colour in
  the tint table; apply as a halo rect around the symbol bbox, not by
  restyling `<use>` internals (they are not addressable per-instance).
- All overlay text goes through `html.EscapeString`.
- New bridge registrations surfaced by the overlay (a control the ring/
  tint reacts to, a sensor behind a readout) follow the same
  plans/DESC_SKILL.md rule as new parts: drop the `.md`, then
  `cd web && make descriptions`.

### Changing layout behaviour

Tune graph attributes in BuildDot (`sep`, `nodesep`, `ranksep`). Verify the
resulting `Scale` value in the regenerated file — sinking disconnected
nodes adds ranks and can cancel out tightening gains. `rank=sink` inside a
cluster subgraph is ignored by dot (verified empirically); use invisible
constraint edges instead.

## Animation phase plumbing

The web UI re-fetches `getSvg()` and remounts via innerHTML, which restarts
CSS animations every swap (~1 Hz throttle). Any CSS animation in the
document therefore needs its phase derived from simulation time, not wall
clock: root element sets `style="--phase:<ms>"` and the animation uses
`animation-delay: calc(var(--phase,0) * -1ms)`. Thread sim time through
`RenderOptions.PhaseMs` (`WritePlantSVGWithOptions`) from the wasm handle;
static exports use zero. When this pattern is extended (flow dash
animation), keep the phase derivation at exactly one place — the root
style attribute.

## Testing rules

- **No exact coordinate assertions.** Graphviz versions shift layouts.
  Assert properties instead: bounds containment, span fractions, rank
  order where the acyclic structure guarantees it, aspect ratio,
  centering symmetry. Parsers get round-trip fixtures built from named
  expected values.
- **Manifest test is load-bearing.** Every referenced symbol id must exist
  in emitted `<defs>`; it caught a nonexistent heat-exchanger asset on its
  first run. Extend its `refs` set whenever a new symbol reference appears.
- **Missing anchors are a failure mode.** Skipped parts emit
  `<!-- anchors missing from layout: ... -->`; a test asserts absence.
  Never silently skip a part without recording it.
- **Determinism tests compare structure, not bytes**: same plant ->
  identical Pos/Routes/Scale across two ComputeLayout calls.

## Verification recipe (render audit)

After any change, render and audit numerically — do not eyeball:

```bash
go generate ./core/svg && go run ./cmd/nsim -svg /tmp/nsim-nsim-alt5/out.svg
```

Then script-check: strip `<defs>...</defs>` first (nested `<use>` elements
inside symbols legitimately use local negative coordinates), verify every
top-level `<use>` and pipe segment lies within 0..1200 x 0..900, count
overlaps among major symbols, confirm polylines exist (not line fallbacks)
and carry `marker-mid`, confirm valve-percentage texts and pump dots
appear, and assert the missing-anchor comment is absent. Rasterizing is
never required.

## Empiricism rule

Never trust assumed graphviz behaviour — probe it with a two-line DOT file
through the exact output format before coding against it. Established this
session: coordinates are Y-up; `rank=sink` is ignored inside clusters;
plain-ext edge lines carry `<n>` point pairs; JSON output additionally
reports cluster bboxes and spline points (parsed variant exists in git
history if needed); image="..." filenames need not exist when we consume
only coordinates.

## House style

- Layout-as-parameter over globals: prefer `WritePlantSVGWith(w, p, l)`;
  `SetLayout` + the embedded default exist only as convenience for the
  wasm path.
- Tables over switch-walls: parts, readouts, rings, tints, and hop
  mappings are data. Adding a row should never require editing control
  flow.
- Generated files round to 3 decimals so regeneration diffs stay
  meaningful across graphviz versions.
- plans/* and specs/* are never committed.

## Text metrics

- Any text drawn in document units must scale with `Layout.Scale` the way
  symbol footprints do, or a shrunk layout turns captions into giants (a
  horizontal pump draws ~7 units under Scale 0.183 while a fixed
  font-size 10 label stood taller than the glyph). Size symbol-relative
  text as a ratio of the drawn height with a legibility floor and the
  historical size as a cap (`scaledFont`), and scale hand-tuned offsets
  by the same ratio. Catch: render the real diagram and grep emitted
  `font-size` values against `<use>` footprint heights instead of
  eyeballing; debug-only overlays may keep fixed sizes.

## Generated markup audits

- Presentation helpers can silently emit non-markup: a wedge helper
  returned bare path data ("M ... Z") where an element was required, so
  the open-fraction indicator never rendered while every test stayed
  green (they pinned presence of other strings). After adding or
  changing an emitted fragment, grep the rendered document for the
  element itself (`<path d=`), not just for substrings of it, and
  measure placed geometry against polylines with a transform-aware
  script - local <use> coordinates inside translate/rotate groups read
  as the origin otherwise.

## Containment invariant

- `TestSymbolGeometryInsideViewBox` walks every embedded asset family and
  asserts strict glyph containment (boundary counts as clipped: strokes
  lose half their width at the edge). Invisible routing-anchor markers
  (`fill="none" stroke="none"`) are exempt - they draw nothing, so
  clipping them is moot; audits that count them chase phantoms. A
  symbol that cannot be fixed locally (composed elsewhere via `<use>`)
  takes an entry in `symbolWaivers` naming the retune it needs; a waiver
  is visible debt, never a pass.
