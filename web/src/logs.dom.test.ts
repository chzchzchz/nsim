// logs.dom.test.ts — logs banner buttons View/X under
// happy-dom: View scrolls to the newest row and hides the banner,
// X hides it, and the text click forwards to View.

// @vitest-environment happy-dom

import { beforeEach, describe, expect, it } from "vitest";
import { initLogs } from "./logs";

let host: HTMLElement;
let panel: ReturnType<typeof initLogs>;

beforeEach(() => {
    host = document.createElement("section");
    host.id = "logs";
    document.body.appendChild(host);
    const client = {
        postLogComment: () =>
            Promise.resolve({ seq: 99, comment: "", atMs: 0 }),
    } as never;
    panel = initLogs(host, client);
});

describe("logs banner buttons", () => {
    it("shows the banner on foreign entries and hides on X", () => {
        panel.append([
            { seq: 1, kind: "comment", atMs: 1000, comment: "hi" },
        ]);
        panel.noteForeign(1, { seq: 1, kind: "comment", atMs: 1000, comment: "hi" });

        const banner = host.querySelector<HTMLElement>(".logs-banner")!;
        expect(banner.hidden).toBe(false);
        expect(banner.textContent).toContain("1 new entry");

        const dismiss = Array.from(banner.querySelectorAll("button")).find(
            (b) => b.textContent === "×",
        )!;
        dismiss.click();
        expect(banner.hidden).toBe(true);
    });

    it("View scrolls to the newest row and hides the banner", () => {
        panel.append([
            { seq: 1, kind: "comment", atMs: 1000, comment: "first" },
            { seq: 2, kind: "comment", atMs: 2000, comment: "foreign" },
        ]);
        panel.noteForeign(1, { seq: 2, kind: "comment", atMs: 2000, comment: "foreign" });

        const banner = host.querySelector<HTMLElement>(".logs-banner")!;
        expect(banner.hidden).toBe(false);

        const list = host.querySelector<HTMLElement>(".logs-list")!;
        const last = list.lastElementChild as HTMLElement;
        let scrolled: HTMLElement | null = null;
        last.scrollIntoView = (opts?: ScrollIntoViewOptions) => {
            scrolled = last;
        };

        const viewBtn = Array.from(banner.querySelectorAll("button")).find(
            (b) => b.textContent === "View",
        )!;
        viewBtn.click();
        expect(scrolled).toBe(last);
        expect(banner.hidden).toBe(true);
    });

    it("text click forwards to View", () => {
        panel.append([
            { seq: 1, kind: "operation", atMs: 1000, control: "A/B", value: "1" },
        ]);
        panel.noteForeign(1, { seq: 1, kind: "operation", atMs: 1000, control: "A/B", value: "1" });
        const banner = host.querySelector<HTMLElement>(".logs-banner")!;
        const span = banner.querySelector("span")!;
        span.click();
        expect(banner.hidden).toBe(true);
    });

    it("keeps working when foreign entries accumulate", () => {
        panel.append([
            { seq: 1, kind: "comment", atMs: 1000, comment: "a" },
        ]);
        panel.noteForeign(1, { seq: 1, kind: "comment", atMs: 1000, comment: "a" });
        const banner = host.querySelector<HTMLElement>(".logs-banner")!;
        expect(banner.textContent).toContain("1 new entry");
        // Second foreign entry: count increments, buttons still live.
        panel.append([
            { seq: 2, kind: "comment", atMs: 2000, comment: "b" },
        ]);
        panel.noteForeign(1, { seq: 2, kind: "comment", atMs: 2000, comment: "b" });
        expect(banner.textContent).toContain("2 new entries");
        const dismiss = Array.from(banner.querySelectorAll("button")).find(
            (b) => b.textContent === "×",
        )!;
        dismiss.click();
        expect(banner.hidden).toBe(true);
    });
});
