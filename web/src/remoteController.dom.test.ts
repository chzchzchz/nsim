// remoteController.dom.test.ts — RemoteController behavior
// under happy-dom with a mocked fetch: connect/disconnect mode
// transitions, visibility guarantees, writable gating, and the
// pollOne failure path. The controller must never be reachable
// in a half-connected state (the class of bug the extraction
// from main.ts fixes).

// @vitest-environment happy-dom

import { describe, expect, it, vi, afterEach, beforeEach } from "vitest";
import { RemoteController } from "./remoteController";

function jsonResponse(body: unknown, status = 200): Response {
    return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
    });
}

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

function makeHost() {
    const tilesEl = document.createElement("div");
    const alarmsEl = document.createElement("div");
    const consoleEl = document.createElement("div");
    const logsEl = document.createElement("section");
    logsEl.style.display = "none";
    const statusEl = document.createElement("span");
    statusEl.id = "sim-status";
    const startBtn = document.createElement("button");
    const stepBtn = document.createElement("button");
    const resetBtn = document.createElement("button");
    const spsInput = document.createElement("input");
    const spsEl = document.createElement("span");
    const writeModeBtn = document.createElement("button");
    writeModeBtn.style.display = "none";
    const pidWrap = document.createElement("div");
    pidWrap.className = "pid-wrap";
    const pidEl = document.createElement("div");
    pidEl.id = "pid";
    const handle = document.createElement("div");
    handle.className = "pid-resize-handle";
    pidWrap.append(pidEl, handle);
    document.body.append(tilesEl, alarmsEl, consoleEl, logsEl, statusEl, pidWrap);
    const scenarioNameEl = document.createElement("span");
    scenarioNameEl.textContent = "named";
    return {
        host: {
            tilesEl, alarmsEl, consoleEl, logsEl, statusEl,
            startBtn, stepBtn, resetBtn, spsInput, spsEl, writeModeBtn, pidEl,
        },
        scenarioNameEl,
        els: { tilesEl, logsEl, startBtn, stepBtn, resetBtn, writeModeBtn },
    };
}

beforeEach(() => {
    document.body.innerHTML = "";
});

afterEach(() => {
    vi.unstubAllGlobals();
});



