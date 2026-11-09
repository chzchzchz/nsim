// P&ID viewport ownership
// ========================
//
// PidViewport is the single owner for all state and behavior around the
// P&ID surface: pane height, the resize handle, pan/zoom gestures,
// viewport persistence, and DOM-listener disposal.
//
// Responsibilities and boundaries:
// - PidViewport owns viewport state: heightVh, scale, panX, and panY.
// - It is the sole persistence writer for the versioned PID_VIEWPORT_KEY.
// - It observes pane-size changes and re-anchors pan offsets when height
//   changes, preserving the operator's selected scale.
// - It owns listeners registered through its private on() helper and the
//   ResizeObserver. dispose() removes all of them.
// - It expects index.html to provide the static structure:
//     <div class="pid-wrap">
//       <div id="pid" aria-label="P&ID diagram"></div>
//       <div class="pid-resize-handle"></div>
//     </div>
//   It does not create the .pid-wrap container. If the handle is absent,
//   it creates that handle as a defensive fallback and then treats it like
//   the markup-provided one.
// - It creates and owns its internal .pid-canvas element. Callers refresh
//   SVG content through mountSvg() rather than replacing the canvas or
//   manipulating its transform directly.
// - It does not own the zoom button UI. svg.ts mounts that UI and calls
//   this class's public zoomFactor() and resetView() methods.
// - It may reveal and flash a referenced control after a component click,
//   but it does not own control-panel bindings.
//
// Cross-module rules:
// - main.ts should pass #pid and SVG text to mountPlant(). It should not
//   manage --pid-height, .pid-wrap, .pid-resize-handle, pan offsets, or the
//   .pid-canvas transform directly.
// - svg.ts should remain a thin compatibility layer: create at most one
//   PidViewport, append the zoom overlay once, and delegate subsequent
//   SVG updates to mountSvg().
// - styles.css provides presentation for #pid, .pid-wrap, dimensions, and
//   the handle, but it is not a state owner. PidViewport.state is the
//   canonical runtime source of truth, reflected through the
//   --pid-height CSS variable.
// - Anything keyed by height, scale, pan, persistence, resize, or canvas
//   transforms belongs in this module rather than leaking into svg.ts,
//   main.ts, or page-refresh logic.
//
// Invariants:
// - Viewport state is always validated and clamped before it reaches
//   localStorage, the resize calculation, or the CSS variable.
// - Changing height must never reset the selected zoom scale. Size changes
//   only recompute pan offsets so the centered diagram remains anchored.
// - Zero-height layout reads are ignored. They describe pre-layout DOM,
//   not a requested view; recording them would overwrite the restored
//   viewpoint.
// - Gesture listeners and the ResizeObserver must be removable. Reuse
//   dispose() when tearing down a viewport; do not add untracked listeners
//   to this surface.
// - Legacy nsim-pid-height and nsim-pid-view values are a migration input,
//   not an active API. Mutation belongs under PID_VIEWPORT_KEY.

import { REF_FLASH_MS, revealPart } from "./bindings";

export const PID_VIEWPORT_KEY = "nsim-pid-viewport";
export const PID_VIEWPORT_VERSION = 1;

export interface PidViewportState {
    version: number;
    heightVh: number;
    scale: number;
    panX: number;
    panY: number;
}

export const MIN_SCALE = 1;
export const MAX_SCALE = 12;
export const MIN_HEIGHT_VH = 20;
export const MAX_HEIGHT_VH = 90;
export const DEFAULT_HEIGHT_VH = 50;

export function clampHeightVh(v: number): number {
    if (!Number.isFinite(v)) return DEFAULT_HEIGHT_VH;
    return Math.min(MAX_HEIGHT_VH, Math.max(MIN_HEIGHT_VH, v));
}

export function clampScale(s: number): number {
    if (!Number.isFinite(s)) return 1;
    return Math.min(MAX_SCALE, Math.max(MIN_SCALE, s));
}

