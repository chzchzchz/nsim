// scenarios.dom.test.ts — picker and acknowledgement dialog rendering.
// Runs under happy-dom so the real renderScenarioPicker DOM is
// exercised rather than a mock.

// @vitest-environment happy-dom

import { beforeEach, describe, expect, it } from "vitest";
import {
    renderScenarioPicker,
    type ScenarioInfo,
} from "./scenarios";

const entry = (
    id: string,
    number: number,
    category: string,
): ScenarioInfo => ({
    id,
    number,
    category,
    title: `title ${id}`,
    startState: `start ${id}`,
    objective: `objective ${id}`,
    duration: number === 1 ? "45-50 min simulated" : undefined,
});

const catalog: ScenarioInfo[] = [
    entry("alpha", 1, "Startup"),
    entry("beta", 2, "Shutdown"),
];

let host: HTMLElement;

beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
});

function picker() {
    return host.querySelector<HTMLElement>(".scenario-picker")!;
}

describe("renderScenarioPicker", () => {
    it("renders free play plus one card per scenario", () => {
        const started: (string | null)[] = [];
        renderScenarioPicker(host, catalog, {
            onStart: (id) => started.push(id),
        });

        const cards = host.querySelectorAll<HTMLElement>(".scenario-card");
        expect(cards).toHaveLength(catalog.length);
        expect(
            host.querySelector<HTMLElement>(".scenario-freeplay"),
        ).not.toBeNull();
        expect(host.querySelectorAll(".scenario-grid h3")).toHaveLength(0);

        host.querySelector<HTMLElement>(".scenario-freeplay")!.click();
        expect(started).toEqual([null]);
        expect(host.querySelector(".overlay-backdrop")).toBeNull();
    });

    it("groups entries by category with headings", () => {
        renderScenarioPicker(host, catalog, { onStart: () => {} });
        const headings = Array.from(
            host.querySelectorAll<HTMLElement>(".picker-body h3"),
        ).map((h) => h.textContent);
        expect(headings).toEqual(["Startup", "Shutdown"]);
    });

    it("acknowledgement shows start state, objective, and duration", () => {
        renderScenarioPicker(host, catalog, { onStart: () => {} });
        (
            host.querySelector<HTMLElement>(
                '[data-scenario-id="alpha"]',
            ) as HTMLElement
        ).click();

        const text = picker().querySelector(".scenario-detail")!
            .textContent ?? "";
        expect(text).toContain("1. title alpha");
        expect(text).toContain("start alpha");
        expect(text).toContain("objective alpha");
        expect(text).toContain("Expected duration: 45-50 min simulated");

        // Entries without a hint stay silent about duration.
        (
            host.querySelector<HTMLElement>(".scenario-actions .primary")!
        ).click();
    });

    it("begin confirms the choice and closes the overlay", () => {
        const started: (string | null)[] = [];
        renderScenarioPicker(host, catalog, {
            onStart: (id) => started.push(id),
        });
        (
            host.querySelector<HTMLElement>(
                '[data-scenario-id="beta"]',
            ) as HTMLElement
        ).click();
        (
            host.querySelector<HTMLElement>(
                ".scenario-actions .primary",
            ) as HTMLElement
        ).click();

        expect(started).toEqual(["beta"]);
        expect(host.querySelector(".overlay-backdrop")).toBeNull();
    });

    it("back returns to the list without choosing", () => {
        const started: (string | null)[] = [];
        renderScenarioPicker(host, catalog, {
            onStart: (id) => started.push(id),
        });
        (
            host.querySelector<HTMLElement>(
                '[data-scenario-id="beta"]',
            ) as HTMLElement
        ).click();
        (
            host.querySelector<HTMLElement>(
                ".scenario-actions button:not(.primary)",
            ) as HTMLElement
        ).click();

        expect(started).toEqual([]);
        expect(host.querySelectorAll(".scenario-card")).toHaveLength(
            catalog.length,
        );
    });
});
