// svg.ts — mount the SVG document produced by the wasm side.
// The diagram is stateless markup; part highlighting binds via
// data-part attributes, so remounting never loses handlers.

import { revealPart } from "./bindings";

// Zoom state lives here rather than on the mounted <svg> because
// refreshes replace that node wholesale; re-applying the same
// transform after each mount keeps the operator's viewpoint stable
// across ticks.
const MIN_SCALE = 1;
const MAX_SCALE = 12;
const WHEEL_FACTOR = 1.1;
const BUTTON_FACTOR = 1.25;
// A pan gesture ends with a browser click event; jumps from
// clicking a component are suppressed beyond this travel so
// dragging the mimic never triggers navigation.
const CLICK_SUPPRESS_PX = 5;

let canvas: HTMLDivElement | null = null;
let scale = 1;
let panX = 0;
let panY = 0;

const boundHosts = new WeakSet<HTMLElement>();

export function mountPlant(target: HTMLElement, svgText: string): void {
    if (!boundHosts.has(target)) {
        const viewport = document.createElement("div");
        viewport.className = "pid-canvas";
        target.appendChild(viewport);
        target.appendChild(buildZoomOverlay(target));
        bindZoomGestures(target);
        canvas = viewport;
    }
    canvas!.innerHTML = svgText;
    applyTransform();
}

function applyTransform(): void {
    canvas!.style.transform =
        `translate(${panX}px, ${panY}px) scale(${scale})`;
}

function zoomAt(
    target: HTMLElement,
    clientX: number,
    clientY: number,
    factor: number,
): void {
    const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale * factor));
    if (next === scale) return;
    const rect = target.getBoundingClientRect();
    // The canvas is laid out centred inside the host, so the scaled
    // coordinate space starts at the canvas corner, not the host
    // corner. offsetLeft/offsetTop ignore transforms and therefore
    // report that true origin.
    const px = clientX - rect.left - canvas!.offsetLeft;
    const py = clientY - rect.top - canvas!.offsetTop;
    // Keep the diagram point under the cursor fixed while the
    // scale changes.
    panX = px - ((px - panX) / scale) * next;
    panY = py - ((py - panY) / scale) * next;
    scale = next;
    if (scale === MIN_SCALE) {
        panX = 0;
        panY = 0;
    }
    applyTransform();
}

function resetView(): void {
    scale = 1;
    panX = 0;
    panY = 0;
    applyTransform();
}

function buildZoomOverlay(target: HTMLElement): HTMLElement {
    const bar = document.createElement("div");
    bar.className = "pid-zoom";
    const mk = (
        label: string,
        title: string,
        onClick: (ev: MouseEvent) => void,
    ): HTMLButtonElement => {
        const btn = document.createElement("button");
        btn.textContent = label;
        btn.title = title;
        btn.addEventListener("click", onClick);
        return btn;
    };
    const centerFactor = (factor: number): void => {
        const rect = target.getBoundingClientRect();
        zoomAt(target, rect.left + rect.width / 2, rect.top + rect.height / 2, factor);
    };
    bar.appendChild(mk("−", "Zoom out", () => centerFactor(1 / BUTTON_FACTOR)));
    bar.appendChild(mk("＋", "Zoom in", () => centerFactor(BUTTON_FACTOR)));
    bar.appendChild(mk("↺", "Reset view", () => resetView()));
    return bar;
}

function bindZoomGestures(target: HTMLElement): void {
    boundHosts.add(target);

    target.addEventListener("wheel", (ev) => {
        ev.preventDefault();
        const factor = ev.deltaY < 0 ? WHEEL_FACTOR : 1 / WHEEL_FACTOR;
        zoomAt(target, ev.clientX, ev.clientY, factor);
    }, { passive: false });

    let downX = 0;
    let downY = 0;
    let dragging = false;
    let lastX = 0;
    let lastY = 0;
    target.addEventListener("pointerdown", (ev) => {
        downX = ev.clientX;
        downY = ev.clientY;
        if (ev.button !== 0) return;
        // Let the zoom overlay keep its own click behaviour.
        if ((ev.target as HTMLElement | null)?.closest(".pid-zoom")) return;
        dragging = true;
        lastX = ev.clientX;
        lastY = ev.clientY;
        target.setPointerCapture(ev.pointerId);
    });
    target.addEventListener("pointermove", (ev) => {
        if (!dragging) return;
        panX += ev.clientX - lastX;
        panY += ev.clientY - lastY;
        lastX = ev.clientX;
        lastY = ev.clientY;
        applyTransform();
    });
    const endDrag = (ev: PointerEvent): void => {
        if (!dragging) return;
        dragging = false;
        if (target.hasPointerCapture(ev.pointerId)) {
            target.releasePointerCapture(ev.pointerId);
        }
    };
    target.addEventListener("pointerup", endDrag);
    target.addEventListener("pointercancel", endDrag);

    // Clicking a diagram component jumps to the control that can
    // operate it, mirroring how (?) popups cross-reference rows.
    target.addEventListener("click", (ev) => {
        if (
            Math.hypot(ev.clientX - downX, ev.clientY - downY) >
                CLICK_SUPPRESS_PX
        ) {
            return;
        }
        const part = (ev.target as Element | null)?.closest("[data-part]")
            ?.getAttribute("data-part");
        if (part) revealPart(part);
    });
}