describe("RemoteController", () => {
    it("starts in local mode with no client", () => {
        const { host, scenarioNameEl } = makeHost();
        const c = new RemoteController(host, { scenarioNameEl });
        expect(c.mode).toBe("local");
        expect(c.connected).toBe(false);
        expect(c.writable).toBe(false);
        expect(c.client).toBeNull();
        c.stopPoll();
    });

    it("connect enters readonly and shows remote UI", () => {
        const fetched: string[] = [];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            fetched.push(url);
            if (url.includes("/status")) return jsonResponse(statusBody);
            if (url.includes("/controls")) return jsonResponse([]);
            if (url.includes("/sensors")) return jsonResponse([]);
            if (url.includes("/alarms")) return jsonResponse([]);
            if (url.includes("/logs")) return jsonResponse([]);
            return jsonResponse({});
        }));
        const { host, scenarioNameEl, els } = makeHost();
        const c = new RemoteController(host, { scenarioNameEl }, 60_000);
        c.connect("http://127.0.0.1:8080");
        expect(c.mode).toBe("readonly");
        expect(c.connected).toBe(true);
        expect(c.writable).toBe(false);
        expect(c.client?.base).toBe("http://127.0.0.1:8080");
        expect(scenarioNameEl.textContent).toBe("");
        expect(els.logsEl.style.display).toBe("");
        expect(els.writeModeBtn.style.display).toBe("");
        // Readonly disables lifecycle.
        expect(els.startBtn.disabled).toBe(true);
        expect(els.resetBtn.disabled).toBe(true);
        c.disconnect();
    });

    it("toggleWritable flips readonly<->writable and updates labels", () => {
        vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(statusBody)));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        c.toggleWritable();
        expect(c.mode).toBe("writable");
        expect(c.writable).toBe(true);
        expect(els.writeModeBtn.textContent).toBe("✏️ Writable");
        c.toggleWritable();
        expect(c.mode).toBe("readonly");
        expect(els.writeModeBtn.textContent).toBe("🔒 Read-only");
        c.disconnect();
        // Toggle is a no-op in local mode.
        c.toggleWritable();
        expect(c.mode).toBe("local");
    });

    it("disconnect returns to local, hides UI, fires hook", () => {
        vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(statusBody)));
        const { host, els } = makeHost();
        let hooks = 0;
        const c = new RemoteController(host, {
            onDisconnected: () => { hooks++; },
        }, 60_000);
        c.connect("http://127.0.0.1:8080");
        c.toggleWritable();
        c.disconnect();
        expect(c.mode).toBe("local");
        expect(c.client).toBeNull();
        expect(c.connected).toBe(false);
        expect(els.logsEl.style.display).toBe("none");
        expect(els.writeModeBtn.style.display).toBe("none");
        // Regression: disconnect must hand control back to local
        // mode — header lifecycle must not stay disabled.
        expect(els.startBtn.disabled).toBe(false);
        expect(els.stepBtn.disabled).toBe(false);
        expect(els.resetBtn.disabled).toBe(false);
        expect(host.spsInput.disabled).toBe(false);
        expect(hooks).toBe(1);
    });

    it("pollOnce surfaces server header and rebuilds on runId", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const sen = { key: "P/S", part: "P", label: "S", kind: "float", value: "2.5", simTimeMs: 1 };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            if (url.includes("/status")) return jsonResponse(statusBody);
            if (url.includes("/controls")) return jsonResponse([ctl]);
            if (url.includes("/sensors")) return jsonResponse([sen]);
            if (url.includes("/alarms")) return jsonResponse([]);
            if (url.includes("/logs")) return jsonResponse([]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        // Replace the timer-driven startPoll with a direct drain:
        // set client/mode by hand BEFORE connect schedules a poll.
        c.client = { base: "http://127.0.0.1:8080",
            status: () => Promise.resolve(statusBody),
            controls: () => Promise.resolve([ctl]),
            sensors: () => Promise.resolve([sen]),
            alarms: () => Promise.resolve([]),
            logs: () => Promise.resolve([]),
            livez: () => Promise.resolve(true),
        } as never;
        c.mode = "readonly";
        c.setVisible(true);
        await c.drainNow();
        expect(c.lastStatus?.siteName).toBe("TestSite");
        expect(c.lastRunId).toBe(42);
        expect(host.statusEl.textContent).toContain("paused @ 1.50 s");
        expect(els.tilesEl.textContent).toContain("P");
        c.disconnect();
    });

    it("applyScenario returns false on server failure, true on ok", async () => {
        // Success path.
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            if (url.includes("/sim/reset")) return jsonResponse({ runId: 9 });
            return jsonResponse(statusBody);
        }));
        const okHost = makeHost();
        const okC = new RemoteController(okHost.host, {}, 60_000);
        okC.connect("http://127.0.0.1:8080");
        okC.toggleWritable();
        expect(await okC.applyScenario("alpha")).toBe(true);
        okC.disconnect();

        // Failure path: 500 envelope → applyScenario false.
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            if (url.includes("/sim/reset")) {
                return jsonResponse({ error: "boom", code: 500 }, 500);
            }
            return jsonResponse(statusBody);
        }));
        const badHost = makeHost();
        const badC = new RemoteController(badHost.host, {}, 60_000);
        badC.connect("http://127.0.0.1:8080");
        badC.toggleWritable();
        expect(await badC.applyScenario("alpha")).toBe(false);
        badC.disconnect();
    });

    it("applyScenario respects writable + free-play guard", async () => {
        const seen: string[] = [];
        vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
            seen.push(`${init?.method ?? "GET"} ${url}`);
            if (url.includes("/sim/reset")) {
                return jsonResponse({ runId: 43 });
            }
            return jsonResponse(statusBody);
        }));
        const { host } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        // Readonly: no-op.
        expect(await c.applyScenario("alpha")).toBe(false);
        expect(seen.some((s) => s.includes("/sim/reset"))).toBe(false);
        // Writable + null (free play): no-op.
        c.toggleWritable();
        expect(await c.applyScenario(null)).toBe(false);
        // Writable + id: posts.
        expect(await c.applyScenario("alpha")).toBe(true);
        expect(seen.some((s) => s.includes("/sim/reset"))).toBe(true);
        expect(c.lastRunId).toBe(-1);
        c.disconnect();
    });

    it("refreshSvg fetches /state and mounts the wasm-rendered diagram", async () => {
        const plantJson = JSON.stringify({ ReactorBuilding: {} });
        let svgCalls = 0;
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/state")) return jsonResponse(plantJson as unknown as Record<string, unknown>);
            if (u.includes("/status")) return jsonResponse(statusBody);
            if (u.includes("/controls")) return jsonResponse([]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([]);
            return jsonResponse({});
        }));
        // Fake the wasm factory: plantFromJSON is called with the
        // JSON string; return a handle whose getSvg is deterministic.
        (window as unknown as Record<string, unknown>).nsimPlantFromJSON = () => {
            svgCalls++;
            return {
                getSvg: () => "<svg/>",
            };
        };
        const { host } = makeHost();
        const pidEl = host.pidEl;
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        c.stopPoll();
        await c.drainNow();
        await new Promise((r) => setTimeout(r, 30));
        expect(svgCalls).toBeGreaterThan(0);
        expect(pidEl.innerHTML).toContain("<svg");
        c.disconnect();
    });

    it("disconnect resets the SVG swap guards so local restore re-renders", () => {
        const { host } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        c.stopPoll();
        c.lastSvgHtml = "<svg>remote</svg>";
        c.lastSvgSimTimeMs = 999;
        c.disconnect();
        expect(c.lastSvgHtml).toBe("");
        expect(c.lastSvgSimTimeMs).toBe(-1);
    });

    it("setVisible(false) keeps toggling logs independent of serverBtn", () => {
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        c.setVisible(false);
        expect(els.writeModeBtn.style.display).toBe("none");
        expect(els.logsEl.style.display).toBe("none");
        c.disconnect();
    });
});
