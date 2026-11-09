// remote.test.ts — URL normalization and health-probe
// behavior under happy-dom with a mocked fetch: healthy
// connects, 404/refused errors, ready-gating, and the
// uniform error-envelope unwrap.

// @vitest-environment happy-dom

import { describe, expect, it, vi, afterEach } from "vitest";
import { normalizeBase, RemoteClient, RemoteError, toAlarmPanelShape } from "./remote";

afterEach(() => {
    vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200): Response {
    return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
    });
}

describe("normalizeBase", () => {
    it("accepts a clean URL and strips one trailing slash", () => {
        expect(normalizeBase("http://127.0.0.1:8080")).toEqual({
            base: "http://127.0.0.1:8080",
        });
        expect(normalizeBase("http://127.0.0.1:8080/")).toEqual({
            base: "http://127.0.0.1:8080",
        });
        expect(normalizeBase("  https://h/nsim  ")).toEqual({
            base: "https://h/nsim",
        });
    });

    it("rejects empty and scheme-less input", () => {
        expect(normalizeBase("").error).toBeDefined();
        expect(normalizeBase("localhost:8080").error).toBeDefined();
        expect(normalizeBase("ftp://x").error).toContain("http");
    });
});

describe("toAlarmPanelShape (GET /alarms wire → panel)", () => {
    it("maps Go uint8 severity and nanosecond timestamp", () => {
        expect(toAlarmPanelShape({
            ID: "rps.reactor_trip",
            Description: "Reactor trip",
            Severity: 2,
            Active: true,
            Acknowledged: false,
            Timestamp: 1_500_000_000,
        })).toEqual({
            id: "rps.reactor_trip",
            description: "Reactor trip",
            severity: "trip",
            active: true,
            acknowledged: false,
            time_s: 1.5,
        });
        expect(toAlarmPanelShape({
            ID: "x", Description: "y", Severity: 1,
            Active: false, Acknowledged: true, Timestamp: 0,
        }).severity).toBe("warning");
        expect(toAlarmPanelShape({
            ID: "x", Description: "y", Severity: 0,
            Active: true, Acknowledged: false, Timestamp: 500_000_000,
        })).toEqual({
            id: "x", description: "y", severity: "info",
            active: true, acknowledged: false, time_s: 0.5,
        });
    });
});

describe("RemoteClient health probe", () => {
    it("livez true on 200 ok:true, false on refused", async () => {
        vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({ ok: true })));
        const client = new RemoteClient("http://127.0.0.1:8080");
        await expect(client.livez()).resolves.toBe(true);

        vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("refused"); }));
        await expect(client.livez()).resolves.toBe(false);
    });

    it("readyz false during reset (503)", async () => {
        vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({}, 503)));
        const client = new RemoteClient("http://127.0.0.1:8080");
        await expect(client.readyz()).resolves.toBe(false);
    });

    it("unwraps the {error,code} envelope verbatim", async () => {
        vi.stubGlobal("fetch", vi.fn(async () =>
            jsonResponse({ error: "unknown control \"X\"", code: 404 }, 404)));
        const client = new RemoteClient("http://127.0.0.1:8080");
        await expect(client.putControl("X", "1", "c"))
            .rejects.toThrowError(RemoteError);
        try {
            await client.putControl("X", "1", "c");
        } catch (err) {
            expect((err as RemoteError).message).toContain("unknown control");
            expect((err as RemoteError).code).toBe(404);
        }
    });

    it("logs(lo) encodes the range query", async () => {
        const fetched: string[] = [];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            fetched.push(url);
            return jsonResponse([]);
        }));
        const client = new RemoteClient("http://127.0.0.1:8080");
        await client.logs(4);
        expect(fetched[0]).toContain("/logs?lo=4");
    });

    it("putControl percent-encodes slash keys as %2F", async () => {
        const fetched: string[] = [];
        vi.stubGlobal("fetch", vi.fn(async (url: string) => {
            fetched.push(url);
            return jsonResponse({ key: "A/B", part: "A", label: "B", kind: "bool", value: "1", seq: 3 });
        }));
        const client = new RemoteClient("http://127.0.0.1:8080");
        const res = await client.putControl("A/B", "1", "why");
        expect(fetched[0]).toContain("/controls/A%2FB");
        expect(res.seq).toBe(3);
    });
});