// anchoredPan (pure): keeps the diagram point under (px, py) fixed
// while the scale moves from prevScale to nextScale.
export function anchoredPan(
    px: number,
    py: number,
    prevScale: number,
    nextScale: number,
    prevPanX: number,
    prevPanY: number,
): { x: number; y: number } {
    return {
        x: px - ((px - prevPanX) / prevScale) * nextScale,
        y: py - ((py - prevPanY) / prevScale) * nextScale,
    };
}

// snapHeightForResize: a drag's pixel delta to vh using the live
// viewport height (not a fixed assumption).
export function snapHeightForResize(
    deltaPx: number,
    viewportHeightPx: number,
    currentVh: number,
): number {
    if (!Number.isFinite(deltaPx) || !Number.isFinite(viewportHeightPx) || viewportHeightPx <= 0) {
        return clampHeightVh(currentVh);
    }
    return clampHeightVh(currentVh + (deltaPx / viewportHeightPx) * 100);
}

function defaultState(): PidViewportState {
    return {
        version: PID_VIEWPORT_VERSION,
        heightVh: DEFAULT_HEIGHT_VH,
        scale: 1,
        panX: 0,
        panY: 0,
    };
}

function validate(raw: unknown): PidViewportState | null {
    if (typeof raw !== "object" || raw === null) return null;
    const v = raw as Partial<PidViewportState>;
    if (
        v.version !== PID_VIEWPORT_VERSION ||
        typeof v.heightVh !== "number" ||
        typeof v.scale !== "number" ||
        typeof v.panX !== "number" ||
        typeof v.panY !== "number" ||
        !Number.isFinite(v.heightVh) ||
        !Number.isFinite(v.scale) ||
        !Number.isFinite(v.panX) ||
        !Number.isFinite(v.panY)
    ) {
        return null;
    }
    return {
        version: PID_VIEWPORT_VERSION,
        heightVh: clampHeightVh(v.heightVh),
        scale: clampScale(v.scale),
        panX: v.panX,
        panY: v.panY,
    };
}

const LEGACY_HEIGHT_KEY = "nsim-pid-height";
const LEGACY_VIEW_KEY = "nsim-pid-view";

function getItem(key: string): string | null {
    try {
        return localStorage.getItem(key);
    } catch {
        return null;
    }
}

export function loadViewportState(): PidViewportState {
    const raw = getItem(PID_VIEWPORT_KEY);
    if (raw !== null) {
        try {
            const v = validate(JSON.parse(raw));
            if (v) return v;
        } catch {
            // fall through to legacy migration
        }
    }
    // One-time migration from the two legacy keys (height + view).
    const legacyH = getItem(LEGACY_HEIGHT_KEY);
    const legacyV = getItem(LEGACY_VIEW_KEY);
    if (legacyH !== null || legacyV !== null) {
        const def = defaultState();
        const heightVh = legacyH !== null ? clampHeightVh(Number(legacyH)) : def.heightVh;
        let scale = def.scale, panX = def.panX, panY = def.panY;
        if (legacyV !== null) {
            try {
                const p = JSON.parse(legacyV) as Partial<PidViewportState>;
                if (typeof p.scale === "number" && Number.isFinite(p.scale)) {
                    scale = clampScale(p.scale);
                }
                if (typeof p.panX === "number" && Number.isFinite(p.panX)) panX = p.panX;
                if (typeof p.panY === "number" && Number.isFinite(p.panY)) panY = p.panY;
            } catch {
                // ignore malformed legacy view
            }
        }
        return { ...def, heightVh, scale, panX, panY };
    }
    return defaultState();
}

export function saveViewportState(s: PidViewportState): void {
    try {
        localStorage.setItem(PID_VIEWPORT_KEY, JSON.stringify({
            ...s,
            heightVh: clampHeightVh(s.heightVh),
            scale: clampScale(s.scale),
        }));
    } catch {
        /* session-only */
    }
}

