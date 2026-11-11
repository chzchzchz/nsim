// remoteTiles.ts — remote-mode tile renderer. Mirrors the
// local renderTile/renderPanel structure but is fed from
// fetched catalog snapshots instead of a PlantHandle, so
// no async rewrite of the wasm surface is needed.
// Control writes go through the confirm dialog (see
// confirmChange.ts) so every change carries a comment.

import { isAlarmControl, makeHelpButton, type PartBinding, type PartDisplay } from "./bindings";
import { isFaultEntry, showFaultInjectors } from "./faults";
import { helpFor } from "./descriptions";
import { rangeFor, unitFor } from "./units";
import { formatSensorValue } from "./units";
import { isAlarmKind } from "./alarms";
import type { SensorRow } from "./sensors";
import type { ControlEntry, SensorEntry } from "./remote";
import { confirmControlChange } from "./confirmChange";
import { RemoteError } from "./remote";

export type ControlWriter = (key: string, value: string, comment: string) => Promise<{ seq: number }>;

const CTL_ERROR_FLASH_MS = 1500;

export function buildRemoteBindings(
    controls: ControlEntry[],
    sensors: SensorEntry[],
): PartBinding[] {
    const byPart = new Map<string, PartBinding>();
    const bucketFor = (part: string): PartBinding => {
        const b = byPart.get(part);
        if (b) return b;
        const fresh: PartBinding = { part, controls: [], sensors: [] };
        byPart.set(part, fresh);
        return fresh;
    };
    for (const c of controls) {
        bucketFor(c.part).controls.push({
            id: c.key,
            meta: { part: c.part, label: c.label, kind: c.kind, enum: c.enum, fault: c.fault },
        });
    }
    for (const s of sensors) {
        bucketFor(s.part).sensors.push({
            id: s.key,
            meta: { part: s.part, label: s.label, kind: s.kind },
        });
    }
    return Array.from(byPart.values()).sort((a, b) => a.part.localeCompare(b.part));
}

export function renderRemotePanel(
    host: HTMLElement,
    bindings: PartBinding[],
    display: PartDisplay,
    values: Record<string, string>,
    writable: () => boolean,
    writeControl: ControlWriter,
): void {
    host.innerHTML = "";
    const showFaults = showFaultInjectors();
    const controlParts = bindings
        .map((b) => ({
            part: b.part,
            controls: b.controls.filter(
                (e) => !isAlarmControl(e.id) && (showFaults || !isFaultEntry(e)),
            ),
            sensors: b.sensors,
        }))
        .filter((b) => b.controls.length > 0);
    if (controlParts.length === 0) {
        host.textContent = "(no controls)";
        return;
    }
    for (const part of controlParts) {
        host.appendChild(renderPart(part, display, values, writable, writeControl));
    }
}

function renderPart(
    part: PartBinding,
    display: PartDisplay,
    values: Record<string, string>,
    writable: () => boolean,
    writeControl: ControlWriter,
): HTMLElement {
    const fs = document.createElement("fieldset");
    const lg = document.createElement("legend");
    lg.innerHTML = display(part.part);
    fs.appendChild(lg);
    for (const e of part.controls) {
        fs.appendChild(renderControl(e, display, values, writable, writeControl));
    }
    return fs;
}

