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
    });
});