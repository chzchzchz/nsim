# nsim

Command-line inspector for the `nsim` nuclear power plant simulator.
Dumps plant state and renders the P&ID diagram; the web UI is served
by `web/` instead.

## Usage

```bash
go run ./cmd/nsim/ [flags]
```

Flags:

- `-dot <file>` — write the plant fluid graph in Graphviz DOT format and exit.
- `-svg <file>` — render the P&ID SVG (computes a fresh graphviz layout first) and exit.
- `-svg-layout <file>` — same as `-svg`; kept as an explicit alias for layout debugging.
- `-svg-dot <file>` — write the DOT description used for SVG layout (clusters, node sizes, inline pump/valve hops) and exit. Pipe it through `dot -Tplain-ext`, `-Tjson`, or `-Tsvg` to inspect what the renderer consumes.
- `-svg-debug <file>` — write a debug overlay showing cluster boxes, symbol bounding rectangles, and anchors.
- `-svg-assets <dir>` — export the embedded symbol library to a directory and exit.

## Notes

- Without `-svg*` flags the tool dumps the full plant state to stdout.
- Symbol placement is owned by graphviz: every rendered part is a DOT
  node (fluid nodes, edge hops such as pumps and valves, and
  renderer-only parts), laid out inside building clusters. See
  `core/svg/layout.go`.
- The web UI embeds a generated default layout
  (`core/svg/layout_default_generated.go`) so no dot invocation is
  needed at runtime; refresh it with
  `go generate ./core/svg` after changing the plant graph.

## Example

Render a snapshot of the default plant:

```bash
go run ./cmd/nsim/ -svg plant.svg
```