function renderControl(
    e: { id: string; meta: { kind: string; label: string; enum?: string[] } },
    _display: PartDisplay,
    values: Record<string, string>,
    writable: () => boolean,
    writeControl: ControlWriter,
): HTMLElement {
    const { kind, label } = e.meta;
    const row = document.createElement("div");
    row.className = "ctl-row";
    row.dataset.bridgeId = e.id;

    const text = document.createElement("span");
    text.className = "label-text";
    text.textContent = label;
    row.appendChild(text);

    const desc = helpFor(e.id);
    if (desc) row.appendChild(makeHelpButton(desc));

    const current = values[e.id] ?? "";
    let widget: HTMLInputElement | HTMLSelectElement | HTMLButtonElement;

    if (kind === "bool") {
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = current === "1" || current === "true";
        widget = cb;
        cb.addEventListener("change", () => {
            void onChange(e.id, current, cb.checked ? "1" : "0", cb, writable, writeControl);
        });
        row.appendChild(cb);
    } else if (kind === "enum" && e.meta.enum && e.meta.enum.length > 1) {
        const sel = document.createElement("select");
        for (let i = 0; i < e.meta.enum.length; i++) {
            const opt = document.createElement("option");
            opt.value = String(i);
            opt.textContent = e.meta.enum[i];
            sel.appendChild(opt);
        }
        sel.value = current;
        widget = sel;
        sel.addEventListener("change", () => {
            void onChange(e.id, current, sel.value, sel, writable, writeControl);
        });
        row.appendChild(sel);
    } else if (kind === "enum" && e.meta.enum && e.meta.enum.length === 1) {
        const btn = document.createElement("button");
        btn.textContent = e.meta.enum[0];
        btn.className = "btn";
        widget = btn;
        btn.addEventListener("click", () => {
            void onChange(e.id, current, "0", btn, writable, writeControl);
        });
        row.appendChild(btn);
    } else if (kind === "string" || (kind === "enum" && (!e.meta.enum || e.meta.enum.length === 0))) {
        const inp = document.createElement("input");
        inp.type = "text";
        inp.size = 8;
        inp.value = current;
        widget = inp;
        inp.addEventListener("change", () => {
            void onChange(e.id, current, inp.value, inp, writable, writeControl);
        });
        row.appendChild(inp);
    } else {
        const num = document.createElement("input");
        num.type = "number";
        num.step = "0.01";
        num.size = 6;
        num.maxLength = 8;
        const r = rangeFor(kind);
        if (r) {
            const m = r.match(/^\[(-?[\d.]+),\s*(-?[\d.]+)\]$/);
            if (m) {
                num.min = m[1];
                num.max = m[2];
            }
        }
        num.value = current === "" ? "0" : current;
        widget = num;
        num.addEventListener("change", () => {
            void onChange(e.id, current, num.value, num, writable, writeControl);
        });
        row.appendChild(num);
        const u = unitFor(kind);
        if (u) {
            const unit = document.createElement("span");
            unit.textContent = u;
            unit.className = "ctl-hint";
            row.appendChild(unit);
        }
        if (r) {
            const hint = document.createElement("span");
            hint.textContent = r;
            hint.className = "ctl-hint faint";
            row.appendChild(hint);
        }
    }
    if (!writable()) {
        widget.disabled = true;
    }
    // Keep a handle so poll updates can refresh untouched widgets.
    // The row's data-remote-current is the single source of
    // truth for "last polled value"; the widget's closure
    // `current` goes stale on every poll.
    (row as HTMLElement & { _remoteWidget?: HTMLElement })._remoteWidget = widget as HTMLElement;
    row.dataset.remoteCurrent = current;
    return row;
}

// staleOldValue returns the last polled value for a widget's
// row, or the closure-captured fallback. The closure's
// `current` is build-time state; the poll keeps
// `_remoteCurrent` fresh on every tick, so always prefer
// the row's value for the dialog's "from" and for reverts.
function staleOldValue(widget: HTMLElement): string {
    const row = widget.closest(".ctl-row") as HTMLElement | null;
    const prev = row?.dataset.remoteCurrent;
    return prev !== undefined ? prev : "";
}

async function onChange(
    id: string,
    _closureOld: string,
    newValue: string,
    widget: HTMLElement,
    writable: () => boolean,
    writeControl: ControlWriter,
): Promise<void> {
    if (!writable()) {
        flash(widget);
        return;
    }
    const oldValue = staleOldValue(widget);
    if (oldValue === newValue) return;
    const result = await confirmControlChange(id, oldValue, newValue);
    if (!result.submitted || result.comment === undefined) {
        // Revert the widget to its last polled value.
        const prev = staleOldValue(widget);
        if (widget instanceof HTMLInputElement && widget.type === "checkbox") {
            widget.checked = prev === "1" || prev === "true";
        } else if ("value" in widget) {
            (widget as { value: string }).value = prev;
        }
        return;
    }
    try {
        const res = await writeControl(id, newValue, result.comment);
        const row = widget.closest(".ctl-row") as HTMLElement | null;
        if (row) {
            row.dataset.remoteCurrent = newValue;
        }
        void res;
    } catch (err) {
        const msg = err instanceof RemoteError ? err.message : String(err);
        console.error(`${id}: ${msg}`);
        flash(widget);
        // Revert on rejection: the plant kept the old value.
        const prev = staleOldValue(widget);
        if ("value" in widget) {
            (widget as { value: string }).value = prev;
        }
    }
}

