// svg.test.ts — the pure zoom-anchor math behind the P&ID viewport:
// zooming must keep the diagram point under the cursor fixed, for
// both directions and arbitrary prior pan offsets.

import { describe, expect, it } from "vitest";
import { anchoredPan } from "./svg";

describe("anchoredPan", () => {
    it("keeps the cursor's diagram point fixed across a scale change", () => {
        const cases = [
            { px: 0, py: 0, prev: 1, next: 2, panX: 0, panY: 0 },
            { px: 300, py: 150, prev: 1, next: 1.25, panX: -40, panY: 12 },
            { px: -50, py: 900, prev: 4, next: 1, panX: 120, panY: -300 },
            { px: 12.5, py: -3.25, prev: 2.5, next: 6.5, panX: 7.75, panY: 0.5 },
        ];
        for (const c of cases) {
            const panned = anchoredPan(c.px, c.py, c.prev, c.next, c.panX, c.panY);
            const before =
                (c.px - c.panX) / c.prev;
            const afterX = (c.px - panned.x) / c.next;
            const afterY = (c.py - panned.y) / c.next;
            expect(afterX).toBeCloseTo(before, 9);
            const beforeY = (c.py - c.panY) / c.prev;
            expect(afterY).toBeCloseTo(beforeY, 9);
        }
    });

    it("is the identity when the scale does not change", () => {
        const panned = anchoredPan(200, 100, 3, 3, 15, -45);
        expect(panned).toEqual({ x: 15, y: -45 });
    });
});
