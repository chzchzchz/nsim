// sensors.ts — build the sensor read-out table and provide a
// refresh path that writes into <td data-sensor> cells.

import type { PlantHandle } from "./plant";

export interface SensorRow {
    id: string;
    label: string;
    cell: HTMLElement;
}

export function renderReadout(host: HTMLElement, plant: PlantHandle): SensorRow[] {
    host.innerHTML = "";
    const rows: SensorRow[] = [];
    const ids = plant.sensorKeys();
    if (ids.length === 0) {
        host.textContent = "(no sensors)";
        return rows;
    }
    const table = document.createElement("table");
    table.style.width = "100%";
    table.style.borderCollapse = "collapse";
    const thead = document.createElement("thead");
    thead.innerHTML = `<tr><th align="left">Sensor</th><th align="left">Value</th></tr>`;
    table.appendChild(thead);
    const tbody = document.createElement("tbody");
    table.appendChild(tbody);
    host.appendChild(table);

    for (const id of ids) {
        const tr = document.createElement("tr");
        const td1 = document.createElement("td");
        td1.textContent = id.slice(id.lastIndexOf("/") + 1);
        const td2 = document.createElement("td");
        td2.setAttribute("data-sensor", id);
        tr.appendChild(td1);
        tr.appendChild(td2);
        tbody.appendChild(tr);
        rows.push({ id, label: td1.textContent ?? "", cell: td2 });
    }
    return rows;
}

export function refreshSensors(rows: SensorRow[], plant: PlantHandle): void {
    for (const r of rows) {
        const v = plant.sensorJSON(r.id);
        r.cell.textContent = typeof v === "string" ? v : `(error: ${(v as Error).message})`;
    }
}
