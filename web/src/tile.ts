// tile.ts — shared scaffolding for dashboard panels: a heading
// with the prefix label plus a minimize toggle, and a body host
// for panel content. Used by both the prefix tiles in main.ts and
// the alarm panel so every panel collapses the same way.

const MINIMIZE_GLYPH = "−";
const RESTORE_GLYPH = "+";

export interface Tile {
    root: HTMLElement;
    body: HTMLElement;
}

// makeTile assembles the collapsed-state container. The toggle
// button lives inside the <h2> so it stays visible when the body
// is hidden; presses on it are excluded from the drag-to-swap
// gesture (see grid.ts).
export function makeTile(title: string): Tile {
    const root = document.createElement("div");
    root.className = "tile";

    const h = document.createElement("h2");
    const label = document.createElement("span");
    label.textContent = title;
    h.appendChild(label);
    h.appendChild(renderMinimizeToggle(root));
    root.appendChild(h);

    const body = document.createElement("div");
    body.className = "tile-body";
    root.appendChild(body);

    return { root, body };
}

function renderMinimizeToggle(tile: HTMLElement): HTMLElement {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "tile-toggle";
    btn.textContent = MINIMIZE_GLYPH;
    btn.title = "Minimize panel";
    btn.setAttribute("aria-label", btn.title);
    btn.addEventListener("click", () => {
        const minimized = tile.classList.toggle("minimized");
        btn.textContent = minimized ? RESTORE_GLYPH : MINIMIZE_GLYPH;
        btn.title = minimized ? "Restore panel" : "Minimize panel";
        btn.setAttribute("aria-label", btn.title);
    });
    return btn;
}
