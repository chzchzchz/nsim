// scenarios.test.ts — catalog grouping for the selection screen. The
// picker mirrors the catalog's section layout, so grouping must keep
// both the first-seen category order and each group's item order.

import { describe, expect, it } from "vitest";
import {
    groupByCategory,
    objectiveStatusText,
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
});

describe("groupByCategory", () => {
    it("buckets entries under their categories", () => {
        const groups = groupByCategory([
            entry("a", 1, "Startup"),
            entry("b", 2, "Shutdown"),
        ]);
        expect(groups).toEqual([
            { category: "Startup", items: [expect.objectContaining({ id: "a" })] },
            { category: "Shutdown", items: [expect.objectContaining({ id: "b" })] },
        ]);
    });

    it("keeps repeated categories in one bucket at first-seen position", () => {
        const groups = groupByCategory([
            entry("a", 1, "Startup"),
            entry("b", 2, "Malfunction transients"),
            entry("c", 3, "Startup"),
            entry("d", 4, "Malfunction transients"),
        ]);
        expect(groups.map((g) => g.category)).toEqual([
            "Startup",
            "Malfunction transients",
        ]);
        expect(groups[0].items.map((s) => s.id)).toEqual(["a", "c"]);
        expect(groups[1].items.map((s) => s.number)).toEqual([2, 4]);
    });

    it("returns no groups for an empty catalog", () => {
        expect(groupByCategory([])).toEqual([]);
    });
});

describe("objectiveStatusText", () => {
    it("names the unmet bounds while pending", () => {
        expect(objectiveStatusText(false, "rods fully inserted")).toBe(
            "pending — rods fully inserted",
        );
    });

    it("confirms completion without detail noise", () => {
        expect(objectiveStatusText(true, "ignored")).toBe("✓ objective met");
    });
});
