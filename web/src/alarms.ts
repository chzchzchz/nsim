// alarms.ts — render an alarm panel that stays separate from the
// scalar sensor readout. Sensors of Kind === "alarm" are routed
// here; everything else continues through sensors.ts.

import { marked } from "marked";

import { makeTile } from "./tile";
import { crossReferenceHref, revealElement } from "./bindings";
import { helpFor } from "./descriptions";

export interface AlarmEntry {
    id: string;
    description: string;
    severity: string;   // "info" | "warning" | "trip"
    active: boolean;
    acknowledged: boolean;
    time_s: number;
}

// AlarmPanel is the typed handle returned to the caller so the
// tick loop can drive updates without any casts.
export interface AlarmPanel {
    refresh(json: string): void;
}

const ALARM_COLUMNS = ["Severity", "ID", "Description", "Time (s)"];

// Acknowledge control per row. An emoji keeps it visually distinct
// from the surrounding text so row clicks stay reserved for the
// detail expansion.
const ACK_EYES = "👁";
const TOGGLE_EXPAND = "➕";
const TOGGLE_COLLAPSE = "➖";

// AlarmRowPair bundles the visible alarm row with its expandable
// detail sibling; both share the lifecycle of one alarm entry.
// lastJSON is the serialized entry behind the current cells, so a
// refresh that carries an identical snapshot skips the DOM rewrite
// instead of rebuilding stable rows every frame.
interface AlarmRowPair {
    row: HTMLTableRowElement;
    detail: HTMLTableRowElement;
    lastJSON?: string;
}

export function renderAlarms(
    container: HTMLElement,
    plant: { sensorJSON(id: string): string | Error; setControl(id: string, value: string): Error | null },
    initialAlarms: AlarmEntry[],
): AlarmPanel {
    container.innerHTML = "";

    const { root, body } = makeTile("Alarms");
    root.id = "alarm-panel";

    const ackBtn = document.createElement("button");
    ackBtn.textContent = "Acknowledge All";
    ackBtn.className = "btn";
    ackBtn.addEventListener("click", () => {
        const err = plant.setControl("Alarms/AcknowledgeAll", "0");
        if (err instanceof Error) console.error("ack-all failed:", err.message);
    });
    body.appendChild(ackBtn);

    const scrollHost = document.createElement("div");
    scrollHost.className = "scroll-host";
    body.appendChild(scrollHost);

    const table = document.createElement("table");
    table.className = "data-table";
    const thead = document.createElement("thead");
    const headRow = document.createElement("tr");
    for (const name of ALARM_COLUMNS) {
        const th = document.createElement("th");
        th.textContent = name;
        headRow.appendChild(th);
    }
    thead.appendChild(headRow);
    table.appendChild(thead);

    const tbody = document.createElement("tbody");
    table.appendChild(tbody);
    scrollHost.appendChild(table);

    const rowMap = new Map<string, AlarmRowPair>();
    for (const a of initialAlarms) {
        const pair = buildAlarmRow(plant, a);
        pair.lastJSON = JSON.stringify(a);
        tbody.append(pair.row, pair.detail);
        rowMap.set(a.id, pair);
    }

    container.appendChild(root);

    // Raw payload behind the current rows; an identical string from
    // the wasm side skips the parse and the per-entry diffing.
    let lastRaw = "";

    function refresh(json: string): void {
        if (json === lastRaw) {
            return;
        }
        let list: AlarmEntry[];
        try {
            list = JSON.parse(json) as AlarmEntry[];
        } catch {
            console.error("bad alarm JSON:", json);
            return;
        }
        lastRaw = json;
        const seen = new Set<string>();
        for (const a of list) {
            seen.add(a.id);
            const snapshot = JSON.stringify(a);
            const pair = rowMap.get(a.id);
            if (!pair) {
                const built = buildAlarmRow(plant, a);
                built.lastJSON = snapshot;
                tbody.append(built.row, built.detail);
                rowMap.set(a.id, built);
                continue;
            }
            if (pair.lastJSON === snapshot) {
                continue;
            }
            pair.lastJSON = snapshot;
            updateAlarmRow(plant, pair, a);
        }
        for (const [id, pair] of rowMap) {
            if (!seen.has(id)) {
                pair.row.remove();
                pair.detail.remove();
                rowMap.delete(id);
            }
        }
    }

    return { refresh };
}

