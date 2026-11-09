// main.dom.test.ts — init smoke test: main() boots under
// happy-dom with a mocked wasm catalog and completes without a
// TDZ/reference error. This is the class of failure that vitest
// alone could not catch while remote state lived below the
// synchronous picker open.

// @vitest-environment happy-dom

import { beforeEach, afterEach, describe, expect, it } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import { setWasmLoaderForTest } from "./wasm";

const FakePlant = () => {
    return {
        step: () => {},
        simTimeMs: () => 0,
        controlValuesJSON: () => JSON.stringify({ values: {} }),
        sensorJSON: () => "[]",
        sensorValuesJSON: () => JSON.stringify({ values: {} }),
        getSvg: () => "<svg/>",
        getMeta: () => new Error("unknown"),
        controlKeys: () => [],
        sensorKeys: () => [],
        setControl: () => null,
        submitCommands: () => null,
        commandQueueRev: () => 0,
        commandQueueJSON: () => JSON.stringify({ rev: 0, entries: [] }),
        commandLogJSON: () => "[]",
        objectiveJSON: () => JSON.stringify({ active: false, done: false, detail: "" }),
        toJSON: () => "{}",
    };
};

const fakeCatalog = [
    {
        id: "alpha",
        number: 1,
        category: "Startup",
        title: "Alpha",
        startState: "cold",
        objective: "pressurize",
    },
];

beforeEach(() => {
    // Cheap DOM: enough ids for main() to proceed without null hits.
    const body = fs.readFileSync(
        path.join(process.cwd(), "index.html"),
        "utf8",
    );
    const html = body.match(/<body>([\s\S]*)<\/body>/)?.[1] ?? "";
    document.body.innerHTML = html.replace(/<script[\s\S]*?<\/script>/g, "");
    // main() calls loadWasm() then loadDescriptions(), both of
    // which invoke fetch. Stub them before the import resolves.
    const origFetch = globalThis.fetch as typeof globalThis.fetch;
    (globalThis as unknown as { fetch: typeof fetch }).fetch = (async (
        url: string | URL | Request,
    ): Promise<Response> => {
        const u = String(url);
        if (u.endsWith("descriptions.json")) {
            return new Response(JSON.stringify({}), {
                headers: { "Content-Type": "application/json" },
            });
        }
        if (u.endsWith("nsim.wasm")) {
            // loadWasm compiles the real blob; in tests we never
            // want the wasm. Reject so it falls into try/catch
            // paths? No — wasm.ts has no catch-all; instead make
            // loadWasm appear to succeed by short-circuiting below.
            return (origFetch as unknown as (url: string, init?: object) => Promise<Response>)(u) as never;
        }
        return new Response("{}", { status: 404 });
    }) as typeof globalThis.fetch;

    (window as unknown as Record<string, unknown>).nsimNewPlant = FakePlant;
    (window as unknown as Record<string, unknown>).nsimScenarios = () =>
        JSON.stringify(fakeCatalog);
    // requestAnimationFrame: happy-dom schedules real timers; a
    // no-op prevents an endless tick loop in the test.
    (window as unknown as Record<string, unknown>).requestAnimationFrame = () => 0;
    (globalThis as unknown as Record<string, unknown>).requestAnimationFrame = () => 0;
    // wasm.ts instantiates with go.run; stub window.Go when present.
});

afterEach(() => {
    document.body.innerHTML = "";
});

describe("main() init", () => {
    it("boots with a non-empty catalog without TDZ errors", async () => {
        // Silence the real wasm loader by flagging the window.
        // wasm.ts does `new window.Go()` — if Go is absent it
        // throws, so we install a no-op polyfill ahead of time.
        (window as unknown as Record<string, unknown>).Go = class {
            importObject = {};
            async run() {
                // No-op: never exits, never resolves.
                return new Promise<void>(() => {});
            }
        };
        // Skip the real fetch for the wasm blob: stub WebAssembly.
        const origInstantiateStreaming = WebAssembly.instantiateStreaming;
        (WebAssembly as unknown as Record<string, unknown>).instantiateStreaming =
            async () => ({
                instance: {},
                module: {},
            });
        const origInstantiate = WebAssembly.instantiate;
        (WebAssembly as unknown as Record<string, unknown>).instantiate =
            async () => ({
                instance: {},
                module: {},
            });

        try {
            // Import main.ts → its top-level main().catch runs.
            // A TDZ error would leave #sim-status showing
            // "init failed: ReferenceError...".
            setWasmLoaderForTest(async () => {});
            // @ts-expect-error: query suffix forces a fresh module instance
            await import("./main.ts?init-smoke");
            // Give microtasks a chance to settle.
            await new Promise((r) => setTimeout(r, 50));
            const status = document.getElementById("sim-status");
            expect(status).not.toBeNull();
            expect(status?.textContent ?? "").not.toContain("init failed");
            // The scenario picker auto-opens on a non-empty catalog
            // and must include the remote entry point.
            expect(
                document.querySelector(".scenario-server-btn"),
            ).not.toBeNull();
            expect(document.querySelector(".scenario-card")).not.toBeNull();
        } finally {
            (WebAssembly as unknown as Record<string, unknown>).instantiateStreaming =
                origInstantiateStreaming;
            (WebAssembly as unknown as Record<string, unknown>).instantiate =
                origInstantiate;
        }
    });
});
