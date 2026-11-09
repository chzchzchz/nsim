// serverPicker.dom.test.ts — Server dialog connect flow under
// happy-dom with a mocked fetch: the livez/readyz gates prevent
// a half-switch to remote mode.

// @vitest-environment happy-dom

import { describe, expect, it, vi, afterEach, beforeEach } from "vitest";
import { renderServerPicker } from "./serverPicker";

function jsonResponse(body: unknown, status = 200): Response {
    return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
    });
}

let host: HTMLElement;

beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
});

afterEach(() => {
    vi.unstubAllGlobals();
    host.remove();
});

function input() {
    return host.querySelector<HTMLInputElement>("#server-url-input")!;
}

function connectBtn() {
    const btns = Array.from(host.querySelectorAll("button"));
    return btns.find((b) => b.textContent === "Connect")!;
}

describe("renderServerPicker", () => {
    it("connects after livez and readyz pass", async () => {
        vi.stubGlobal(
            "fetch",
            vi.fn(async (url: string) => {
                if (url.includes("/livez")) return jsonResponse({ ok: true });
                if (url.includes("/readyz")) return jsonResponse({ ready: true });
                return jsonResponse({}, 404);
            }),
        );
        const connected: string[] = [];
        renderServerPicker(host, null, {
            onConnected: (b) => connected.push(b),
            onDisconnected: () => {},
        });
        input().value = "http://127.0.0.1:8080";
        connectBtn().click();
        await new Promise((r) => setTimeout(r, 50));
        expect(connected).toEqual(["http://127.0.0.1:8080"]);
        expect(host.querySelector(".overlay-backdrop")).toBeNull();
    });

    it("fails when livez is false", async () => {
        vi.stubGlobal(
            "fetch",
            vi.fn(async () => jsonResponse({ ok: false })),
        );
        const connected: string[] = [];
        renderServerPicker(host, null, {
            onConnected: (b) => connected.push(b),
            onDisconnected: () => {},
        });
        input().value = "http://127.0.0.1:8080";
        connectBtn().click();
        await new Promise((r) => setTimeout(r, 50));
        expect(connected).toEqual([]);
        expect(host.querySelector(".server-error")!.textContent).toContain(
            "no nsimserve",
        );
        expect(connectBtn().disabled).toBe(false);
        expect(connectBtn().textContent).toBe("Connect");
    });

    it("fails when the server is alive but not ready", async () => {
        vi.stubGlobal(
            "fetch",
            vi.fn(async (url: string) => {
                if (url.includes("/livez")) return jsonResponse({ ok: true });
                if (url.includes("/readyz")) return jsonResponse({ ready: false });
                return jsonResponse({}, 404);
            }),
        );
        const connected: string[] = [];
        renderServerPicker(host, null, {
            onConnected: (b) => connected.push(b),
            onDisconnected: () => {},
        });
        input().value = "http://127.0.0.1:8080";
        connectBtn().click();
        await new Promise((r) => setTimeout(r, 60));
        expect(connected).toEqual([]);
        expect(host.querySelector(".server-error")!.textContent).toContain(
            "rebuilding",
        );
    });

    it("shows disconnect button and disconnects when connected", () => {
        const events: string[] = [];
        renderServerPicker(host, "http://127.0.0.1:8080", {
            onConnected: (b) => events.push(`conn:${b}`),
            onDisconnected: () => events.push("disc"),
        });
        host
            .querySelectorAll("button")
            .forEach((b) => {
                if (b.textContent === "Disconnect") b.click();
            });
        expect(events).toEqual(["disc"]);
    });

    it("rejects an invalid URL inline without network", async () => {
        vi.stubGlobal(
            "fetch",
            vi.fn(async () => jsonResponse({ ok: true })),
        );
        renderServerPicker(host, null, {
            onConnected: () => {},
            onDisconnected: () => {},
        });
        input().value = "not-a-url";
        connectBtn().click();
        await new Promise((r) => setTimeout(r, 30));
        expect(host.querySelector(".server-error")!.textContent).toContain(
            "valid URL",
        );
    });
});
