// help-button.dom.test.ts — verifies that the "?" popup stays open
// when a cross-reference link is clicked, and that the browser
// jumps to / flashes the target row instead of swapping popup text.

// @vitest-environment happy-dom

import { describe, expect, it, beforeEach } from "vitest";
import { makeHelpButton } from "./bindings";
import { seedDescriptions, helpFor } from "./descriptions";

const DESCS = {
    "ReactorBuilding/RPV/Pressure":
        "RCS pressure in psig. See also [core temp](ReactorBuilding/RPV/CoreTemp).",
    "ReactorBuilding/RPV/CoreTemp": "Core temperature in °R.",
};

describe("help popup (?) cross-reference behavior", () => {
    beforeEach(() => {
        seedDescriptions(DESCS);
    });

    it("keeps the popup open and flashes the target row when a link is clicked", () => {
        const btn = makeHelpButton(helpFor("ReactorBuilding/RPV/Pressure") ?? "");
        document.body.appendChild(btn);

        const target = document.createElement("div");
        target.dataset.bridgeId = "ReactorBuilding/RPV/CoreTemp";
        document.body.appendChild(target);
        // Happy-dom reports rect.top 0 for every element; the
        // flashRow fast-path now requires clearance from the
        // sticky header, so give the target a visible position.
        target.getBoundingClientRect = () =>
            ({ top: 200, bottom: 300, left: 0, right: 100, x: 0, y: 200, width: 100, height: 100, toJSON: () => ({}) }) as DOMRect;
        let scrollOpts: ScrollIntoViewOptions | undefined;
        target.scrollIntoView = (o?: ScrollIntoViewOptions) => { scrollOpts = o; };

        btn.click();
        const popup = btn.nextElementSibling;
        expect(popup).toBeTruthy();

        const initialHtml = popup?.innerHTML ?? "";

        const link = popup?.querySelector<HTMLAnchorElement>("a");
        expect(link).toBeTruthy();
        window.innerHeight = 1200;
        link?.click();

        expect(btn.nextElementSibling?.innerHTML).toBe(initialHtml);
        expect(target.classList.contains("ref-flash")).toBe(true);
        // The on-screen fast-path must not scroll at all.
        expect(scrollOpts).toBeUndefined();
    });
});