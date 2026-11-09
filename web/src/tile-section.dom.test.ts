// tile-section.dom.test.ts — Controls and Sensors sections of a
// tile collapse independently: clicking or Enter/Space on a
// section heading toggles only that section, and the sibling
// stays visible so an operator can watch sensor impact while
// its controls are hidden.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { applySectionCollapsed, buildTile } from "./tile";

function build(host: HTMLElement): void {
    const rows: never[] = [];
    buildTile(host, "PlantOperation", () => {}, () => [], rows as never);
}

describe("tile section collapse", () => {
    it("collapses the controls section without hiding sensors", () => {
        const host = document.createElement("div");
        document.body.appendChild(host);
        build(host);

        const controls = host.querySelector<HTMLElement>(
            ".tile-section:first-child",
        );
        const sensors = host.querySelector<HTMLElement>(
            ".tile-section:last-child",
        );
        expect(controls).not.toBeNull();
        expect(sensors).not.toBeNull();
        expect(
            controls!.querySelector(".tile-section-heading")?.textContent,
        ).toContain("Controls");

        let emitted = 0;
        host.addEventListener("section-layout-changed", () => {
            emitted++;
        });

        controls!.querySelector<HTMLElement>(".tile-section-heading")!.click();
        expect(controls!.classList.contains("section-collapsed")).toBe(true);
        expect(sensors!.classList.contains("section-collapsed")).toBe(false);
        expect(emitted).toBe(1);

        const heading = sensors!.querySelector<HTMLElement>(
            ".tile-section-heading",
        )!;
        heading.dispatchEvent(
            new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
        );
        expect(sensors!.classList.contains("section-collapsed")).toBe(true);

        applySectionCollapsed(controls!, false);
        expect(controls!.classList.contains("section-collapsed")).toBe(false);
        expect(
            controls!.querySelector(".tile-section-toggle")?.textContent,
        ).toBe("−");

        host.remove();
    });
});
