// layout.test.ts — persistence validation gates. The dangerous
// failure is a saved layout silently accepted despite not matching
// the current panel set; each rejection path gets its own case.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { loadPanelLayout, savePanelLayout, PANEL_LAYOUT_VERSION } from "./layout";

const store = new Map<string, string>();
const KEY = "nsim-panel-layout";

beforeEach(() => {
    store.clear();
    vi.stubGlobal("localStorage", {
        getItem: (k: string) => store.get(k) ?? null,
        setItem: (k: string, v: string) => void store.set(k, String(v)),
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe("loadPanelLayout", () => {
    it("round-trips what savePanelLayout wrote", () => {
        savePanelLayout({ version: 1, order: ["B", "A"], minimized: ["A"] });
        expect(loadPanelLayout(["A", "B"])).toEqual({
            version: PANEL_LAYOUT_VERSION,
            order: ["B", "A"],
            minimized: ["A"],
        });
    });

    it("returns null for an absent key", () => {
        expect(loadPanelLayout(["A"])).toBeNull();
    });

    it("drops layouts with a different schema version", () => {
        store.set(KEY, '{"version":999,"order":["A"],"minimized":[]}');
        expect(loadPanelLayout(["A"])).toBeNull();
    });

    it("drops malformed JSON", () => {
        store.set(KEY, "{not json");
        expect(loadPanelLayout(["A"])).toBeNull();
    });

    it("drops non-object payloads", () => {
        store.set(KEY, '["A"]');
        expect(loadPanelLayout(["A"])).toBeNull();
    });

    it("drops orders naming an unknown panel", () => {
        store.set(KEY, '{"version":1,"order":["A","X"],"minimized":[]}');
        expect(loadPanelLayout(["A", "B"])).toBeNull();
    });

    it("drops orders missing a current panel", () => {
        store.set(KEY, '{"version":1,"order":["A"],"minimized":[]}');
        expect(loadPanelLayout(["A", "B"])).toBeNull();
    });

    it("rejects a non-array or non-string order", () => {
        store.set(KEY, '{"version":1,"order":"all","minimized":[]}');
        expect(loadPanelLayout(["A"])).toBeNull();
        store.set(KEY, '{"version":1,"order":[1,2],"minimized":[]}');
        expect(loadPanelLayout(["A", "B"])).toBeNull();
    });

    it("filters stale minimized entries instead of dropping the layout", () => {
        store.set(
            KEY,
            '{"version":1,"order":["A","B"],"minimized":["A","GONE"]}',
        );
        expect(loadPanelLayout(["A", "B"])?.minimized).toEqual(["A"]);
    });

    it("survives storage that throws (private browsing)", () => {
        vi.stubGlobal("localStorage", {
            getItem: () => {
                throw new Error("denied");
            },
            setItem: () => {
                throw new Error("denied");
            },
        });
        expect(loadPanelLayout(["A"])).toBeNull();
        expect(() =>
            savePanelLayout({ version: 1, order: ["A"], minimized: [] }),
        ).not.toThrow();
    });
});
