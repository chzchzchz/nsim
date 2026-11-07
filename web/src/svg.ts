// SVG-content boundary: mountPlant() replaces diagram markup and owns the
// one-time zoom-overlay shell. It does not own runtime viewport state.
//
// PidViewport is the single owner for pane height, pan/zoom behavior,
// viewport persistence, resize listeners, and canvas transform state.
// This module calls those public methods but should not duplicate state
// here, mutate the canvas directly, register pan/height listeners, or
// create the resize surface. The static #pid/.pid-wrap/.pid-resize-handle
// structure lives in index.html.
//
// Compatibility note: keep mountPlant(target, svgText) as the call-site
// API used by main.ts. Refactoring the viewport internals should not
// require refresh-loop changes in main.ts.

import { PidViewport } from "./pidViewport";

let viewport: PidViewport | null = null;

export function mountPlant(target: HTMLElement, svgText: string): void {
    if (!viewport) {
        viewport = new PidViewport(target);
        target.appendChild(buildZoomOverlay(target));
    }
    viewport.mountSvg(svgText);
}

function buildZoomOverlay(target: HTMLElement): HTMLElement {
    const bar = document.createElement("div");
    bar.className = "pid-zoom";
    const mk = (
        label: string,
        title: string,
        onClick: () => void,
    ): HTMLButtonElement => {
        const btn = document.createElement("button");
        btn.textContent = label;
        btn.title = title;
        btn.addEventListener("click", onClick);
        return btn;
    };
    const BUTTON_FACTOR = 1.25;
    bar.appendChild(mk("−", "Zoom out", () => viewport?.zoomFactor(1 / BUTTON_FACTOR)));
    bar.appendChild(mk("＋", "Zoom in", () => viewport?.zoomFactor(BUTTON_FACTOR)));
    bar.appendChild(mk("↺", "Reset view", () => viewport?.resetView()));
    return bar;
}
