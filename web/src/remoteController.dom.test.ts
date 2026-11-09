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
    const fpsInput = document.createElement("input");
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
            startBtn, stepBtn, resetBtn, spsInput, fpsInput, spsEl, writeModeBtn, pidEl,
        },
        scenarioNameEl,
        els: { tilesEl, logsEl, startBtn, stepBtn, resetBtn, writeModeBtn, fpsInput },
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

    it("poll interval is capped at 1 fps and follows the FPS box", () => {
        const { host } = makeHost();
        // pollMsFor is a pure helper for unit tests.
        expect(RemoteController.pollMsFor(16)).toBe(1000);
        expect(RemoteController.pollMsFor(1000)).toBe(1000);
        expect(RemoteController.pollMsFor(2000)).toBe(2000);
        const capped = new RemoteController(host, {}, 16);
        expect(capped.pollIntervalMs()).toBe(1000);
        const c = new RemoteController(host, {}, 60_000);
        // A large interval is kept (only the fast side is capped).
        expect(c.pollIntervalMs()).toBe(60_000);
        // setFramesPerSecond: 60 fps -> 1000/60 = 16.6 ms -> capped.
        c.setFramesPerSecond(60);
        expect(c.pollIntervalMs()).toBe(1000);
        // 1 fps -> 1000 ms.
        c.setFramesPerSecond(1);
        expect(c.pollIntervalMs()).toBe(1000);
        // 0.5 fps -> 2000 ms (below the cap).
        c.setFramesPerSecond(0.5);
        expect(c.pollIntervalMs()).toBe(2000);
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

    it("log selection outlines matching control and clears on deselect", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const ctl2 = { key: "P/B", part: "P", label: "B", kind: "bool", value: "0" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse([ctl, ctl2]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(2);
        });
        const rows = Array.from(els.tilesEl.querySelectorAll<HTMLElement>(".ctl-row"));
        const rowA = rows.find((r) => r.dataset.bridgeId === "P/A")!;
        const rowB = rows.find((r) => r.dataset.bridgeId === "P/B")!;
        const logRow = els.logsEl.querySelector<HTMLElement>(".log-row")!;
        logRow.click();
        expect(rowA.classList.contains("log-selected")).toBe(true);
        expect(rowB.classList.contains("log-selected")).toBe(false);
        expect(c.selectedLogControl).toBe("P/A");
        logRow.click();
        expect(rowA.classList.contains("log-selected")).toBe(false);
        expect(c.selectedLogControl).toBeNull();
        c.disconnect();
    });

    it("pollLogs auto-follows foreign entries when follow is on", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const initial = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        const fresh = { seq: 6, kind: "operation", atMs: 2000, control: "P/A", value: "0", comment: "crisis03: close it" };
        let logBatch: unknown[] = [initial];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 6 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse(logBatch);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        // Opt in to follow and route the next poll to the fresh entry.
        c.logsPanel!.setFollow(true);
        logBatch = [fresh];
        // lastSeq advanced to 5 on first poll (initial). Force a
        // poll cycle that sees status.latestSeq=6 and fetches seq 6.
        c.lastSeq = 5;
        await c.drainNow();
        await vi.waitFor(() => {
            expect(c.selectedLogControl).toBe("P/A");
        });
        expect(rowA.classList.contains("log-selected")).toBe(true);
        expect(c.logsPanel!.selected()?.seq).toBe(6);
        expect(document.getElementById("alert-strip")?.hidden).toBe(false);
        expect(document.getElementById("alert-strip")?.textContent).toContain("crisis03: close it");
        // Dismiss it (same as click-again / Esc): selecting null
        // clears the control outline.
        c.logsPanel!.select(null);
        expect(rowA.classList.contains("log-selected")).toBe(false);
        expect(c.selectedLogControl).toBeNull();
        // Alert strip stays until dismissed.
        expect(document.getElementById("alert-strip")?.hidden).toBe(false);
        c.disconnect();
    });

    it("pollLogs auto-follows a burst with a single jump to the last entry", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const initial = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        const burst = [
            { seq: 6, kind: "operation", atMs: 2000, control: "P/A", value: "0", comment: "first" },
            { seq: 7, kind: "operation", atMs: 3000, control: "P/A", value: "1", comment: "second" },
            { seq: 8, kind: "operation", atMs: 4000, control: "P/A", value: "0", comment: "last" },
        ];
        let logBatch: unknown[] = [initial];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 8 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse(logBatch);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        const scrolls: ScrollIntoViewOptions[] = [];
        rowA.scrollIntoView = (o?: ScrollIntoViewOptions) => { scrolls.push(o ?? {}); };
        c.logsPanel!.setFollow(true);
        logBatch = burst;
        c.lastSeq = 5;
        await c.drainNow();
        // Exactly one jump, to the last entry of the burst.
        expect(scrolls.map((o) => o.block)).toEqual(["center"]);
        expect(c.selectedLogControl).toBe("P/A");
        expect(c.logsPanel!.selected()?.seq).toBe(8);
        const auto = document.getElementById("alert-strip") as HTMLElement;
        expect(auto.textContent).toContain("last");
        expect(auto.textContent).not.toContain("first");
        c.disconnect();
    });

    it("pollLogs auto-follows a pure comment with a toast but no jump", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const initial = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        const fresh = { seq: 6, kind: "comment", atMs: 2000, comment: "operator note" };
        let logBatch: unknown[] = [initial];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 6 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse(logBatch);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        let jumped = false;
        rowA.scrollIntoView = () => { jumped = true; };
        c.logsPanel!.setFollow(true);
        logBatch = [fresh];
        c.lastSeq = 5;
        await c.drainNow();
        expect(jumped).toBe(false);
        expect(c.selectedLogControl).toBeNull();
        expect(document.getElementById("alert-strip")?.textContent).toContain("operator note");
        c.disconnect();
    });

    it("pollLogs does not auto-follow when follow is off", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const initial = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        const fresh = { seq: 6, kind: "operation", atMs: 2000, control: "P/A", value: "0", comment: "crisis03: close it" };
        let logBatch: unknown[] = [initial];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 6 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse(logBatch);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        c.logsPanel!.setFollow(false);
        logBatch = [fresh];
        c.lastSeq = 5;
        await c.drainNow();
        expect(c.selectedLogControl).toBeNull();
        expect(rowA.classList.contains("log-selected")).toBe(false);
        // No strip exists yet (never shown) and it stays hidden.
        expect(document.getElementById("alert-strip")).toBeNull();
        c.disconnect();
    });

    it("selection missing from new catalog reconciles via logsPanel.select(null)", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        let controls: unknown[] = [ctl];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse(controls);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const logRow = els.logsEl.querySelector<HTMLElement>(".log-row")!;
        logRow.click();
        expect(c.selectedLogControl).toBe("P/A");
        const spy = vi.spyOn(c.logsPanel!, "select");
        // Simulate a run change where the control is gone from
        // the catalog, then rebuild tiles.
        controls = [];
        await c.buildTiles();
        expect(spy).toHaveBeenCalledWith(null);
        expect(c.selectedLogControl).toBeNull();
        expect(c.logsPanel!.selected()).toBeNull();
        c.disconnect();
    });

    it("selection survives a same-catalog rebuild and does not scroll", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const logRow = els.logsEl.querySelector<HTMLElement>(".log-row")!;
        logRow.click();
        let scrolled = false;
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        rowA.scrollIntoView = () => { scrolled = true; };
        const spy = vi.spyOn(c.logsPanel!, "select");
        await c.buildTiles();
        // Rebuild re-applies outline without scrolling and
        // without clearing the selection.
        const newRowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        expect(newRowA.classList.contains("log-selected")).toBe(true);
        expect(scrolled).toBe(false);
        expect(spy).not.toHaveBeenCalled();
        expect(c.selectedLogControl).toBe("P/A");
        c.disconnect();
    });

    it("scroll lockout suppresses control scroll after banner View", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        let scrolled = false;
        rowA.scrollIntoView = () => { scrolled = true; };
        // Seed a foreign entry and click View (starts lockout).
        c.logsPanel!.append([{ seq: 6, kind: "comment", atMs: 2000, comment: "x" }]);
        c.logsPanel!.noteForeign(1, { seq: 6, kind: "comment", atMs: 2000, comment: "x" });
        const viewBtn = Array.from(
            els.logsEl.querySelectorAll<HTMLElement>(".logs-banner button"),
        ).find((b) => b.textContent === "View")!;
        const lastLogRow = els.logsEl.querySelector<HTMLElement>(".log-row:last-child")!;
        lastLogRow.scrollIntoView = () => {};
        viewBtn.click();
        // Within the lockout, selecting a log entry should outline
        // but not scroll.
        const logRow = els.logsEl.querySelector<HTMLElement>(".log-row.operation")!;
        logRow.click();
        expect(rowA.classList.contains("log-selected")).toBe(true);
        expect(scrolled).toBe(false);
        c.disconnect();
    });

    it("log selection scroll centers the control in the viewport", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        let opts: ScrollIntoViewOptions | undefined;
        rowA.scrollIntoView = (o?: ScrollIntoViewOptions) => { opts = o; };
        els.logsEl.querySelector<HTMLElement>(".log-row")!.click();
        expect(rowA.classList.contains("log-selected")).toBe(true);
        expect(opts?.block).toBe("center");
        expect(opts?.behavior).toBe("smooth");
        c.disconnect();
    });

    it("log click records a backtick return to the log view", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        Object.defineProperty(window, "scrollY", { value: 250, configurable: true });
        Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
        // Control off-screen (under header) so the jump records.
        rowA.getBoundingClientRect = () =>
            ({ top: 0, bottom: 50, left: 0, right: 100, x: 0, y: 0, width: 100, height: 50, toJSON: () => ({}) }) as DOMRect;
        rowA.scrollIntoView = () => {};
        els.logsEl.querySelector<HTMLElement>(".log-row")!.click();
        // Backtick now returns to the recorded page scrollY.
        const scrolled: number[] = [];
        window.scrollTo = ((o?: ScrollToOptions) => {
            if (o && typeof o.top === "number") scrolled.push(o.top);
        }) as typeof window.scrollTo;
        const { jumpBack } = await import("./bindings");
        expect(jumpBack()).toBe(true);
        expect(scrolled).toEqual([250]);
        c.disconnect();
    });

    it("minimized tile suppresses scroll but keeps the outline", async () => {
        const ctl = { key: "P/A", part: "P", label: "A", kind: "bool", value: "1" };
        const logEntry = { seq: 5, kind: "operation", atMs: 1000, control: "P/A", value: "1" };
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            const u = String(url);
            if (u.includes("/status")) return jsonResponse({ ...statusBody, latestSeq: 5 });
            if (u.includes("/controls")) return jsonResponse([ctl]);
            if (u.includes("/sensors")) return jsonResponse([]);
            if (u.includes("/alarms")) return jsonResponse([]);
            if (u.includes("/logs")) return jsonResponse([logEntry]);
            return jsonResponse({});
        }));
        const { host, els } = makeHost();
        const c = new RemoteController(host, {}, 60_000);
        c.connect("http://127.0.0.1:8080");
        await vi.waitFor(() => {
            expect(els.tilesEl.querySelectorAll(".ctl-row").length).toBe(1);
        });
        // Minimize the tile containing the control.
        const tile = els.tilesEl.querySelector<HTMLElement>(".tile")!;
        tile.classList.add("minimized");
        const rowA = els.tilesEl.querySelector<HTMLElement>(".ctl-row")!;
        let scrolled = false;
        rowA.scrollIntoView = () => { scrolled = true; };
        const logRow = els.logsEl.querySelector<HTMLElement>(".log-row")!;
        logRow.click();
        expect(rowA.classList.contains("log-selected")).toBe(true);
        expect(c.selectedLogControl).toBe("P/A");
        expect(scrolled).toBe(false);
        c.disconnect();
    });
});