function flash(widget: HTMLElement): void {
    widget.classList.add("ctl-error");
    setTimeout(() => widget.classList.remove("ctl-error"), CTL_ERROR_FLASH_MS);
}

// refreshRemoteControls updates non-focused, non-dialog widgets
// from a fresh bulk snapshot. Widgets that currently hold focus
// (operator mid-edit) or that have an open confirm dialog are
// left untouched so the poll never clobbers an in-flight edit.
export function refreshRemoteControls(
    tilesEl: HTMLElement,
    values: Record<string, string>,
): void {
    const rows = tilesEl.querySelectorAll<HTMLElement>(".ctl-row");
    for (const row of Array.from(rows)) {
        const key = row.dataset.bridgeId ?? "";
        const next = values[key];
        if (next === undefined) continue;
        const widget = (row as HTMLElement & { _remoteWidget?: HTMLElement })._remoteWidget;
        if (!widget) continue;
        if (document.activeElement === widget) continue;
        if (row.dataset.remoteCurrent === next) continue;
        if (widget instanceof HTMLInputElement && widget.type === "checkbox") {
            widget.checked = next === "1" || next === "true";
        } else if ("value" in widget) {
            (widget as { value: string }).value = next;
        }
        row.dataset.remoteCurrent = next;
    }
}

export function renderRemoteReadout(
    host: HTMLElement,
    bindings: PartBinding[],
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
        const scalar = part.sensors.filter((e) => !isAlarmKind(e.meta.kind));
        const wrap = document.createElement("div");
        wrap.className = "readout-part";
        const h = document.createElement("div");
        h.innerHTML = display(part.part);
        h.className = "sub-heading";
        wrap.appendChild(h);
        const table = document.createElement("table");
        table.className = "data-table";
        const tbody = document.createElement("tbody");
        table.appendChild(tbody);
        wrap.appendChild(table);
        for (const e of scalar) {
            const tr = document.createElement("tr");
            tr.dataset.bridgeId = e.id;
            const td1 = document.createElement("td");
            const desc = helpFor(e.id);
            if (desc) td1.appendChild(makeHelpButton(desc));
            const labelSpan = document.createElement("span");
            labelSpan.className = "label-text";
            labelSpan.textContent = e.meta.label;
            td1.appendChild(labelSpan);
            tr.appendChild(td1);
            const td2 = document.createElement("td");
            td2.setAttribute("data-sensor", e.id);
            td2.className = "mono";
            td2.textContent = "…";
            tr.appendChild(td2);
            rows.push({ id: e.id, kind: e.meta.kind, cell: td2 });
            tbody.appendChild(tr);
        }
        host.appendChild(wrap);
    }
    return rows;
}

export function refreshRemoteSensors(
    rows: SensorRow[],
    values: Record<string, string>,
): void {
    for (const r of rows) {
        const v = values[r.id];
        if (v === undefined) continue;
        let out: string;
        // The displayed two-decimal value shares the compact
        // tabular column; keep the exact raw value on hover.
        let title: string | null = null;
        if (r.kind === "bool") {
            out = v === "true" || v === "1" ? "✅" : "❌";
        } else {
            const formatted = formatSensorValue(r.kind, v);
            const u = unitFor(r.kind);
            out = u ? `${formatted} ${u}` : formatted;
            if (formatted !== v) {
                title = u ? `${v} ${u}` : v;
            }
        }
        if (r.cell.textContent !== out) {
            r.cell.textContent = out;
        }
        const currentTitle = r.cell.getAttribute("title");
        if (title === null) {
            if (currentTitle !== null) {
                r.cell.removeAttribute("title");
            }
        } else if (currentTitle !== title) {
            r.cell.setAttribute("title", title);
        }
    }
}
