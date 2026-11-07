// pidViewport.dom.test.ts — the single owner wired up against the DOM:
// the wrap lives in markup (not runtime), the handle drag resizes,
// persistence migrates legacy keys, and disposal removes listeners
// without a resize-loop remnant.

// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PidViewport, PID_VIEWPORT_KEY } from "./pidViewport";

const store = new Map<string, string>();

// Minimal ResizeObserver stub: immediately invokes the callback
// synchronously once observed (happy-dom has none).
class FakeResizeObserver {
    constructor(private cb: () => void) {}
    observe(): void { this.cb(); }
    unobserve(): void {}
    disconnect(): void {}
}

beforeEach(() => {
    store.clear();
    vi.stubGlobal("localStorage", {
        getItem: (k: string) => store.get(k) ?? null,
        setItem: (k: string, v: string) => void store.set(k, String(v)),
        removeItem: (k: string) => void store.delete(k),
    });
    vi.stubGlobal("ResizeObserver", FakeResizeObserver);
    document.documentElement.style.removeProperty("--pid-height");
    document.body.innerHTML = "";
});

afterEach(() => {
    vi.unstubAllGlobals();
});

function mountPid(): { pane: HTMLElement; wrap: HTMLElement; handle: HTMLElement } {
    const wrap = document.createElement("div");
    wrap.className = "pid-wrap";
    const pane = document.createElement("div");
    pane.id = "pid";
    const handle = document.createElement("div");
    handle.className = "pid-resize-handle";
    wrap.append(pane, handle);
    document.body.appendChild(wrap);
    return { pane, wrap, handle };
}

function pointer(el: HTMLElement, type: string, clientY: number): void {
    el.dispatchEvent(new PointerEvent(type, { bubbles: true, clientY, button: 0, pointerId: 1 }));
}

describe("PidViewport mount", () => {
    it("requires the pane to be inside .pid-wrap", () => {
        const pane = document.createElement("div");
        document.body.appendChild(pane);
        expect(() => new PidViewport(pane)).toThrow(/\.pid-wrap/);
    });

    it("does not build the wrap at runtime", () => {
        const { wrap } = mountPid();
        expect(wrap.querySelectorAll(".pid-wrap").length).toBe(0);
        expect(wrap.childElementCount).toBe(2);
    });

    it("applies the stored height as a css variable", () => {
        store.set(PID_VIEWPORT_KEY, JSON.stringify({
            version: 1, heightVh: 65, scale: 1, panX: 0, panY: 0,
        }));
        const { pane } = mountPid();
        new PidViewport(pane);
        expect(document.documentElement.style.getPropertyValue("--pid-height")).toBe("65svh");
    });

    it("renders the svg into the canvas", () => {
        const { pane } = mountPid();
        const vp = new PidViewport(pane);
        vp.mountSvg("<svg viewBox='0 0 100 100'></svg>");
        expect(pane.querySelector(".pid-canvas svg")).toBeTruthy();
    });
});

describe("PidViewport drag handle", () => {
    it("dragging down grows the pane and persists on release", () => {
        const { pane, handle } = mountPid();
        new PidViewport(pane);
        window.innerHeight = 1000;
        pointer(handle, "pointerdown", 100);
        pointer(handle, "pointermove", 200);
        expect(document.documentElement.style.getPropertyValue("--pid-height")).toBe("60svh");
        pointer(handle, "pointerup", 200);
        const saved = JSON.parse(store.get(PID_VIEWPORT_KEY) ?? "{}");
        expect(saved.heightVh).toBe(60);
    });

    it("clamps at min and max", () => {
        const { pane, handle } = mountPid();
        new PidViewport(pane);
        window.innerHeight = 1000;
        pointer(handle, "pointerdown", 1000);
        pointer(handle, "pointermove", 0);
        expect(document.documentElement.style.getPropertyValue("--pid-height")).toBe("20svh");
        pointer(handle, "pointerup", 0);
        pointer(handle, "pointerdown", 0);
        pointer(handle, "pointermove", 99999);
        expect(document.documentElement.style.getPropertyValue("--pid-height")).toBe("90svh");
        pointer(handle, "pointerup", 99999);
    });

    it("disposal removes the handle listeners", () => {
        const { pane, handle } = mountPid();
        const vp = new PidViewport(pane);
        vp.dispose();
        window.innerHeight = 1000;
        pointer(handle, "pointerdown", 100);
        pointer(handle, "pointermove", 200);
        // After dispose a drag must not change the height.
        expect(document.documentElement.style.getPropertyValue("--pid-height")).toBe("50svh");
    });
});

describe("PidViewport host switching", () => {
    it("two panes get independent canvases", () => {
        const a = mountPid();
        const b = mountPid();
        new PidViewport(a.pane);
        new PidViewport(b.pane);
        expect(a.pane.querySelectorAll(".pid-canvas").length).toBe(1);
        expect(b.pane.querySelectorAll(".pid-canvas").length).toBe(1);
    });
});
