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