/**
 * The single owner for the P&ID viewport surface.
 *
 * Construct once per pane: it binds the handle, pan/zoom gestures, the
 * ResizeObserver, and presentation state for this surface. Retain the
 * instance for zoomFactor(), resetView(), and dispose(); do not re-create
 * it on every SVG refresh. See the ownership contract at the top of this
 * file before changing responsibilities, persistence keys, or markup
 * expectations.
 */
export class PidViewport {
    private readonly pane: HTMLElement;
    private handle: HTMLElement;
    private canvas: HTMLDivElement;
    private state: PidViewportState;
    private resizer: ResizeObserver | null = null;
    private disposers: Array<() => void> = [];
    private appliedH = 0;

    constructor(pane: HTMLElement) {
        const wrap = pane.closest(".pid-wrap");
        if (!wrap) throw new Error("PidViewport: pane must live inside .pid-wrap (see index.html)");
        this.pane = pane;

        this.state = loadViewportState();
        this.applyHeight();

        // Handle: lives in the markup (index.html); if absent,
        // create it so the surface is always resizable.
        this.handle = wrap.querySelector<HTMLElement>(".pid-resize-handle")
            ?? this.createHandle(wrap);

        this.canvas = document.createElement("div");
        this.canvas.className = "pid-canvas";
        pane.appendChild(this.canvas);

        this.bindHandle();
        this.bindGestures();
        this.bindResizer();
    }

    // TRUST BOUNDARY: svgText must be wasm-generated (either from
    // the local plant or from a plant reconstructed from remote
    // JSON via plantFromJSON). Never fetch raw SVG markup from a
    // remote server — server SVG would need DOMParser parsing with
    // <script>/<foreignObject> stripping before any innerHTML
    // assignment (see sanitize.ts).
    mountSvg(svgText: string): void {
        this.canvas.innerHTML = svgText;
        this.applyTransform();
    }

    zoomFactor(factor: number): void {
        const rect = this.pane.getBoundingClientRect();
        this.zoomAt(rect.left + rect.width / 2, rect.top + rect.height / 2, factor);
    }

    resetView(): void {
        this.state.scale = 1;
        this.state.panX = 0;
        this.state.panY = 0;
        this.applyTransform();
        this.save();
    }

    currentHeightPx(): number {
        return this.pane.getBoundingClientRect().height;
    }

    applyHeight(): void {
        document.documentElement.style.setProperty(
            "--pid-height",
            `${clampHeightVh(this.state.heightVh)}svh`,
        );
    }

    snapHeight(deltaPx: number): void {
        this.state.heightVh = snapHeightForResize(
            deltaPx, window.innerHeight, this.state.heightVh,
        );
        this.applyHeight();
        this.save();
    }

    private applyTransform(): void {
        this.canvas.style.transform =
            `translate(${this.state.panX}px, ${this.state.panY}px) scale(${this.state.scale})`;
    }

    private save(): void {
        saveViewportState(this.state);
    }

    private createHandle(wrap: Element): HTMLElement {
        const el = document.createElement("div");
        el.className = "pid-resize-handle";
        el.title = "Drag to resize the P&ID pane";
        wrap.appendChild(el);
        return el;
    }

    private on<K extends keyof HTMLElementEventMap>(
        el: HTMLElement,
        type: K,
        handler: (ev: HTMLElementEventMap[K]) => void,
        opts?: AddEventListenerOptions,
    ): void {
        el.addEventListener(type, handler as EventListener, opts);
        this.disposers.push(() =>
            el.removeEventListener(type, handler as EventListener, opts),
        );
    }

    private bindHandle(): void {
        let dragging = false;
        let startY = 0;
        let startVh = this.state.heightVh;
        this.on(this.handle, "pointerdown", (ev: PointerEvent) => {
            if (ev.button !== 0) return;
            ev.stopPropagation();
            dragging = true;
            startY = ev.clientY;
            startVh = this.state.heightVh;
            try { this.handle.setPointerCapture(ev.pointerId); } catch { /* noop */ }
        });
        this.on(this.handle, "pointermove", (ev: PointerEvent) => {
            if (!dragging) return;
            this.state.heightVh = snapHeightForResize(
                ev.clientY - startY, window.innerHeight, startVh,
            );
            this.applyHeight();
        });
        const end = (): void => {
            if (!dragging) return;
            dragging = false;
            this.save();
        };
        this.on(this.handle, "pointerup", end);
        this.on(this.handle, "pointercancel", end);
    }

