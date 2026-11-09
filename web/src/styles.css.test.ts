// styles.css.test.ts — static CSS regression checks.
//
// The logs banner bug was class-of-bug: a component rule with
// `display: flex` silently overrides the browser's default
// `[hidden] { display: none }` because author stylesheets beat UA
// stylesheets, so `element.hidden = true` appeared dead. Happy-dom
// does not apply CSS, so no dom test can catch it.
//
// These tests parse styles.css as text and assert the structural
// invariant: a global `[hidden] { display: none !important; }`
// guard exists. If it is ever removed, the class of bug returns.

import { describe, expect, it } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";

const css = (): string =>
    fs.readFileSync(path.join(process.cwd(), "src/styles.css"), "utf8");

function stripComments(s: string): string {
    return s.replace(/\/\*[\s\S]*?\*\//g, "");
}

describe("styles.css hidden guard", () => {
    it("has a global [hidden] rule that beats display overrides", () => {
        const body = stripComments(css());
        const match = body.match(/\[hidden\]\s*\{[^}]*\}/);
        expect(match, "no [hidden] rule found").not.toBeNull();
        expect(match?.[0]).toContain("display");
        expect(match?.[0]).toContain("none");
        // !important is required: without it, a component rule
        // like `.logs-banner { display: flex; }` wins on
        // specificity and the hidden attribute stops working.
        expect(match?.[0]).toContain("!important");
    });

    it("does not rely on per-component [hidden] overrides", () => {
        const body = stripComments(css());
        // The global guard makes per-component rules redundant;
        // seeing one means someone worked around the guard instead
        // of keeping it. Flag it so the pattern doesn't drift.
        const perComponent = body.match(
            /[.#][\w-]*\[hidden\][^{]*\{[^}]*display:\s*none(?!\s*!important)/,
        );
        expect(perComponent).toBeNull();
    });

    it("every explicit display rule is paired with the hidden guard", () => {
        // Guard present → no per-component rule can silently
        // defeat hidden, so this is a belt-and-suspenders check
        // that the guard still exists after any future edit.
        const body = stripComments(css());
        expect(body).toMatch(/\[hidden\]\s*\{[^}]*display:\s*none\s*!important/);
    });
});

describe("styles.css selection highlight", () => {
    it("log-selected control rows use box-shadow, not bare outline", () => {
        // The red border must actually be visible: an outline on
        // .ctl-row is clipped by the flex line box and the
        // fieldset padding, so it never showed. The rule must use
        // box-shadow plus a filled background, matching the P&ID
        // .ref-flash treatment.
        const body = stripComments(css());
        const match = body.match(/\.ctl-row\.log-selected\s*\{[^}]*\}/);
        expect(match, "no .ctl-row.log-selected rule").not.toBeNull();
        expect(match?.[0]).toContain("box-shadow");
        expect(match?.[0]).toContain("var(--log-selected)");
        expect(match?.[0]).toContain("background");
    });

    it("selected + flashing rows keep the persistent red box", () => {
        const body = stripComments(css());
        const match = body.match(/\.ctl-row\.log-selected\.ref-flash\s*\{[^}]*\}/);
        expect(match, "missing combined selector").not.toBeNull();
        expect(match?.[0]).toContain("box-shadow");
        expect(match?.[0]).toContain("var(--log-selected)");
    });
});

describe("styles.css sticky header offset", () => {
    it("jump targets declare scroll-margin-top from --header-offset", () => {
        const body = stripComments(css());
        // --header-offset must be defined so scroll-margin-top
        // resolves to a real length in every target rule.
        expect(body).toMatch(/--header-offset:\s*\d+px/);
        for (const selector of [".ctl-row", ".log-row"]) {
            const esc = selector.replace(".", "\\.");
            const match = body.match(new RegExp(`${esc}\\s*\\{[^}]*\\}`));
            expect(match, `${selector} rule missing`).not.toBeNull();
            expect(match?.[0], `${selector} must respect the header`).toContain(
                "scroll-margin-top: var(--header-offset)",
            );
        }
    });
});

describe("styles.css compact sensor/controls tiles", () => {
    const compactRule = (
        body: string,
        selector: RegExp,
        needle: string,
    ): string => {
        const match = body.match(selector)?.[0] ?? "";
        expect(match, `missing rule ${selector}`).not.toBe("");
        expect(match).toContain(needle);
        return match;
    };

    it("keeps tile row spacing packed", () => {
        const body = stripComments(css());
        const tile = body.match(/\.tile\s*\{[^}]*\}/)?.[0] ?? "";
        expect(tile).toContain("font-size: 11px");
        expect(tile).toContain("line-height: 1.25");
        const tileBody = body.match(/\n\s*\.tile-body\s*\{[^}]*\}/)?.[0] ?? "";
        // Stacked single column: controls above sensors.
        expect(tileBody).toContain("grid-template-columns: 1fr");
        expect(tileBody).toContain("gap: 8px");
        const fieldset = body.match(/\.tile fieldset\s*\{[^}]*\}/)?.[0] ?? "";
        expect(fieldset).toContain("margin: 0 0 4px 0");
        expect(fieldset).toContain("padding: 2px 6px");
        const dataCell = body.match(/\.data-table td\s*\{[^}]*\}/)?.[0] ?? "";
        expect(dataCell).toContain("padding: 1px 4px");
        const ctlRow = body.match(/\.ctl-row\s*\{[^}]*\}/)?.[0] ?? "";
        expect(ctlRow).toContain("gap: 6px");
        expect(ctlRow).toContain("margin: 2px 0");
    });

    it("reserves the sensor value column under fixed layout", () => {
        const body = stripComments(css());
        compactRule(
            body,
            /\.tile \.readout-part \.data-table,\s*[\s\S]*?\{[^}]*\}/,
            "table-layout: fixed",
        );
        compactRule(
            body,
            /\.tile \.readout-part \.data-table td:first-child,[\s\S]*?\{[^}]*\}/,
            "width: 62%",
        );
        compactRule(
            body,
            /\.tile \.readout-part \.data-table td:nth-child\(2\),[\s\S]*?\{[^}]*\}/,
            "width: 38%",
        );
        const value = body.match(
            /\.tile \.readout-part \.data-table td:nth-child\(2\),[\s\S]*?\{[^}]*\}/,
        )?.[0] ?? "";
        expect(value).toContain("white-space: nowrap");
        expect(value).toContain("text-align: right");
        expect(value).toContain("font-variant-numeric: tabular-nums");
    });

    it("keeps control inputs pinned while labels shrink", () => {
        const body = stripComments(css());
        compactRule(
            body,
            /\.ctl-row \.label-text\s*\{[^}]*\}/,
            "flex: 1 1 0",
        );
        compactRule(
            body,
            /\.tile input\[type="number"\],[\s\S]*?\{[^}]*\}/,
            "flex: 0 0 auto",
        );
        const ctlRow = body.match(/\.ctl-row\s*\{[^}]*\}/)?.[0] ?? "";
        expect(ctlRow).toContain("flex-wrap: nowrap");
    });

    it("collapses tile sections independently", () => {
        const body = stripComments(css());
        compactRule(
            body,
            /\.tile-section\.section-collapsed \.tile-section-body\s*\{[^}]*\}/,
            "display: none",
        );
        compactRule(
            body,
            /\.tile-section-heading\s*\{[^}]*\}/,
            "cursor: pointer",
        );
    });
});
