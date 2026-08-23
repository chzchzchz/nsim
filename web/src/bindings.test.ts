// bindings.test.ts — pure grouping/label helpers. The wasm-facing
// buildBindings needs a live plant handle and stays covered by the
// manual loop in `make dev`.

import { describe, expect, it } from "vitest";
import {
    displayPart,
    groupByPrefix,
    topLevelPrefix,
    type PartBinding,
} from "./bindings";

function part(part: string): PartBinding {
    return { part, controls: [], sensors: [] };
}

describe("topLevelPrefix", () => {
    it("returns the leading slash-delimited segment", () => {
        expect(topLevelPrefix("ReactorBuilding/Pressurizer/HeatersOn")).toBe(
            "ReactorBuilding",
        );
    });

    it("returns the whole string when unrooted", () => {
        expect(topLevelPrefix("Plant")).toBe("Plant");
        expect(topLevelPrefix("")).toBe("");
    });
});

describe("displayPart", () => {
    it("strips the tile prefix for headings", () => {
        const display = displayPart("Outside");
        expect(display("Outside/DieselGenerator")).toBe("DieselGenerator");
        expect(display("Outside/DieselGenerator/Generator")).toBe(
            "DieselGenerator/Generator",
        );
    });

    it("passes through paths from another prefix unchanged", () => {
        expect(displayPart("Outside")("ReactorBuilding/RPV")).toBe(
            "ReactorBuilding/RPV",
        );
    });
});

describe("groupByPrefix", () => {
    it("buckets by prefix in alphabetical order", () => {
        const groups = groupByPrefix([
            part("TurbineBuilding/Turbine"),
            part("AuxiliaryBuilding/EFWS"),
            part("ReactorBuilding/RPV"),
        ]);
        expect(groups.map((g) => g.prefix)).toEqual([
            "AuxiliaryBuilding",
            "ReactorBuilding",
            "TurbineBuilding",
        ]);
    });

    it("keeps parts sharing a prefix together in input order", () => {
        const groups = groupByPrefix([
            part("Outside/CirculatingWaterPump"),
            part("Outside/DieselGenerator"),
            part("ReactorBuilding/RPV"),
        ]);
        expect(groups).toHaveLength(2);
        expect(groups[0].prefix).toBe("Outside");
        expect(groups[0].bindings.map((b) => b.part)).toEqual([
            "Outside/CirculatingWaterPump",
            "Outside/DieselGenerator",
        ]);
    });

    it("handles an empty binding list", () => {
        expect(groupByPrefix([])).toEqual([]);
    });
});
