// faults.dom.test.ts — fault-injector visibility in both consoles.
// The shared header toggle (#fault-toggle) hides fault injectors by
// default; the same persisted state drives the local WASM console
// (bindings → controls.ts renderPanel) and the remote console
// (remoteTiles renderRemotePanel + /sim/faults fetch in
// RemoteController.rebuildControlTiles). The toggle is a UX guard,
// not a security boundary.
//
// @vitest-environment happy-dom

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import {
    isFaultEntry,
    setShowFaultInjectors,
    showFaultInjectors,
    initFaultToggle,
} from "./faults";

const store = new Map<string, string>();
import { renderPanel } from "./controls";
import { buildRemoteBindings, renderRemotePanel } from "./remoteTiles";
import { displayPart, type PartBinding } from "./bindings";
import { RemoteController } from "./remoteController";

const faultKey = "ReactorBuilding/Faults/PORVStuckOpen";
const ordinaryKey = "ReactorBuilding/BlockValve/Open";
const faultControlEntry = {
    key: faultKey,
    part: "ReactorBuilding/Faults",
    label: "PORVStuckOpen",
    kind: "bool",
    value: "1",
    fault: true,
};
const ordinaryControlEntry = {
    key: ordinaryKey,
    part: "ReactorBuilding/BlockValve",
    label: "Open",
    kind: "normalized",
    value: "0",
};

function makeHost() {
    document.body.innerHTML = "";
    const tilesEl = document.createElement("div");
    const alarmsEl = document.createElement("div");
    const consoleEl = document.createElement("div");
    const logsEl = document.createElement("section");
    logsEl.style.display = "none";
    const statusEl = document.createElement("span");
    const startBtn = document.createElement("button");
    const stepBtn = document.createElement("button");
    const resetBtn = document.createElement("button");
    const spsInput = document.createElement("input");
    const fpsInput = document.createElement("input");
    const spsEl = document.createElement("span");
    const writeModeBtn = document.createElement("button");
    writeModeBtn.style.display = "none";
    const pidEl = document.createElement("div");
    document.body.append(
        tilesEl, alarmsEl, consoleEl, logsEl, statusEl, pidEl, writeModeBtn,
    );
    return {
        host: {
            tilesEl, alarmsEl, consoleEl, logsEl, statusEl,
            startBtn, stepBtn, resetBtn, spsInput, fpsInput, spsEl,
            writeModeBtn, pidEl,
        },
        els: { tilesEl, consoleEl },
    };
}

beforeEach(() => {
    store.clear();
    vi.stubGlobal("localStorage", {
        getItem: (k: string) => store.get(k) ?? null,
        setItem: (k: string, v: string) => void store.set(k, String(v)),
    });
    setShowFaultInjectors(false);
    document.body.innerHTML = "";
    document.body.insertAdjacentHTML(
        "beforeend",
        '<label id="fault-toggle" hidden><input type="checkbox" id="fault-toggle-cb" /> Faults</label>',
    );
});

afterEach(() => {
    vi.unstubAllGlobals();
    setShowFaultInjectors(false);
});

describe("faults module", () => {
    it("is fault-aware by default is hidden", () => {
        expect(showFaultInjectors()).toBe(false);
        expect(isFaultEntry({ meta: { fault: true } })).toBe(true);
        expect(isFaultEntry({ meta: { fault: false } })).toBe(false);
        expect(isFaultEntry({ meta: {} })).toBe(false);
    });

    it("toggle persists across reads in the same session", () => {
        setShowFaultInjectors(true);
        expect(showFaultInjectors()).toBe(true);
        expect(store.get("nsim.showFaultInjectors")).toBe("1");
        setShowFaultInjectors(false);
        expect(showFaultInjectors()).toBe(false);
    });

    it("initFaultToggle reveals the header label and wires the change event", () => {
        let calls = 0;
        initFaultToggle(() => { calls++; });
        const label = document.getElementById("fault-toggle");
        const cb = document.getElementById("fault-toggle-cb") as HTMLInputElement;
        expect(label?.hidden).toBe(false);
        expect(cb.checked).toBe(false);
        cb.click();
        expect(calls).toBe(1);
        expect(showFaultInjectors()).toBe(true);
        expect(cb.checked).toBe(true);
        cb.click();
        expect(calls).toBe(2);
        expect(showFaultInjectors()).toBe(false);
    });
});

