// tile.ts — shared scaffolding for dashboard panels: a heading
// with the prefix label plus a minimize toggle, and a body host
// for panel content. Used by both the prefix tiles in main.ts and
// the alarm panel so every panel collapses the same way.
//
// Also owns the shared tile-group builder used by the local plant
// (main.ts) and remote mode (remoteController.ts) so order,
// layout, and minimized-state restore cannot drift apart.

import type { SensorRow } from "./sensors";

const MINIMIZE_GLYPH = "−";
const RESTORE_GLYPH = "+";
export interface Tile {
    root: HTMLElement;
    body: HTMLElement;
}

// makeTile assembles the collapsed-state container. The toggle
// button lives inside the <h2> so it stays visible when the body
// is hidden; presses on it are excluded from the drag-to-swap
// gesture (see grid.ts). The title doubles as the stable layout
// persistence id via data-panel.
export function makeTile(title: string): Tile {
    const root = document.createElement("div");
    root.className = "tile";
    root.dataset.panel = title;

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

// applyMinimized drives a tile's collapsed state from code (e.g.
// restoring a saved layout); the header toggle calls it too so the
// button glyph always matches the class.
export function applyMinimized(tile: HTMLElement, minimized: boolean): void {
    tile.classList.toggle("minimized", minimized);
    const btn = tile.querySelector<HTMLElement>(".tile-toggle");
    if (!btn) {
        return;
    }
    btn.textContent = minimized ? RESTORE_GLYPH : MINIMIZE_GLYPH;
    const label = minimized ? "Restore panel" : "Minimize panel";
    btn.title = label;
    btn.setAttribute("aria-label", label);
}

function renderMinimizeToggle(tile: HTMLElement): HTMLElement {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "tile-toggle";
    btn.textContent = MINIMIZE_GLYPH;
    btn.title = "Minimize panel";
    btn.setAttribute("aria-label", btn.title);
    btn.addEventListener("click", () => {
        const minimized = !tile.classList.contains("minimized");
        applyMinimized(tile, minimized);
        // Bubble through #tiles so main.ts persists the layout.
        tile.dispatchEvent(
            new CustomEvent("panel-layout-changed", { bubbles: true }),
        );
    });
    return btn;
}



// buildTile creates a single Controls|Sensors tile for one prefix
// group via the supplied render callbacks and appends it to host.
// Returns the tile root so callers can track sensor-row rows.
export function buildTile(
    host: HTMLElement,
    prefix: string,
    renderControls: (host: HTMLElement) => void,
    renderSensors: (host: HTMLElement) => SensorRow[],
    sensorRows: SensorRow[],
): HTMLElement {
    const { root, body } = makeTile(prefix || "(other)");
    host.appendChild(root);

    const ctlCol = document.createElement("div");
    const ctlH = document.createElement("h3");
    ctlH.textContent = "Controls";
    ctlCol.appendChild(ctlH);
    const ctlHost = document.createElement("div");
    ctlCol.appendChild(ctlHost);
    renderControls(ctlHost);

    const senCol = document.createElement("div");
    const senH = document.createElement("h3");
    senH.textContent = "Sensors";
    senCol.appendChild(senH);
    const senHost = document.createElement("div");
    senCol.appendChild(senHost);
    sensorRows.push(...renderSensors(senHost));

    body.appendChild(ctlCol);
    body.appendChild(senCol);
    return root;
}
