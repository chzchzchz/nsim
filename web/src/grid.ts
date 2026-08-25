// grid.ts — drag-to-swap rearrangement for the tile grid. Pressing
// on a tile heading (the prefix line) arms a gesture: dragging past
// SWAP_THRESHOLD_PX along one axis marks the neighbouring tile in
// that direction as the pending swap, falling back inside the band
// clears it again, and releasing applies whatever swap is still
// pending.
//
// Pointer events (not mouse events) drive the gesture so it works
// for touch and pen as well; the heading carries touch-action: none
// so drags are not hijacked by page scrolling.

const SWAP_THRESHOLD_PX = 100;

type Dir = "left" | "right" | "up" | "down";

interface DragState {
    tile: HTMLElement;
    startX: number;
    startY: number;
    target: HTMLElement | null;
}

// enablePanelSwaps installs one delegated pointerdown listener on the
// grid container, so tiles created later (e.g. by a reset rebuild)
// are draggable without re-wiring.
export function enablePanelSwaps(grid: HTMLElement): void {
    let drag: DragState | null = null;

    function clearPending(): void {
        if (!drag) {
            return;
        }
        drag.tile.classList.remove("swap-source");
        if (drag.target) {
            drag.target.classList.remove("swap-target");
        }
        drag.target = null;
    }

    function onMove(ev: PointerEvent): void {
        if (!drag) {
            return;
        }
        const dx = ev.clientX - drag.startX;
        const dy = ev.clientY - drag.startY;
        clearPending();
        const ax = Math.abs(dx);
        const ay = Math.abs(dy);
        if (Math.max(ax, ay) < SWAP_THRESHOLD_PX) {
            return;
        }
        const dir: Dir =
            ax >= ay ? (dx > 0 ? "right" : "left") : (dy > 0 ? "down" : "up");
        const target = neighbour(grid, drag.tile, dir);
        if (!target) {
            return;
        }
        drag.target = target;
        drag.tile.classList.add("swap-source");
        target.classList.add("swap-target");
    }

    function onUp(): void {
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", onUp);
        window.removeEventListener("pointercancel", onUp);
        if (!drag) {
            return;
        }
        const { tile, target } = drag;
        clearPending();
        tile.classList.remove("tile-grabbing");
        drag = null;
        if (target) {
            swapTiles(tile, target);
            // Bubble through #tiles so main.ts persists the layout.
            tile.dispatchEvent(
                new CustomEvent("panel-layout-changed", { bubbles: true }),
            );
        }
    }

    grid.addEventListener("pointerdown", (ev: PointerEvent) => {
        if (ev.button !== 0 || drag) {
            return;
        }
        if (ev.target instanceof Element && ev.target.closest(".tile-toggle")) {
            return;
        }
        const heading =
            ev.target instanceof Element
                ? ev.target.closest<HTMLElement>(".tile > h2")
                : null;
        const tile = heading?.closest<HTMLElement>(".tile");
        if (!heading || !tile || tile.parentElement !== grid) {
            return;
        }
        // preventDefault keeps the press from starting a text
        // selection while the operator drags across panels.
        ev.preventDefault();
        drag = { tile, startX: ev.clientX, startY: ev.clientY, target: null };
        tile.classList.add("tile-grabbing");
        window.addEventListener("pointermove", onMove);
        window.addEventListener("pointerup", onUp);
        window.addEventListener("pointercancel", onUp);
    });
}

// neighbour resolves the tile adjacent to `tile` in `dir` using the
// rendered geometry of the auto-fit grid: tiles sharing an offset
// top form a visual row, so neighbours are found without guessing
// how many columns the viewport currently produces.
function neighbour(
    grid: HTMLElement,
    tile: HTMLElement,
    dir: Dir,
): HTMLElement | null {
    const tiles = Array.from(
        grid.querySelectorAll<HTMLElement>(":scope > .tile"),
    );
    if (tiles.length === 0) {
        return null;
    }
    const rows: { top: number; items: HTMLElement[] }[] = [];
    const rowOf = new Map<HTMLElement, number>();
    for (const t of tiles) {
        const top = t.offsetTop;
        let i = rows.findIndex((r) => r.top === top);
        if (i < 0) {
            i = rows.length;
            rows.push({ top, items: [] });
        }
        rows[i].items.push(t);
        rowOf.set(t, i);
    }
    for (const r of rows) {
        r.items.sort((a, b) => a.offsetLeft - b.offsetLeft);
    }
    rows.sort((a, b) => a.top - b.top);

    const ri = rowOf.get(tile);
    if (ri === undefined) {
        return null;
    }
    const row = rows[ri];
    const col = row.items.indexOf(tile);
    switch (dir) {
        case "right":
            return row.items[col + 1] ?? null;
        case "left":
            return row.items[col - 1] ?? null;
        case "up":
            return ri > 0 ? closestByLeft(rows[ri - 1].items, tile) : null;
        case "down":
            return ri < rows.length - 1
                ? closestByLeft(rows[ri + 1].items, tile)
                : null;
    }
}

function closestByLeft(items: HTMLElement[], tile: HTMLElement): HTMLElement {
    let best = items[0];
    let bestDist = Infinity;
    for (const t of items) {
        const d = Math.abs(t.offsetLeft - tile.offsetLeft);
        if (d < bestDist) {
            best = t;
            bestDist = d;
        }
    }
    return best;
}

function swapTiles(a: HTMLElement, b: HTMLElement): void {
    const parent = a.parentElement;
    if (!parent || b.parentElement !== parent || a === b) {
        return;
    }
    // Capture both insertion points up front; rebasing aNext when b
    // directly follows a keeps adjacent swaps from being no-ops.
    const aNext = a.nextSibling === b ? a : a.nextSibling;
    const bNext = b.nextSibling;
    parent.insertBefore(b, aNext);
    parent.insertBefore(a, bNext);
}
