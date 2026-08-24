// alarms.ts — render an alarm panel that stays separate from the
// scalar sensor readout. Sensors of Kind === "alarm" are routed
// here; everything else continues through sensors.ts.

import { makeTile } from "./tile";
import { makeHelpButton } from "./bindings";
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

    const rowMap = new Map<string, HTMLElement>();
    for (const a of initialAlarms) {
        const el = buildAlarmRow(plant, a);
        tbody.appendChild(el);
        rowMap.set(a.id, el);
    }

    container.appendChild(root);

    function refresh(json: string): void {
        let list: AlarmEntry[];
        try {
            list = JSON.parse(json) as AlarmEntry[];
        } catch {
            console.error("bad alarm JSON:", json);
            return;
        }
        const seen = new Set<string>();
        for (const a of list) {
            seen.add(a.id);
            const existing = rowMap.get(a.id);
            if (existing) {
                updateAlarmRow(existing, a);
            } else {
                const el = buildAlarmRow(plant, a);
                tbody.appendChild(el);
                rowMap.set(a.id, el);
            }
        }
        for (const [id, el] of rowMap) {
            if (!seen.has(id)) {
                el.parentElement?.removeChild(el);
                rowMap.delete(id);
            }
        }
    }

    return { refresh };
}

function buildAlarmRow(
    plant: { setControl(id: string, value: string): Error | null },
    a: AlarmEntry,
): HTMLTableRowElement {
    const tr = document.createElement("tr") as HTMLTableRowElement;
    updateAlarmRow(tr, a);
    // The dim state comes from the alarm data itself on every
    // refresh; clicking writes the acknowledgment back through
    // the bridge so the visual state and plant state agree.
    tr.dataset.alarmId = a.id;
    tr.style.cursor = "pointer";
    tr.title = "Click to acknowledge (sends Reset)";
    tr.addEventListener("click", () => {
        const err = plant.setControl("Alarms/Reset", tr.dataset.alarmId ?? "");
        if (err instanceof Error) console.error("ack failed:", err.message);
    });
    return tr;
}

function updateAlarmRow(tr: HTMLElement, a: AlarmEntry): void {
    tr.dataset.alarmId = a.id;
    // Acknowledged alarms stay visible but dimmed.
    tr.classList.toggle("alarm-acked", a.acknowledged);

    // The row rebuilds on every refresh tick; carry the help button
    // and an open popup across so they survive into the new cells.
    // Recreating the button each tick would leave clicks landing on
    // a detached node whenever a rebuild fires mid-press.
    const openTip = tr.querySelector<HTMLElement>(".help-text");
    const prevHelpBtn =
        tr.querySelector<HTMLButtonElement>("button.ctl-help");

    while (tr.firstChild) tr.removeChild(tr.firstChild);

    const tdSev = document.createElement("td");
    const badge = document.createElement("span");
    badge.textContent = a.severity.toUpperCase();
    badge.className = `sev-badge sev-${a.severity}`;
    tdSev.appendChild(badge);
    tr.appendChild(tdSev);

    const tdId = document.createElement("td");
    tdId.className = "mono";
    tdId.textContent = a.id;
    const help = helpFor(a.id);
    let helpBtn: HTMLButtonElement | null = null;
    if (help) {
        // Anchor for description cross-references and the (?)
        // toggle; stopPropagation inside the button keeps a help
        // click from acknowledging the alarm.
        tr.dataset.bridgeId = a.id;
        helpBtn = prevHelpBtn ?? makeHelpButton(help);
        tdId.appendChild(helpBtn);
    }
    tr.appendChild(tdId);

    const tdDesc = document.createElement("td");
    tdDesc.textContent = a.description;
    tr.appendChild(tdDesc);

    const tdTime = document.createElement("td");
    tdTime.textContent = a.time_s.toFixed(1) + " s";
    tdTime.className = "nowrap";
    tr.appendChild(tdTime);

    if (openTip && helpBtn) {
        helpBtn.insertAdjacentElement("afterend", openTip);
    }
}

export function isAlarmKind(kind: string | undefined): boolean {
    return kind === "alarm";
}
