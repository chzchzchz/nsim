// sensors.ts — build the sensor read-out table from PartBinding[].
// Each bridge.Sensor becomes a row under a part-level heading that
// reads "Part.Label"; the value cell is updated in place by
// refreshSensors() on every animation frame. Metadata is immutable,
// so it is captured once at render time and the per-frame refresh
// pulls all readings in a single batched call.

import { makeHelpButton, type Entry, type PartBinding, type PartDisplay } from "./bindings";
import type { PlantHandle } from "./plant";
import { helpFor } from "./descriptions";
import { formatSensorValue, unitFor } from "./units";
import { isAlarmKind } from "./alarms";

export interface SensorRow {
    id: string;
    // Immutable bridge metadata captured at render time so the
    // hot refresh path never re-fetches it.
    kind: string;
    cell: HTMLElement;
}

// Raw snapshot behind the current cells of each rendered row set,
// keyed by array identity: an identical payload skips the parse
// and the per-cell writes entirely, and a rebuild (new row array)
// invalidates it so fresh placeholder cells always get filled.
const lastRawByRows = new WeakMap<SensorRow[], string>();
const lastSnapByRows = new WeakMap<SensorRow[], SensorSnapshot>();

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
        host.appendChild(renderPartTable(part, rows, scalar, display));
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
    wrap.className = "readout-part";
    const h = document.createElement("div");
    h.textContent = display(part.part);
    h.className = "sub-heading";
    wrap.appendChild(h);

    const table = document.createElement("table");
    table.className = "data-table";
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
    // Anchor for description cross-references (see bindings.ts).
    tr.dataset.bridgeId = e.id;

    const td1 = document.createElement("td");
    const desc = helpFor(e.id);
    if (desc) {
        td1.appendChild(makeHelpButton(desc));
    }
    const labelSpan = document.createElement("span");
    labelSpan.className = "label-text";
    labelSpan.innerHTML = label.replace(/\//g, "/<wbr>");
    td1.appendChild(labelSpan);
    tr.appendChild(td1);

    const td2 = document.createElement("td");
    td2.setAttribute("data-sensor", e.id);
    td2.className = "mono";
    td2.textContent = kind === "bool" ? "…" : (unitFor(kind) ? `… ${unitFor(kind)}` : "…");
    tr.appendChild(td2);

    rows.push({ id: e.id, kind, cell: td2 });
    return tr;
}

interface SensorSnapshot {
    values: Record<string, string>;
    errors: Record<string, string>;
}

// refreshSensors updates every value cell from one batched wasm
// call and returns the parsed snapshot so callers can reuse other
// readings in it (e.g. the alarm list) without a second crossing.
// On a failed or malformed batch the previous frame's cells are
// left untouched rather than blanking the read-out, and null is
// returned. A payload identical to the previous frame skips
// parsing and writing, and cells whose formatted text did not
// change skip the write so idle rows do not invalidate layout.
export function refreshSensors(
    rows: SensorRow[],
    plant: PlantHandle,
): SensorSnapshot | null {
    const raw = plant.sensorValuesJSON();
    if (typeof raw !== "string") {
        return null;
    }
    if (lastRawByRows.get(rows) === raw) {
        return lastSnapByRows.get(rows) ?? null;
    }
    let snap: SensorSnapshot;
    try {
        snap = JSON.parse(raw) as SensorSnapshot;
    } catch {
        console.error("bad sensor snapshot:", raw);
        return null;
    }
    for (const r of rows) {
        let out: string;
        const errMsg = snap.errors[r.id];
        if (errMsg !== undefined) {
            out = `(error: ${errMsg})`;
        } else {
            const v = snap.values[r.id];
            if (v === undefined) {
                out = "(unknown)";
            } else if (r.kind === "bool") {
                out = v === "true" || v === "1" ? "✅" : "❌";
            } else {
                const formatted = formatSensorValue(r.kind, v);
                const u = unitFor(r.kind);
                out = u ? `${formatted} ${u}` : formatted;
            }
        }
        if (r.cell.textContent !== out) {
            r.cell.textContent = out;
        }
    }
    lastRawByRows.set(rows, raw);
    lastSnapByRows.set(rows, snap);
    return snap;
}
