// tile-layout.dom.test.ts — tile grid must not spill sensor values
// off the page when a label is a long unbroken token such as
// CorrosionProductControlDegraded. Stylesheet rules alone were the
// previous guard; this builds the real DOM shape via buildTile
// and asserts, through the happy-dom cascade, that the sensor
// value column stays pinned and the label can shrink/wrap.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";

import { buildTile } from "./tile";

function installStyles(): void {
    const css = fs.readFileSync(
        path.join(process.cwd(), "src/styles.css"),
        "utf8",
    );
    const style = document.createElement("style");
    style.textContent = css;
    document.head.appendChild(style);
}

describe("tile layout under long labels", () => {
    it("keeps sensor values inside the tile with long labels", () => {
        installStyles();

        const host = document.createElement("div");
        host.id = "tiles";
        document.body.appendChild(host);

        const renderControls = (h: HTMLElement): void => {
            const fsEl = document.createElement("fieldset");
            const legend = document.createElement("legend");
            legend.textContent = "CVCS";
            fsEl.appendChild(legend);
            const row = document.createElement("div");
            row.className = "ctl-row";
            const label = document.createElement("span");
            label.className = "label-text";
            label.textContent = "CorrosionProductControlDegraded";
            row.appendChild(label);
            const input = document.createElement("input");
            input.type = "number";
            row.appendChild(input);
            fsEl.appendChild(row);
            h.appendChild(fsEl);
        };

        const renderSensors = (h: HTMLElement) => {
            const wrap = document.createElement("div");
            wrap.className = "readout-part";
            const table = document.createElement("table");
            table.className = "data-table";
            const tbody = document.createElement("tbody");
            const tr = document.createElement("tr");
            const td1 = document.createElement("td");
            const label = document.createElement("span");
            label.className = "label-text";
            label.textContent = "CorrosionProductControlDegraded";
            td1.appendChild(label);
            tr.appendChild(td1);
            const td2 = document.createElement("td");
            td2.className = "mono";
            td2.textContent = "2250.00 psig";
            tr.appendChild(td2);
            tbody.appendChild(tr);
            table.appendChild(tbody);
            wrap.appendChild(table);
            h.appendChild(wrap);
            return [];
        };

        const rows: ReturnType<typeof buildTile>[] = [];
        buildTile(host, "CVCS", renderControls, renderSensors, rows as never);

        const table = host.querySelector<HTMLTableElement>(
            ".readout-part .data-table",
        );
        expect(table, "sensor table missing").not.toBeNull();
        expect(window.getComputedStyle(table!).tableLayout).toBe("fixed");

        const cells = table!.querySelectorAll("td");
        expect(cells.length).toBe(2);
        const valueStyle = window.getComputedStyle(cells[1]);
        expect(valueStyle.whiteSpace).toBe("nowrap");
        expect(valueStyle.textAlign).toBe("right");
        expect(valueStyle.fontVariantNumeric).toContain("tabular-nums");

        const ctlInput = host.querySelector<HTMLInputElement>(
            ".ctl-row input[type='number']",
        );
        expect(ctlInput).not.toBeNull();
        expect(window.getComputedStyle(ctlInput!).flex).toBe("0 0 auto");

        const ctlLabel = host.querySelector<HTMLElement>(
            ".ctl-row .label-text",
        );
        expect(ctlLabel).not.toBeNull();
        expect(window.getComputedStyle(ctlLabel!).overflowWrap).toBe("break-word");

        // Stacked single column: every widget gets full tile width.
        const tileBody = host.querySelector<HTMLElement>(".tile-body");
        expect(tileBody).not.toBeNull();
        expect(
            window.getComputedStyle(tileBody!).gridTemplateColumns,
        ).toBe("1fr");

        document.head.querySelectorAll("style").forEach((s) => s.remove());
        host.remove();
    });
});