function buildAlarmRow(
    plant: { setControl(id: string, value: string): Error | null },
    a: AlarmEntry,
): AlarmRowPair {
    const row = document.createElement("tr") as HTMLTableRowElement;
    const detail = document.createElement("tr");
    detail.className = "alarm-detail";
    detail.hidden = true;
    const detailCell = document.createElement("td");
    detailCell.colSpan = ALARM_COLUMNS.length;
    const detailBody = document.createElement("div");
    detailBody.className = "alarm-detail-body md";
    detailCell.appendChild(detailBody);
    detail.appendChild(detailCell);

    // Description cross-references jump to the referenced row
    // instead of navigating away; ids without a rendered row are
    // ignored.
    detailBody.addEventListener("click", (ev) => {
        const href = crossReferenceHref(ev);
        if (href !== "") {
            revealElement(href);
        }
    });

    updateAlarmRow(plant, { row, detail }, a);

    // The dim state comes from the alarm data itself on every
    // refresh; the eyes button writes the acknowledgment back
    // through the bridge so the visual state and plant state agree.
    row.dataset.bridgeId = a.id;
    row.style.cursor = "pointer";
    row.title = "Click for details";
    row.addEventListener("click", () => {
        const open = row.classList.toggle("alarm-open");
        detail.hidden = !open;
    });

    return { row, detail };
}

function updateAlarmRow(
    plant: { setControl(id: string, value: string): Error | null },
    pair: AlarmRowPair,
    a: AlarmEntry,
): void {
    const { row, detail } = pair;
    row.dataset.alarmId = a.id;
    // Acknowledged alarms stay visible but dimmed.
    row.classList.toggle("alarm-acked", a.acknowledged);

    // The row rebuilds on every refresh tick; carry the eyes and
    // toggle buttons across so they survive into the new cells.
    // Recreating them each tick would leave clicks landing on a
    // detached node whenever a rebuild fires mid-press.
    const prevEyes = row.querySelector<HTMLButtonElement>("button.alarm-eyes");
    const prevToggle =
        row.querySelector<HTMLButtonElement>("button.alarm-toggle");

    while (row.firstChild) row.removeChild(row.firstChild);

    const tdSev = document.createElement("td");
    const badge = document.createElement("span");
    badge.textContent = a.severity.toUpperCase();
    badge.className = `sev-badge sev-${a.severity}`;
    tdSev.appendChild(badge);
    row.appendChild(tdSev);

    const tdId = document.createElement("td");
    tdId.className = "mono";

    const eyes = prevEyes ?? makeAckButton(plant, a.id);
    eyes.disabled = a.acknowledged;

    const label = document.createElement("span");
    label.className = "alarm-id-label";
    label.textContent = a.id;

    const toggle = prevToggle ?? makeToggleButton(pair);
    const open = row.classList.contains("alarm-open");
    toggle.textContent = open ? TOGGLE_COLLAPSE : TOGGLE_EXPAND;
    toggle.title = open ? "Collapse details" : "Expand details";

    tdId.append(eyes, label, toggle);
    row.appendChild(tdId);

    const tdDesc = document.createElement("td");
    tdDesc.textContent = a.description;
    row.appendChild(tdDesc);

    const tdTime = document.createElement("td");
    tdTime.textContent = a.time_s.toFixed(1) + " s";
    tdTime.className = "nowrap";
    row.appendChild(tdTime);

    fillAlarmDetail(detail, a);
}

function fillAlarmDetail(detail: HTMLTableRowElement, a: AlarmEntry): void {
    const body = detail.querySelector<HTMLDivElement>(".alarm-detail-body");
    if (!body) return;
    body.innerHTML = "";

    const facts = document.createElement("p");
    const ackState = a.acknowledged ? "; acknowledged" : "";
    facts.textContent =
        `Fired at ${a.time_s.toFixed(1)} s; severity ${a.severity}${ackState}.`;
    body.appendChild(facts);

    const help = helpFor(a.id);
    if (help !== undefined) {
        const md = document.createElement("div");
        md.innerHTML = marked.parse(help, { async: false });
        while (md.firstChild) body.appendChild(md.firstChild);
    } else {
        const desc = document.createElement("p");
        desc.textContent = a.description;
        body.appendChild(desc);
    }
}

function makeAckButton(
    plant: { setControl(id: string, value: string): Error | null },
    id: string,
): HTMLButtonElement {
    const btn = document.createElement("button");
    btn.textContent = ACK_EYES;
    btn.className = "alarm-eyes";
    btn.title = "Acknowledge (sends Reset)";
    // Stop propagation so an acknowledge click does not also toggle
    // the row's detail expansion.
    btn.addEventListener("click", (ev) => {
        ev.stopPropagation();
        const err = plant.setControl("Alarms/Reset", id);
        if (err instanceof Error) console.error("ack failed:", err.message);
    });
    return btn;
}

function makeToggleButton(pair: AlarmRowPair): HTMLButtonElement {
    const btn = document.createElement("button");
    btn.className = "alarm-toggle";
    // Stop propagation so a toggle click does not also fire the
    // row-level expansion handler.
    btn.addEventListener("click", (ev) => {
        ev.stopPropagation();
        const open = pair.row.classList.toggle("alarm-open");
        pair.detail.hidden = !open;
    });
    return btn;
}

export function isAlarmKind(kind: string | undefined): boolean {
    return kind === "alarm";
}
