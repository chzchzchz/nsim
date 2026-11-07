// pidViewport.test.ts — the single-owner viewport state machine:
// persistence validation, legacy migration, clamping, and the pure
// anchor/snap helpers all hold before a number touches CSS or storage.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
    anchoredPan,
    clampHeightVh,
    clampScale,
    loadViewportState,
    saveViewportState,
    snapHeightForResize,
    DEFAULT_HEIGHT_VH,
    MAX_HEIGHT_VH,
    MIN_HEIGHT_VH,
    PID_VIEWPORT_KEY,
} from "./pidViewport";

const store = new Map<string, string>();

beforeEach(() => {
    store.clear();
    vi.stubGlobal("localStorage", {
        getItem: (k: string) => store.get(k) ?? null,
        setItem: (k: string, v: string) => void store.set(k, String(v)),
        removeItem: (k: string) => void store.delete(k),
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe("clampHeightVh / clampScale", () => {
    it("clamps height to 20-90vh", () => {
        expect(clampHeightVh(5)).toBe(MIN_HEIGHT_VH);
        expect(clampHeightVh(200)).toBe(MAX_HEIGHT_VH);
        expect(clampHeightVh(NaN)).toBe(DEFAULT_HEIGHT_VH);
    });

    it("clamps scale to 1-12", () => {
        expect(clampScale(0.5)).toBe(1);
        expect(clampScale(99)).toBe(12);
        expect(clampScale(NaN)).toBe(1);
    });
});

describe("loadViewportState / saveViewportState", () => {
    it("round-trips the full state", () => {
        const s = { version: 1 as const, heightVh: 65, scale: 2.5, panX: 10, panY: -20 };
        saveViewportState(s);
        expect(loadViewportState()).toEqual(s);
    });

    it("defaults when empty", () => {
        const v = loadViewportState();
        expect(v.heightVh).toBe(DEFAULT_HEIGHT_VH);
        expect(v.scale).toBe(1);
        expect(v.panX).toBe(0);
        expect(v.panY).toBe(0);
    });

    it("defaults on malformed JSON", () => {
        store.set(PID_VIEWPORT_KEY, "{not json");
        expect(loadViewportState().heightVh).toBe(DEFAULT_HEIGHT_VH);
    });

    it("drops wrong-version blobs", () => {
        store.set(PID_VIEWPORT_KEY, '{"version":999,"heightVh":50,"scale":1,"panX":0,"panY":0}');
        expect(loadViewportState().heightVh).toBe(DEFAULT_HEIGHT_VH);
    });

    it("migrates legacy height + view keys", () => {
        store.set("nsim-pid-height", "70");
        store.set("nsim-pid-view", '{"version":1,"scale":2,"panX":5,"panY":-3}');
        const v = loadViewportState();
        expect(v.heightVh).toBe(70);
        expect(v.scale).toBe(2);
        expect(v.panX).toBe(5);
        expect(v.panY).toBe(-3);
    });

    it("handles throwing localStorage", () => {
        vi.stubGlobal("localStorage", {
            getItem: () => { throw new Error("denied"); },
            setItem: () => { throw new Error("denied"); },
        });
        expect(loadViewportState().heightVh).toBe(DEFAULT_HEIGHT_VH);
        expect(() => saveViewportState({ version: 1 as const, heightVh: 40, scale: 1, panX: 0, panY: 0 })).not.toThrow();
    });
});

describe("anchoredPan", () => {
    it("keeps the cursor point fixed across scale changes", () => {
        const cases = [
            { px: 0, py: 0, prev: 1, next: 2, panX: 0, panY: 0 },
            { px: 300, py: 150, prev: 1, next: 1.25, panX: -40, panY: 12 },
            { px: -50, py: 900, prev: 4, next: 1, panX: 120, panY: -300 },
        ];
        for (const c of cases) {
            const p = anchoredPan(c.px, c.py, c.prev, c.next, c.panX, c.panY);
            expect((c.px - p.x) / c.next).toBeCloseTo((c.px - c.panX) / c.prev, 9);
            expect((c.py - p.y) / c.next).toBeCloseTo((c.py - c.panY) / c.prev, 9);
        }
    });
});

describe("snapHeightForResize", () => {
    it("maps pixel deltas to vh one-to-one", () => {
        expect(snapHeightForResize(100, 1000, 50)).toBe(60);
        expect(snapHeightForResize(-100, 1000, 50)).toBe(40);
    });

    it("clamps to min/max", () => {
        expect(snapHeightForResize(99999, 1000, 50)).toBe(MAX_HEIGHT_VH);
        expect(snapHeightForResize(-99999, 1000, 50)).toBe(MIN_HEIGHT_VH);
    });

    it("ignores invalid viewport height", () => {
        expect(snapHeightForResize(100, 0, 50)).toBe(50);
    });
});
