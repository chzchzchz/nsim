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
});

describe("log entry selection", () => {
    it("clicking an operation row selects it and clicking again deselects", () => {
        const seen: (unknown)[] = [];
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const client = {
            postLogComment: () => Promise.resolve({ seq: 99 }),
        } as never;
        const p = initLogs(host2, client, (e) => seen.push(e));
        const entry = { seq: 7, kind: "operation", atMs: 1000, control: "A/B", value: "1" };
        p.append([entry]);
        const row = host2.querySelector<HTMLElement>(".log-row")!;
        row.click();
        expect(p.selected()).toEqual(entry);
        expect(row.classList.contains("log-selected")).toBe(true);
        row.click();
        expect(p.selected()).toBeNull();
        expect(row.classList.contains("log-selected")).toBe(false);
        expect(seen).toEqual([entry, null]);
    });

    it("clicking another row moves the selection", () => {
        panel.append([
            { seq: 1, kind: "operation", atMs: 1000, control: "A/B", value: "1" },
            { seq: 2, kind: "operation", atMs: 2000, control: "A/C", value: "0" },
        ]);
        const rows = Array.from(host.querySelectorAll<HTMLElement>(".log-row"));
        rows[0].click();
        rows[1].click();
        expect(rows[0].classList.contains("log-selected")).toBe(false);
        expect(rows[1].classList.contains("log-selected")).toBe(true);
        expect(panel.selected()?.seq).toBe(2);
    });

    it("comment rows are not selectable", () => {
        panel.append([
            { seq: 1, kind: "comment", atMs: 1000, comment: "hi" },
        ]);
        const row = host.querySelector<HTMLElement>(".log-row")!;
        expect(row.getAttribute("role")).toBeNull();
        row.click();
        expect(panel.selected()).toBeNull();
    });

    it("select(null) programmatically deselects and fires the callback", () => {
        const seen: (unknown)[] = [];
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        const p = initLogs(host2, client, (e) => seen.push(e));
        const entry = { seq: 4, kind: "operation", atMs: 1000, control: "A/B", value: "1" };
        p.append([entry]);
        host2.querySelector<HTMLElement>(".log-row")!.click();
        p.select(null);
        expect(p.selected()).toBeNull();
        expect(seen).toEqual([entry, null]);
        expect(host2.querySelector(".log-selected")).toBeNull();
    });

    it("eviction of the selected row clears the selection", () => {
        const seen: (unknown)[] = [];
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        const p = initLogs(host2, client, (e) => seen.push(e));
        const first = { seq: 1, kind: "operation", atMs: 1000, control: "A/B", value: "1" };
        p.append([first]);
        host2.querySelector<HTMLElement>(".log-row")!.click();
        expect(p.selected()).toEqual(first);
        // Push 200 more entries so the selected row is evicted.
        const batch: unknown[] = [];
        for (let i = 2; i <= 202; i++) {
            batch.push({ seq: i, kind: "comment", atMs: i * 1000, comment: `c${i}` });
        }
        p.append(batch as never);
        expect(p.selected()).toBeNull();
        expect(seen).toEqual([first, null]);
        expect(host2.querySelectorAll(".log-row")).toHaveLength(200);
    });

    it("Escape deselects from the row and from elsewhere in the panel", () => {
        const seen: (unknown)[] = [];
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        const p = initLogs(host2, client, (e) => seen.push(e));
        const entry = { seq: 8, kind: "operation", atMs: 1000, control: "A/B", value: "1" };
        p.append([entry]);
        const row = host2.querySelector<HTMLElement>(".log-row")!;
        row.click();
        expect(p.selected()).toEqual(entry);
        row.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
        expect(p.selected()).toBeNull();
        expect(seen).toEqual([entry, null]);
        // Re-select and Escape from the composer input.
        row.click();
        const input = host2.querySelector("input")!;
        input.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
        expect(p.selected()).toBeNull();
        expect(seen).toEqual([entry, null, entry, null]);
    });

    it("roving tabindex: only one operation row is tabbable", () => {
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const p = initLogs(host2, client, () => {});
        p.append([
            { seq: 1, kind: "operation", atMs: 1000, control: "A/B", value: "1" },
            { seq: 2, kind: "comment", atMs: 2000, comment: "hi" },
            { seq: 3, kind: "operation", atMs: 3000, control: "A/C", value: "0" },
            { seq: 4, kind: "operation", atMs: 4000, control: "A/D", value: "0" },
        ]);
        const opRows = Array.from(host2.querySelectorAll<HTMLElement>(".log-row[role='button']"));
        expect(opRows.map((r) => r.tabIndex)).toEqual([0, -1, -1]);
        const commentRow = host2.querySelector<HTMLElement>(".log-row.comment")!;
        expect(commentRow.tabIndex).toBe(-1);
        // Selecting the second op row makes it the stop.
        opRows[1].click();
        expect(opRows.map((r) => r.tabIndex)).toEqual([-1, 0, -1]);
        // Deselect (Escape or second click) returns the stop to the
        // first operation row.
        opRows[1].click();
        expect(opRows.map((r) => r.tabIndex)).toEqual([0, -1, -1]);
        // Keyboard activation also moves the stop.
        const last = opRows[2];
        last.focus();
        last.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
        expect(p.selected()?.seq).toBe(4);
        expect(opRows.map((r) => r.tabIndex)).toEqual([-1, -1, 0]);
    });

    it("selected row exposes aria-pressed, not aria-selected", () => {
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const p = initLogs(host2, client, () => {});
        p.append([
            { seq: 9, kind: "operation", atMs: 1000, control: "A/B", value: "1" },
            { seq: 10, kind: "comment", atMs: 2000, comment: "hi" },
        ]);
        const opRow = host2.querySelector<HTMLElement>(".log-row.operation")!;
        expect(opRow.getAttribute("aria-pressed")).toBe("false");
        opRow.click();
        expect(opRow.getAttribute("aria-pressed")).toBe("true");
        expect(opRow.getAttribute("aria-selected")).toBeNull();
        const commentRow = host2.querySelector<HTMLElement>(".log-row.comment")!;
        expect(commentRow.getAttribute("aria-pressed")).toBeNull();
    });

    it("focus follows the new stop after the focused row is pruned", () => {
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const p = initLogs(host2, client, () => {});
        p.append([
            { seq: 1, kind: "operation", atMs: 1000, control: "A/B", value: "1" },
        ]);
        const row = host2.querySelector<HTMLElement>(".log-row.operation")!;
        row.focus();
        expect(document.activeElement).toBe(row);
        // Add a second operation row that will survive pruning to
        // become the new stop; fill the rest with comments.
        p.append([
            { seq: 2, kind: "operation", atMs: 2000, control: "A/C", value: "0" },
        ]);
        const batch: unknown[] = [];
        for (let i = 3; i <= 201; i++) {
            batch.push({ seq: i, kind: "comment", atMs: i * 1000, comment: `c${i}` });
        }
        p.append(batch as never);
        // Row seq=1 was pruned; happy-dom keeps the detached
        // element as activeElement, whereas real browsers drop
        // focus to <body>. Either way syncRovingTabindex should
        // not yank focus when the new stop already has it, and the
        // surviving operation row should be the sole tabindex=0.
        const opRows = Array.from(host2.querySelectorAll<HTMLElement>(".log-row.operation"));
        expect(opRows).toHaveLength(1);
        expect(opRows[0].tabIndex).toBe(0);
        expect(opRows[0].dataset.seq).toBe("2");
    });

    it("isScrollLockoutActive is true right after View and false after 500ms", () => {
        const client = { postLogComment: () => Promise.resolve({ seq: 99 }) } as never;
        host.innerHTML = "";
        document.body.removeChild(host);
        const host2 = document.createElement("section");
        document.body.appendChild(host2);
        const p = initLogs(host2, client, () => {});
        p.append([
            { seq: 1, kind: "comment", atMs: 1000, comment: "foreign" },
        ]);
        p.noteForeign(1, { seq: 1, kind: "comment", atMs: 1000, comment: "foreign" });
        expect(p.isScrollLockoutActive()).toBe(false);
        const viewBtn = Array.from(
            host2.querySelectorAll<HTMLElement>(".logs-banner button"),
        ).find((b) => b.textContent === "View")!;
        const last = host2.querySelector<HTMLElement>(".log-row")!;
        last.scrollIntoView = () => {};
        viewBtn.click();
        expect(p.isScrollLockoutActive()).toBe(true);
        expect(host2.querySelector<HTMLElement>(".logs-banner")!.hidden).toBe(true);
    });
});

describe("logs banner buttons", () => {
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
