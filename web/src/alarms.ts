// alarms.ts — render an alarm panel that stays separate from the
// scalar sensor readout. Sensors of Kind === "alarm" are routed
// here; everything else continues through sensors.ts.

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

const SEVERITY_STYLE: Record<string, { bg: string; fg: string }> = {
    trip:    { bg: "#c0392b", fg: "#fff" },
    warning: { bg: "#d68910", fg: "#fff" },
    info:    { bg: "#229954", fg: "#fff" },
};

export function renderAlarms(
    container: HTMLElement,
    plant: { sensorJSON(id: string): string | Error; setControl(id: string, value: string): Error | null },
    initialAlarms: AlarmEntry[],
): AlarmPanel {
    container.innerHTML = "";

    const panel = document.createElement("div");
    panel.className = "tile";
    panel.id = "alarm-panel";

    const h = document.createElement("h2");
    h.textContent = "Alarms";
    panel.appendChild(h);

    const ackBtn = document.createElement("button");
    ackBtn.textContent = "Acknowledge All";
    ackBtn.className = "btn";
    ackBtn.addEventListener("click", () => {
        const err = plant.setControl("Alarms/AcknowledgeAll", "0");
        if (err instanceof Error) console.error("ack-all failed:", err.message);
    });
    panel.appendChild(ackBtn);

    const scrollHost = document.createElement("div");
    scrollHost.className = "scroll-host";
    panel.appendChild(scrollHost);

    const table = document.createElement("table");
    table.className = "data-table";
    scrollHost.appendChild(table);

    const tbody = document.createElement("tbody");
    table.appendChild(tbody);

    const rowMap = new Map<string, HTMLElement>();
    for (const a of initialAlarms) {
        const el = buildAlarmRow(a);
        tbody.appendChild(el);
        rowMap.set(a.id, el);
    }

    container.appendChild(panel);

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
                const el = buildAlarmRow(a);
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

function buildAlarmRow(a: AlarmEntry): HTMLTableRowElement {
    const tr = document.createElement("tr") as HTMLTableRowElement;
    updateAlarmRow(tr, a);
    tr.addEventListener("click", () => {
        tr.style.opacity = tr.style.opacity === "0.5" ? "1" : "0.5";
    });
    tr.style.cursor = "pointer";
    tr.title = "Click to toggle acknowledged (sends Reset)";
    tr.addEventListener("dblclick", () => {
        window.dispatchEvent(new CustomEvent("ack-alarm", { detail: a }));
    });
    return tr;
}

function updateAlarmRow(tr: HTMLElement, a: AlarmEntry): void {
    const styles = SEVERITY_STYLE[a.severity] ?? SEVERITY_STYLE.info;
    // Acknowledged alarms stay visible but dimmed.
    tr.style.opacity = a.acknowledged ? "0.5" : "1";

    while (tr.firstChild) tr.removeChild(tr.firstChild);

    const tdSev = document.createElement("td");
    const badge = document.createElement("span");
    badge.textContent = a.severity.toUpperCase();
    badge.style.cssText =
        `display:inline-block;padding:1px 6px;border-radius:3px;` +
        `background:${styles.bg};color:${styles.fg};font-size:10px;font-weight:bold;`;
    tdSev.appendChild(badge);
    tr.appendChild(tdSev);

    const tdId = document.createElement("td");
    tdId.textContent = a.id;
    tdId.className = "mono";
    tr.appendChild(tdId);

    const tdDesc = document.createElement("td");
    tdDesc.textContent = a.description;
    tr.appendChild(tdDesc);

    const tdTime = document.createElement("td");
    tdTime.textContent = a.time_s.toFixed(1) + " s";
    tdTime.className = "nowrap";
    tr.appendChild(tdTime);
}

export function isAlarmKind(kind: string | undefined): boolean {
    return kind === "alarm";
}
