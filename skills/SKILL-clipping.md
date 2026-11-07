# Skill: SVG Clipping Prevention

## Goal

Ensure no geometry is silently clipped by `<symbol>` viewBox boundaries.
Apply this skill whenever adding geometry (nozzles, anchors, text, labels)
to embedded SVG symbols.

## Core Rule

> Every pixel of every drawn element — including invisible anchors — must
> lie strictly inside the viewBox rectangle.

The viewBox `"min-x min-y width height"` defines the clip window.
Anything outside this rectangle is removed without error.

## Checklist When Creating or Editing a Symbol

1. **Inventory all geometry extents** — list every element in the symbol
   (paths, rects, circles, text). Find the min/max x and y across all
   elements.

2. **Add margin** — expand the viewBox by at least 2–4 px beyond those
   extents. A bottom anchor at y=56 needs a viewBox bottom that is > 56,
   not = 56.

3. **Verify anchors are inside** — invisible `<circle>` anchors used for
   pipe routing must also be inside the viewBox. Anchors outside the
   viewBox can never be reached by a router.

4. **Verify container symbols** — if a compositing symbol (e.g. a bank
   of 4 accumulators) contains sub-symbols, compute the widest/tallest
   extent across all sub-symbol instances plus any standalone features
   (text, ports). Set the viewBox to enclose that full bounding box.

5. **Update the useAt call** — in the Go renderer, the `<use>` dimensions
   must match the viewBox exactly so no scaling is introduced. Formula:
   `useAt(id, docX - viewBoxMinX, docY - viewBoxMinY, viewBoxWidth, viewBoxHeight)`.

6. **Regenerate and grep** — after editing:
   ```bash
   go run ./cmd/nsim/ -svg cmd/nsim/out.svg
   grep -A2 '<g data-part=' cmd/nsim/out.svg | grep <symbol-id>
   ```
   Confirm the rendered `<use>` has the correct x, y, width, height
   and that downstream elements (e.g. a label) are not overlapping.

7. **Run tests** — `go test ./core/svg/ -v` must pass. Tests verify symbol
   presence, not viewBox exact values, so changing a viewBox will not
   break tests unless the symbol id is renamed or removed.

## Common Off-by-N Patterns

- Anchor at `cy=56` but viewBox `height=80` starting at `y=-40` → bottom
  at `y=40`. Clip by 16 px. Fix: expand viewBox to start lower or add
  height.
- Nozzle text drawn at `y=-58` but viewBox top at `y=-50`. Fix: expand
  viewBox min-y to `-58` plus margin.
- Sub-symbol extends to `x=130` but viewBox width=200 from min-x=-100
  gives right edge at `x=100`. Fix: increase width or shift min-x.