describe("console renderers hide fault injectors by default", () => {
    const ordinaryBinding = (): PartBinding => ({
        part: "ReactorBuilding/BlockValve",
        controls: [{
            id: ordinaryKey,
            meta: { part: "ReactorBuilding/BlockValve", label: "Open", kind: "normalized" },
        }],
        sensors: [],
    });
    const faultBinding = (): PartBinding => ({
        part: "ReactorBuilding/Faults",
        controls: [{
            id: faultKey,
            meta: { part: "ReactorBuilding/Faults", label: "PORVStuckOpen", kind: "bool", fault: true },
        }],
        sensors: [],
    });

    it("local console (renderPanel) hides fault rows unless toggled", () => {
        const host = document.createElement("div");
        const plant = {
            controlValuesJSON: () => JSON.stringify({ values: {} }),
            setControl: () => null,
        } as never;
        renderPanel(host, [ordinaryBinding(), faultBinding()], plant, displayPart("ReactorBuilding"));
        expect(host.querySelector(`[data-bridge-id="${ordinaryKey}"]`)).not.toBeNull();
        expect(host.querySelector(`[data-bridge-id="${faultKey}"]`)).toBeNull();
        setShowFaultInjectors(true);
        renderPanel(host, [ordinaryBinding(), faultBinding()], plant, displayPart("ReactorBuilding"));
        expect(host.querySelector(`[data-bridge-id="${faultKey}"]`)).not.toBeNull();
    });

    it("remote console (renderRemotePanel) hides fault rows unless toggled", () => {
        const bindings = buildRemoteBindings(
            [ordinaryControlEntry, faultControlEntry],
            [],
        );
        const host = document.createElement("div");
        const values = { [ordinaryKey]: "0", [faultKey]: "1" };
        const writable = () => true;
        const writeControl = async () => ({ seq: 1 });
        renderRemotePanel(host, bindings, displayPart("ReactorBuilding"), values, writable, writeControl);
        expect(host.querySelector(`[data-bridge-id="${ordinaryKey}"]`)).not.toBeNull();
        expect(host.querySelector(`[data-bridge-id="${faultKey}"]`)).toBeNull();
        setShowFaultInjectors(true);
        renderRemotePanel(host, bindings, displayPart("ReactorBuilding"), values, writable, writeControl);
        expect(host.querySelector(`[data-bridge-id="${faultKey}"]`)).not.toBeNull();
    });
});

describe("remote fault catalog", () => {
    const statusBody = {
        siteName: "TestSite",
        runId: 42,
        simTimeMs: 1500,
        timestepMs: 1,
        stepsPerSecond: 1000,
        running: false,
        paused: true,
        rev: 1,
        latestSeq: 0,
        objective: { active: false, done: false, detail: "" },
    };

    function stubFetch(fetched: string[]) {
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            fetched.push(url);
            if (url.includes("/sim/faults")) {
                return new Response(JSON.stringify([faultControlEntry]), {
                    status: 200,
                    headers: { "Content-Type": "application/json" },
                });
            }
            if (url.includes("/status")) {
                return new Response(JSON.stringify(statusBody), {
                    status: 200,
                    headers: { "Content-Type": "application/json" },
                });
            }
            if (url.includes("/controls")) {
                return new Response(JSON.stringify([ordinaryControlEntry]), {
                    status: 200,
                    headers: { "Content-Type": "application/json" },
                });
            }
            if (url.includes("/sensors") || url.includes("/alarms") || url.includes("/logs")) {
                return new Response(JSON.stringify([]), {
                    status: 200,
                    headers: { "Content-Type": "application/json" },
                });
            }
            return new Response("{}", { status: 200 });
        }) as never);
    }

    it("toggle OFF: no /sim/faults request, no fault rows", async () => {
        const fetched: string[] = [];
        stubFetch(fetched);
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelector(`[data-bridge-id="${ordinaryKey}"]`)).not.toBeNull();
        });
        expect(fetched.some((u) => u.includes("/sim/faults"))).toBe(false);
        expect(els.tilesEl.querySelector(`[data-bridge-id="${faultKey}"]`)).toBeNull();
        c.disconnect();
    });

    it("toggle ON: rebuild requests /sim/faults, shows fault rows, preserves console", async () => {
        const fetched: string[] = [];
        stubFetch(fetched);
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.consoleEl.querySelector("#cmd-input")).not.toBeNull();
        });
        const consoleBefore = els.consoleEl.querySelector("#cmd-input");

        setShowFaultInjectors(true);
        await c.rebuildControlTiles();

        expect(fetched.some((u) => u.includes("/sim/faults"))).toBe(true);
        expect(els.tilesEl.querySelector(`[data-bridge-id="${faultKey}"]`)).not.toBeNull();
        // The alarm/console surfaces survive a visibility-only rebuild.
        expect(els.consoleEl.querySelector("#cmd-input")).toBe(consoleBefore);
        c.disconnect();
    });
});
