// sensors.test.ts — refreshSensors caching and formatting under
// happy-dom: identical payloads skip re-fetch and cell writes, a
// rebuilt row set re-fetches, and value/error rendering matches the
// kind table.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { refreshSensors, type SensorRow } from "./sensors";
import type { PlantHandle } from "./plant";

interface CountingPlant {
    calls: number;
}

function plantWithPayload(raw: string): {
    plant: CountingPlant;
    handle: PlantHandle;
} {
    const plant = {
        calls: 0,
        sensorValuesJSON(this: CountingPlant) {
            this.calls++;
            return raw;
        },
    };
    return { plant, handle: plant as unknown as PlantHandle };
}

const snapshot = JSON.stringify({
    values: { "RCS/Pressure": "2250", "RCS/RCPRunning": "1" },
    errors: { "RCS/Broken": "sensor fault" },
});

function row(id: string, kind: string, cell: HTMLElement): SensorRow {
    return { id, kind, cell };
}

describe("refreshSensors", () => {
    it("formats values, units, bools, and errors per kind", () => {
        const cells = ["pressure", "pump", "broken"].map(() => {
            const c = document.createElement("td");
            document.body.appendChild(c);
            return c;
        });
        const rows = [
            row("RCS/Pressure", "psig", cells[0]),
            row("RCS/RCPRunning", "bool", cells[1]),
            row("RCS/Broken", "psig", cells[2]),
        ];
        const p = plantWithPayload(snapshot);
        refreshSensors(rows, p.handle);
        expect(cells[0].textContent).toBe("2250.00 psig");
        expect(cells[1].textContent).toBe("✅");
        expect(cells[2].textContent).toBe("(error: sensor fault)");
        expect(cells[0].getAttribute("title")).toBe("2250 psig");
        expect(cells[1].getAttribute("title")).toBeNull();
    });

    it("skips cell writes when the payload is unchanged", () => {
        const cell = document.createElement("td");
        document.body.appendChild(cell);
        let writes = 0;
        let text = "";
        Object.defineProperty(cell, "textContent", {
            get: () => text,
            set: (v: string) => {
                writes++;
                text = v;
            },
        });
        const rows = [row("RCS/Pressure", "psig", cell)];
        const p = plantWithPayload(snapshot);
        refreshSensors(rows, p.handle);
        refreshSensors(rows, p.handle);
        expect(writes).toBe(1);
    });

    it("keeps the exact raw value in the tooltip when display rounds it", () => {
        const cell = document.createElement("td");
        document.body.appendChild(cell);
        const raw = JSON.stringify({
            values: { "RCS/Pressure": "937483401827.15" },
            errors: {},
        });
        const p = plantWithPayload(raw);
        refreshSensors([row("RCS/Pressure", "psig", cell)], p.handle);
        expect(cell.textContent).toBe("9.375e11 psig");
        expect(cell.getAttribute("title")).toBe("937483401827.15 psig");
    });

    it("fills a rebuilt row set's fresh cells despite identical payload", () => {
        const cellA = document.createElement("td");
        const cellB = document.createElement("td");
        document.body.append(cellA, cellB);
        const p = plantWithPayload(snapshot);
        refreshSensors([row("RCS/Pressure", "psig", cellA)], p.handle);
        expect(cellA.textContent).toBe("2250.00 psig");
        refreshSensors([row("RCS/Pressure", "psig", cellB)], p.handle);
        expect(cellB.textContent).toBe("2250.00 psig");
    });
});
