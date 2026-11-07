# Skill: Updating the Web UI

Use this skill whenever the task asks to add, remove, or change a
visible element in the `web/` dashboard (buttons, panels, widgets,
tooltips, layout). It captures the conventions and pitfalls
discovered while adding the "Export Plant" button to the command
console.

## Core principle

**The shipped UI is the Vite bundle, not the TypeScript source.**  
Always verify the final `dist/assets/index-*.js` and
`dist/assets/index-*.css` contain the intended strings and classes.
Dev-mode behavior (`make dev`) can diverge silently because `src/`
contains stale `.js` shadow files that Vite/Rollup may resolve
before `.ts`.

## Directory contracts

| Path | Role |
|---|---|
| `web/src/*.ts` | Canonical source. Edit these. |
| `web/src/*.js` | Gitignored. Rollup must resolve `.ts` because `.js` is shadowed. Delete any that appear; tsc emits to `web/js/` instead. |
| `web/js/*.js` | Gitignored tsc emit directory (`outDir: "js"`). Contains `wasm_exec.js` (copied by Makefile) and any other emitted JS. Do not edit. |
| `web/src/styles.css` | Single stylesheet for the whole app. All component styling lives here. |
| `web/public/` | Static assets copied verbatim into `dist/` by Vite. |
| `web/dist/` | Vite build output. Inspect this to confirm what ships. |

## The ".js shadow" rule

Rollup resolves imports in this order: `.js` before `.ts`. If
`web/src/console.js` exists alongside `web/src/console.ts`, Rollup
may bundle the `.js` even though the project convention is `.ts`.

**Delete any `.js` twin so Rollup must resolve `.ts`.** `tsc` now emits to `web/js/` (`outDir: "js"` in `tsconfig.json`), so `.js` files in `src/` are always stale shadows. Delete them. If `tsc` or another tool regenerates them, delete again; do not edit them.

Quick check:
```sh
ls src/*.js
```
Any output means stale shadows exist in `src/`. Delete them. Do not edit them.

## How to add a UI element (pattern from "Export Plant")

### 1. Find the nearest existing sibling

The command console header already had:
- A `<h2>` title
- An **Export log (.json)** button (`#btn-cmd-export`)
- A textarea + Submit button below

New buttons should live in the same container (`cmd-header`) and
follow the same visual pattern.

### 2. DOM structure in the TypeScript source

```typescript
// In web/src/console.ts (or the relevant file):

const exportPlantBtn = document.createElement("button");
exportPlantBtn.id = "btn-cmd-export-plant";
exportPlantBtn.textContent = "\u2b07 Export Plant (.json)";
exportPlantBtn.setAttribute("aria-label", "Export current plant state as JSON file");
exportPlantBtn.title = "Download the current plant state as a JSON file (distinct from the command log export).";
exportPlantBtn.addEventListener("click", () => {
    const raw = plant.toJSON();  // or whatever data source
    const blob = new Blob([raw], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "nsim-plant.json";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    errorLine.textContent = "";
});

// Group related buttons together
const btnGroup = document.createElement("div");
btnGroup.className = "cmd-export-btns";
btnGroup.appendChild(exportBtn);           // existing button
btnGroup.appendChild(exportPlantBtn);      // new button
header.appendChild(btnGroup);
```

### 3. CSS conventions

Add styles in `web/src/styles.css` near the related component:

```css
/* Group container: flex row, right-justified */
.cmd-export-btns {
    display: flex;
    gap: 8px;
    margin-left: auto;
}

/* Button base style — copy from the nearest existing button */
#btn-cmd-export-plant {
    background: #1e6b52;
    color: white;
    border: 1px solid #14503c;
    padding: 3px 10px;
    cursor: pointer;
    font-size: 12px;
    white-space: nowrap;
}
#btn-cmd-export-plant:hover { background: #2a8a6b; }
```

Observed patterns:
- Header rows use `display: flex; align-items: center; justify-content: space-between;`
- Button groups that should sit together and flush right use a flex child with `margin-left: auto`
- Export buttons get a dark-green (`#1e6b52`) background with `hover` lightening to `#2a8a6b`
- Padding is `3px 10px`, font-size `12px`, `white-space: nowrap` to keep labels on one line
- Unicode symbols precede labels: `\u2b07` (⬇) for downloads

### 4. Delete the `.js` shadow

Rollup resolves `.js` before `.ts`. Because `web/src/*.js` is gitignored, **the only reliable fix is to delete the `.js` twin** rather than try to keep it in sync. Verify Rollup still produces a clean bundle:
```sh
rm web/src/<module>.js
npx vite build --outDir dist
```

### 5. Verify the bundle

```sh
# Check the JS chunk contains the button text and id
rg -o 'Export Plant.*json\|btn-cmd-export-plant' dist/assets/index-*.js

# Check the CSS chunk contains the new rules
rg -o 'cmd-export-btns\|btn-cmd-export-plant' dist/assets/index-*.css
```

