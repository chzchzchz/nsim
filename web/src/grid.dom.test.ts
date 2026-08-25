// grid.dom.test.ts — drag-to-swap gesture under happy-dom using
// real pointer events: a heading drag past the threshold swaps the
// dragged tile with the neighbour in that direction, releases below
// it are no-ops, and presses on other rows never start a drag.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { enablePanelSwaps } from "./grid";

const SWAP_THRESHOLD_PX = 100;

function makeTile(grid: HTMLElement, title: string): HTMLElement {
    const tile = document.createElement("div");
    tile.className = "tile";
    const h2 = document.createElement("h2");
    h2.textContent = title;
    tile.appendChild(h2);
    const body = document.createElement("div");
    body.className = "tile-body";
    tile.appendChild(body);
    grid.appendChild(tile);
    // happy-dom leaves offsetTop/offsetLeft at 0; neighbour lookup
    // treats same-top tiles as one visual row ordered by left.
    return tile;
}

function pointer(
    el: HTMLElement,
    type: string,
    x: number,
    y: number,
): void {
    el.dispatchEvent(
        new PointerEvent(type, {
            button: 0,
            clientX: x,
            clientY: y,
            bubbles: true,
        }),
    );
}

describe("enablePanelSwaps", () => {
    it("swaps rightward past the threshold and persists once", () => {
        const grid = document.createElement("div");
        grid.id = "tiles";
        document.body.appendChild(grid);
        const a = makeTile(grid, "A");
        makeTile(grid, "B");
        const changes: Event[] = [];
        grid.addEventListener("panel-layout-changed", (e) => changes.push(e));
        enablePanelSwaps(grid);

        pointer(a.querySelector("h2")!, "pointerdown", 10, 10);
        expect(a.classList.contains("tile-grabbing")).toBe(true);

        window.dispatchEvent(
            new PointerEvent("pointermove", {
                clientX: 10 + SWAP_THRESHOLD_PX + 5,
                clientY: 10,
            }),
        );
        expect(a.classList.contains("swap-source")).toBe(true);
        expect(grid.querySelectorAll(".swap-target")).toHaveLength(1);

        window.dispatchEvent(
            new PointerEvent("pointerup", { clientX: 115, clientY: 10 }),
        );

        expect(grid.children[0].textContent).toContain("B");
        expect(grid.children[1].textContent).toContain("A");
        expect(changes).toHaveLength(1);
        expect(document.querySelectorAll(".tile-grabbing")).toHaveLength(0);
    });

    it("does not swap when released inside the dead zone", () => {
        const grid = document.createElement("div");
        document.body.appendChild(grid);
        const a = makeTile(grid, "A");
        makeTile(grid, "B");
        enablePanelSwaps(grid);

        pointer(a.querySelector("h2")!, "pointerdown", 10, 10);
        window.dispatchEvent(
            new PointerEvent("pointermove", {
                clientX: 10 + SWAP_THRESHOLD_PX - 1,
                clientY: 10,
            }),
        );
        window.dispatchEvent(
            new PointerEvent("pointerup", { clientX: 109, clientY: 10 }),
        );
        expect(grid.children[0].textContent).toContain("A");
    });

    it("ignores presses outside tile headings", () => {
        const grid = document.createElement("div");
        document.body.appendChild(grid);
        const tile = makeTile(grid, "A");
        makeTile(grid, "B");
        enablePanelSwaps(grid);

        pointer(tile.querySelector(".tile-body")!, "pointerdown", 5, 5);
        expect(document.querySelectorAll(".tile-grabbing")).toHaveLength(0);
    });
});
