// svg.ts — mount the SVG document produced by the wasm side.
// The diagram is stateless markup; part highlighting binds via
// data-part attributes, so remounting never loses handlers.

import { REF_FLASH_MS, revealPart } from "./bindings";

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

// The viewpoint survives reloads through localStorage, versioned so
// a schema change discards stale blobs instead of misreading them.
const VIEW_STORAGE_KEY = "nsim-pid-view";
const VIEW_VERSION = 1;

interface PidView {
    version: number;
    scale: number;
    panX: number;
    panY: number;
}

function loadView(): PidView | null {
    let raw: string | null;
    try {
        raw = localStorage.getItem(VIEW_STORAGE_KEY);
    } catch {
        return null;
    }
    if (raw === null) return null;
    let parsed: unknown;
    try {
        parsed = JSON.parse(raw);
    } catch {
        return null;
    }
    const v = parsed as Partial<PidView>;
    if (
        v.version !== VIEW_VERSION ||
        typeof v.scale !== "number" ||
        typeof v.panX !== "number" ||
        typeof v.panY !== "number" ||
        !Number.isFinite(v.scale) ||
        !Number.isFinite(v.panX) ||
        !Number.isFinite(v.panY)
    ) {
        return null;
    }
    return {
        version: VIEW_VERSION,
        scale: Math.min(MAX_SCALE, Math.max(MIN_SCALE, v.scale)),
        panX: v.panX,
        panY: v.panY,
    };
}

function saveView(): void {
    const view: PidView = {
        version: VIEW_VERSION,
        scale,
        panX,
        panY,
    };
    try {
        localStorage.setItem(VIEW_STORAGE_KEY, JSON.stringify(view));
    } catch {
        // Private-browsing quotas leave the viewpoint session-only.
    }
}

let canvas: HTMLDivElement | null = null;
let scale = 1;
let panX = 0;
let panY = 0;
// Restoration runs once per page load; later mounts re-apply the
// live module state instead of racing fresh reads.
let restored = false;

const boundHosts = new WeakSet<HTMLElement>();

export function mountPlant(target: HTMLElement, svgText: string): void {
    if (!boundHosts.has(target)) {
        if (!restored) {
            restored = true;
            const view = loadView();
            if (view) {
                scale = view.scale;
                panX = view.panX;
                panY = view.panY;
            }
        }
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
    saveView();
}

function resetView(): void {
    scale = 1;
    panX = 0;
    panY = 0;
    applyTransform();
    saveView();
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
    // Capture is deferred until travel exceeds the click threshold:
    // capturing immediately would retarget the compatibility click
    // event to the host and hide the component that was clicked.
    let pressed = false;
    let dragging = false;
    let lastX = 0;
    let lastY = 0;
    target.addEventListener("pointerdown", (ev) => {
        downX = ev.clientX;
        downY = ev.clientY;
        pressed = ev.button === 0 &&
            !(ev.target as HTMLElement | null)?.closest(".pid-zoom");
        if (pressed) {
            lastX = ev.clientX;
            lastY = ev.clientY;
        }
    });
    target.addEventListener("pointermove", (ev) => {
        if (!pressed) return;
        if (!dragging) {
            if (
                Math.hypot(ev.clientX - downX, ev.clientY - downY) <
                    CLICK_SUPPRESS_PX
            ) {
                return;
            }
            dragging = true;
            target.setPointerCapture(ev.pointerId);
        }
        panX += ev.clientX - lastX;
        panY += ev.clientY - lastY;
        lastX = ev.clientX;
        lastY = ev.clientY;
        applyTransform();
    });
    const endDrag = (ev: PointerEvent): void => {
        pressed = false;
        if (!dragging) return;
        dragging = false;
        saveView();
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
        const g = (ev.target as Element | null)?.closest("[data-part]");
        const part = g?.getAttribute("data-part");
        if (!g || !part) return;
        if (revealPart(part)) return;
        // Components without bridge rows (e.g. the heater drain
        // pump has no bindings yet) still acknowledge the click so
        // it does not feel dead.
        g.classList.add("ref-flash");
        window.setTimeout(
            () => g.classList.remove("ref-flash"),
            REF_FLASH_MS,
        );
    });
}
