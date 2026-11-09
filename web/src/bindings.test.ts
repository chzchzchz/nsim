// bindings.test.ts — pure grouping/label helpers. The wasm-facing
// buildBindings needs a live plant handle and stays covered by the
// manual loop in `make dev`.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import {
    displayPart,
    groupByPrefix,
    headerClearance,
    HEADER_OFFSET_PX,
    isAlarmControl,
    jumpBack,
    recordReturn,
    rememberReturn,
    takeReturn,
    bindReturnKey,
    sortBindings,
    topLevelPrefix,
    type Entry,
    type PartBinding,
    type ReturnSlot,
} from "./bindings";

function part(part: string): PartBinding {
    return { part, controls: [], sensors: [] };
}

describe("isAlarmControl", () => {
    it("matches the alarm acknowledgment namespace", () => {
        expect(isAlarmControl("Alarms/Reset")).toBe(true);
        expect(isAlarmControl("Alarms/AcknowledgeAll")).toBe(true);
    });

    it("leaves every other id alone", () => {
        expect(isAlarmControl("Plant/Alarms")).toBe(false);
        expect(isAlarmControl("Alarms/Active")).toBe(true);
        expect(
            isAlarmControl("ReactorBuilding/ReactorProtectionSystem/Reset"),
        ).toBe(false);
    });
});

describe("sortBindings", () => {
    function entry(id: string): Entry {
        return { id, meta: { part: "", label: id, kind: "bool" } };
    }

    it("sorts controls and sensors within each part by id", () => {
        const sorted = sortBindings([
            {
                part: "ReactorBuilding/Pressurizer",
                controls: [
                    entry("ReactorBuilding/Pressurizer/SprayOpen"),
                    entry("ReactorBuilding/Pressurizer/HeatersOn"),
                ],
                sensors: [
                    entry("ReactorBuilding/Pressurizer/Water"),
                    entry("ReactorBuilding/Pressurizer/Pressure"),
                ],
            },
        ]);
        expect(sorted[0].controls.map((e) => e.id)).toEqual([
            "ReactorBuilding/Pressurizer/HeatersOn",
            "ReactorBuilding/Pressurizer/SprayOpen",
        ]);
        expect(sorted[0].sensors.map((e) => e.id)).toEqual([
            "ReactorBuilding/Pressurizer/Pressure",
            "ReactorBuilding/Pressurizer/Water",
        ]);
    });

    it("orders parts alphabetically and does not mutate inputs", () => {
        const first: PartBinding = {
            part: "TurbineBuilding/Turbine",
            controls: [],
            sensors: [],
        };
        const second: PartBinding = {
            part: "AuxiliaryBuilding/DieselGenerator",
            controls: [],
            sensors: [],
        };
        const sorted = sortBindings([first, second]);
        expect(sorted.map((b) => b.part)).toEqual([
            "AuxiliaryBuilding/DieselGenerator",
            "TurbineBuilding/Turbine",
        ]);
        expect([first, second]).toEqual([
            { part: "TurbineBuilding/Turbine", controls: [], sensors: [] },
            {
                part: "AuxiliaryBuilding/DieselGenerator",
                controls: [],
                sensors: [],
            },
        ]);
    });
});

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
            "DieselGenerator/<wbr>Generator",
        );
    });

    it("passes through paths from another prefix unchanged", () => {
        expect(displayPart("Outside")("ReactorBuilding/RPV")).toBe(
            "ReactorBuilding/<wbr>RPV",
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

describe("return slot (backtick)", () => {
    it("records the prior position and returns it once", () => {
        const slot: ReturnSlot = { y: null };
        recordReturn(slot, 300, false);
        expect(takeReturn(slot)).toEqual({ y: 300 });
        expect(takeReturn(slot)).toBeNull();
    });

    it("skips recording when the target is already on screen", () => {
        const slot: ReturnSlot = { y: null };
        recordReturn(slot, 300, true);
        expect(takeReturn(slot)).toBeNull();
    });

    it("drops the older position when a newer jump overwrites it", () => {
        const slot: ReturnSlot = { y: null };
        recordReturn(slot, 900, false);
        recordReturn(slot, 1500, false);
        expect(takeReturn(slot)).toEqual({ y: 1500 });
        expect(takeReturn(slot)).toBeNull();
    });

    it("jumpBack restores the recorded position once", () => {
        // Off-screen jump: rememberReturn records the current
        // scrollY, and the backtick-style jumpBack scrolls to it.
        const offRow = document.createElement("div");
        offRow.getBoundingClientRect = () =>
            ({ top: 0, bottom: 50, left: 0, right: 100, x: 0, y: 0, width: 100, height: 50, toJSON: () => ({}) }) as DOMRect;
        Object.defineProperty(window, "scrollY", { value: 400, configurable: true });
        Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
        rememberReturn(offRow);
        const scrolled: ScrollToOptions[] = [];
        window.scrollTo = ((o?: ScrollToOptions) => {
            if (o) scrolled.push(o);
        }) as typeof window.scrollTo;
        expect(jumpBack()).toBe(true);
        expect(scrolled).toEqual([{ top: 400, behavior: "smooth" }]);
        // Slot is single-shot: a second backtick does nothing.
        expect(jumpBack()).toBe(false);
    });
});

describe("headerClearance (sticky header offset)", () => {
    const H = 800;
    it("treats a rect under the header as off-screen", () => {
        // Row flush against the top edge (e.g. pinned under the
        // sticky banner): rect.top = 0/20 looks "on screen" to
        // the old rect.top >= 0 check but is hidden by the header.
        expect(headerClearance({ top: 0, bottom: 100 }, H)).toBe(false);
        expect(headerClearance({ top: HEADER_OFFSET_PX - 1, bottom: 200 }, H)).toBe(false);
        expect(headerClearance({ top: HEADER_OFFSET_PX, bottom: 200 }, H)).toBe(true);
        expect(headerClearance({ top: 600, bottom: 700 }, H)).toBe(true);
    });

    it("still respects the viewport bottom edge", () => {
        // A rect whose bottom goes past the viewport is off-screen
        // regardless of header.
        expect(headerClearance({ top: HEADER_OFFSET_PX, bottom: H + 1 }, H)).toBe(false);
        expect(headerClearance({ top: HEADER_OFFSET_PX, bottom: H }, H)).toBe(true);
    });
});
