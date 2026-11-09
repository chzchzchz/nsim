// sanitize.dom.test.ts — sanitization helpers under
// happy-dom. Remote JSON must never become executable markup:
// check the spoof vectors a hostile nsimserve could send via
// controls/labels/logs/alarms.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { escapeHtml, sanitizeMarkdownHtml } from "./sanitize";

describe("escapeHtml", () => {
    it("escapes the five dangerous characters", () => {
        expect(escapeHtml(`<script>alert(1)</script>`)).toBe(
            "&lt;script&gt;alert(1)&lt;/script&gt;",
        );
        expect(escapeHtml(`a & "b" 'c'`)).toBe(
            "a &amp; &quot;b&quot; &#39;c&#39;",
        );
    });

    it("round-trips benign text unchanged", () => {
        expect(escapeHtml("Reactor pressure")).toBe("Reactor pressure");
    });
});

describe("sanitizeMarkdownHtml", () => {
    it("strips script/style/iframe subtrees", () => {
        const out = sanitizeMarkdownHtml(
            `<p>hi</p><script>alert(1)</script><style>body{}</style><iframe src="x"></iframe>`,
        );
        const doc = document.createElement("div");
        doc.innerHTML = out;
        expect(out).not.toContain("<script");
        expect(out).not.toContain("<style");
        expect(out).not.toContain("<iframe");
        expect(doc.textContent).toContain("hi");
    });

    it("strips on* handlers and javascript: URIs", () => {
        const out = sanitizeMarkdownHtml(
            `<p onclick="alert(1)"><a href="javascript:alert(1)">click</a><a href="https://ok.example/">ok</a></p>`,
        );
        expect(out).not.toContain("onclick");
        expect(out).not.toContain("javascript:");
        expect(out).toContain("https://ok.example/");
    });

    it("removes foreignObject and namespaced handlers", () => {
        const out = sanitizeMarkdownHtml(
            `<p>x</p><svg><foreignObject><body><p onload="alert(1)">evil</p></body></foreignObject></svg>`,
        );
        const doc = document.createElement("div");
        doc.innerHTML = out;
        expect(out).not.toContain("foreignObject");
        expect(out).not.toContain("onload");
        expect(doc.textContent).not.toContain("evil");
    });

    it("strips tab/space-obfuscated javascript: URIs", () => {
        const out = sanitizeMarkdownHtml(
            `<p><a href=" java\x09script:alert(1)">bad</a></p>`.replace("\\x09", "\t"),
        );
        expect(out).not.toContain("javascript:");
        expect(out.indexOf("java") === -1 || !out.includes("alert"));
    });

    it("strips url() javascript: carriers in style", () => {
        const out = sanitizeMarkdownHtml(
            `<p style="background:url(javascript:alert(1))">x</p>`,
        );
        expect(out).not.toContain("javascript:");
    });
    it("preserves emphasis and internal anchor markup", () => {
        const out = sanitizeMarkdownHtml(
            `<p><em>core temp</em> <a href="ReactorBuilding/RPV/CoreTemp">ref</a></p>`,
        );
        expect(out).toContain("<em>core temp</em>");
        expect(out).toContain('href="ReactorBuilding/RPV/CoreTemp"');
    });
});

// Regression tests for the sinks found in the audit:
// - displayPart(part).innerHTML  → padded with escaped HTML, <wbr> only
// - openTip(img onerror ...)  → scripts/loaders removed
import { displayPart } from "./bindings";
import { renderScenarioPicker, type ScenarioInfo } from "./scenarios";

describe("sink regression tests", () => {
    it("displayPart escapes remote part paths", () => {
        const d = displayPart("R");
        const out = d("R/<img src=x onerror=alert(1)>/Core");
        expect(out).not.toContain("<img");
        expect(out).toContain("&lt;img");
        // Legitimate wrapping still present.
        expect(out).toContain("<wbr>");
    });

    it("renderScenarioPicker title renders as text (no script tags)", () => {
        const host = document.createElement("div");
        document.body.appendChild(host);
        const evil: ScenarioInfo[] = [
            {
                id: "x",
                number: 1,
                category: "Cat",
                title: "<img src=x onerror=alert(1)>",
                startState: "<script>alert(1)</script>",
                objective: '"><svg onload=alert(1)>',
            },
        ];
        renderScenarioPicker(host, evil, { onStart: () => {} });
        expect(host.querySelector("img")).toBeNull();
        expect(host.querySelector("script")).toBeNull();
        expect(host.querySelector("svg")).toBeNull();
        expect(host.textContent).toContain("<img");
    });
});

// logs.ts sink: formatRow output is assigned to textContent, never
// innerHTML, so arbitrary HTML from control/comment/value renders
// literally.
import { initLogs } from "./logs";

describe("logs.txt content never becomes markup", () => {
    it("appends hostile log text as a literal string", () => {
        const hostEl = document.createElement("section");
        document.body.appendChild(hostEl);
        const client = {
            postLogComment: () => Promise.resolve({ seq: 99, comment: "", atMs: 0 }),
        } as never;
        const panel = initLogs(hostEl, client);
        panel.append([
            {
                seq: 1,
                kind: "comment",
                atMs: 1000,
                comment: '<img src=x onerror=alert(1)><script>alert(1)</script>',
            },
            {
                seq: 2,
                kind: "operation",
                atMs: 2000,
                control: "X<script>alert(2)</script>",
                value: '"><svg onload=alert(3)>',
            },
        ]);
        expect(hostEl.querySelector("img")).toBeNull();
        expect(hostEl.querySelector("script")).toBeNull();
        expect(hostEl.querySelector("svg")).toBeNull();
        expect(hostEl.textContent).toContain("<img");
        expect(hostEl.textContent).toContain("<svg");
    });
});
