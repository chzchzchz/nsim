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

const SEVERITY_STYLE: Record<string, { bg: string; fg: string }> = {
    trip:    { bg: "#c0392b", fg: "#fff" },
    warning: { bg: "#d68910", fg: "#fff" },
    info:    { bg: "#229954", fg: "#fff" },
};

export function renderAlarms(container: HTMLElement, plant: { sensorJSON(id: string): string | Error; setControl(id: string, value: string): Error | null }, initialAlarms: AlarmEntry[]): HTMLElement {
    container.innerHTML = "";

    const panel = document.createElement("div");
    panel.className = "tile";
    panel.id = "alarm-panel";

    const h = document.createElement("h2");
    h.textContent = "Alarms";
    panel.appendChild(h);

    const ackBtn = document.createElement("button");
    ackBtn.textContent = "Acknowledge All";
    ackBtn.style.cssText =
        "margin-bottom:8px;padding:4px 12px;background:#2c3e50;color:white;border:1px solid #1a252f;cursor:pointer;font-size:13px;";
    ackBtn.addEventListener("click", () => {
        const err = plant.setControl("Alarms/AcknowledgeAll", "0");
        if (err instanceof Error) console.error("ack-all failed:", err.message);
    });
    panel.appendChild(ackBtn);

    const scrollHost = document.createElement("div");
    scrollHost.style.maxHeight = "300px";
    scrollHost.style.overflow = "auto";
    scrollHost.style.border = "1px solid #ddd";
    scrollHost.style.borderRadius = "4px";
    panel.appendChild(scrollHost);

    const table = document.createElement("table");
    table.style.width = "100%";
    table.style.borderCollapse = "collapse";
    table.style.fontSize = "12px";
    scrollHost.appendChild(table);

    const tbody = document.createElement("tbody");
    table.appendChild(tbody);

    const rowMap = new Map<string, HTMLElement>();
    for (const a of initialAlarms) {
        const el = buildAlarmRow(a);
        tbody.appendChild(el);
        rowMap.set(a.id, el);
    }

    (panel as any).refresh = (json: string): void => {
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
    };

    container.appendChild(panel);
    return panel;
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
    tr.style.borderBottom = "1px solid #eee";
    tr.style.opacity = a.acknowledged ? "0.5" : "1";

    while (tr.firstChild) tr.removeChild(tr.firstChild);

    const tdSev = document.createElement("td");
    const badge = document.createElement("span");
    badge.textContent = a.severity.toUpperCase();
    badge.style.cssText =
        `display:inline-block;padding:1px 6px;border-radius:3px;` +
        `background:${styles.bg};color:${styles.fg};font-size:10px;font-weight:bold;`;
    tdSev.appendChild(badge);
    tdSev.style.padding = "4px 6px";
    tdSev.style.borderBottom = "1px solid #eee";
    tr.appendChild(tdSev);

    const tdId = document.createElement("td");
    tdId.textContent = a.id;
    tdId.style.padding = "4px 6px";
    tdId.style.borderBottom = "1px solid #eee";
    tdId.style.fontFamily = "monospace";
    tr.appendChild(tdId);

    const tdDesc = document.createElement("td");
    tdDesc.textContent = a.description;
    tdDesc.style.padding = "4px 6px";
    tdDesc.style.borderBottom = "1px solid #eee";
    tr.appendChild(tdDesc);

    const tdTime = document.createElement("td");
    tdTime.textContent = a.time_s.toFixed(1) + " s";
    tdTime.style.padding = "4px 6px";
    tdTime.style.borderBottom = "1px solid #eee";
    tdTime.style.whiteSpace = "nowrap";
    tr.appendChild(tdTime);
}

export function isAlarmKind(kind: string | undefined): boolean {
    return kind === "alarm";
}