    private bindResizer(): void {
        this.resizer = new ResizeObserver(() => this.syncPane());
        this.resizer.observe(this.pane);
    }

    private syncPane(): void {
        const h = this.currentHeightPx();
        if (h === this.appliedH || h === 0) return;
        if (this.appliedH !== 0) {
            const rect = this.pane.getBoundingClientRect();
            const px = rect.width / 2 - this.canvas.offsetLeft;
            const py = rect.height / 2 - this.canvas.offsetTop;
            const s = this.state.scale;
            ({ x: this.state.panX, y: this.state.panY } =
                anchoredPan(px, py, s, s, this.state.panX, this.state.panY));
        }
        this.appliedH = h;
        this.applyTransform();
    }

    private bindGestures(): void {
        const pane = this.pane;
        this.on(pane, "wheel", (ev: WheelEvent) => {
            ev.preventDefault();
            const factor = ev.deltaY < 0 ? 1.1 : 1 / 1.1;
            this.zoomAt(ev.clientX, ev.clientY, factor);
        }, { passive: false });

        let downX = 0, downY = 0, lastX = 0, lastY = 0;
        let pressed = false, dragging = false;
        this.on(pane, "pointerdown", (ev: PointerEvent) => {
            downX = ev.clientX; downY = ev.clientY;
            pressed = ev.button === 0 &&
                !(ev.target as HTMLElement | null)?.closest(".pid-zoom");
            if (pressed) { lastX = ev.clientX; lastY = ev.clientY; }
        });
        this.on(pane, "pointermove", (ev: PointerEvent) => {
            if (!pressed) return;
            if (!dragging) {
                if (Math.hypot(ev.clientX - downX, ev.clientY - downY) < 5) return;
                dragging = true;
                pane.setPointerCapture(ev.pointerId);
            }
            this.state.panX += ev.clientX - lastX;
            this.state.panY += ev.clientY - lastY;
            lastX = ev.clientX; lastY = ev.clientY;
            this.applyTransform();
        });
        const end = (ev: PointerEvent): void => {
            pressed = false;
            if (!dragging) return;
            dragging = false;
            this.save();
            if (pane.hasPointerCapture(ev.pointerId)) {
                pane.releasePointerCapture(ev.pointerId);
            }
        };
        this.on(pane, "pointerup", end);
        this.on(pane, "pointercancel", end);
        this.on(pane, "click", (ev: MouseEvent) => {
            if (Math.hypot(ev.clientX - downX, ev.clientY - downY) > 5) return;
            const g = (ev.target as Element | null)?.closest("[data-part]");
            const part = g?.getAttribute("data-part");
            if (!g || !part) return;
            if (revealPart(part)) return;
            g.classList.add("ref-flash");
            window.setTimeout(() => g.classList.remove("ref-flash"), REF_FLASH_MS);
        });
    }

    private zoomAt(clientX: number, clientY: number, factor: number): void {
        const next = clampScale(this.state.scale * factor);
        if (next === this.state.scale) return;
        const rect = this.pane.getBoundingClientRect();
        const px = clientX - rect.left - this.canvas.offsetLeft;
        const py = clientY - rect.top - this.canvas.offsetTop;
        ({ x: this.state.panX, y: this.state.panY } = anchoredPan(
            px, py, this.state.scale, next, this.state.panX, this.state.panY,
        ));
        this.state.scale = next;
        if (next === MIN_SCALE) { this.state.panX = 0; this.state.panY = 0; }
        this.applyTransform();
        this.save();
    }

    dispose(): void {
        this.disposers.forEach((d) => d());
        this.disposers = [];
        this.resizer?.disconnect();
        this.resizer = null;
    }
}
