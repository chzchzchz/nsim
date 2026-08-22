// sensors.ts — build the sensor read-out table from PartBinding[].
// Each bridge.Sensor becomes a row under a part-level heading that
// reads "Part.Label"; the value cell is updated in place by
// refreshSensors() on every animation frame.

import type { Entry, PartBinding, PartDisplay } from "./bindings";
import type { PlantHandle } from "./plant";
import { formatSensorValue, unitFor } from "./units";
import { isAlarmKind } from "./alarms";

export interface SensorRow {
    id: string;
    cell: HTMLElement;
}

export function renderReadout(
    host: HTMLElement,
    bindings: PartBinding[],
    plant: PlantHandle,
    display: PartDisplay,
): SensorRow[] {
    host.innerHTML = "";
    const sensorParts = bindings.filter((b) => b.sensors.length > 0);
    if (sensorParts.length === 0) {
        host.textContent = "(no sensors)";
        return [];
    }

    const rows: SensorRow[] = [];
    for (const part of sensorParts) {
        // alarm-kind sensors are rendered in the dedicated alarm
        // panel; skip them here so no raw JSON leaks into the table.
        const scalar = part.sensors.filter((e) => !isAlarmKind(e.meta.kind));
        rows.push(...renderPartTable(part, rows, scalar, display));
    }
    return rows;
}

function renderPartTable(
    part: PartBinding,
    rows: SensorRow[],
    sensors: Entry[],
    display: PartDisplay,
): HTMLElement {
    const wrap = document.createElement("div");
    wrap.style.marginBottom = "8px";
    const h = document.createElement("div");
    h.textContent = display(part.part);
    h.style.fontWeight = "bold";
    h.style.borderBottom = "1px solid #ccc";
    h.style.marginBottom = "2px";
    wrap.appendChild(h);

    const table = document.createElement("table");
    table.style.width = "100%";
    table.style.borderCollapse = "collapse";
    const tbody = document.createElement("tbody");
    table.appendChild(tbody);
    wrap.appendChild(table);

    for (const e of sensors) {
        tbody.appendChild(renderSensorRow(e, rows));
    }
    return wrap;
}

function renderSensorRow(e: Entry, rows: SensorRow[]): HTMLElement {
    const { label, kind } = e.meta;
    const tr = document.createElement("tr");

    const td1 = document.createElement("td");
    td1.textContent = label;
    td1.style.padding = "2px 6px";
    td1.style.borderBottom = "1px solid #eee";
    tr.appendChild(td1);

    const td2 = document.createElement("td");
    td2.setAttribute("data-sensor", e.id);
    td2.style.padding = "2px 6px";
    td2.style.borderBottom = "1px solid #eee";
    td2.style.fontFamily = "monospace";
    td2.textContent = kind === "bool" ? "…" : (unitFor(kind) ? `… ${unitFor(kind)}` : "…");
    tr.appendChild(td2);

    rows.push({ id: e.id, cell: td2 });
    return tr;
}

export function refreshSensors(rows: SensorRow[], plant: PlantHandle): void {
    for (const r of rows) {
        const v = plant.sensorJSON(r.id);
        const m = plant.getMeta(r.id);
        if (m instanceof Error) {
            r.cell.textContent = "(unknown)";
            continue;
        }
        if (typeof v !== "string") {
            r.cell.textContent = `(error: ${(v as Error).message})`;
            continue;
        }
        if (m.kind === "bool") {
            const on = v === "true" || v === "1";
            r.cell.textContent = on ? "✅" : "❌";
            continue;
        }
        const formatted = formatSensorValue(m.kind, v);
        const u = unitFor(m.kind);
        r.cell.textContent = u ? `${formatted} ${u}` : formatted;
    }
}