If either search returns nothing, the change did not make it into the
bundle. Re-inspect the `.ts` source and make sure no `.js` shadow
is resurfacing.

## Common layout patterns seen in the app

| Pattern | CSS |
|---|---|
| Header row with title left, actions right | `.cmd-header { display: flex; align-items: center; justify-content: space-between; }` |
| Button group flush right | Wrapper `<div class="cmd-export-btns">` with `display: flex; gap: 8px; margin-left: auto;` |
| Tile heading with minimize button | Flex row; heading grows, button is `margin-left: auto` |
| Form row | `.cmd-input-row` with textarea + submit button; flex or block |

## Adding a new top-level panel (heavier change)

If the task requires a new panel region (like alarms, console, tiles):

1. Add a mount point in `web/index.html`:
   ```html
   <section id="my-panel"></section>
   ```
2. Create `web/src/my-panel.ts`. Do **not** create a `.js` twin — `.js` files in `src/` are gitignored and will only resurface if someone runs bare `tsc`.
3. Call it from `web/src/main.ts` during bootstrap, passing the
   element by id.
4. Add CSS for the new panel in `web/src/styles.css`.
5. Build and inspect `dist/` to confirm inclusion.

## Testing the UI change

- `make dev` is not sufficient. The dev server serves `src/` directly
  and may load the stale `.js` instead of the edited `.ts`.
- Always run `npx vite build --outDir dist` and inspect the emitted
  bundle. That is what `make web.tgz` ships.
- Optional: extract `web.tgz` and serve it with a static file server
  from a subpath (e.g. `/nsim-web/`) to confirm relative URLs still
  resolve.

## Checklist before calling a web UI change done

- [ ] `web/src/*.ts` edited with the new component or behavior.
- [ ] `web/src/styles.css` updated with rules that match the
      existing visual conventions (colors, spacing, font size).
- [ ] No stale `.js` shadows left in `web/src/` (`ls src/*.js` should be empty). `wasm_exec.js` lives in `web/js/` now.
- [ ] `npx vite build --outDir dist` succeeds.
- [ ] `rg` confirms the new id / label / class appears in
      `dist/assets/index-*.js` and `dist/assets/index-*.css`.
- [ ] No root-absolute `fetch("/...")` paths introduced in the
      new or modified code (use `./` or `import.meta.env.BASE_URL`).
- [ ] If the change adds a button that triggers an action, the
      action uses the same Blob-download pattern as **Export log**
      unless there is a specific reason to diverge.

## Part-path wrapping in tile headings

Long part paths like `EmergencyFeedwaterSystem/SubsystemA/TurbinePump`
can exceed container widths in tile headings. The correct fix is
**word-break opportunities at slashes**, applied centrally in
`displayPart` (`bindings.ts`), so every heading inherits the behavior
without touching individual labels or controls:

```typescript
export function displayPart(prefix: string): PartDisplay {
    const head = prefix + "/";
    return (part: string): string =>
        part.startsWith(head)
            ? part.slice(head.length).replace(/\//g, "/<wbr>")
            : part.replace(/\//g, "/<wbr>");
}
```

Because the return value contains `<wbr>` tags, **use `innerHTML`** when
rendering it, e.g.:

```typescript
h.innerHTML = display(part.part);
```

Using `textContent` will HTML-escape the `<wbr>` and display it as
literal text.

**Do not** apply wrapping to control/sensor labels (e.g. `BreakerClosed`).
Those strings are used for sorting, cross-reference lookups, and screen
readers; mutating them breaks those paths. Labels stay plain.

## Help-popup behavior

Cross-reference links inside a `?` description should:
1. Keep the popup open and unchanged.
2. If the target has a rendered control/sensor row, scroll it into view and flash it (`revealElement`).
3. If no rendered row exists, swap the popup to the referenced description.
4. Closing the popup remains an explicit click on the `?` button.

Test with `vitest` + `happy-dom` (see `help-button.dom.test.ts`).

## Anti-patterns to avoid

- Editing only `*.ts` and leaving `*.js` stale. The production bundle
  may contain the old `.js` logic.
- Using root-absolute paths in `fetch()`, `<a href>`, or `<img src>`.
  They break when the bundle is hosted under any subpath.
- Inventing new colors or spacing scales. Reuse the green export
  buttons, the `8px` gap, the `12px` button font.
- Adding a stylesheet or `<style>` block inline. All CSS lives in
  `src/styles.css` so Vite can bundle it.
- Relying on `make dev` as the final test. It overlays source files
  and can mask `.js` shadow problems.

## Repo-specific references

- Existing export button: `web/src/console.ts:60-88` (log) and
  `web/src/console.ts:94-118` (plant).
- Button group CSS: `web/src/styles.css:345-380` (`.cmd-header` and
  `#btn-cmd-export` rules).
- Vite config: `web/vite.config.ts` (`base: "./"`, `publicDir:
  "public"`).
- Build skill: `plans/WEBBUILD-SKILL.md` for the full dev-vs-prod
  divergence guide.